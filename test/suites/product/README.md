# E2E Suites

| Suite | Scope | Purpose |
|---|---|---|
| `endpoint` | endpoint | Single endpoint agent, owned sensor, local control, local signals, restart, health, and capability. |
| `platform` | platform | Gateway, manager, storage, policy, response, control contracts, and local agent-gateway-manager roundtrips. |
| `topology` | topology | Product-path scenarios across manager, gateway, endpoint, attacker, and container/VM topology. |

Prefer the scope-level Make targets:

```bash
make -C test test-endpoint
make -C test test-topology
make -C test test-platform
make -C test test-platform-full
```
