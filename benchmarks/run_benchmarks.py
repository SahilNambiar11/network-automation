#!/usr/bin/env python3
"""Run end-to-end benchmarks against the local Docker Compose stack.

The harness intentionally uses only the public API plus Docker Compose orchestration.
It does not alter backend behavior or inject test-only code.
"""

from __future__ import annotations

import argparse
import datetime as dt
import json
import os
import pathlib
import statistics
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from typing import Any


ROOT = pathlib.Path(__file__).resolve().parents[1]
RESULTS_ROOT = ROOT / "benchmarks" / "results"
API_BASE = "http://localhost:8080"
TERMINAL_STATUSES = {"success", "failed", "timeout"}
COMPOSE_PROJECT_NAME = "network-automation-bench"


def main() -> int:
    parser = argparse.ArgumentParser(description="Benchmark distributed network controller")
    parser.add_argument("--output-dir", type=pathlib.Path, default=RESULTS_ROOT / "latest")
    parser.add_argument("--worker-counts", default="1,2,4")
    parser.add_argument("--job-sizes", default="10,50,100,500")
    parser.add_argument("--worker-concurrency", type=int, default=3)
    parser.add_argument("--skip-build", action="store_true")
    args = parser.parse_args()

    worker_counts = parse_ints(args.worker_counts)
    job_sizes = parse_ints(args.job_sizes)
    output_dir = args.output_dir
    output_dir.mkdir(parents=True, exist_ok=True)

    results: dict[str, Any] = {
        "metadata": {
            "started_at": now_iso(),
            "worker_concurrency": args.worker_concurrency,
            "worker_counts": worker_counts,
            "job_sizes": job_sizes,
            "notes": [
                "Measurements are from the existing implementation with mock device deployments.",
                "The implementation has deterministic failure and timeout injection by device name.",
                "It does not include a transient-failure mode that later succeeds without changing backend behavior.",
            ],
        },
        "throughput": [],
        "worker_scaling": [],
        "lease_recovery": {},
        "retry_timeout": {},
        "drift_accuracy": {},
        "heartbeat_monitoring": {},
    }

    if not args.skip_build:
        compose(["build", "controller", "worker"])

    try:
        results["throughput"] = run_throughput_matrix(
            worker_counts=worker_counts,
            job_sizes=job_sizes,
            worker_concurrency=args.worker_concurrency,
        )
        results["worker_scaling"] = calculate_scaling(results["throughput"], job_sizes)
        results["lease_recovery"] = run_lease_recovery(args.worker_concurrency)
        results["retry_timeout"] = run_retry_timeout(args.worker_concurrency)
        results["drift_accuracy"] = run_drift_accuracy(args.worker_concurrency)
        results["heartbeat_monitoring"] = run_heartbeat_monitoring(args.worker_concurrency)
    finally:
        results["metadata"]["finished_at"] = now_iso()

    write_results(output_dir, results)
    print(f"wrote {output_dir / 'results.json'}")
    print(f"wrote {output_dir / 'report.md'}")
    return 0


def run_throughput_matrix(worker_counts: list[int], job_sizes: list[int], worker_concurrency: int) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    for worker_count in worker_counts:
        print(f"\n=== throughput: {worker_count} worker(s) ===", flush=True)
        reset_stack(worker_count, worker_concurrency)
        for job_count in job_sizes:
            print(f"deploying {job_count} jobs with {worker_count} worker(s)", flush=True)
            row = measure_successful_deployment(job_count, worker_count)
            rows.append(row)
            print(
                f"  {row['completion_seconds']:.3f}s, "
                f"{row['jobs_per_second']:.2f} jobs/sec, "
                f"avg latency {row['average_job_latency_ms']:.2f} ms",
                flush=True,
            )
    return rows


def measure_successful_deployment(job_count: int, worker_count: int) -> dict[str, Any]:
    payload = generate_network_yaml(job_count, "bench")
    start = time.monotonic()
    response = post_yaml("/deployments", payload)
    deployment_id = response["deployment_id"]
    jobs = wait_for_deployment_jobs(deployment_id, expected_jobs=job_count)
    completion_seconds = time.monotonic() - start
    latencies = job_latencies_ms(jobs)

    return {
        "worker_count": worker_count,
        "job_count": job_count,
        "deployment_id": deployment_id,
        "completion_seconds": round(completion_seconds, 3),
        "jobs_per_second": round(job_count / completion_seconds, 3),
        "average_job_latency_ms": round(statistics.mean(latencies), 3) if latencies else None,
        "p95_job_latency_ms": round(percentile(latencies, 95), 3) if latencies else None,
        "success_jobs": count_status(jobs, "success"),
        "failed_jobs": count_status(jobs, "failed"),
        "timeout_jobs": count_status(jobs, "timeout"),
    }


def calculate_scaling(throughput_rows: list[dict[str, Any]], job_sizes: list[int]) -> list[dict[str, Any]]:
    scaling_rows: list[dict[str, Any]] = []
    for job_count in job_sizes:
        by_worker = {row["worker_count"]: row for row in throughput_rows if row["job_count"] == job_count}
        baseline = by_worker.get(1)
        scaled = by_worker.get(4)
        if not baseline or not scaled:
            continue
        improvement = (
            (baseline["completion_seconds"] - scaled["completion_seconds"])
            / baseline["completion_seconds"]
            * 100
        )
        throughput_gain = (
            (scaled["jobs_per_second"] - baseline["jobs_per_second"])
            / baseline["jobs_per_second"]
            * 100
        )
        scaling_rows.append(
            {
                "job_count": job_count,
                "one_worker_seconds": baseline["completion_seconds"],
                "four_worker_seconds": scaled["completion_seconds"],
                "completion_time_improvement_percent": round(improvement, 2),
                "one_worker_jobs_per_second": baseline["jobs_per_second"],
                "four_worker_jobs_per_second": scaled["jobs_per_second"],
                "throughput_improvement_percent": round(throughput_gain, 2),
            }
        )
    return scaling_rows


def run_lease_recovery(worker_concurrency: int) -> dict[str, Any]:
    print("\n=== lease recovery ===", flush=True)
    worker_count = 1
    job_count = 6
    reset_stack(worker_count, worker_concurrency)
    payload = generate_timeout_recovery_yaml(job_count)
    response = post_yaml("/deployments", payload)
    deployment_id = response["deployment_id"]

    jobs_at_kill = wait_for_running_jobs(deployment_id, minimum_running=job_count, timeout_seconds=15)
    running_at_kill = count_status(jobs_at_kill, "running")
    claimed_at_kill = len([job for job in jobs_at_kill if job.get("claimed_by")])
    kill_time = time.monotonic()
    compose(["kill", "worker"], check=True)

    # Leave jobs abandoned long enough to require lease expiry, then restart workers.
    compose(["up", "-d", "--scale", f"worker={worker_count}", "worker"], env=compose_env(worker_concurrency))
    jobs = wait_for_deployment_jobs(deployment_id, expected_jobs=job_count, timeout_seconds=180)
    recovery_seconds = time.monotonic() - kill_time
    recovered_jobs = len([job for job in jobs if job.get("status") in TERMINAL_STATUSES and job.get("attempts", 0) > 1])
    completed_after_failure = len([job for job in jobs if job.get("status") in TERMINAL_STATUSES])

    return {
        "deployment_id": deployment_id,
        "job_count": job_count,
        "worker_count": worker_count,
        "running_jobs_at_worker_kill": running_at_kill,
        "claimed_jobs_at_worker_kill": claimed_at_kill,
        "recovered_jobs": recovered_jobs,
        "recovery_seconds": round(recovery_seconds, 3),
        "completed_after_worker_failure": completed_after_failure,
        "completion_after_failure_percent": round(completed_after_failure / job_count * 100, 2),
        "success_jobs": count_status(jobs, "success"),
        "timeout_jobs": count_status(jobs, "timeout"),
        "note": "Lease recovery uses timeout-named mock devices so jobs remain running long enough to abandon and reclaim without changing backend behavior.",
    }


def run_retry_timeout(worker_concurrency: int) -> dict[str, Any]:
    print("\n=== retry and timeout ===", flush=True)
    reset_stack(worker_count=2, worker_concurrency=worker_concurrency)
    payload = generate_failure_timeout_yaml(success_count=10, failure_count=5, timeout_count=5)
    response = post_yaml("/deployments", payload)
    deployment_id = response["deployment_id"]
    start = time.monotonic()
    jobs = wait_for_deployment_jobs(deployment_id, expected_jobs=20, timeout_seconds=180)
    elapsed = time.monotonic() - start

    retried_jobs = [job for job in jobs if job.get("attempts", 0) > 1]
    retried_successes = [job for job in retried_jobs if job.get("status") == "success"]
    success_jobs = count_status(jobs, "success")
    timeout_jobs = count_status(jobs, "timeout")
    failed_jobs = count_status(jobs, "failed")

    return {
        "deployment_id": deployment_id,
        "job_count": 20,
        "success_jobs": success_jobs,
        "failed_jobs": failed_jobs,
        "timeout_jobs": timeout_jobs,
        "retried_jobs": len(retried_jobs),
        "retried_successes": len(retried_successes),
        "retry_success_rate_percent": round(len(retried_successes) / len(retried_jobs) * 100, 2) if retried_jobs else None,
        "final_deployment_success_percent": round(success_jobs / 20 * 100, 2),
        "completion_seconds": round(elapsed, 3),
        "note": "Existing mock failure/timeout injection is deterministic by device name; retried fail/timeout jobs do not later become successful without backend changes.",
    }


def run_drift_accuracy(worker_concurrency: int) -> dict[str, Any]:
    print("\n=== drift accuracy ===", flush=True)
    reset_stack(worker_count=2, worker_concurrency=worker_concurrency)
    payload = generate_network_yaml(2, "drift", names=["core-router", "access-switch"])
    response = post_yaml("/deployments", payload)
    deployment_id = response["deployment_id"]
    wait_for_deployment_jobs(deployment_id, expected_jobs=2)

    post_json("/devices/core-router/mutate", {"remove_vlan": 10})
    post_json("/devices/core-router/mutate", {"clear_firewall_rules": True})
    report = get_json("/drift/core-router")

    detected_items = len(report.get("missing_vlans") or []) + len(report.get("missing_firewall_rules") or [])
    injected_items = 2

    return {
        "deployment_id": deployment_id,
        "device_name": "core-router",
        "drift_items_injected": injected_items,
        "drift_items_detected": detected_items,
        "detection_accuracy_percent": round(detected_items / injected_items * 100, 2),
        "missing_vlans": report.get("missing_vlans") or [],
        "missing_firewall_rules_count": len(report.get("missing_firewall_rules") or []),
        "raw_report": report,
    }


def run_heartbeat_monitoring(worker_concurrency: int) -> dict[str, Any]:
    print("\n=== heartbeat monitoring ===", flush=True)
    worker_count = 2
    reset_stack(worker_count=worker_count, worker_concurrency=worker_concurrency)
    samples: list[list[dict[str, Any]]] = []
    sample_started = time.monotonic()
    for _ in range(8):
        samples.append(get_json("/agents"))
        time.sleep(2)
    sample_window_seconds = time.monotonic() - sample_started
    average_interval = average_heartbeat_interval(samples)

    kill_time = time.monotonic()
    compose(["kill", "worker"], check=True)
    unhealthy_delay = wait_for_unhealthy_agents(expected_unhealthy=worker_count, timeout_seconds=60) - kill_time
    agents = get_json("/agents")
    unhealthy_count = len([agent for agent in agents if agent.get("status") == "unhealthy"])

    return {
        "worker_count": worker_count,
        "sample_window_seconds": round(sample_window_seconds, 3),
        "average_heartbeat_interval_seconds": round(average_interval, 3) if average_interval else None,
        "unhealthy_detection_delay_seconds": round(unhealthy_delay, 3),
        "unhealthy_agents_reported": unhealthy_count,
        "agent_availability_reporting_accuracy_percent": round(unhealthy_count / worker_count * 100, 2),
    }


def reset_stack(worker_count: int, worker_concurrency: int) -> None:
    compose(["down", "-v"], check=True)
    compose(
        ["up", "-d", "--scale", f"worker={worker_count}", "postgres", "controller", "worker"],
        env=compose_env(worker_concurrency),
        check=True,
    )
    wait_for_controller()
    wait_for_agents(worker_count)


def wait_for_controller(timeout_seconds: float = 90) -> None:
    deadline = time.monotonic() + timeout_seconds
    last_error = ""
    while time.monotonic() < deadline:
        try:
            health = get_json("/health", timeout=2)
            if health.get("status") == "ok":
                return
        except Exception as exc:  # noqa: BLE001
            last_error = str(exc)
        time.sleep(1)
    raise RuntimeError(f"controller did not become healthy: {last_error}")


def wait_for_agents(expected: int, timeout_seconds: float = 60) -> None:
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        agents = get_json("/agents")
        healthy = [agent for agent in agents if agent.get("status") == "healthy"]
        if len(healthy) >= expected:
            return
        time.sleep(1)
    raise RuntimeError(f"expected {expected} healthy agents")


def wait_for_unhealthy_agents(expected_unhealthy: int, timeout_seconds: float) -> float:
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        agents = get_json("/agents")
        unhealthy = [agent for agent in agents if agent.get("status") == "unhealthy"]
        if len(unhealthy) >= expected_unhealthy:
            return time.monotonic()
        time.sleep(0.5)
    raise RuntimeError(f"expected {expected_unhealthy} unhealthy agents")


def wait_for_deployment_jobs(deployment_id: str, expected_jobs: int, timeout_seconds: float = 120) -> list[dict[str, Any]]:
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        jobs = get_json(f"/deployments/{deployment_id}/jobs")
        if len(jobs) == expected_jobs and all(job.get("status") in TERMINAL_STATUSES for job in jobs):
            return jobs
        time.sleep(0.25)
    jobs = get_json(f"/deployments/{deployment_id}/jobs")
    statuses = {status: count_status(jobs, status) for status in ["pending", "running", "success", "failed", "timeout"]}
    raise RuntimeError(f"deployment {deployment_id} did not finish in time: {statuses}")


def wait_for_running_jobs(deployment_id: str, minimum_running: int, timeout_seconds: float) -> list[dict[str, Any]]:
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        jobs = get_json(f"/deployments/{deployment_id}/jobs")
        if count_status(jobs, "running") >= minimum_running:
            return jobs
        time.sleep(0.05)
    jobs = get_json(f"/deployments/{deployment_id}/jobs")
    statuses = {status: count_status(jobs, status) for status in ["pending", "running", "success", "failed", "timeout"]}
    raise RuntimeError(f"deployment {deployment_id} did not reach {minimum_running} running jobs: {statuses}")


def generate_network_yaml(job_count: int, prefix: str, names: list[str] | None = None) -> str:
    device_names = names or [f"{prefix}-device-{index:04d}" for index in range(1, job_count + 1)]
    lines = ["devices:"]
    for index, name in enumerate(device_names):
        device_type = ["router", "switch", "firewall"][index % 3]
        lines.extend([f"  - name: {name}", f"    type: {device_type}"])
    lines.extend(
        [
            "",
            "vlans:",
            "  - id: 10",
            "    name: engineering",
            "    subnet: 10.10.0.0/24",
            "  - id: 20",
            "    name: guest",
            "    subnet: 10.20.0.0/24",
            "",
            "firewall_rules:",
            "  - source: guest",
            "    destination: engineering",
            "    port: 22",
            "    action: deny",
        ]
    )
    return "\n".join(lines) + "\n"


def generate_failure_timeout_yaml(success_count: int, failure_count: int, timeout_count: int) -> str:
    names = [f"success-device-{index:02d}" for index in range(1, success_count + 1)]
    names += [f"fail-device-{index:02d}" for index in range(1, failure_count + 1)]
    names += [f"timeout-device-{index:02d}" for index in range(1, timeout_count + 1)]
    return generate_network_yaml(len(names), "retry", names=names)


def generate_timeout_recovery_yaml(job_count: int) -> str:
    names = [f"timeout-recovery-device-{index:02d}" for index in range(1, job_count + 1)]
    return generate_network_yaml(job_count, "lease", names=names)


def get_json(path: str, timeout: float = 10) -> Any:
    with urllib.request.urlopen(f"{API_BASE}{path}", timeout=timeout) as response:
        return json.loads(response.read().decode("utf-8"))


def post_yaml(path: str, payload: str) -> Any:
    request = urllib.request.Request(
        f"{API_BASE}{path}",
        data=payload.encode("utf-8"),
        method="POST",
    )
    return read_json_request(request)


def post_json(path: str, payload: dict[str, Any]) -> Any:
    request = urllib.request.Request(
        f"{API_BASE}{path}",
        data=json.dumps(payload).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    return read_json_request(request)


def read_json_request(request: urllib.request.Request) -> Any:
    try:
        with urllib.request.urlopen(request, timeout=20) as response:
            return json.loads(response.read().decode("utf-8"))
    except urllib.error.HTTPError as exc:
        body = exc.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"{request.full_url} failed with {exc.code}: {body}") from exc


def compose(args: list[str], env: dict[str, str] | None = None, check: bool = True) -> subprocess.CompletedProcess[str]:
    merged_env = os.environ.copy()
    merged_env["COMPOSE_PROJECT_NAME"] = COMPOSE_PROJECT_NAME
    if env:
        merged_env.update(env)
    command = ["docker", "compose", *args]
    print(f"$ {' '.join(command)}", flush=True)
    completed = subprocess.run(
        command,
        cwd=ROOT,
        env=merged_env,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
    )
    if completed.returncode != 0 and check:
        print(completed.stdout, flush=True)
        raise subprocess.CalledProcessError(completed.returncode, command, output=completed.stdout)
    return completed


def compose_env(worker_concurrency: int) -> dict[str, str]:
    return {"WORKER_CONCURRENCY": str(worker_concurrency)}


def count_status(jobs: list[dict[str, Any]], status: str) -> int:
    return len([job for job in jobs if job.get("status") == status])


def job_latencies_ms(jobs: list[dict[str, Any]]) -> list[float]:
    latencies: list[float] = []
    for job in jobs:
        started = parse_time(job.get("started_at"))
        completed = parse_time(job.get("completed_at"))
        if started and completed:
            latencies.append((completed - started).total_seconds() * 1000)
    return latencies


def parse_time(value: str | None) -> dt.datetime | None:
    if not value:
        return None
    return dt.datetime.fromisoformat(value.replace("Z", "+00:00"))


def percentile(values: list[float], pct: int) -> float:
    if not values:
        return 0
    values = sorted(values)
    index = (len(values) - 1) * pct / 100
    lower = int(index)
    upper = min(lower + 1, len(values) - 1)
    weight = index - lower
    return values[lower] * (1 - weight) + values[upper] * weight


def average_heartbeat_interval(samples: list[list[dict[str, Any]]]) -> float | None:
    by_agent: dict[str, list[dt.datetime]] = {}
    for sample in samples:
        for agent in sample:
            heartbeat = parse_time(agent.get("last_heartbeat"))
            if not heartbeat:
                continue
            by_agent.setdefault(agent["id"], [])
            if not by_agent[agent["id"]] or by_agent[agent["id"]][-1] != heartbeat:
                by_agent[agent["id"]].append(heartbeat)

    intervals: list[float] = []
    for timestamps in by_agent.values():
        for previous, current in zip(timestamps, timestamps[1:]):
            intervals.append((current - previous).total_seconds())
    return statistics.mean(intervals) if intervals else None


def write_results(output_dir: pathlib.Path, results: dict[str, Any]) -> None:
    (output_dir / "results.json").write_text(json.dumps(results, indent=2) + "\n")
    (output_dir / "report.md").write_text(render_report(results))


def render_report(results: dict[str, Any]) -> str:
    lines: list[str] = [
        "# Benchmark Report",
        "",
        f"Generated: `{results['metadata'].get('finished_at', now_iso())}`",
        "",
        "These measurements were collected by `benchmarks/run_benchmarks.py` against the existing Docker Compose implementation.",
        "No benchmark-only backend behavior was added.",
        "",
        "## Job Throughput",
        "",
        "| Workers | Jobs | Completion Time (s) | Jobs/sec | Avg Job Latency (ms) | P95 Job Latency (ms) |",
        "|---:|---:|---:|---:|---:|---:|",
    ]

    for row in results["throughput"]:
        lines.append(
            "| {worker_count} | {job_count} | {completion_seconds:.3f} | {jobs_per_second:.3f} | {avg} | {p95} |".format(
                worker_count=row["worker_count"],
                job_count=row["job_count"],
                completion_seconds=row["completion_seconds"],
                jobs_per_second=row["jobs_per_second"],
                avg=format_number(row["average_job_latency_ms"]),
                p95=format_number(row["p95_job_latency_ms"]),
            )
        )

    lines.extend(
        [
            "",
            "## Worker Scaling",
            "",
            "| Jobs | 1 Worker Time (s) | 4 Worker Time (s) | Completion Time Improvement | 1 Worker Jobs/sec | 4 Worker Jobs/sec | Throughput Improvement |",
            "|---:|---:|---:|---:|---:|---:|---:|",
        ]
    )
    for row in results["worker_scaling"]:
        lines.append(
            f"| {row['job_count']} | {row['one_worker_seconds']:.3f} | {row['four_worker_seconds']:.3f} | "
            f"{row['completion_time_improvement_percent']:.2f}% | {row['one_worker_jobs_per_second']:.3f} | "
            f"{row['four_worker_jobs_per_second']:.3f} | {row['throughput_improvement_percent']:.2f}% |"
        )

    lease = results["lease_recovery"]
    lines.extend(
        [
            "",
            "## Lease Recovery",
            "",
            "| Jobs | Running At Kill | Claimed At Kill | Recovered Jobs | Recovery Time (s) | Completed After Failure | Timeout Jobs |",
            "|---:|---:|---:|---:|---:|---:|---:|",
            f"| {lease.get('job_count')} | {lease.get('running_jobs_at_worker_kill')} | {lease.get('claimed_jobs_at_worker_kill')} | "
            f"{lease.get('recovered_jobs')} | {format_number(lease.get('recovery_seconds'))} | "
            f"{lease.get('completion_after_failure_percent')}% | {lease.get('timeout_jobs')} |",
            "",
            f"Note: {lease.get('note')}",
            "",
            "## Retry and Timeout",
            "",
            "| Jobs | Success | Failed | Timeout | Retried Jobs | Retry Success Rate | Final Success % | Completion Time (s) |",
            "|---:|---:|---:|---:|---:|---:|---:|---:|",
        ]
    )
    retry = results["retry_timeout"]
    lines.append(
        f"| {retry.get('job_count')} | {retry.get('success_jobs')} | {retry.get('failed_jobs')} | {retry.get('timeout_jobs')} | "
        f"{retry.get('retried_jobs')} | {format_percent(retry.get('retry_success_rate_percent'))} | "
        f"{retry.get('final_deployment_success_percent')}% | {format_number(retry.get('completion_seconds'))} |"
    )
    lines.extend(["", f"Note: {retry.get('note')}", ""])

    drift = results["drift_accuracy"]
    heartbeat = results["heartbeat_monitoring"]
    lines.extend(
        [
            "## Drift Detection Accuracy",
            "",
            "| Device | Injected Items | Detected Items | Accuracy | Missing VLANs | Missing Firewall Rules |",
            "|---|---:|---:|---:|---|---:|",
            f"| {drift.get('device_name')} | {drift.get('drift_items_injected')} | {drift.get('drift_items_detected')} | "
            f"{drift.get('detection_accuracy_percent')}% | {', '.join(map(str, drift.get('missing_vlans', [])))} | "
            f"{drift.get('missing_firewall_rules_count')} |",
            "",
            "## Heartbeat Monitoring",
            "",
            "| Workers Killed | Avg Heartbeat Interval (s) | Unhealthy Detection Delay (s) | Unhealthy Agents Reported | Reporting Accuracy |",
            "|---:|---:|---:|---:|---:|",
            f"| {heartbeat.get('worker_count')} | {format_number(heartbeat.get('average_heartbeat_interval_seconds'))} | "
            f"{format_number(heartbeat.get('unhealthy_detection_delay_seconds'))} | {heartbeat.get('unhealthy_agents_reported')} | "
            f"{heartbeat.get('agent_availability_reporting_accuracy_percent')}% |",
            "",
            "## Resume-Ready Metrics",
            "",
        ]
    )

    resume_lines = resume_metrics(results)
    lines.extend([f"- {line}" for line in resume_lines])
    lines.append("")
    return "\n".join(lines)


def resume_metrics(results: dict[str, Any]) -> list[str]:
    throughput_100_4 = find_throughput(results["throughput"], 100, 4)
    throughput_500_4 = find_throughput(results["throughput"], 500, 4)
    scaling_500 = next((row for row in results["worker_scaling"] if row["job_count"] == 500), None)
    lease = results["lease_recovery"]
    drift = results["drift_accuracy"]

    lines = []
    if throughput_100_4:
        lines.append(
            f"Processed 100 deployment jobs in {throughput_100_4['completion_seconds']:.3f}s across 4 workers "
            f"({throughput_100_4['jobs_per_second']:.2f} jobs/sec)."
        )
    if throughput_500_4:
        lines.append(
            f"Processed 500 deployment jobs in {throughput_500_4['completion_seconds']:.3f}s across 4 workers "
            f"({throughput_500_4['jobs_per_second']:.2f} jobs/sec)."
        )
    if scaling_500:
        lines.append(
            f"Reduced 500-job deployment completion time by {scaling_500['completion_time_improvement_percent']:.2f}% "
            "when scaling from 1 to 4 workers."
        )
    if lease:
        lines.append(
            f"Recovered {lease.get('recovered_jobs')} abandoned jobs to terminal state after worker termination and completed "
            f"{lease.get('completion_after_failure_percent')}% of jobs through lease-based failover."
        )
    if drift:
        lines.append(
            f"Detected {drift.get('detection_accuracy_percent')}% of injected configuration drift items "
            f"({drift.get('drift_items_detected')}/{drift.get('drift_items_injected')})."
        )
    return lines


def find_throughput(rows: list[dict[str, Any]], job_count: int, worker_count: int) -> dict[str, Any] | None:
    return next((row for row in rows if row["job_count"] == job_count and row["worker_count"] == worker_count), None)


def format_number(value: Any) -> str:
    if value is None:
        return "n/a"
    if isinstance(value, (float, int)):
        return f"{value:.3f}"
    return str(value)


def format_percent(value: Any) -> str:
    if value is None:
        return "n/a"
    return f"{value}%"


def parse_ints(value: str) -> list[int]:
    return [int(part.strip()) for part in value.split(",") if part.strip()]


def now_iso() -> str:
    return dt.datetime.now(dt.timezone.utc).isoformat()


if __name__ == "__main__":
    sys.exit(main())
