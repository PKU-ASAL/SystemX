# Agent Manager Contract

This document is the stable contract for production agent-manager traffic.

## Transports

| Plane | Direction | Service | Purpose |
|---|---|---|---|
| Data | Agent -> Manager | `AgentDataPlaneService.AppendBatch(DataBatch)` | Append durable event/signal batches from the agent spool/WAL. |
| Control | Bidirectional | `AgentControlPlaneService.Connect` | Agent health/capability/acks/results and manager policy/content/response/evidence commands. |
| Local operator | Local only | Unix socket gRPC | `sysarmorctl --agent-sock` debug/operator side channel over local spool/WAL. |

Production data and control traffic use gRPC with mTLS. Local ctl is not a production cloud data plane.

## Local Operator Boundary

`sysarmorctl --agent-sock` talks to the local agent over Unix socket gRPC. It is intentionally a local operator/debug side channel:

- it does not send data to the cloud manager;
- watch/get commands read the same local `AgentSpool` WAL used by the data plane;
- local policy/content apply commands affect only the local agent process;
- cloud-originated debug, response, policy, content, and evidence workflows must use `AgentControlPlaneService.Connect`.

This keeps production agent-manager traffic on one stable data plane and one stable control plane while still allowing local inspection without adding a second event buffer.

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
| `STATUS_ACCEPTED` | `accepted` | Commit local WAL cursor and remove batch. |
| `STATUS_DUPLICATE` | `duplicate` | Treat as already committed and remove batch. |
| `STATUS_RETRYABLE` | `retryable_server_error` | Keep batch and retry after `retry_after_ms` when present. |
| `STATUS_REJECTED` | `invalid_upload` | Terminal payload rejection; drop batch and surface health error. |
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
2. Manager returns `policy_update`, `resume`, and pending commands.
3. Agent periodically sends `health_report` and `capability_report`.
4. Manager sends policy/content/response/evidence commands on the same stream.
5. Agent sends `response_ack` and `evidence_pullback_result`.
6. On disconnect, agent reconnects with bounded backoff.
7. On reconnect, agent starts a new stream sequence at `1` and uses manager resume/data cursors for durable state.
