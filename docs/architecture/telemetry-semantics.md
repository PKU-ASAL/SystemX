# Telemetry Semantics

SysArmor uses four security data concepts.

## Event

An event is normalized telemetry from the endpoint.

- Source: agent/sensor
- Examples: process exec, file write, network connect
- Transport: agent streams batches to gateway
- Storage: searchable telemetry index
- Rule: events are high-volume data and may be queried by labels, behavior and time

## Signal

A signal is a detection assertion derived from events or health state.

- Source: endpoint detection engine or platform analytics worker
- Endpoint signal: emitted by the agent close to the sensor stream
- Cloud signal: emitted by platform analytics after correlating uploaded telemetry
- Storage: searchable telemetry index
- Rule: signal IDs must be stable for the same semantic assertion so retries and recomputation do not amplify results

## Evidence

Evidence is the supporting bundle for a signal or incident report.

- Source: endpoint signal references, platform graph, or explicit pullback
- Examples: contributing signals, entity graph, shortest path, raw event references
- Storage: searchable OpenSearch projection
- Rule: evidence should explain why a signal or incident exists, but should not be required for lightweight event transport

## Incident

An incident is a reproducible report assembled from related signals and evidence. It is not a mutable case or ticket.

- Source: platform analytics worker
- Examples: fileless C2 chain, staged drop chain
- Storage: OpenSearch only; PostgreSQL does not persist incident reports
- Rule: report IDs and tenant labels must be stable so retries update the same report instead of creating duplicates

Human triage state is intentionally outside the report model. If case management is added later, a PostgreSQL `IncidentCase` will reference report IDs without changing report ownership.

## Query Ownership

Endpoint tests may read local agent streams to validate endpoint behavior.
Topology tests that include manager/gateway must use manager APIs as the query
boundary. Manager reads incidents from OpenSearch and requires `tenant_id`.

## Deduplication

The platform indexes derived documents with stable projection keys:

- signal: stable signal projection key
- incident: stable incident projection key
- evidence: stable incident report key plus document kind

This keeps Kafka retries and analytics recomputation idempotent from the manager
query perspective.

The Worker commits a Kafka source message only after required OpenSearch writes succeed. Transient failures are retried without committing. Permanently malformed payloads are committed only after a dead-letter envelope is written to `<source-topic>.dlq`.

Worker correlation uses an OpenSearch history window bounded to 15 minutes by
`tenant_id`, analysis scope, and `@timestamp`. Process memory is not a source of
historical truth. Event, Signal, Evidence, and Incident Report documents are
submitted through one Bulk request with deterministic IDs; partial Bulk success
converges through replay of the same IDs.
