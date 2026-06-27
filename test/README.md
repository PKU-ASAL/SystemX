# SysArmor Test Framework

`test/` contains SysArmor Next functional E2E tests, benchmark runners, test
inputs, topology definitions, and shared test utilities.

The directory is split by test type:

```text
test/
├── README.md                  this overview
├── TEST_ITEMS.md              runnable test item table
├── Makefile                   stable entrypoints
├── e2e/                       product E2E suites
├── benchmarks/                matrix, perf, and module benchmarks
├── environments/              Docker/Vagrant topology inputs
├── data/                      scenarios, workloads, policies, content
├── shared/                    harness, assertions, reports, diagnostics
└── .results/                  generated outputs, ignored by git
```

## Boundaries

| Area | Owns | Should Not Own |
|---|---|---|
| `e2e/` | Product-facing pass/fail suites by system boundary | generic harness logic |
| `benchmarks/` | Performance/effectiveness runners and module microbenchmarks | product E2E assertions |
| `environments/` | Container/VM topology, images, provision scripts, shared tracing resources | test results |
| `data/scenarios/` | Security/functional contracts with `expected.yaml` and optional `labels.yaml` | synthetic pressure workloads |
| `data/workloads/` | Repeatable benign pressure sources for benchmark windows | security assertions |
| `data/policies/` | Collection/detection/resource/telemetry/response policy samples | generated effective policy |
| `data/content/` | IOC/context/rulepack content referenced by policies | runtime cache |
| `shared/harness/` | start/stop/cleanup/wait/build/query glue | product semantics |
| `shared/` | reusable assertions, recorder, diagnostics, fixtures, reports, VM sync helpers | suite-specific pass/fail policy |

Suite scripts decide what is correct. Harness code only starts processes,
waits, collects artifacts, and cleans up.

## Evaluation Scopes

| Scope | Evaluates | Typical Entrypoints |
|---|---|---|
| `local` | agent spool/WAL, endpoint signals, local ctl, drops, parse errors, agent/sensor cost | `e2e/local-agent/`, `benchmarks/matrix/` |
| `manager` | ingested events/signals, cloud signals, incidents, graph/evidence, manager APIs | `e2e/manager-cloud/` |
| `control` | `AgentControlPlaneService.Connect`, mTLS, command/session contract | `e2e/control-plane/` |
| `storage` | store/Postgres projection and query paths | `e2e/storage/`, Go tests |
| `module` | local implementation unit without topology | `benchmarks/modules/rule-engine/` |
| `diagnostic` | perf/pprof/strace/debug capture | `shared/diagnostics/` |

## Scenarios And Workloads

Scenarios are functional/security contracts. Workloads are repeatable pressure
inputs. Keep them separate so effectiveness and cost can be measured cleanly.

| Name | Kind | Purpose |
|---|---|---|
| `apt-fileless-c2` | malicious scenario | one lineage contains download, execution, reverse shell, and credential read evidence |
| `apt-staged-drop` | malicious scenario | download/write and later execution are split across lineages |
| `benign-ci-noise` | benign scenario | CI/cache/artifact noise must not produce attack conclusions |
| `lifecycle-smoke` | lifecycle scenario | agent registration and basic event visibility |
| `business-normal` | workload | normal build/cache/checksum activity |
| `host-activity-heavy` | workload | heavy process and ordinary file activity |
| `edr-activity-heavy` | workload | heavy EDR-relevant exec/file/local-network activity |

Functional assertions use `expected.yaml`. Benchmark effectiveness uses
`labels.yaml` as ground truth and computes event/signal precision-recall inside
the workload window.

## Matrix Model

`bench-matrix-vm` compares policy profiles against workload and scenario cases.

| Dimension | Label | Examples |
|---|---|---|
| policy | `policy_profile` | `minimal-high-signal`, `edr-balanced`, `incident-deep` |
| workload | `workload` | `business-normal`, `host-activity-heavy`, `edr-activity-heavy` |
| scenario | `scenario` | `apt-fileless-c2`, `apt-staged-drop`, `benign-ci-noise` |

Default mode is `MATRIX_MODE=cross`, which runs policy x scenario with
`business-normal` as background workload. Other useful modes:

```bash
make -C test bench-matrix-vm MATRIX_MODE=workload
make -C test bench-matrix-vm MATRIX_MODE=scenario
make -C test bench-matrix-vm MATRIX_MODE=all
```

## Common Commands

Run from the repository root:

```bash
# Topology lifecycle
make -C test up TOPO=container
make -C test down TOPO=container
make -C test status TOPO=vm

# Scenario E2E
make -C test e2e TOPO=container SCENARIO=apt-fileless-c2
make -C test e2e TOPO=vm SCENARIO=apt-staged-drop
make -C test e2e TOPO=container SCENARIO=benign-ci-noise

# Suite groups
make -C test test-local-agent
make -C test test-agent-runtime
make -C test test-manager-cloud
make -C test test-control-plane
make -C test test-reliability
make -C test test-storage

# Focused gates
make -C test e2e-agent-mtls
make -C test e2e-policy-all
make -C test e2e-response-all
make -C test e2e-graph-all
make -C test e2e-postgres-all

# Rule engine and matcher checks
make -C test test-rule-engine
make -C test test-rule-engine-effectiveness
BENCHTIME=1s COUNT=3 make -C test bench-rule-engine
make -C test test-matcher
BENCHTIME=1s COUNT=3 make -C test bench-matcher

# VM performance and effectiveness
make -C test sync-vm-agent
make -C test bench-collection-vm DIAG_SCENARIO=business-normal
make -C test bench-matrix-vm
make -C test effectiveness-report TOPO=vm RUN_ID=manual

# Recorder and diagnostics
make -C test recorder-vm-start RUN_ID=my-run
make -C test recorder-vm-mark RUN_ID=my-run PHASE=workload_start DETAIL=edr-activity-heavy
make -C test recorder-vm-stop RUN_ID=my-run
make -C test recorder-vm-report RUN_ID=my-run
make -C test diag-tetragon-vm-workload DIAG_SCENARIO=edr-activity-heavy
```

For the full runnable target list, see `TEST_ITEMS.md`.

## Outputs

All generated files go under `test/.results/`.

```text
test/.results/
├── <topology>.<scenario>.events.ndjson
├── <topology>.<scenario>.signals.ndjson
├── <topology>.<scenario>.json
├── recordings/<run-id>/
├── rule-engine/<run-id>/
├── bench-collection-vm/<run-id>/
├── bench-matrix-vm/<run-id>/
└── effectiveness/<run-id>/
```

Important benchmark outputs:

| File | Meaning |
|---|---|
| `recordings/<run-id>/timeline.csv` | CPU/RSS/EPS/drop/health samples |
| `recordings/<run-id>/summary.json` | phase summaries from recorder markers |
| `bench-matrix-vm/<run-id>/matrix.csv` | policy x case performance summary |
| `effectiveness/<run-id>/matrix.csv` | event/signal effectiveness metrics |
| `effectiveness/<run-id>/truth_steps.csv` | label-level hit/miss explanation |

## Conventions

- Add new product-facing tests under `e2e/<suite>/`.
- Add module benchmarks under `benchmarks/modules/<module>/`.
- Add matrix/perf runners under `benchmarks/matrix/` or `benchmarks/perf/`.
- Add topology data under `environments/`.
- Add scenario/workload/policy/content inputs under `data/`.
- Add generic helpers under `shared/`.
- Keep generated artifacts under `.results/`.
