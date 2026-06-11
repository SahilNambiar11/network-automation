# Benchmark Suite

This directory contains an automated benchmark harness for the distributed Go network automation controller.

Run from the repository root:

```sh
python3 benchmarks/run_benchmarks.py
```

The script writes:

- `benchmarks/results/latest/results.json`
- `benchmarks/results/latest/report.md`

It drives the existing Docker Compose stack and HTTP API. It does not modify backend or frontend behavior.

The full run resets Docker volumes multiple times and includes lease recovery and timeout tests, so expect it to take several minutes.
