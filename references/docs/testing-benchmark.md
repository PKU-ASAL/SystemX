# Testing And Benchmarking

This document defines the test and benchmark model for SysArmor Next.

## Test Roles

The test system answers five questions:

| Role | Question |
|---|---|
| Scenario | Does a security behavior produce expected events, signals, response, evidence, or incident state? |
| Workload | Can we generate stable non-security or synthetic pressure? |
| Recorder | What are CPU, RSS, EPS, drops, and signal counts over time? |
| Benchmark | How do sensor, policy, and workload combinations compare? |
| Diagnostic | Why is a sensor or agent expensive under a specific load? |

Functional E2E, benchmark, and diagnostic logic should stay separate.

## Directory Contract

```text
test/
  env/                 topology setup for VM/container
  scenarios/           security scenarios with expected behavior
  workloads/           performance workloads without security assertions
  policies/            collection/detection/resource policy samples
  harness/             functional E2E orchestration
  tools/
    recorder/          long-running low-disturbance timeline sampler
    benchmarks/        matrix runners and reports
    diagnostics/       perf/pprof/strace helpers
  .results/            generated outputs
```

## Scenario Vs Workload

Scenario is a functional contract. Examples:

- `apt-fileless-c2`;
- `apt-staged-drop`;
- `benign-ci-noise`.

It should assert whether expected events, signals, incidents, evidence, and responses appear or do not appear.

Workload is a pressure source. Examples:

- `business-normal`;
- `host-activity-heavy`;
- `edr-activity-heavy`.

`business-normal` is the default workload for the slim matrix. Heavier workloads such as `host-activity-heavy` and `edr-activity-heavy` are stress/cost extensions and should be enabled explicitly with `WORKLOADS=...`.

Workloads should be repeatable, configurable, and not depend on unstable external downloads. Benign workloads must not touch C2 IoCs, payload paths, persistence paths, or sensitive credential paths unless the scenario is explicitly testing false positive controls.

## Labels And Watch Filters

Tests must not force product architecture to depend on a test-only `scenario` field. Current event/signal watch filtering uses generic labels:

```text
benchmark_run=...
workload=...
policy_profile=...
sensor_runtime=...
scope_type=...
```

Watch filters support:

- labels;
- `after_sequence`;
- `since_observed_at`;
- `until_observed_at`.

This is useful for tests and production debugging.

## Recorder

The VM recorder is the current performance baseline tool:

```bash
make -C test recorder-vm-start RUN_ID=my-run
make -C test recorder-vm-mark RUN_ID=my-run PHASE=workload_start DETAIL=business-normal
make -C test recorder-vm-stop RUN_ID=my-run
make -C test recorder-vm-report RUN_ID=my-run
```

Recorder output:

```text
test/.results/recordings/<run-id>/
  timeline.csv
  markers.ndjson
  events.ndjson
  signals.ndjson
  summary.json
```

`timeline.csv` is for resource and health samples:

- agent CPU/RSS;
- sensor CPU/RSS;
- total EDR CPU/RSS;
- sensor global events seen;
- scoped event/signal counters;
- drops and parse errors;
- active policy id/version.

`events.ndjson` and `signals.ndjson` are scoped by labels and sequence cursor. Reports count phase event/signal deltas by frame `observedAt` and marker windows.

## Benchmark

`bench-collection-vm` composes one case across the selected policy set:

```bash
make -C test bench-collection-vm \
  SYSARMOR_BENCH_WORKLOAD=business-normal \
  POLICIES='test/policies/collection-minimal-high-signal.json'
```

`bench-matrix-vm` is the default endpoint effectiveness/performance gate. Its slim default runs:

```text
3 collection policies x 3 scenarios x 1 background workload = 9 cases
```

Default policy set:

- `test/policies/collection-minimal-high-signal.json`;
- `test/policies/collection-edr-balanced.json`;
- `test/policies/collection-incident-deep.json`.

Default workload:

- `business-normal`.

Default scenarios:

- `apt-fileless-c2`;
- `apt-staged-drop`;
- `benign-ci-noise`.

`collection-debug-wide` is no longer part of the supported default matrix. If a broad debug policy is needed for a one-off investigation, create it outside the default benchmark set and keep it out of long-running EDR comparisons.

Each case should:

1. attach labels;
2. start recorder;
3. mark baseline;
4. apply content;
5. apply collection policy;
6. mark settle and steady windows;
7. run workload or scenario;
8. stop recorder;
9. generate summary and matrix.

Important output:

```text
test/.results/bench-collection-vm/<run-id>/
  matrix.csv
  <policy>/
    timeline.csv
    markers.ndjson
    events.ndjson
    signals.ndjson
    summary.json
    collection-apply.json
    workload.out
    workload.err
```

Matrix runs also write:

```text
test/.results/bench-matrix-vm/<run-id>/matrix.csv
test/.results/effectiveness/<run-id>/matrix.csv
test/.results/effectiveness/<run-id>/truth_steps.csv
```

## Resource Metrics

For VM deployment, report resource use from inside the VM. For containerized deployment, report host/cgroup perspective.

Track:

- baseline without EDR;
- EDR running idle;
- EDR during policy apply;
- EDR during steady state;
- EDR during workload;
- drops and parse errors;
- business workload latency/throughput when available.

Policy apply and sensor reload spikes must be separated from steady-state cost.

## Diagnostics

Diagnostics explain cost; they are not official resource conclusions.

Allowed diagnostic tools:

- perf top/record/report;
- pprof where available;
- strace summary;
- backend-specific telemetry.

Useful pattern:

```text
start diagnostic sampling
run workload or scenario
stop diagnostic sampling
compare with recorder timeline
```

## Current Practical Gates

Use these based on change type:

```bash
go test ./internal/agent/... ./internal/endpoint/... ./cmd/sysarmorctl
make -C test e2e-agent-real-tetragon-owned-vm
make -C test bench-matrix-vm
```

For focused iteration, run `bench-collection-vm` with explicit `SYSARMOR_BENCH_WORKLOAD` and/or `SYSARMOR_BENCH_SCENARIO`. For final endpoint refinement, prefer the slim `bench-matrix-vm` gate because it checks effectiveness, resource cost, drops, parse errors, and benign false positives in one pass.

Cloud/platform paths have broader functional gates, but endpoint refinement work should prefer the local agent + VM real sensor path.
