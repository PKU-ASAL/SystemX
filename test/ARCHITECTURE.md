# SysArmor Test Architecture

This document defines how tests under `test/` are organized. The goal is to keep each test suite focused on one system boundary and one evaluation question.

## Concepts

| Concept | Meaning | Owns |
|---|---|---|
| Suite | A product-facing test objective. It answers what is being tested and what counts as success. | SUT boundary, evaluation scope, pass/fail rules, reports |
| Harness | Shared execution glue. It answers how to start, wait, query, collect, and clean up. | Topology lifecycle, process control, log/result collection |
| Tool | Reusable helper used by one or more suites. | Recorder, report generators, diagnostics, fixtures |
| Scenario | Security semantics contract. | Attack/benign script plus `expected.yaml` |
| Workload | Pressure source. | Repeatable exec/file/network/business activity, no security assertions |
| Policy | Test variable. | Collection/detection/response/resource profiles |

Suites may call harness scripts and tools. Harness scripts should not decide product semantics such as whether a cloud incident is required or whether a local benchmark score is good.

## Suite Boundaries

| Suite | SUT | Evaluation scope | Primary transport | Should not test |
|---|---|---|---|---|
| `local-agent` | `sysarmor-agent` + sensor + local spool/WAL + local ctl | `local` | `sysarmorctl --socket` | manager cloud signals, incidents, storage projection |
| `manager-cloud` | manager ingest + analytics + store + manager ctl query | `manager` | `AgentDataPlaneService.AppendBatch` + `sysarmorctl manager ...` | local agent CPU/RSS benchmark |
| `control-plane` | `AgentControlPlaneService.Connect`, mTLS, command/session contract | `control` | bidirectional gRPC | event collection effectiveness |
| `reliability` | agent spool/WAL, outage, restart, backpressure | `local` or `full`, per case | agent + fake/real manager | detection rule quality |
| `storage` | Postgres/store projection and query paths | `storage` | Go tests / manager API tests | runtime agent behavior |
| `diagnostics` | perf/pprof/strace/debug capture | `diagnostic` | host/VM tools | product pass/fail contract |

## Evaluation Scopes

`local` evaluates endpoint-visible facts:

- local events from the agent spool/WAL;
- endpoint signals;
- local negative assertions, drops, parse errors;
- agent/sensor CPU, RSS, EPS, and cost-per-event metrics.

`manager` evaluates manager-visible facts:

- ingested events/signals from the manager store;
- cloud signals;
- incidents, graph/evidence, rarity, and manager query APIs.

`full` evaluates both local and manager facts and should only be used by suites that intentionally exercise both planes.

Every benchmark-style output should state its `evaluation_scope`. Requirements outside the selected scope should be reported as `out_of_scope`, not silently counted as pass or fail.

For example, `local-agent` benchmark reports use `evaluation_scope=local`: event, endpoint signal, and local negative assertions are scored; cloud signal and incident assertions from the same `expected.yaml` are preserved in the structured report as `out_of_scope`.

## Result Layout

New suite outputs should use this shape:

```text
test/.results/<suite>/<run-id>/
  manifest.json
  summary.json
  matrix.csv
  matrix.json
  artifacts/
```

`manifest.json` should include:

- suite name and evaluation scope;
- topology;
- git commit when available;
- policies, workloads, and scenarios;
- tenant/agent identity;
- start/end time;
- relevant config or policy hashes.

Existing legacy result paths under `test/.results/` remain supported while suites are migrated.

## Migration Rules

1. New user-facing test entrypoints go under `test/suites/<suite>/`.
2. Shared process-control logic goes under `test/harness/`.
3. Reusable reporting, recorder, benchmark, and diagnostic utilities go under `test/tools/`.
4. Existing `test/harness/e2e-*.sh` scripts are treated as legacy suite implementations until moved.
5. Do not add manager queries to `local-agent` benchmarks unless the suite explicitly changes to `full` scope.
6. Do not add CPU/RSS benchmarking to manager-cloud functional tests unless the suite explicitly changes to `full` scope.

## Harness Library

Shared shell helpers live under `test/harness/lib/`. They should stay product-agnostic:

- path/bootstrap helpers such as `sa_init_repo_paths`;
- temporary directory and random port helpers;
- process cleanup helpers;
- wait helpers such as `sa_wait_contains`, `sa_wait_url_contains`, `sa_wait_glob`, and `sa_wait_no_glob`;
- thin query helpers such as `sa_manager_ctl`.

Suite scripts own business semantics and assertions. The harness library should not know whether a signal, incident, or policy result is correct.
