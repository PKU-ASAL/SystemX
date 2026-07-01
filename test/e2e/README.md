# E2E Suites

| Suite | Scope | Purpose |
|---|---|---|
| `local-agent` | endpoint | Single endpoint agent, owned sensor, local control, local signals. |
| `agent-runtime` | endpoint/topology | Daemon lifecycle, managed sensor, restart, health, capability. |
| `manager-cloud` | platform/topology | Manager ingest/query, cloud signals, incidents, policy, response. |
| `control-plane` | platform | Agent-manager gRPC contract and mTLS identity. |
| `reliability` | endpoint/platform | Spool, outage, shutdown, and backpressure behavior. |
| `storage` | platform | Store status, Postgres projection, and query contracts. |

Prefer the scope-level Make targets:

```bash
make -C test test-endpoint
make -C test test-topology
make -C test test-platform
```
