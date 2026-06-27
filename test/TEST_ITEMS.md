# SysArmor Test Items

This table is the root index for runnable test targets. Run targets from the
repository root as:

```bash
make -C test <target>
```

## Core Flow

| Target | Area | Purpose |
|---|---|---|
| `up` | topology | Start `TOPO=container|vm`. |
| `down` | topology | Stop `TOPO=container|vm`. |
| `provision` | topology | VM rsync and provision. |
| `status` | topology | Show selected topology status. |
| `capture` | scenario | Run one scenario capture for selected topology. |
| `assert` | scenario | Assert captured scenario output. |
| `e2e` | scenario | Run `up`, `capture`, and `assert`. |
| `report` | reporting | Aggregate simple result matrix. |
| `clean` | cleanup | Clean topology/results through the shared harness. |

## Suite Entrypoints

| Target | Suite | Purpose |
|---|---|---|
| `test-local-agent` | local-agent | VM real Tetragon endpoint smoke. |
| `bench-local-agent` | local-agent | VM policy x workload/scenario benchmark matrix. |
| `test-agent-runtime` | agent-runtime | Local fake-sensor daemon, capability, parse/drop, and session smoke tests. |
| `test-manager-cloud` | manager-cloud | Manager ingest/query/cloud signal/incident/policy/response tests. |
| `test-control-plane` | control-plane | Control stream contract and mTLS identity. |
| `test-reliability` | reliability | Spool, outage, shutdown, and backpressure reliability. |
| `test-storage` | storage | Store status, query pagination, and Postgres/store contract. |
| `diagnostics-vm` | diagnostics | VM diagnostic capture for agent-owned Tetragon. |

## Agent Runtime And Control

| Target | Purpose |
|---|---|
| `e2e-agent-manager-contract` | DataBatch append, control stream sequence/replay, request id, DataAck classes, and daemon control loop tests. |
| `e2e-manager-idempotency` | Repeated DataBatch does not amplify manager ingest. |
| `e2e-agent-mtls` | gRPC mTLS client certificate identity binding and rejection cases. |
| `e2e-agent-daemon` | Local fake sensor daemon to health/data-plane smoke. |
| `e2e-agent-daemon-container` | Container topology fake daemon smoke. |
| `e2e-agent-sensor-restart` | Managed fake Tetragon restart, degraded health, and tamper signal. |
| `e2e-agent-sensor-recover` | Managed fake Tetragon degrades once then recovers. |
| `e2e-agent-spool` | Manager outage, durable spool, and recovery drain. |
| `e2e-agent-outage-soak` | Manager outage queues multiple batches and drains all. |
| `e2e-agent-shutdown` | Graceful shutdown drains queued batch. |
| `e2e-agent-reliability-soak` | Longer outage/retry/restart/shutdown reliability soak. |
| `e2e-agent-capability` | Startup bundle/checksum capability failure gives degraded health. |
| `e2e-agent-capability-btf` | Required BTF capability failure gives degraded health. |
| `e2e-agent-capability-bpffs` | Required bpffs capability failure gives degraded health. |
| `e2e-agent-parse-health` | Sensor parse errors degrade health and emit tamper signal. |
| `e2e-agent-dropped-health` | Sensor dropped events degrade health and emit tamper signal. |
| `e2e-agent-backpressure` | Spool `max_bytes` backpressure/drop degrades health. |
| `e2e-agent-health` | `sysarmorctl manager` agents/health/metrics smoke. |
| `e2e-agent-session` | Data append updates agent session cursor and resume cursor. |
| `e2e-agent-control-plane-contract` | Long-lived `AgentControlPlaneService.Connect` daemon contract. |
| `e2e-agent-control-plane-all` | Session plus control-plane contract tests. |
| `e2e-agent-all` | Common local/manager agent smoke bundle. |
| `e2e-agent-runtime-all` | Agent smoke bundle plus container detection and real owned-path runtime gates. |

## Managed Sensor And Owned Sensor

| Target | Purpose |
|---|---|
| `e2e-agent-managed-container` | Container managed fake Tetragon bundle smoke. |
| `e2e-agent-managed-recover-container` | Container managed degraded-to-recovered smoke. |
| `e2e-agent-managed-restart-container` | Container managed fake Tetragon restart/tamper smoke. |
| `e2e-agent-systemd-vm` | VM systemd agent daemon health/restart smoke. |
| `e2e-agent-managed-vm` | VM systemd managed fake Tetragon bundle smoke. |
| `e2e-agent-managed-recover-vm` | VM systemd managed degraded-to-recovered smoke. |
| `e2e-agent-real-tetragon-owned-vm` | VM systemd agent-owned real Tetragon to local ctl events/signals. |
| `e2e-agent-real-tetragon-owned-container` | Container agent-owned real Tetragon detection smoke. |

## Scenario Detection

| Target | Purpose |
|---|---|
| `e2e-agent-apt-container` | Container `apt-fileless-c2` through agent-managed Tetra subscription. |
| `e2e-agent-staged-container` | Container `apt-staged-drop` through agent-managed Tetra subscription. |
| `e2e-agent-benign-container` | Container `benign-ci-noise` through agent-managed Tetra subscription. |
| `e2e-agent-detection-container-all` | All three container managed detection smokes. |

## Policy, Response, Incident, And Graph

| Target | Purpose |
|---|---|
| `e2e-policy-endpoint-disable` | Effective policy disables an endpoint rule. |
| `e2e-policy-agent-refresh` | Agent refreshes effective policy without restart. |
| `e2e-policy-cloud-disable` | Manager policy assignment disables a cloud rule. |
| `e2e-policy-publish` | Draft policy must be published before assignment/effective use and is audited. |
| `e2e-operator-role-bindings` | Operator role bindings authorize control-plane writes. |
| `e2e-policy-all` | All policy/control-plane gates. |
| `e2e-response-observe-only` | Observe-only response command and audit. |
| `e2e-response-policy-deny` | Response policy denies destructive actions by default. |
| `e2e-response-scope-deny` | Response command must match agent runtime scope. |
| `e2e-response-audit` | Response intent converts into auditable observe decision. |
| `e2e-response-approval` | Approval-required response waits before control delivery. |
| `e2e-response-multi-approval` | Response policy can require multi-approval threshold and roles. |
| `e2e-response-all` | All response/enforce observe-only gates. |
| `e2e-graph-evidence` | Incident evidence is queryable as graph JSON. |
| `e2e-incident-lifecycle` | Incident lifecycle status can be updated and queried. |
| `e2e-incident-attach-evidence` | Incident evidence can be attached and queried. |
| `e2e-incident-merge` | Incidents can be merged by explicit id. |
| `e2e-rarity-baseline` | Rarity baseline is updated by ingest without duplicate amplification. |
| `e2e-graph-all` | Graph, evidence, incident, and rarity gates. |

## Storage And Postgres

| Target | Purpose |
|---|---|
| `e2e-store-status` | Store backend and migration status are queryable. |
| `e2e-query-pagination` | Query APIs support limit/offset pagination. |
| `e2e-postgres-store` | Postgres snapshot adapter migration, persistence, and close behavior. |
| `e2e-postgres-idempotency` | Postgres duplicate ingest idempotency across reopen. |
| `e2e-postgres-policy-persistence` | Postgres persists policy assignment, audit, and incident state. |
| `e2e-postgres-manager-api` | Postgres backs manager ingest/query/policy/incident APIs. |
| `e2e-postgres-agent-projection` | Postgres projects agents and health into table paths. |
| `e2e-postgres-ingest-projection` | Postgres projects events and signals into table paths. |
| `e2e-postgres-event-query` | Postgres queries events through table path. |
| `e2e-postgres-signal-query` | Postgres queries signals through table path. |
| `e2e-postgres-incident-query` | Postgres queries incidents through table path. |
| `e2e-postgres-control-projection` | Postgres projects rules and evidence pullbacks into table paths. |
| `e2e-postgres-control-audit-projection` | Postgres projects policy audit and operator roles into table paths. |
| `e2e-postgres-response-projection` | Postgres projects response audit into table path. |
| `e2e-postgres-response-write` | Postgres writes response audit through table path. |
| `e2e-postgres-response-query` | Postgres queries response audit through table path. |
| `e2e-postgres-policy-projection` | Postgres projects policies and assignments into table paths. |
| `e2e-postgres-policy-write` | Postgres writes policy control tables directly. |
| `e2e-postgres-policy-query` | Postgres queries policy control tables. |
| `e2e-postgres-policy-get` | Postgres gets a policy through table path. |
| `e2e-postgres-effective-policy-query` | Postgres resolves effective policy through table paths. |
| `e2e-postgres-incident-projection` | Postgres projects incidents and evidence into table paths. |
| `e2e-postgres-observability-projection` | Postgres projects incident events and metrics into table paths. |
| `e2e-postgres-agent-session-projection` | Postgres projects agent sessions into table path. |
| `e2e-postgres-rarity-projection` | Postgres projects rarity baseline into table path. |
| `e2e-postgres-all` | All current Postgres/store foundation gates. |

## Performance, Benchmark, And Diagnostics

| Target | Purpose |
|---|---|
| `perf` | Short getevents performance baseline for selected topology. |
| `perf-resource` | Short EDR resource usage time series. |
| `perf-resource-container` | Container idle EDR resource sampling. |
| `perf-resource-vm` | VM idle EDR resource sampling. |
| `perf-resource-all` | Container and VM idle EDR resource sampling. |
| `test-rule-engine` | Local Go tests for endpoint detection rule engine semantics. |
| `test-rule-engine-effectiveness` | Local scenario-effectiveness tests for endpoint rule output, covering fileless C2, staged drop, and benign CI noise. |
| `bench-rule-engine` | Local Go microbenchmarks for endpoint detection rule engine. |
| `test-matcher` | Local Go tests for matcher algorithms. |
| `bench-matcher` | Local Go microbenchmarks for matcher algorithms. |
| `sync-vm-agent` | Upload current agent/ctl binaries into VM and verify local socket. |
| `recorder-vm-start` | Start VM long-running performance recorder. |
| `recorder-vm-mark` | Add a recorder marker with `PHASE=<name>` and optional `DETAIL`. |
| `recorder-vm-stop` | Stop VM recorder and pull timeline. |
| `recorder-vm-report` | Generate recorder summary from timeline and markers. |
| `diag-tetragon-vm` | Capture VM Tetragon diagnostics. |
| `diag-tetragon-vm-workload` | Capture VM Tetragon diagnostics while running `DIAG_SCENARIO`. |
| `bench-collection-vm` | Run VM real-path resource/EPS benchmark for collection policies. |
| `bench-matrix-vm` | Run fixed VM policy x workload/scenario matrix and effectiveness report. |
| `bench-edr-lifecycle-vm` | Capture VM EDR baseline/start/apply/steady/workload lifecycle curve. |
| `bench-e2e-vm` | Wrap a VM functional E2E with recorder. |
| `effectiveness-report` | Build event/signal precision-recall report from labels and benchmark outputs. |
