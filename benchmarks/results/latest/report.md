# Benchmark Report

Generated: `2026-06-11T17:39:58.890278+00:00`

These measurements were collected by `benchmarks/run_benchmarks.py` against the existing Docker Compose implementation.
No benchmark-only backend behavior was added.

## Job Throughput

| Workers | Jobs | Completion Time (s) | Jobs/sec | Avg Job Latency (ms) | P95 Job Latency (ms) |
|---:|---:|---:|---:|---:|---:|
| 1 | 10 | 2.075 | 4.819 | 22.514 | 35.692 |
| 1 | 50 | 2.349 | 21.288 | 36.955 | 47.736 |
| 1 | 100 | 2.344 | 42.669 | 38.881 | 46.982 |
| 1 | 500 | 4.490 | 111.349 | 44.745 | 54.207 |
| 2 | 10 | 2.099 | 4.764 | 23.360 | 35.479 |
| 2 | 50 | 2.096 | 23.853 | 33.802 | 45.113 |
| 2 | 100 | 2.085 | 47.964 | 37.059 | 44.426 |
| 2 | 500 | 3.471 | 144.033 | 45.198 | 54.802 |
| 4 | 10 | 2.055 | 4.865 | 26.548 | 40.487 |
| 4 | 50 | 2.076 | 24.090 | 33.811 | 45.590 |
| 4 | 100 | 2.093 | 47.775 | 34.100 | 44.776 |
| 4 | 500 | 2.646 | 188.980 | 46.479 | 56.419 |

## Worker Scaling

| Jobs | 1 Worker Time (s) | 4 Worker Time (s) | Completion Time Improvement | 1 Worker Jobs/sec | 4 Worker Jobs/sec | Throughput Improvement |
|---:|---:|---:|---:|---:|---:|---:|
| 10 | 2.075 | 2.055 | 0.96% | 4.819 | 4.865 | 0.95% |
| 50 | 2.349 | 2.076 | 11.62% | 21.288 | 24.090 | 13.16% |
| 100 | 2.344 | 2.093 | 10.71% | 42.669 | 47.775 | 11.97% |
| 500 | 4.490 | 2.646 | 41.07% | 111.349 | 188.980 | 69.72% |

## Lease Recovery

| Jobs | Running At Kill | Claimed At Kill | Recovered Jobs | Recovery Time (s) | Completed After Failure | Timeout Jobs |
|---:|---:|---:|---:|---:|---:|---:|
| 6 | 6 | 6 | 6 | 51.094 | 100.0% | 6 |

Note: Lease recovery uses timeout-named mock devices so jobs remain running long enough to abandon and reclaim without changing backend behavior.

## Retry and Timeout

| Jobs | Success | Failed | Timeout | Retried Jobs | Retry Success Rate | Final Success % | Completion Time (s) |
|---:|---:|---:|---:|---:|---:|---:|---:|
| 20 | 10 | 5 | 5 | 10 | 0.0% | 50.0% | 27.186 |

Note: Existing mock failure/timeout injection is deterministic by device name; retried fail/timeout jobs do not later become successful without backend changes.

## Drift Detection Accuracy

| Device | Injected Items | Detected Items | Accuracy | Missing VLANs | Missing Firewall Rules |
|---|---:|---:|---:|---|---:|
| core-router | 2 | 2 | 100.0% | 10 | 1 |

## Heartbeat Monitoring

| Workers Killed | Avg Heartbeat Interval (s) | Unhealthy Detection Delay (s) | Unhealthy Agents Reported | Reporting Accuracy |
|---:|---:|---:|---:|---:|
| 2 | 5.007 | 14.017 | 2 | 100.0% |

## Resume-Ready Metrics

- Processed 100 deployment jobs in 2.093s across 4 workers (47.77 jobs/sec).
- Processed 500 deployment jobs in 2.646s across 4 workers (188.98 jobs/sec).
- Reduced 500-job deployment completion time by 41.07% when scaling from 1 to 4 workers.
- Recovered 6 abandoned jobs to terminal state after worker termination and completed 100.0% of jobs through lease-based failover.
- Detected 100.0% of injected configuration drift items (2/2).
