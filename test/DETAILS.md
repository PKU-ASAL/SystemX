# Test Environments And Reports

This document defines how SysArmor test results are produced and interpreted.
Commands are intentionally kept in `make -C test help` and suite-local README
files so this document does not become a second Makefile.

## Lifecycle

```bash
make -C test up ENV=container
make -C test up ENV=vm-endpoint
make -C test up ENV=vm-topology
make -C test status ENV=vm-endpoint
make -C test down-all
```

Suites normally own their environment lifecycle. Manual `up` is useful for
diagnosis; always run the matching `down` after VM work.

## Test Data

- **Policy** selects collection and detection behavior.
- **Workload** creates repeatable background activity without a security
  assertion.
- **Scenario** performs malicious or benign behavior and carries expected
  labels.

Effectiveness tests combine all three. Performance tests use workload for the
resource window and may add a scenario to correlate cost with detection.

## Product Results

Product tests pass only when their declared boundary works. A local contract
test may use constructed input; a topology test uses the real distribution and
enrollment path. Each suite README states whether its sensor and infrastructure
are real or simulated.

Product success does not imply detection quality or acceptable resource cost.

## Effectiveness Results

The topology suite queries Manager for scoring inputs:

```text
manager.events.ndjson
manager.signals.ndjson
manager.incidents.ndjson
```

Local Agent watch files are diagnostics, not the default topology scoring
source. Reports include a case matrix and per-truth-step matches under:

```text
test/.results/effectiveness-topology/<run-id>/
test/.results/effectiveness/<run-id>/
```

Malicious scenarios define required events, signals, or incidents. Benign
scenarios define forbidden detections. Missing required truth or producing a
forbidden result fails the gate.

## Performance Results

Endpoint performance samples Agent and sensor processes on `node-a`. Platform
performance samples Manager, Gateway, Worker, Kafka, PostgreSQL, Redis, and
OpenSearch on `mgr`. These are separate cost surfaces and must not be added or
compared as if they were the same benchmark.

Endpoint reports use these phases:

| Phase | Meaning |
|---|---|
| `startup` | Installation, Agent start, policy apply, and sensor reload |
| `steady` | Settled runtime without the benchmark workload |
| `workload` | Full background workload interval |
| `activity` | Explicit scenario execution |
| `persistence` | Observation after scenario execution |
| `overall` | Entire recorded interval, including startup peaks |

Formal endpoint comparisons use `steady` and `workload`; `overall` explains
total run cost but is sensitive to provisioning and startup activity.

Typical endpoint output:

```text
test/.results/performance-endpoint/<run-id>/
  manifest.json
  matrix.csv
  <policy>/summary.json
  <policy>/timeline.csv
  <policy>/events.ndjson
  <policy>/signals.ndjson
```

A valid result has a non-empty timeline, the expected workload/scenario exit
status, no unexplained watcher errors, and explicit drop/parse-error counts.

## Result Discipline

- Do not compare a reused VM with a fresh VM as a strict regression result.
- Do not use a short smoke window as a stable CPU or memory baseline.
- Keep raw data when publishing a summary.
- Treat drops, parse errors, watcher termination, and failed workloads as test
  failures or explicitly explained limitations.
- State what a test does not prove alongside its conclusion.
