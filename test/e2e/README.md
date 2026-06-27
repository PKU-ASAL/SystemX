# Test Suites

Suites are the stable test entrypoints. A suite defines the system under test, the evaluation scope, and the report contract.

| Suite | Purpose | Main entrypoints |
|---|---|---|
| `local-agent` | Local endpoint collection, endpoint detection, real Tetragon owned path, and resource benchmark. | `bench-matrix-vm.sh`, `e2e-real-tetragon-owned-container.sh`, `e2e-real-tetragon-owned-vm.sh` |
| `agent-runtime` | Agent daemon, fake sensor, health, capability, recovery, session, container, and VM runtime smoke tests. | `e2e-daemon.sh`, `e2e-sensor-restart.sh`, `e2e-capability.sh`, `e2e-session.sh`, `e2e-managed-container.sh`, `e2e-managed-vm.sh` |
| `manager-cloud` | Manager ingest/query, cloud signals, incidents, graph/evidence, response/policy APIs. | `e2e-scenarios-container.sh`, `e2e-scenario-apt-container.sh`, `e2e-policy.sh`, `e2e-response.sh`, `e2e-incident.sh`, `e2e-manager-idempotency.sh`, `e2e-agent-health.sh` |
| `control-plane` | Agent-facing control stream, mTLS, session, command, ack/replay contract. | `e2e-contract.sh`, `e2e-mtls.sh` |
| `reliability` | Spool/WAL, outage, restart, shutdown, backpressure behavior. | `e2e-spool.sh`, `e2e-outage-soak.sh`, `e2e-shutdown.sh`, `e2e-backpressure.sh`, `e2e-reliability-soak.sh` |
| `storage` | Store status, query pagination, Postgres projection and query contract. | `e2e-store-status.sh`, `e2e-query-pagination.sh`, `e2e-postgres.sh` |

Scripts under `test/shared/harness/` are shared glue and topology helpers. New product
test entrypoints should live in a suite directory.
