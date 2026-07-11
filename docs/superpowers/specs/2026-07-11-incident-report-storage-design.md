# Incident Report Storage Design

**Status:** Proposed

**Goal:** Make Incident a reproducible analysis report stored only in OpenSearch, remove PostgreSQL Incident persistence, and prevent acknowledged Kafka messages from silently losing search projections.

## Decision

An Incident is an `IncidentReport`: a Worker-generated synthesis of Events and Signals. It is not a case, ticket, or mutable workflow object.

- OpenSearch is the sole persistent query store for Events, Signals, and Incident Reports.
- PostgreSQL stores control-plane state only. It does not store Incident Reports, Incident evidence graphs, or Incident-to-Event projections.
- Kafka transports replayable data batches. A message is committed only after all required processing and OpenSearch writes succeed, or after a permanent poison message is durably written to the dead-letter topic.
- Manager reads Incident Reports through an OpenSearch-backed query boundary and exposes no Incident mutation operations.

## Scope

### Included

- Propagate OpenSearch serialization and indexing failures from the ingest processor.
- Do not commit a Kafka message when processing fails transiently.
- Classify malformed or permanently invalid Kafka messages as poison messages and publish them to a dead-letter topic before committing the source message.
- Use deterministic OpenSearch document IDs so retries and replays are idempotent.
- Read Incident lists and details from OpenSearch in Manager.
- Remove PostgreSQL Incident persistence and Incident-specific relational projections.
- Remove or disable Manager endpoints for status changes, evidence attachment, and Incident merging.
- Remove the Worker dependency on PostgreSQL Incident reads and writes.
- Add focused tests for retry, commit, dead-letter, idempotency, and OpenSearch-backed Incident queries.

### Excluded

- Human closure, false-positive marking, suppression, comments, ownership, assignment, or case merging.
- A future `IncidentCase` table, repository, API, or empty extension framework.
- Object storage or immutable compliance archives.
- Redesigning unrelated Agent, policy, enrollment, response, or control-command persistence.
- General Store refactoring beyond changes required to remove Incident persistence.

## Data Ownership

| Data | System of record | Property |
|---|---|---|
| Event | OpenSearch | High-volume, replayable telemetry |
| Signal | OpenSearch | Derived but reproducible security telemetry |
| Incident Report | OpenSearch | Derived, searchable, and reproducible report |
| Agent identity and certificate | PostgreSQL | Transactional control-plane state |
| Policy and assignment | PostgreSQL | Transactional control-plane state |
| Control command, response, and audit | PostgreSQL | Transactional and auditable state |
| Transport message and offset | Kafka | Replay and delivery state |

OpenSearch loss is recoverable by replaying retained Kafka data with the corresponding analysis version. PostgreSQL is not a fallback Incident source.

## Incident Report Contract

The existing protobuf `Incident` remains the wire representation during this change to avoid an unnecessary breaking protocol migration. In architecture and new code it is treated as `IncidentReport`.

Each indexed report must include or expose:

- `report_id`: deterministic report identity.
- `tenant_id`: mandatory tenant boundary.
- `correlation_key`: stable identity of the correlated activity.
- `analysis_version`: rules and analysis semantics used to produce the report.
- `first_observed_at` and `last_observed_at`: report time range.
- contributing Signal IDs and relevant Event IDs.
- summary, severity, MITRE techniques, lineage IDs, evidence graph, and convergence trace.

The deterministic document ID is derived from normalized `tenant_id`, `correlation_key`, and `analysis_version`. Reprocessing the same logical report updates the same OpenSearch document. The implementation must not use a process-local counter as persistent identity.

The existing `status`, `status_reason`, and `status_actor` protobuf fields are no longer authoritative and must not be written by new analysis code. They remain temporarily for wire compatibility and should be marked deprecated in a separate compatible protocol change.

## Processing Flow

```text
Gateway -> Kafka data topic -> Worker
                              | validate and decode
                              | persist Event documents
                              | persist Signal documents
                              | calculate Incident Reports
                              | persist Incident Report documents
                              v
                         commit source offset
```

The Worker processes each source message with at-least-once delivery semantics. All OpenSearch writes use deterministic IDs, making replay idempotent.

### Transient Failure

Network errors, timeouts, OpenSearch `429`, and OpenSearch `5xx` responses are transient.

- Return an explicit processing error.
- Do not commit the Kafka source message.
- Retry with bounded exponential backoff and jitter.
- After the configured in-process attempts are exhausted, return from the Worker so the service supervisor can restart it; the uncommitted source message remains replayable.

The first implementation uses small constants rather than user-facing configuration: three attempts with backoff capped at two seconds. Configuration is added only when operational evidence requires it.

### Permanent Failure And DLQ

Malformed protobuf JSON, a missing required batch identity, or an unsupported contract version is permanent for that payload.

- Publish a dead-letter envelope to `<source-topic>.dlq`.
- The envelope contains source topic, partition, offset, key, failure class, failure message, observed timestamp, and original payload.
- Require the DLQ publish to succeed before committing the source message.
- If DLQ publishing fails, do not commit the source message.

OpenSearch indexing failures are not poison-message failures and must not be sent directly to DLQ.

## Manager Query Boundary

Manager depends on a focused `IncidentReportSearcher` rather than the general Store. The boundary supports only current read requirements:

```go
type IncidentReportSearcher interface {
    SearchIncidentReports(context.Context, IncidentReportSearch) ([]json.RawMessage, error)
    GetIncidentReport(context.Context, string, string) (json.RawMessage, bool, error)
}
```

Every operation requires `tenant_id`. The OpenSearch implementation applies the tenant filter server-side. Manager must not query all tenants and filter results in memory.

Incident mutation endpoints are removed. Existing clients receive `404` after route removal; no compatibility shim writes mutable state into OpenSearch.

## PostgreSQL Changes

The active schema no longer creates or projects:

- `incidents`
- `incident_events`
- Incident-owned `evidence`

Because deployed databases may already contain these tables, the migration must not destructively drop them automatically. A new migration stops using them and documents a later operator-controlled cleanup migration. Fresh databases do not need these tables after the schema transition.

The Store backend no longer exposes `ListIncidents`, and `SaveState` no longer projects Incident data. Any in-memory Incident collection retained for isolated analytics tests is not a platform persistence boundary and must not be used by Manager.

## Future Case Management

If human triage becomes a product requirement, add a separate PostgreSQL `IncidentCase` aggregate referencing one or more immutable `report_id` values:

```text
IncidentReport (OpenSearch) <- report_id -> IncidentCase (PostgreSQL)
```

`IncidentCase` may then own status, disposition, assignee, comments, merge relationships, optimistic version, and activity audit. Incident Report remains reproducible and does not acquire mutable workflow fields.

No Case interfaces, tables, or placeholder implementations are created now.

## Observability

Expose counters for:

- OpenSearch indexing attempts and failures by document type.
- source messages processed, retried, committed, and rejected.
- DLQ publish success and failure.
- Incident Reports created and updated.

Logs include tenant ID, batch ID, Kafka topic/partition/offset, document type, and stable error class. Original payloads and credentials must not be logged.

## Testing And Acceptance

The change is accepted when all of the following are verified:

1. An OpenSearch indexing failure causes processing to fail and the source Kafka message remains uncommitted.
2. A transient OpenSearch failure followed by recovery produces one logical document and commits once.
3. A malformed message is committed only after its DLQ envelope is successfully published.
4. A DLQ publishing failure leaves the source message uncommitted.
5. Replaying a batch produces the same Event, Signal, and Incident Report document IDs.
6. Worker restart does not require PostgreSQL Incident state to continue processing.
7. Manager Incident list and detail queries read OpenSearch and enforce `tenant_id` server-side.
8. Manager no longer exposes Incident mutation endpoints.
9. Fresh PostgreSQL setup has no active Incident persistence dependency.
10. Existing Agent, policy, response, enrollment, and control-plane tests remain green.

## Migration And Rollout

1. Deploy Worker failure propagation, idempotent IDs, retry, and DLQ support before removing the PostgreSQL projection.
2. Backfill or rebuild the OpenSearch Incident index from retained source data where required.
3. Switch Manager Incident reads to OpenSearch and remove mutation routes.
4. Stop PostgreSQL Incident projection writes and remove the persistence interface.
5. Keep legacy tables untouched for one compatibility window; document manual cleanup after validation.

Rollback before legacy-table cleanup consists of restoring the previous Manager and Worker binaries. No automated destructive database rollback is required.
