# Agent Gateway Contract

This document is the stable contract for production agent-gateway traffic.

## Transports

| Plane | Direction | Service | Purpose |
|---|---|---|---|
| Data | Agent -> Gateway | `AgentDataPlaneService.AppendBatch(DataBatch)` | Append lightweight event/signal telemetry batches from the agent sender. |
| Control | Bidirectional | `AgentControlPlaneService.Connect` | Agent health/capability/acks/results and gateway-delivered policy/content/response/evidence commands. |
| Local operator | Local only | Unix socket gRPC | `sysarmorctl --socket` debug/operator side channel over the in-process telemetry bus. |

Production data and control traffic use gRPC with mTLS. Local ctl is not a production cloud data plane.

Sample production-shaped certificate material and config live under `deployments/pki/agent-plane-mtls/`. The helper script is:

```bash
tools/pki/gen-agent-plane-mtls.sh ./pki default agent-prod-001 sysarmor-gateway.example.com
```

## Local Operator Boundary

`sysarmorctl --socket` talks to the local agent over Unix socket gRPC. It is intentionally a local operator/debug side channel:

- it does not send data to the cloud gateway;
- watch commands subscribe to the same in-process telemetry bus used before batching and sending;
- local policy/content apply commands affect only the local agent process;
- cloud-originated debug, response, policy, content, and evidence workflows must use `AgentControlPlaneService.Connect`.

This keeps production agent-gateway traffic on one stable data plane and one stable control plane while still allowing local inspection without a durable endpoint WAL.

## Identity

| Field | Rule |
|---|---|
| Canonical certificate identity | `spiffe://sysarmor.local/tenant/<tenant_id>/agent/<agent_id>` URI SAN |
| Compatibility fallback | certificate CN can encode `tenant_id:<tenant_id>,agent_id:<agent_id>` |
| Data identity check | certificate identity must match `DataBatch.header.tenant_id/agent_id` |
| Control identity check | certificate identity must match `ControlFrame.context.tenant_id/agent_id` |
| Binding | the first accepted certificate principal is bound to the agent registry |
| Principal mismatch | same tenant/agent with a different certificate principal is rejected |

## DataAck

| Status | Reason code | Agent action |
|---|---|---|
| `STATUS_ACCEPTED` | `accepted` | Treat the sent telemetry batch as delivered. |
| `STATUS_DUPLICATE` | `duplicate` | Treat as already delivered. |
| `STATUS_RETRYABLE` | `retryable_server_error` | Keep batch and retry after `retry_after_ms` when present. |
| `STATUS_REJECTED` | `invalid_data_batch` | Terminal payload rejection; drop batch and surface health error. |
| `STATUS_REJECTED` | `server_error` | Terminal server-side rejection when not retryable. |

Authentication and mTLS failures are gRPC status errors because the agent has not entered the data-plane contract.

## AgentControlPlaneService.Connect

| Envelope field | Rule |
|---|---|
| `contract_version` | must be `1` |
| `request_id` | required on every agent-to-manager frame |
| `sequence` | per stream, starts at `1`, strictly increases |
| Duplicate `request_id` | idempotent when sent with a new valid sequence; manager replays the prior response |
| Replayed sequence | rejected with `ControlError.code=AlreadyExists` |
| Sequence gap | rejected with `ControlError.code=FailedPrecondition` |
| Unsupported version or missing request id | rejected with `ControlError.code=InvalidArgument` |

Server-to-agent frames also carry `contract_version=1` and monotonically increasing per-stream `sequence` values.

## Long Connection Shape

The production agent runner uses a long-lived `AgentControlPlaneService.Connect`:

1. Agent opens stream and sends `hello`.
2. Gateway returns `policy_update`, `resume`, and pending commands from the control plane.
3. Agent periodically sends `health_report` and `capability_report`.
4. Gateway sends `policy_update`, `content_update`, `response_command`, and `evidence_pullback` frames on the same stream.
5. Agent sends `ack` for `policy_update` and `content_update`, plus `response_ack` and `evidence_pullback_result` for command/result workflows.
6. On disconnect, agent reconnects with bounded backoff.
7. On reconnect, agent starts a new stream sequence at `1`; ordinary telemetry is best-effort and durable evidence transport is handled separately when enabled.

`response_command` and `evidence_pullback` use `labels` for workload, scenario, tenant-specific routing, and other extensible attribution. The control-plane contract does not carry a top-level `scenario` field.

The `policy_update` returned during `hello` synchronizes the current effective policy for the session. Policy publish and assignment APIs update desired state; they do not imply an immediate control downlink by default. When an operator needs immediate delivery, the manager creates a persisted `ControlCommand` either through `/api/v1/control-commands` or by using `downlink=true` on an agent-specific policy assignment. A command uses `command_id` as the control-frame `request_id`; manager records `pending -> sent -> applied/rejected/failed` status, actor, reason, immutable payload JSON, send time, ack time, ack message, and error text. This makes content and policy downlinks auditable without adding a second control path.

### ControlCommand Lifecycle

| Status | Meaning | Next normal transition |
|---|---|---|
| `pending` | Persisted and eligible for manager delivery on `AgentControlPlaneService.Connect`. | `sent`, `canceled`, `expired` |
| `sent` | Written to the control stream at least once. `attempt_count` and `last_sent_at` are updated on each send. | `applied`, `rejected`, `failed`, `canceled`, `expired` |
| `applied` | Agent accepted and applied the command. Terminal. | none |
| `rejected` | Agent rejected the payload or policy/content build. Terminal unless operator retries. | `pending` via retry |
| `failed` | Agent attempted execution but failed. Terminal unless operator retries. | `pending` via retry |
| `canceled` | Operator canceled an undelivered or in-flight command. Terminal unless operator retries. | `pending` via retry |
| `expired` | Manager expired an undelivered or in-flight command. Terminal unless operator retries. | `pending` via retry |

Manager API actions use the same audit object:

| Action | API | Required role | Effect |
|---|---|---|---|
| create | `POST /api/v1/control-commands` | `control_admin` | Create `pending` `policy_update` or `content_update`. |
| cancel | `POST /api/v1/control-commands {"action":"cancel"}` | `control_admin` | Mark non-terminal command `canceled`. |
| retry | `POST /api/v1/control-commands {"action":"retry"}` | `control_admin` | Move rejected/failed/canceled/expired/sent command back to `pending`; `applied` is not retried. |
| expire | `POST /api/v1/control-commands {"action":"expire"}` | `control_admin` | Mark non-terminal command `expired`. |

## Manager API Role Matrix

Manager HTTP APIs are an operator/admin surface, not the agent data/control transport. Read APIs are query surfaces; write APIs require explicit operator roles.

| Role | Write APIs | Purpose |
|---|---|---|
| `admin` | `POST /api/v1/reset`, `POST /api/v1/operator-role-bindings` | Platform administration and role binding. |
| `policy_admin` | `POST /api/v1/policies`, `POST /api/v1/policy-publish`, `POST /api/v1/policy-assignments` | Desired-state policy lifecycle. |
| `control_admin` | `POST /api/v1/control-commands`, policy assignment with `downlink=true` | Auditable agent downlink operations. |
| `responder` | `POST /api/v1/responses`, `POST /api/v1/response-decisions`, `POST /api/v1/response-approvals` | Endpoint response decision and approval workflow. |
| `incident_admin` | `POST /api/v1/incident-lifecycle`, `POST /api/v1/incident-evidence`, `POST /api/v1/incident-merge`, `POST /api/v1/evidence-pullbacks` | Incident operations, evidence mutation, and pullback requests. |

`X-SysArmor-Operator-Token` authenticates operator writes when configured. `X-SysArmor-Actor` identifies the operator principal. `X-SysArmor-Role` must match either an explicitly presented role or a stored role binding for the actor.

## Detection Hot Update

Policy/content updates that affect endpoint detection use two-phase runtime application:

1. Build a candidate detection engine with the new policy/content snapshot.
2. If build status is `applied` or `degraded`, commit the content/policy and atomically switch the engine.
3. If build status is `rejected`, keep the previous effective engine and content/policy state.

Agent health reports include `detection` runtime status with the active detection policy version, applied content refs, last apply status, and last apply error.

Cloud-originated `policy_update` and `content_update` frames use the same two-phase path as local `sysarmorctl` apply operations. A rejected update returns `ControlAck(status="rejected")` on the control stream, keeps the previous runtime state, updates the command audit record when the frame came from a persisted command, and does not disconnect the session.
