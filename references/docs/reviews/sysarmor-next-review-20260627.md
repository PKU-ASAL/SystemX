# SysArmor Next Code Review - 2026-06-27

This review records the main architecture and implementation findings from a
code review of the current SysArmor Next repository. It focuses on issues that
affect product safety, runtime correctness, durability, and future architecture
boundaries.

## Scope

Reviewed areas:

- manager startup and HTTP/gRPC agent-facing surfaces;
- agent data/control plane contract implementation;
- ingest worker and analytics recompute path;
- store and Postgres projection boundary;
- security posture against the documented architecture.

Verification run:

```bash
go test ./internal/agentplane ./internal/managerapi ./internal/store ./internal/workers/ingest
```

Result: all listed packages passed.

## Summary

The overall direction is solid for a prototype: the repository has clear
concepts for agent runtime, sensor runtime, data/control plane, policy/content,
incident/evidence, and platform stores. The main risk is that development
convenience paths are still mixed into the product default path.

The highest-priority issues are:

1. Manager agent/operator authentication and gRPC TLS/mTLS are disabled by
   default.
2. Several HTTP read APIs expose security-sensitive platform data without
   operator authorization.
3. DataPlane nil-batch handling can panic before returning a structured
   rejected ack.
4. DataBatch idempotency only checks the latest cursor, not a durable accepted
   batch ledger.
5. Analytics and query behavior depend on in-process telemetry working sets
   that are lost on manager restart.
6. The store abstraction currently mixes platform state, telemetry working set,
   and backend projection responsibilities.

## Findings

### 1. Manager defaults run with open auth and optional plaintext gRPC

Severity: high

Evidence:

- `cmd/sysarmor-manager/main.go` defines empty defaults for `--dev-token`,
  `--operator-token`, `--grpc-tls-cert`, `--grpc-tls-key`,
  `--grpc-client-ca`, and `--grpc-require-client-cert`.
- `internal/managerapi/http.go` treats empty `authToken` and `operatorToken`
  as authorized.
- `internal/agentplane/grpc.go` treats an empty agent token as authorized.

Impact:

If the manager is bound to a reachable interface without explicit tokens or
mTLS config, unauthenticated clients can append agent data, connect to control
plane services, and perform operator actions where the handler calls
`requireOperator`.

This conflicts with the architecture contract that production agent-manager
communication should use mTLS or equivalent workload identity, and that
operator actions require tenant-aware authorization and audit.

Recommended action:

- Add an explicit development mode flag such as `--dev-insecure`.
- Fail startup unless one of these is true:
  - gRPC TLS and client identity verification are configured;
  - a non-empty development token is configured and the server is explicitly in
    dev mode.
- Apply the same rule to operator authorization.
- Document the secure and dev startup modes separately.

### 2. HTTP read APIs expose sensitive state without authorization

Severity: high

Evidence:

Unauthenticated GET handlers include:

- `/api/v1/events`
- `/api/v1/signals`
- `/api/v1/incidents`
- `/api/v1/incident-evidence`
- `/api/v1/responses`
- `/api/v1/agent-health`
- `/api/v1/agents`
- `/api/v1/metrics`
- `/api/v1/store-status`

Examples:

- `internal/managerapi/http.go` `events`, `signals`, and `incidents` handlers
  directly write store contents.
- `internal/managerapi/http.go` `responses` allows GET without
  `requireOperator`.

Impact:

An unauthenticated network client can read endpoint telemetry, signals,
incidents, response commands, health, host IDs, tenant IDs, agent versions, and
certificate identity metadata.

Recommended action:

- Introduce read roles such as `viewer`, `incident_viewer`,
  `telemetry_viewer`, `response_viewer`, and `agent_viewer`.
- Require operator authorization on all non-healthz HTTP APIs.
- Keep `/healthz` minimal and avoid returning detailed store information unless
  a diagnostic role is present.

### 3. gRPC DataPlane nil batch can panic before contract rejection

Severity: medium-high

Evidence:

- `internal/agentplane/grpc.go` calls
  `validatePeerDataIdentity(ctx, batch.GetHeader())` before validating
  `batch != nil`.
- `validatePeerDataIdentity` expects a header and calls header accessors.
- The HTTP/processor path has nil/header validation, but the gRPC path reaches
  identity validation first.

Impact:

A malformed gRPC request can crash the handler path instead of receiving a
structured `DataAck` with `STATUS_REJECTED` and `invalid_data_batch` style
reasoning.

Recommended action:

- Validate `batch == nil || batch.GetHeader() == nil` at the top of
  `AppendBatch`.
- Return a structured rejected `DataAck` when possible.
- Add a gRPC contract test for nil batch and missing header.

### 4. DataBatch idempotency only checks the latest ack cursor

Severity: medium

Evidence:

- `internal/managerapi/http.go` `isDuplicateBatch` only compares the incoming
  `batch_id` with `AgentSession.LastAckCursor`.
- `RecordDataBatchAppend` overwrites `LastAckCursor` with the newest accepted
  batch id.

Impact:

If an agent replays an older already-accepted batch that is not the current last
cursor, the manager treats it as new and appends it to Kafka or processes it
again. Event/signal-level de-duplication may reduce downstream duplication, but
raw telemetry append, metrics, and side effects can still be repeated.

Recommended action:

- Add a durable accepted-batch ledger keyed by
  `(tenant_id, agent_id, batch_id)`.
- Treat the ack cursor as sequencing/resume state, not the idempotency index.
- Return `STATUS_DUPLICATE` for any accepted batch id already in the ledger.

### 5. Production analytics depends on volatile in-process telemetry state

Severity: medium

Evidence:

- `internal/workers/ingest/processor.go` recomputes touched scopes from
  `store.ListEvents` and `store.ListSignals`.
- `internal/store/store.go` documents that `ListEvents` and `ListSignals` read
  from the in-process working set only.
- `internal/store/postgres/snapshot.go` deliberately excludes events and
  signals from Postgres projection.

Impact:

The design correctly avoids pushing high-volume telemetry into Postgres, but
there is no production telemetry reader boundary that can restore or query the
event/signal working set from Kafka/OpenSearch after manager restart.

After restart, incident records may survive, but recompute, hunting, signal
lookup for response decisions, and long-window analytics can lose context.

Recommended action:

- Define a `TelemetryReader` or `SecurityDataReader` interface for analytics
  and manager query paths.
- Keep the in-memory store as a development implementation.
- Add OpenSearch-backed and/or stream-replay-backed implementations for
  production recompute and query.
- Make restart behavior explicit in tests.

### 6. Store abstraction mixes too many responsibilities

Severity: medium

Evidence:

- `internal/store.Store` contains agents, events, signals, incidents, health,
  rules, policies, assignments, audits, responses, pullbacks, control commands,
  sessions, operator roles, metrics, and rarity baseline.
- `SaveState` exports a full state snapshot, while Postgres selectively
  projects low-volume platform state.

Impact:

The current store is practical for a prototype, but it blurs ownership between
control-plane state, incident state, hot state, telemetry working sets, and
durable backend projection. This will make concurrency, scaling, restart
semantics, and future storage substitutions harder.

Recommended action:

- Split interfaces along product boundaries:
  - `ControlStore`
  - `PolicyStore`
  - `IncidentStore`
  - `ResponseStore`
  - `AgentSessionStore`
  - `TelemetryWorkingSet`
  - `HotState`
- Keep a composite facade only for tests or the manager wiring layer.
- Move telemetry query APIs away from `Store` once `TelemetryReader` exists.

## Suggested Priority Order

1. Secure manager defaults and require explicit dev-insecure mode.
2. Add authorization to HTTP read APIs.
3. Fix gRPC nil-batch handling and add contract tests.
4. Add durable DataBatch idempotency ledger.
5. Define telemetry reader/replay boundary for production analytics.
6. Gradually split `Store` into smaller interfaces.

## Notes

This review intentionally does not mark the current design as broken. It is a
good prototype shape, but several prototype shortcuts are now close to product
boundaries. The next architecture step should be turning those shortcuts into
explicit development-only paths while hardening the default runtime path.
