# SysArmor Test Framework

`test/` is organized by the question a run answers, not by historical script
shape.

## Scopes

| Scope | Answers | Default environment |
|---|---|---|
| `unit` | Does local code behave correctly? | none |
| `endpoint` | Does one endpoint agent/sensor work, and what does it cost? | `vm-endpoint` |
| `topology` | Does the manager-agent-attacker product path work? | `vm-topology` |
| `platform` | Do manager APIs, storage, policy, RBAC, and control contracts work? | `container` or local processes |

## Environments

| ENV | Shape | Use |
|---|---|---|
| `container` | compose-based lightweight topology | Fast platform and manager checks |
| `vm-endpoint` | one VM named `node-a` | Endpoint detection, resource cost, profiling, soak |
| `vm-topology` | `mgr`, `node-a`, `attacker` VMs | Full product path and C2 scenarios |

`vm-endpoint` is the source of truth for endpoint CPU/memory conclusions.
`vm-topology` may collect resource samples, but those are topology observations,
not clean endpoint-cost conclusions.

## Layout

```text
test/
  environments/
    container/
    vm-endpoint/
    vm-topology/
  e2e/
    local-agent/       endpoint-owned sensor and local control checks
    agent-runtime/     daemon/runtime/restart/capability checks
    manager-cloud/     manager ingest/query/policy/response/incident checks
    control-plane/     gRPC contract and mTLS checks
    reliability/       spool/outage/shutdown/backpressure checks
    storage/           store and query checks
  benchmarks/
    endpoint/          endpoint benchmark runner and report
    topology/          topology benchmark runner and report
    matrix/            shared lifecycle/effectiveness report helpers
    modules/           Go microbenchmarks
    perf/              short legacy samplers
  data/
    scenarios/         security behavior contracts
    workloads/         benign or synthetic pressure sources
    policies/          collection/detection/resource/response samples
    content/           IOC/context/rulepack content
  shared/
    harness/           environment lifecycle
    recorder/          VM timeline recorder
    diagnostics/       perf/pprof/strace helpers
    reports/           report aggregation
```

## Common Commands

```bash
make -C test up ENV=vm-endpoint
make -C test down ENV=vm-endpoint
make -C test status ENV=vm-topology

make -C test test-unit
make -C test test-endpoint
make -C test test-topology SCENARIO=apt-fileless-c2
make -C test test-platform

make -C test bench-endpoint SYSARMOR_BENCH_PROFILE=quick SYSARMOR_BENCH_WORKLOAD=business-normal
make -C test bench-endpoint SYSARMOR_BENCH_PROFILE=medium SYSARMOR_BENCH_WORKLOAD=business-normal SYSARMOR_BENCH_SCENARIO=apt-fileless-c2-local SYSARMOR_BENCH_POLICIES='test/data/policies/collection-balanced.json'
make -C test bench-endpoint SYSARMOR_BENCH_PROFILE=long SYSARMOR_BENCH_WORKLOAD=business-normal
make -C test bench-topology ENV=vm-topology
make -C test diag-endpoint SYSARMOR_BENCH_WORKLOAD=edr-activity-heavy

make -C test recorder-start RUN_ID=my-run
make -C test recorder-mark RUN_ID=my-run PHASE=workload_start DETAIL=business-normal
make -C test recorder-stop RUN_ID=my-run
make -C test recorder-report RUN_ID=my-run
```

## Endpoint Benchmark Contract

`bench-endpoint` recreates a fresh `vm-endpoint` VM by default and writes:

```text
test/.results/bench-endpoint/<run-id>/
  manifest.json
  matrix.csv
  <policy>/
    manifest.json
    artifacts.json
    timeline.csv
    markers.ndjson
    events.ndjson
    events.scope.ndjson
    signals.ndjson
    signals.scope.ndjson
    scope-labels.json
    summary.json
    raw/
    raw.tar
    profiles/
```

Use `SYSARMOR_BENCH_PROFILE=quick` for local smoke runs and CI-shaped checks,
`medium` for daily detection plus performance correlation, and `long` for
endpoint CPU/RSS conclusions. Profile defaults live in
`test/benchmarks/endpoint/profiles/` and every value can still be overridden by
`SYSARMOR_BENCH_*` environment variables.

| Profile | Purpose | Default shape |
|---|---|---|
| `quick` | Prove benchmark wiring and catch obvious regressions | seconds-scale windows, no host baseline |
| `medium` | Correlate detection result and resource cost during iteration | about 10 minutes per policy |
| `long` | Produce credible endpoint CPU/RSS data | long steady/workload/persistence windows for CPU/RSS conclusions |

The standard report phases are:

| Phase | Meaning |
|---|---|
| `startup` | Fresh VM, agent/sensor readiness, content/policy apply, and stabilization. Useful for troubleshooting, not steady resource conclusions. |
| `steady` | Policy is active, with no benchmark workload and no scenario activity. This is the protected idle cost. |
| `workload` | Benign background workload only. Activity and persistence windows are excluded from this derived report phase. |
| `activity` | Scenario execution window when `SYSARMOR_BENCH_SCENARIO` is set. |
| `persistence` | Observation window after scenario execution, used for signal/alert latency and attribution cost. |
| `overall` | Full recorder lifecycle for the policy run. |

The raw marker stream may keep lower-level execution markers such as
`policy_apply_start`, `workload_start`, `scenario_start`, or
`scenario_observe_start`. Reports normalize those markers into the standard
phases above.

Resource conclusions should report CPU percentage semantics, RSS, sample
count, drops, parse errors, workload, policy, duration, and whether profiling
was enabled.

Raw artifacts are intentionally kept for later analysis:

- `manifest.json`: run or policy metadata, benchmark profile, phase durations, recorder cadence, and artifact paths;
- `artifacts.json`: policy-level artifact meaning for tools and humans;
- `timeline.csv`: low-disturbance process CPU/RSS samples;
- `raw/*.health.json`: low-frequency semantic health snapshots;
- `events.ndjson` and `signals.ndjson`: continuous raw event/signal watch streams;
- `events.scope.ndjson` and `signals.scope.ndjson`: offline label-scoped streams derived from the raw streams;
- `profiles/*`: raw pprof/runtime outputs for enabled phases.

## Topology Benchmark Contract

`bench-topology` uses `vm-topology` and composes endpoint benchmark cases across
workloads and scenarios. Its output is:

```text
test/.results/bench-topology/<run-id>/
  manifest.json
  matrix.csv
  cases/
```

Use this for product-path comparisons and effectiveness matrices, not for the
clean endpoint-cost baseline.
