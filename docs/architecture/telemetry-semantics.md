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

Evidence is the supporting bundle for a signal or incident.

- Source: endpoint signal references, platform graph, or explicit pullback
- Examples: contributing signals, entity graph, shortest path, raw event references
- Storage: incident/evidence projection
- Rule: evidence should explain why a signal or incident exists, but should not be required for lightweight event transport

## Incident

An incident is a converged case assembled from related signals and evidence.

- Source: platform analytics worker
- Examples: fileless C2 chain, staged drop chain
- Storage: incident projection plus searchable index
- Rule: incident IDs must be stable for the same case scope so recomputation updates the case instead of creating duplicates

## Query Ownership

Endpoint tests may read local agent streams to validate endpoint behavior.
Topology tests that include manager/gateway must use manager APIs as the source
of truth for events, signals and incidents.

## Deduplication

The platform indexes derived documents with stable projection keys:

- signal: stable signal projection key
- incident: stable incident projection key
- evidence/timeline: stable incident projection key plus document kind

This keeps Kafka retries and analytics recomputation idempotent from the manager
query perspective.
