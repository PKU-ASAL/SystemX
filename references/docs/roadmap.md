# Roadmap

This roadmap records the current implementation direction. It is not a changelog and should stay short.

## Current Focus

The current phase is endpoint refinement:

- make the agent concrete and testable without depending on manager;
- use `sysarmorctl` as a local manager over gRPC Unix Domain Socket;
- own Tetragon lifecycle from the agent distribution;
- compile SysArmor Collection Policy into backend policy;
- produce real Events and Signals from a real VM sensor path;
- benchmark collection policies against workload and attack scenarios.

## Completed Or Partially Working

Endpoint:

- agent daemon with local control service;
- agent-owned Tetragon VM install path;
- behavior-first collection policy;
- content package apply for ContextSet / IOCPack / RulePack;
- event and signal local watch;
- labels and WatchFilter;
- event refs and signal-to-event lookup;
- lightweight endpoint detection including builtin and CEP-like rules;
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
   - Complete local control API symmetry.
   - Add stream health/capacity visibility.
   - Make Event/Signal ring buffer and cursor behavior explicit.
   - Keep scenario as legacy/demo only; prefer labels.

2. Improve collection compiler.
   - Expand Tetragon selector coverage.
   - Add capability/degrade/explain reports.
   - Keep collection behavior objective and sensor-neutral.
   - Continue reducing default collection noise.

3. Improve detection content runtime.
   - Stabilize RuleSet/RulePack structure.
   - Finish dependency checks from detection requirements to collection policy.
   - Support expression and short sequence rules without becoming a full cloud graph engine.

4. Response/enforce loop.
   - Keep observe-only by default.
   - Add safe response commands and audit.
   - Gate destructive actions behind explicit policy and authorization.

5. Benchmark and diagnostics.
   - Keep recorder as the official performance timeline.
   - Use NDJSON scoped event/signal frames for exact phase counts.
   - Use diagnostics only to explain cost.
   - Add container recorder and future native sensor matrix.

## Platform Work After Endpoint Refinement

1. Postgres-only control plane.
   - Remove file store from product paths.
   - Make policy, response, incident metadata, agent sessions, and cursors table-first.

2. Agent Gateway.
   - Replace old Link1 naming with Agent Gateway.
   - Provide durable upload, ack/resume, downlink, hot state, and identity.
   - Do not keep legacy compatibility interfaces once the new boundary is ready.

3. Kafka ingest.
   - Gateway ack after durable append.
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
   - gateway;
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
