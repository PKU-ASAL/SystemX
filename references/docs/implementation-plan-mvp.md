# SysArmor MVP Implementation Plan

> Handoff plan for agent developers implementing the SysArmor prototype from the current design-first repository.
>
> Primary goal: make the container topology pass first, then extend the same implementation to VM topology.

## 1. Confirmed Decisions

- Implement in Go.
- Use `api/proto` as the single source of truth and generate Go code with `protoc`.
- Build a real MVP pipeline, not a test-only shim:
  - Tetragon events
  - agent normalize/lineage/fastpath
  - Link1 gRPC
  - manager ingest/analytics/store
  - sysarmorctl JSON queries
- The first execution target is `TOPO=container`; VM support follows after container is green.
- It is acceptable to modify `test/` harness, compose files, Dockerfiles, policies, and capture/assert flow to run the new binaries.
- Analytics may use minimum viable algorithms as long as it preserves the contracts:
  - rarity-weighted behavior, not raw additive scoring
  - causal/graph structure, not isolated point alerts
  - `entities` as join keys
  - `ConvergeTrace.method = rarity+causal-topk`
- The collection policy may be extended to capture WRITE/CHMOD-like evidence needed by Phase 1 scenarios.

## 2. Success Criteria

Container topology must pass these contracts first:

- `apt-fileless-c2`
  - endpoint signals include `web_runtime_spawns_shell`, `download_by_lolbin`, `payload_dropped`, `reverse_shell_pattern`
  - `reverse_shell_pattern` is terminal and has evidence
  - cloud signals include `dropped_payload_executed_and_connects` and `web_shell_chain`
  - exactly 1 incident
  - evidence path includes `java-web`, `bash`, `/dev/shm/x.sh`, `10.66.0.99:443`

- `apt-staged-drop`
  - endpoint signals include `payload_dropped` and `suspicious_exec_connect`
  - no endpoint terminal
  - cloud signal `dropped_payload_executed_and_connects` has `cross_lineage=true`
  - exactly 1 incident
  - incident includes at least 2 lineage ids
  - disabling `cloud.cross_lineage` yields 0 incidents

- `benign-ci-noise`
  - 0 incidents in normal `rarity_structural` mode
  - additive threshold control mode can produce at least 1 incident
  - no endpoint terminal

- `lifecycle-smoke`
  - agent registers
  - policy applies
  - at least one EXEC event is visible with `stable_id` and `lineage_id`
  - no panic/OOM during short soak

After container passes, run the same contracts under VM topology with minimal topology-specific glue.

## 3. Target Repository Shape

Create the design-aligned code tree:

```text
cmd/
  sysarmor-agent/
  sysarmor-manager/
  sysarmorctl/

api/
  proto/
    sensor/v1/
    event/v1/
    signal/v1/
    analytics/v1/
    incident/v1/
    policy/v1/
  schema/

internal/
  sensor/
    contract/
    tetragon/
  endpoint/
    context/
    normalize/
    fastpath/
    evidence/
    ringbuffer/
    uploader/
  analytics/
    ingest/
    entity/
    graph/
    rarity/
    rules/
    correlate/
    converge/
    incident/
    evidence/
  control/
    policy/
    registry/
  transport/
    link1/
  store/

configs/
  policies/
  rules/
    endpoint/
    cloud/
```

Keep `cmd/*` thin. Entrypoints should wire dependencies and parse flags only.

## 4. Analytics Structure

Use this analytics boundary from the start:

```text
analytics/ingest
  Accepts contract objects from Link1, validates ordering/tenant/agent metadata,
  and dispatches them into the analytics pipeline.

analytics/entity
  Normalizes and resolves entity keys: process, file, socket, container, user,
  token. This is where path/socket/container identity cleanup belongs.

analytics/graph
  Maintains the in-memory provenance graph and exposes graph queries:
  upsert node/edge, neighbors, shortest path, k-hop slice, lineage/entity lookup.

analytics/rarity
  Provides a replaceable scoring interface. MVP can use simple counters,
  scenario/workload heuristics, and policy-tuned weights. Later CMS/IDF can
  replace the implementation behind the same interface.

analytics/rules
  Evaluates cloud graph rules and emits cloud Signals. It should not create
  Incidents directly.

analytics/correlate
  Deduplicates related signals, folds repeated terminals, and decides whether
  to update an existing incident candidate or create a new one.

analytics/converge
  Converts graph + scored signals into incident candidates using rarity +
  causal structure. MVP implementation is causal top-k plus shortest path.

analytics/incident
  Assembles final Incident objects: summary, severity, lineage ids, terminals,
  MITRE tags, ConvergeTrace.

analytics/evidence
  Cuts EvidenceSubgraph from the graph and attached evidence bundles.
```

Important rule: analytics consumes contract-level facts and policies, and emits contract-level Signals/Incidents/Evidence. Algorithm internals must not leak into agent, transport, CLI, or tests.

Suggested internal interfaces:

```go
type RarityModel interface {
    Observe(ctx ContextKey, feature FeatureKey)
    Score(ctx ContextKey, feature FeatureKey) float64
}

type GraphView interface {
    Node(id string) (Node, bool)
    Neighbors(id string, opts NeighborOptions) []Edge
    ShortestPath(from, to string, opts PathOptions) ([]Edge, bool)
}

type Converger interface {
    Converge(ctx context.Context, graph GraphView, signals []Signal, opts ConvergeOptions) ([]IncidentCandidate, error)
}
```

## 5. Upstream and Downstream Contracts

### 5.1 Agent to Manager

Use Link1 gRPC and protobuf. The manager must not consume raw Tetragon JSON directly.

Minimum upstream payloads:

- `CanonicalEvent`
- `Signal`
- `EvidenceBundle`
- `AgentHealth`
- registration/session metadata

Ordering guarantees for MVP:

- `agent_id`
- monotonically increasing `seq`
- `mono_ns` for event time
- tolerate late or duplicate events by idempotent upsert

### 5.2 Control to Analytics

Use `DetectionPolicy` and related policy envelopes.

Minimum controls:

- endpoint/cloud rule references
- `cloud.cross_lineage` enable/disable
- converge mode:
  - `rarity_structural`
  - `additive_threshold` for negative control tests only
- rarity/converge tunables:
  - top-k
  - max path hops
  - additive threshold

### 5.3 Manager to CLI/Test

`sysarmorctl` is the query boundary for tests.

Required JSON commands:

```text
sysarmorctl --mgr <addr> signals --scenario <name> --layer endpoint --json
sysarmorctl --mgr <addr> signals --scenario <name> --layer cloud --json
sysarmorctl --mgr <addr> signals --scenario <name> --terminal --json
sysarmorctl --mgr <addr> incidents --scenario <name> --json
```

Returned JSON should match the existing `test/harness/assert.py` expectations:

- signals are arrays
- incidents response has `{ "incidents": [...] }`
- signal fields include `name`, `entities`, `terminal`, `layer`
- incident fields include `lineage_ids`, `converge.method`, and evidence data

## 6. Phased Implementation Plan

### Phase 0: Project Foundation and Proto Contracts

Objective: create the buildable Go skeleton and generated contract types.

Tasks:

- Add `go.mod`.
- Add proto files for:
  - sensor events
  - canonical events
  - signals and evidence bundles
  - Link1 upload/control service
  - incidents and evidence subgraphs
  - policies
- Generate Go code with `protoc`.
- Add minimal `Makefile` targets:
  - `make api`
  - `make build`
  - `make test`
- Add thin command skeletons:
  - `sysarmor-agent --help`
  - `sysarmor-manager --help`
  - `sysarmorctl --help`

Exit criteria:

- `go test ./...` passes.
- `go build ./cmd/sysarmor-agent ./cmd/sysarmor-manager ./cmd/sysarmorctl` passes.
- Generated proto code is committed.

### Phase 1: Manager, Store, and CLI Query Plane

Objective: make manager queryable before connecting real sensor data.

Tasks:

- Implement `transport/link1` gRPC server skeleton.
- Implement `store` with SQLite or a simple file-backed store for MVP.
- Implement manager HTTP or gRPC query endpoints consumed by `sysarmorctl`.
- Implement `sysarmorctl` JSON commands required by `assert.py`.
- Add a dev seed mode for local unit tests only, not as e2e path.

Exit criteria:

- Manager starts in the `mgr` container.
- `sysarmorctl --mgr 10.66.0.10 incidents --scenario apt-fileless-c2 --json` returns valid empty JSON.
- `test/harness/assert.py` no longer dry-runs when `sysarmorctl` is present.

### Phase 2: Agent Sensor Ingestion and Normalize

Objective: consume Tetragon events and upload CanonicalEvents.

Tasks:

- Implement `sensor/contract.Sensor`.
- Implement `sensor/tetragon` adapter for `tetra getevents -o json` output.
- Map Tetragon exec/connect/open/write/chmod-ish events to SensorEvent.
- Implement endpoint context:
  - process table
  - lineage table
  - touch cache
- Implement normalize:
  - `stable_id = hash(host_id, pid, start_time_ns)`
  - lineage inheritance on exec
  - raw refs and monotonic sequence
- Implement uploader with Link1 client.

Container test integration:

- Build agent binary into or mount it into the test topology.
- Run agent where it can read Tetragon events.
- Ensure manager receives events.

Exit criteria:

- `lifecycle-smoke` can observe at least one EXEC event with `stable_id` and `lineage_id`.
- Manager stores CanonicalEvents.

### Phase 3: Endpoint Fastpath and Evidence

Objective: emit endpoint Signals with entities and terminal evidence.

Tasks:

- Implement MVP endpoint rules:
  - `web_runtime_spawns_shell`
  - `download_by_lolbin`
  - `payload_dropped`
  - `reverse_shell_pattern`
  - `sensitive_cred_read`
  - `suspicious_exec_connect`
- Implement touchcache joins:
  - download/write path
  - exec file then connect
- Implement minimal dedup.
- Implement evidence bundle for terminal signals:
  - event refs
  - lineage slice
  - raw refs
  - neighbor entities
- Ensure every Signal has entities.

Exit criteria:

- `apt-fileless-c2` endpoint signals satisfy expected names/entities.
- `reverse_shell_pattern` is terminal and has evidence.
- `apt-staged-drop` has endpoint signals but terminal count is 0.
- `benign-ci-noise` has terminal count 0.

### Phase 4: Analytics Graph, Rules, and Minimum Viable Convergence

Objective: produce cloud Signals and Incidents.

Tasks:

- Implement `analytics/entity` normalizers for:
  - `process:<stable_id>`
  - `file:<path>`
  - `socket:<ip:port>`
  - `container:<id>`
- Implement in-memory graph:
  - process exec/fork edges
  - process write/read file edges
  - process connect socket edges
  - signal-to-entity attachment
  - lineage and scenario indexes
- Implement cloud rules:
  - `dropped_payload_executed_and_connects`
  - `web_shell_chain`
  - optional `credential_access_chain`
- Implement rarity MVP:
  - suppress repeated CI-like behavior
  - mark C2/private test attacker paths as high rarity in attack scenarios
  - expose `Score` and `Observe` through interface
- Implement converge MVP:
  - score = base risk * rarity
  - related-family dedup
  - top-k seeds
  - require causal path or shared entity path
  - cut shortest evidence path
  - method `rarity+causal-topk`
- Implement additive threshold control mode for the negative comparison.

Exit criteria:

- `apt-fileless-c2` produces exactly 1 incident.
- `apt-staged-drop` produces exactly 1 incident when cross-lineage is enabled.
- `apt-staged-drop` produces 0 incidents when cross-lineage is disabled.
- `benign-ci-noise` produces 0 incidents in rarity structural mode.
- additive control mode produces at least 1 incident for `benign-ci-noise`.

### Phase 5: Test Harness Integration for Container

Objective: make `test/` run the real product path.

Tasks:

- Update `test/env/container/compose.yaml`:
  - run manager in `mgr`
  - expose manager query/listen ports as needed
  - mount or copy binaries/configs
- Update manager Dockerfile if needed.
- Update `test/harness/start-container.sh`:
  - build or locate binaries
  - start manager
  - start agent
  - wait for health/registration
  - load extended Tetragon policy
- Update `test/harness/capture-container.sh`:
  - mark scenario start/end for manager if needed
  - run attacks
  - wait for ingestion/convergence
  - call assert or leave `make assert` as separate explicit target
- Update `test/harness/assert.py` only as needed to support real CLI output and control assertions.
- Update `test/Makefile` so there is a real e2e target:
  - `make e2e` can run up, capture, assert
  - keep old capture-only behavior if useful under a separate target.

Exit criteria:

- From `test/`, these pass:
  - `make e2e TOPO=container SCENARIO=apt-fileless-c2`
  - `make e2e TOPO=container SCENARIO=apt-staged-drop`
  - `make e2e TOPO=container SCENARIO=benign-ci-noise`
  - lifecycle smoke target if wired separately

### Phase 6: VM Topology

Objective: reuse the same product code under VM topology.

Tasks:

- Add VM provisioning for manager/agent binaries.
- Ensure agent can consume VM-local Tetragon events.
- Extend VM scripts with the same policy and health checks.
- Keep sysarmorctl query contract identical.

Exit criteria:

- Same scenario assertions pass with `TOPO=vm`.

### Phase 7: Hardening and Developer Ergonomics

Objective: make the prototype maintainable.

Tasks:

- Add unit tests for:
  - stable_id and lineage inheritance
  - Tetragon event mapping
  - endpoint rule emissions
  - entity normalization
  - graph path finding
  - converge decisions
- Add integration tests around manager query API.
- Add metrics:
  - events ingested
  - signals emitted
  - incidents created/updated
  - dropped/duplicate events
  - convergence latency
- Document runbooks:
  - local build
  - container e2e
  - VM e2e
  - debugging manager store

Exit criteria:

- `go test ./...` and container e2e are stable.
- A new developer can run the documented flow without reading all design docs.

## 7. Minimum Proto Surface

Keep proto small but future-proof. Minimum required fields:

```text
CanonicalEvent
  id
  seq
  agent_id
  host_id
  scenario
  mono_ns
  kind
  subject_proc
  object
  parent_stable_id
  lineage_id
  raw_ref

Signal
  id
  name
  layer/where
  base_risk
  local_rarity
  global_rarity
  lineage_id
  entities
  event_refs
  signal_refs
  terminal
  evidence
  scenario

Incident
  id
  scenario
  summary
  severity
  lineage_ids
  terminals
  evidence
  converge

ConvergeTrace
  method
  seed_ids
  path_ids
  score
  controls
```

## 8. Policy and Rule Plan

Create MVP rules in YAML under `configs/rules`.

Endpoint rules:

- `web_runtime_spawns_shell`
- `download_by_lolbin`
- `payload_dropped`
- `reverse_shell_pattern`
- `sensitive_cred_read`
- `suspicious_exec_connect`

Cloud rules:

- `dropped_payload_executed_and_connects`
- `web_shell_chain`
- `credential_access_chain` if needed for evidence richness

Policies under `configs/policies` and mirrored or referenced by `test/policies` should control:

- enabled endpoint/cloud rules
- cross-lineage stitching
- converge mode
- rarity options
- response mode, observe-only for MVP

## 9. Implementation Notes for Scenario Detection

These are MVP heuristics, not permanent detection logic.

`apt-fileless-c2`:

- Detect `curl` to `10.66.0.99:8080` as `download_by_lolbin`.
- Detect creation/write/chmod of `/dev/shm/x.sh` as `payload_dropped`.
- Detect shell process connecting to `10.66.0.99:443` as `reverse_shell_pattern`.
- Build incident path through lineage and file/socket entities.

`apt-staged-drop`:

- Stage A: detect helper download/write to `/var/lib/app/plugins/helper`.
- Stage B: detect execution/connect from helper path to `10.66.0.99:443`.
- Do not emit terminal on endpoint.
- Cloud graph must stitch through the shared file entity.

`benign-ci-noise`:

- Allow endpoint may_contain low-risk signals.
- Rarity/converge should suppress incident because repeated CI behavior is routine and lacks terminal/independent rare structure.
- Additive threshold mode exists only to demonstrate the negative control.

## 10. Risks and Guardrails

- Do not let manager consume Tetragon raw events directly; that bypasses the agent contract.
- Do not encode test scenario names as the primary detection mechanism. Scenario metadata can be used for test isolation and queries, but detection should rely on events/entities/rules.
- Do not create incidents inside cloud rules. Cloud rules emit Signals; converge creates Incidents.
- Do not make endpoint compute global rarity or cumulative incident score.
- Do not make additive threshold the default path.
- Keep `sysarmorctl` output stable; tests depend on it as the product query contract.
- Keep algorithms behind interfaces. CMS/PPR/STP are future implementations, not framework assumptions.

## 11. Recommended Build Order

1. Proto and Go skeleton.
2. Manager empty query plane and sysarmorctl.
3. Agent reads Tetragon and uploads CanonicalEvents.
4. Endpoint signals for `apt-fileless-c2`.
5. Analytics incident for `apt-fileless-c2`.
6. Cross-lineage graph support for `apt-staged-drop`.
7. Rarity suppression and additive control for `benign-ci-noise`.
8. Container harness green.
9. VM harness green.
10. Unit tests and docs.

