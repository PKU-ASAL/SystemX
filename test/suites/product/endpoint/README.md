# Endpoint Product Tests

These tests validate the Agent, its managed Tetragon sensor, local persistence,
and Unix-socket control boundary on one protected endpoint.

```bash
make -C test product-endpoint-standalone
make -C test product-endpoint
make -C test product-endpoint-namespace-container
```

| Target | Boundary |
|---|---|
| `product-endpoint-standalone` | Local identity, policy, events, signals, restart, and bounded storage |
| `product-endpoint` | Fresh VM with Agent-owned real Tetragon |
| `product-endpoint-namespace-container` | Manager-distributed container Agent with `namespace/self` sensor scope |

The namespace test installs the signed `linux-container` artifact returned by
Manager, starts:

```bash
/opt/sysarmor/agent/bin/sysarmor-agent run --config /etc/sysarmor/agent/agent.yaml
```

It proves in-container events are collected while equivalent host events are
excluded for that Agent identity. These tests do not establish detection
recall, platform resource cost, or endpoint performance baselines.
