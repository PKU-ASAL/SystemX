# Testing And Benchmarking

This document defines the SysArmor Next test and benchmark model.

## First Principles

Tests should answer one clear question at the lightest environment that is still
realistic.

| Scope | Question | Environment |
|---|---|---|
| `unit` | Does the local implementation behave correctly? | none |
| `endpoint` | Does one endpoint agent/sensor work, and what does it cost? | `vm-endpoint` |
| `topology` | Does the manager-agent-attacker product path work? | `vm-topology` |
| `platform` | Do manager APIs, storage, policy, RBAC, and control contracts work? | `container` or local processes |

Endpoint CPU and memory conclusions must come from `vm-endpoint`, because it
avoids manager/attacker topology noise and starts faster for repeated profiling.
`vm-topology` remains important for product-path truth, but its resource samples
are topology observations rather than clean endpoint-cost baselines.

## Environment Contract

| ENV | Shape | Purpose |
|---|---|---|
| `container` | compose topology | Fast manager/platform checks |
| `vm-endpoint` | fresh one VM, `node-a` | Endpoint detection, resource usage, profiling, soak |
| `vm-topology` | `mgr`, `node-a`, `attacker` | Manager-agent-C2 integration |

## Benchmark Roles

| Role | Purpose |
|---|---|
| Scenario | Security behavior with expected events/signals/incidents. |
| Workload | Repeatable benign or synthetic pressure without security assertions. |
| Recorder | Low-disturbance timeline of CPU, RSS, health, events, drops, and markers. |
| Benchmark | Compare policy/workload/scenario combinations. |
| Diagnostic | Explain cost with pprof/perf/strace or backend telemetry. |

Functional E2E, benchmark, and diagnostic logic should stay separate.

## Performance Endpoint

Run:

```bash
make -C test performance-endpoint SYSARMOR_BENCH_PROFILE=quick SYSARMOR_BENCH_WORKLOAD=business-normal
make -C test performance-endpoint SYSARMOR_BENCH_PROFILE=medium SYSARMOR_BENCH_WORKLOAD=business-normal SYSARMOR_BENCH_SCENARIO=apt-fileless-c2-local SYSARMOR_BENCH_POLICIES='test/data/policies/collection-balanced.json'
make -C test performance-endpoint SYSARMOR_BENCH_PROFILE=long SYSARMOR_BENCH_WORKLOAD=business-normal
```

Useful knobs:

```bash
SYSARMOR_BENCH_PROFILE=quick|medium|long
SYSARMOR_BENCH_POLICIES='test/data/policies/collection-minimal.json'
SYSARMOR_BENCH_WORKLOAD_SECONDS=600
SYSARMOR_BENCH_WORKLOAD_REPEAT=0
SYSARMOR_BENCH_SCENARIO=apt-fileless-c2-local
SYSARMOR_BENCH_PROFILE_AGENT=1
SYSARMOR_BENCH_PROFILE_TYPES='cpu heap allocs goroutine runtime'
SYSARMOR_BENCH_PROFILE_PHASES='activity persistence'
SYSARMOR_BENCH_ACTIVITY_PROFILE_SECONDS=5
```

Output:

```text
test/.results/performance-endpoint/<run-id>/
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

`SYSARMOR_BENCH_PROFILE=quick` is the default developer profile. It uses short
seconds-scale windows so the runner can be exercised often. It is not a final
resource conclusion.

`SYSARMOR_BENCH_PROFILE=medium` is the daily detection plus performance profile.
It is about ten minutes per policy and is intended for questions like: under a
business workload, does a scenario produce the expected event/signal, and what
are agent/sensor CPU and RSS during steady, workload, activity, and persistence
windows?

`SYSARMOR_BENCH_PROFILE=long` is the default profile for endpoint CPU/RSS
conclusions. It includes long steady, workload, activity/persistence, and overall
windows to smooth startup spikes and collector jitter. Profiling is disabled by
default in all profiles because profile collection changes the workload being
measured; enable it only for diagnostic attribution.

Profile defaults are stored in `test/suites/performance/endpoint/profiles/quick.env`
`medium.env`, and `long.env`. Override individual windows with
`SYSARMOR_BENCH_*` variables when a specific experiment needs a different
duration.

Standard report phases:

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

For credible endpoint resource conclusions, use the `long` profile or longer
experiment-specific windows:

| Window | Suggested duration |
|---|---|
| idle/steady | 10-30 minutes |
| business workload | 30-60 minutes |
| heavy workload | 10-30 minutes |
| soak/leak check | 6-24 hours |

Agent profiling is a diagnostic layer, not the primary resource measurement.
The recorder's per-second `/proc` CPU/RSS samples are the low-disturbance
timeline used for steady/workload/activity/persistence averages. Enable
`SYSARMOR_BENCH_PROFILE_AGENT=1` only when a phase needs root-cause attribution
inside the agent.

When profiling is enabled, raw artifacts are written under each policy's
`profiles/` directory. The default diagnostic phases are `policy_apply activity
persistence`. `activity` CPU profiling uses
`SYSARMOR_BENCH_ACTIVITY_PROFILE_SECONDS` because the local agent debug endpoint
captures fixed-duration CPU profiles. `persistence` uses
`SYSARMOR_BENCH_SCENARIO_OBSERVE_SECONDS`. CPU profiles are serialized because
the agent debug endpoint accepts only one active profile at a time; avoid using
overlapping CPU phases such as `workload activity persistence` in the same
diagnostic run. Profiling runs also emit raw markers such as
`profile_activity_finish_start` and `profile_activity_finish_done`, making
profile collection overhead visible in `markers.ndjson` and
`summary.json.raw_phases` without changing the standard report phases.

## Effectiveness Topology

Run:

```bash
make -C test effectiveness-topology ENV=vm-topology
```

`effectiveness-topology` composes workload/scenario cases and writes:

```text
test/.results/effectiveness-topology/<run-id>/
  manifest.json
  matrix.csv
  cases/
```

Use this for product-path and effectiveness comparisons. Do not use it as the
primary endpoint CPU/RSS baseline.

## Recorder

Run:

```bash
make -C test recorder-start RUN_ID=my-run
make -C test recorder-mark RUN_ID=my-run PHASE=workload_start DETAIL=business-normal
make -C test recorder-stop RUN_ID=my-run
make -C test recorder-report RUN_ID=my-run
```

Recorder output:

```text
test/.results/recordings/<run-id>/
  timeline.csv
  markers.ndjson
  events.ndjson
  signals.ndjson
  summary.json
  raw/
  raw.tar
```

`timeline.csv` tracks:

- agent CPU/RSS;
- sensor CPU/RSS;
- total EDR CPU/RSS;
- event/signal counters;
- dropped events and parse errors;
- active policy id/version;
- marker-aligned phase windows.

The recorder uses `/proc` process sampling for the per-second resource timeline.
Semantic agent calls such as health/watch are sampled at a lower cadence
(`SYSARMOR_RECORDER_SEMANTIC_INTERVAL`, default `10s`) and copied into `raw/`.
For final endpoint resource reports, document sampling interval, observer
overhead, CPU percentage semantics, workload, policy, and duration.

The benchmark copies recorder artifacts into each policy directory and writes
machine-readable `manifest.json` plus `artifacts.json`. There is intentionally no
HTML report at this stage; raw data is the contract.

## Diagnostics

Diagnostics explain cost; they are not official resource conclusions.

Useful tools:

- Go CPU/heap/alloc/goroutine profiles when available;
- `perf top`, `perf record`, `perf report`;
- `strace -c` summaries;
- sensor/backend telemetry.

Pattern:

```text
start recorder
mark phase
start diagnostic window
run workload
stop diagnostic window
stop recorder
generate report
```

The long-term target is a report where CPU/RSS timelines are aligned with
profile windows so each expensive window can be attributed to agent components
such as sensor read, normalize, detection, telemetry bus, telemetry batcher,
sender, local control, policy apply, and runtime/GC.
