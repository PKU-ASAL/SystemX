# Platform Runtime

The platform adds centralized ingestion, analysis, query, policy, and response
to enrolled Agents. It does not replace the Agent's local runtime.

## Data Flow

```text
Enrolled endpoint
  -> sends selected events and detections
  -> platform receives and analyzes them reliably
  -> security data becomes searchable
  -> operators investigate and manage endpoints through the console
```

The implementation separates those responsibilities: Gateway authenticates
Agent identity and receives data; Kafka provides a durable handoff; Worker
correlates and projects security data; Manager serves operator workflows;
PostgreSQL, OpenSearch, and Redis hold control, searchable, and short-lived
connection state respectively.

## Storage Ownership

| Store | Owns |
|---|---|
| PostgreSQL | Agents, enrollment, artifacts, channels, policy, response, audit, and other control-plane state |
| OpenSearch | Events, signals, evidence, and reproducible incident reports |
| Kafka | Durable telemetry awaiting processing |
| Redis | Ephemeral Gateway connection and resume state |

An incident is an analysis report, not a mutable case record. Human case
management is outside the current model.

## Reliability

Agent batches are acknowledged as accepted, duplicate, retryable, or terminally
invalid. The Agent advances its checkpoint only for accepted or duplicate
batches.

Worker correlates each message with a tenant-bound 15-minute OpenSearch history
window. Derived documents use deterministic IDs and one Bulk projection. Kafka
offsets are committed only after required writes succeed. Permanent input
errors are committed only after a dead-letter record is durable.

## Operator Identity

Manager accepts a verified short-lived RS256 JWT and creates a request-scoped
principal from its subject, tenant, and roles. Caller-provided identity headers
do not establish identity.

The local Manager Console uses one bootstrap administrator. Auth.js owns the
encrypted browser session; the server-side BFF signs Manager JWTs. The browser
never receives that JWT or the Manager internal address.

See [Telemetry Semantics](telemetry-semantics.md), [Schema Evolution](schema-evolution.md),
and [Manager UI API Contract](manager-ui-api-contract.md).
