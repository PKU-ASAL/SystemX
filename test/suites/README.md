# Test Suites

Suites are the stable test entrypoints. A suite defines the system under test, the evaluation scope, and the report contract. It may call legacy harness scripts while the test tree is being migrated.

| Suite | Purpose | Main entrypoints |
|---|---|---|
| `local-agent` | Local endpoint collection, endpoint detection, and resource benchmark. | `bench-matrix-vm.sh`, `e2e-real-tetragon-owned-vm.sh` |
| `manager-cloud` | Manager ingest/query, cloud signals, incidents, graph/evidence, response/policy APIs. | `e2e-scenarios-container.sh`, `e2e-policy.sh`, `e2e-response.sh`, `e2e-incident.sh`, `e2e-manager-idempotency.sh`, `e2e-agent-health.sh` |
| `control-plane` | Agent-facing control stream, mTLS, session, command, ack/replay contract. | `e2e-contract.sh`, `e2e-mtls.sh` |
| `reliability` | Spool/WAL, outage, restart, shutdown, backpressure behavior. | `e2e-spool.sh`, `e2e-outage-soak.sh`, `e2e-shutdown.sh`, `e2e-backpressure.sh`, `e2e-reliability-soak.sh` |
| `storage` | Store status, query pagination, Postgres projection and query contract. | `e2e-store-status.sh`, `e2e-query-pagination.sh`, `e2e-postgres.sh` |

Legacy scripts under `test/harness/` remain callable, but new high-level Makefile targets should prefer these suite entrypoints.
