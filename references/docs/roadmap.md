# Roadmap

This roadmap records the current implementation direction. It is not a changelog and should stay short.

## Current Focus

The current phase is endpoint refinement:

- make the agent concrete and testable without depending on manager;
- use `sysarmorctl` as a local manager over gRPC Unix Domain Socket;
- own Tetragon lifecycle from the agent distribution;
- compile SysArmor Collection Policy into backend policy;
- produce real Events and Signals from a real VM sensor path;
- benchmark the three default collection policies against slim workload/scenario matrices.

## Completed Or Partially Working

Endpoint:

- agent daemon with local control service;
- agent-owned Tetragon VM install path;
- behavior-first collection policy;
- content package apply for ContextSet / IOCPack / RulePack;
- WAL-backed event and signal local watch;
- labels and WatchFilter;
- event refs and signal-to-event lookup;
- lightweight endpoint detection including builtin and CEP-like rules;
- collection policy explain with ref resolution, selector pushdown/degrade report, and detection coverage;
- spool/WAL health with backlog, cursor, watcher, backpressure, and data batch drain visibility;
- observe-only response ack loop with explicit would-execute audit semantics;
- VM recorder and benchmark matrix;
- resource timeline with CPU/RSS/EPS/drop/signal counts.

Platform foundations:

- policy/rule/content model prototypes;
- response observe-only skeleton;
- incident/evidence/graph package boundaries;
- Postgres schema and partial projections;
- Kafka/Redis/OpenSearch architectural direction;
- containerized platform direction.

## Near-Term Work

1. Tighten endpoint contracts.
   - Complete local control API symmetry beyond the current health, policy, content, event, signal, and explain paths.
   - Make Event/Signal ring buffer and cursor behavior explicit.
   - Keep workload/scenario as test labels, not product contract fields.

2. Improve collection compiler.
   - Expand Tetragon selector coverage.
   - Keep collection behavior objective and sensor-neutral.
   - Keep `edr-balanced` selective enough for stable CPU: avoid broad shell/interpreter `process.exec`, prefer C2/path-limited `network.connect` and file selectors, and reserve fork/exit/broad exec for `incident-deep`.

3. Improve detection content runtime.
   - Stabilize RuleSet/RulePack structure.
   - Finish dependency checks from detection requirements to collection policy.
   - Support expression and short sequence rules without becoming a full cloud graph engine.
   - Expand representative endpoint rules in loosely coupled families: persistence writes, interpreter-driven C2, credential access, lateral movement, and terminal-anchored lifecycle correlation.

4. Response/enforce loop.
   - Keep observe-only by default.
   - Add safe response command payloads beyond would-execute audit.
   - Gate destructive actions behind explicit policy and authorization.

5. Benchmark and diagnostics.
   - Keep recorder as the official performance timeline.
   - Use NDJSON scoped event/signal frames for exact phase counts.
   - Keep the default VM matrix slim: three default policies, three representative scenarios, and `business-normal` as the single background workload.
   - Use diagnostics only to explain cost.
   - Add container recorder and future native sensor matrix.

## Platform Work After Endpoint Refinement

1. Postgres-only control plane.
   - Remove file store from product paths.
   - Make policy, response, incident metadata, agent sessions, and cursors table-first.

2. Agent data/control plane.
   - Use AgentDataPlaneService.AppendBatch for DataBatch append and AgentControlPlaneService.Connect for control flow.
   - Provide durable append, ack/resume, downlink, hot state, and identity.
   - Treat mTLS certificate URI SAN as the production agent principal and bind it to tenant_id/agent_id in the manager registry.
   - Keep local Unix socket sysarmorctl as a local operator/debug boundary, not a second production data plane.
   - Remove old interfaces once the new boundary is ready.

3. Kafka ingest.
   - DataAck after durable append.
   - Workers consume asynchronously.
   - Topics should follow product data families, for example:
     - `sysarmor.endpoint.events`;
     - `sysarmor.endpoint.signals`;
     - `sysarmor.agent.health`;
     - `sysarmor.response.acks`;
     - `sysarmor.evidence.results`;
     - `sysarmor.incident.timeline`.

4. Redis hot state.
   - online status;
   - last seen;
   - pending downlink;
   - stream owner;
   - hot cursor;
   - leases and rate limits.

5. OpenSearch evidence/search.
   - searchable events/signals/findings;
   - evidence documents;
   - incident timeline;
   - entity pivot and hunting API.

6. Containerized platform environment.
   - manager;
   - agent-facing data/control endpoint;
   - worker;
   - Postgres;
   - Kafka;
   - Redis;
   - OpenSearch.

## Native Sensor Direction

Tetragon is useful and powerful, but can have non-trivial baseline cost. Long term, SysArmor should explore a thinner native sensor path:

- only attach hooks required by active Collection Policy;
- use a small event schema aligned to CanonicalEvent;
- keep capability matrix explicit;
- support host/container/cgroup/namespace/pod scopes;
- compare against Tetragon using the same recorder and workload matrix.

Native sensor should not replace the Sensor Runtime contract; it should be another backend.

## Non-Goals For The Current Phase

- Full cloud XDR correlation in the endpoint.
- Production destructive enforcement by default.
- Manager-dependent endpoint validation.
- Treating Docker/K8s deployment topology as special cases in product data models.
- Forcing high-frequency raw telemetry into Postgres.

## Done Criteria For Endpoint Refinement

Endpoint refinement is done when:

- a VM can install one agent distribution that owns its sensor;
- collection policy can be applied on the fly;
- agent emits real local events and signals;
- sysarmorctl can watch, filter, and resolve signal event refs;
- response commands have a safe observe-only path;
- benchmark can compare policies under benign and attack workloads;
- performance data includes exact event/signal phase counts and CPU/RSS timelines.
