# Repository Layout

This document explains where a change belongs. It is not an inventory of every
directory.

## Ownership

| Area | Owns |
|---|---|
| `cmd/` | Thin executable entrypoints |
| `api/proto/` | Versioned Agent data and control plane wire contracts |
| `internal/agent/` | Agent configuration, local state, enrollment, policy, and daemon lifecycle |
| `internal/endpoint/` | Event normalization, matching, and endpoint detection |
| `internal/sensors/` | Sensor contracts and platform adapters |
| `internal/gateway/` | Agent-facing authenticated gRPC access |
| `internal/workers/` | Durable telemetry consumption and projection |
| `internal/manager/` | Operator API, authorization, and control-plane workflows |
| `internal/store/` | PostgreSQL control-plane persistence |
| `internal/platform/` | Kafka, Redis, and OpenSearch adapters |
| `deployments/` | Installers, service images, Compose, PKI, and runtime configuration |
| `web/manager/` | Manager Console and its authenticated BFF |
| `test/` | Product, effectiveness, and performance validation |

## Boundary Rules

- Executables compose internal packages; business logic does not live in
  `cmd/`.
- Protobuf definitions are the source of truth for Agent wire contracts.
- Endpoint code never reads platform databases directly.
- The browser calls the same-origin BFF, not Manager directly.
- Deployment assets describe how released components run; test-only topology
  setup stays under `test/`.
- Generated binaries and releases live under `dist/` and are not source.

See [Agent Runtime](agent-runtime.md), [Platform Runtime](platform-runtime.md),
and the [test guide](../../test/README.md) for behavioral boundaries.
