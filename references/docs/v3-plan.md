# SysArmor v3 Implementation Plan

Date: 2026-06-16
Status: architecture updated for platform-first implementation

## 1. Goal

v3 的目标是把 v2 已经完成的 **EDR endpoint runtime MVP** 推进成可运营、可扩展、可测试的 **EDR platform foundation**。

v2 已经证明:

```text
agent daemon + sensor runtime + scope contract + policy apply + health + spool + retry
```

v3 要补齐的是:

```text
Agent Gateway + Kafka ingest pipeline
  + Postgres control/state/audit store
  + Redis hot state
  + OpenSearch evidence/search layer
  + policy/rule/response/incident platform contracts
```

换句话说,v3 不再沿用 file snapshot 作为平台主路径。file store 是早期 MVP 快速迭代实现,后续应从正式 backend 中移除。v3 的平台主路径是:agent 只和 Agent Gateway 通信,高频遥测先进 Kafka,控制面和权威状态进 Postgres,热状态进 Redis,可检索安全数据和证据进 OpenSearch。

## 2. Product Positioning

v3 的产品定位是 **EDR platform foundation**。

它仍然 endpoint-first,但系统边界要从单进程 MVP 调整为平台分层:

- endpoint runtime 继续由 agent / sensor runtime 负责。
- Agent Gateway 是 agent 接入、上传、ack、resume、downlink 的统一入口。
- Postgres 是控制面、状态面、审计和 incident metadata 的 source of truth。
- Kafka 是高频 telemetry 的 durable log 和异步处理主干。
- Redis 是在线状态、热 cursor、pending downlink、rate limit、lease 等短 TTL 热状态。
- OpenSearch 是 events/signals/findings/evidence/timeline/hunting 的检索层。

v3 完成后,系统应该从:

```text
能采集、能检出、能长期运行
```

推进到:

```text
能可靠接入 agent、能运营策略和规则、能审计响应意图、
能沉淀 incident/evidence、能用 Kafka/Postgres/Redis/OpenSearch 支撑平台化
```

## 3. Non-goals

v3 不做以下内容:

- 完整 XDR 多源 ingestion。
- 完整 NODLINK/STP/rarity 生产算法。
- 生产级 kill/block/quarantine 默认启用。
- 完整 UI 产品。
- 完整 SIEM/SOAR/数据湖出口。
- 完整多租户商业化权限系统。
- 大规模长期 raw telemetry 湖仓。

v3 要把边界设计正确,但不需要一次性实现所有大规模数据平台能力。

## 4. Guiding Principles

1. **Postgres-only for platform state**
   - file store 从产品 backend 中移除。
   - memory/mock store 只用于单测或轻量 fixture。
   - 在线系统不再依赖 `sysarmor_state` snapshot 恢复业务状态。

2. **Kafka-first for telemetry ingest**
   - Agent Gateway 收到 upload batch 后先完成 durable Kafka append。
   - agent ack 以 durable append 成功为边界,不等待 detection / OpenSearch / incident 构建完成。
   - 所有高频 telemetry 必须有 idempotency key 和 replay 语义。

3. **Postgres stores authority, Redis stores heat**
   - durable cursor checkpoint、session open/close、policy、response、incident metadata 进入 Postgres。
   - live heartbeat、last_seen、stream owner、pending downlink、hot cursor 进入 Redis。
   - Redis 丢失不能导致权威状态丢失。

4. **OpenSearch stores searchable security data**
   - events/signals/findings/evidence/timeline/entity docs 进入 OpenSearch。
   - Postgres 保存 metadata、lifecycle、引用、权限和审计。

5. **先控制面,后响应面**
   - response 必须被 policy 授权。
   - 没有 policy assignment / version / audit 时,不要直接做真实阻断。

6. **先规则内容化,后复杂 DSL**
   - v3 不需要一步到位做完整 CEP/DSL。
   - 先把硬编码规则迁成可版本化、可启停、可下发的 content pack。

7. **容器化全套平台是测试前提**
   - Postgres、Kafka、Redis、OpenSearch、manager、Agent Gateway、workers 都要能用 container compose 启动。
   - VM/container agent e2e 都应能指向同一套容器化平台。

## 5. Architecture Target

目标架构:

```text
sysarmor-agent
  -> sensor runtime
  -> endpoint rule engine
  -> local spool
  -> Agent Gateway stream client
  <- policy downlink
  <- response command
  <- evidence pullback

agent-gateway
  -> authenticate agent
  -> validate upload batch
  -> append raw telemetry to Kafka
  -> update Redis hot session/last_seen/cursor
  -> coalesce durable cursor/session checkpoint to Postgres
  -> serve downlink from Postgres + Redis cache
  -> ack agent after durable ingest

Kafka
  sysarmor.agent.upload.raw
  sysarmor.endpoint.events
  sysarmor.endpoint.signals
  sysarmor.agent.health
  sysarmor.response.acks
  sysarmor.evidence.results
  sysarmor.detection.findings
  sysarmor.incident.timeline

workers
  -> normalize
  -> endpoint detection
  -> cloud correlation
  -> rarity baseline
  -> incident builder
  -> evidence builder
  -> OpenSearch indexer

Postgres
  -> control plane
  -> state plane
  -> audit
  -> incident metadata
  -> evidence metadata
  -> durable Agent Gateway checkpoints

Redis
  -> online status
  -> latest health cache
  -> stream owner
  -> pending downlink cache
  -> hot cursor cache
  -> rate limit / lease

OpenSearch
  -> endpoint events
  -> endpoint/cloud signals
  -> detection findings
  -> evidence documents
  -> incident timeline
  -> entity documents
  -> hunting records

manager-api
  -> policy/rule/response/incident APIs
  -> RBAC and audit
  -> query facade over Postgres / Redis / OpenSearch
```

Naming note: v3 platform docs and product surfaces use **Agent Gateway** / **Agent Channel** as the architecture term. The gateway boundary should stay focused on agent access, durable append, hot state, downlink, and cursor/checkpoint handling.

## 6. Data Placement Contract

### Postgres

Postgres is the source of truth for control, state, audit, and metadata:

```text
tenants
agents
agent_inventory
agent_health_latest
agent_gateway_sessions
agent_gateway_cursors
policies
policy_versions
policy_assignments
policy_audit
rules_metadata
operator_role_bindings
response_commands
response_approvals
response_acks
response_audit
incidents
incident_status
incident_assignees
evidence_metadata
evidence_references
rarity_baseline_metadata
jobs
migrations
```

Postgres may keep small summary counters and latest state, but it must not become the unbounded raw telemetry warehouse.

### Kafka

Kafka is the high-throughput durable event log:

```text
sysarmor.agent.upload.raw
sysarmor.endpoint.events
sysarmor.endpoint.signals
sysarmor.agent.health
sysarmor.response.acks
sysarmor.evidence.results
sysarmor.detection.findings
sysarmor.incident.timeline
```

Kafka message keys should preserve useful order and idempotency:

```text
tenant_id + agent_id
tenant_id + host_id
tenant_id + incident_id
tenant_id + agent_id + batch_id
```

### Redis

Redis is hot state only:

```text
agent_online_status
agent_last_seen
stream_connection_owner
latest_health_cache
policy_assignment_cache
pending_downlink_cache
response_pending_cache
rate_limit
distributed_lock / lease
hot_cursor_cache
```

Redis data must be recoverable from Postgres/Kafka or safe to rebuild.

### OpenSearch

OpenSearch is the searchable security data layer:

```text
sysarmor-events-YYYY.MM.DD
sysarmor-signals-YYYY.MM.DD
sysarmor-findings-YYYY.MM.DD
sysarmor-evidence-YYYY.MM
sysarmor-incident-timeline
sysarmor-entities
sysarmor-hunting
```

OpenSearch stores documents optimized for filtering, timeline, entity pivoting, analyst hunting, and evidence inspection.

## 7. Phase 1: Policy + Rule Content Minimum Loop

Status: partial implementation started.

### Goal

把当前硬编码检测能力推进到可运营的最小控制面。

### Current Implementation Slice

已落地:

- `internal/policy` 定义 rule content、policy、assignment 的最小模型。
- `configs/rules/{endpoint,cloud}/` 和 `configs/policies/default-edr-policy.json` 提供默认 content pack。
- manager HTTP API 支持 rules、policies、policy publish、policy audit、operator role bindings、policy assignments、effective policy。
- `sysarmorctl` 支持查询 rules、policies、policy assignments、effective policy、policy audit,并可发布/取消发布 policy version。
- agent 启动和周期刷新时可拉取 effective policy,并用 endpoint rule references 初始化/切换 endpoint rule engine。
- agent health 会报告实际生效 policy id/version/mode。
- analytics 会按 effective policy 的 cloud rule references 启停 cloud convergence rule。
- 静态 operator token + actor role binding 已提供最小控制面门禁。
- `make -C test e2e-policy-all` 当前聚合 policy gate。

仍需调整:

- policy/rule/assignment/audit 的正式 source of truth 必须迁到 Postgres-only store。
- policy update notification 应从 Postgres 变更事件进入 Redis pending downlink cache,并由 Agent Gateway 下发。
- production auth/RBAC 仍需补齐真实身份、租户级权限、session/JWT、审计签名或集中权限管理。

### Deliverables

- Rule content model and content pack.
- Policy model and versioning.
- Policy publish / assignment / effective policy APIs.
- Postgres policy control tables.
- Redis policy/downlink cache.
- Agent Gateway policy downlink.
- Agent policy fetch/apply and health reporting.

### Acceptance Criteria

- A policy can disable an endpoint rule and e2e proves the corresponding signal disappears.
- A policy can disable a cloud rule and e2e proves the corresponding incident does not converge.
- Agent health reports active policy id/version.
- Manager can query policy assignment by agent/scope.
- Policy writes are durable in Postgres without file snapshot.
- Policy update can reach agent through Agent Gateway downlink.

### Suggested Tests

```bash
go test ./...
make -C test e2e-policy-all
make -C test e2e-postgres-policy-write
make -C test e2e-postgres-policy-query
make -C test e2e-agent-gateway-policy-downlink
```

## 8. Phase 2: Response / Enforce Observe-only Skeleton

Status: partial implementation started.

### Goal

建立可审计的 response/enforce 控制链路,但默认不做生产级真实阻断。

### Current Implementation Slice

已落地:

- `internal/response` 定义 observe-only response command / ack / audit record。
- manager HTTP API 支持 responses、response acks、response decisions、response approvals。
- agent 可接收 pending response command,调用 sensor `Enforce`,并强制以 observe-only / executed=false 上报 ack。
- response policy 支持 allowed actions、allowed modes、approval requirement、destructive action 显式开关、多级审批 threshold/roles。
- manager 会用 agent runtime scope 校验 response command scope。
- Signal proto 已有原生 `response_intent` 字段。
- `make -C test e2e-response-all` 当前聚合 response gate。

仍需调整:

- response command/approval/ack/audit 的正式 source of truth 必须迁到 Postgres-only store。
- pending response downlink 应写入 Redis hot cache,权威状态保留在 Postgres。
- response ack 应作为 Kafka event 进入异步审计/索引 pipeline,并更新 Postgres response audit。

### Deliverables

- Response intent in Signal.
- Response policy.
- Response command / approval / ack model.
- Postgres response audit tables.
- Redis pending response cache.
- Agent Gateway response downlink.
- Kafka `sysarmor.response.acks` event.
- OpenSearch response/evidence timeline document.

### Acceptance Criteria

- Endpoint terminal signal can produce a response intent.
- Manager can convert response intent into an observe-only response decision.
- Agent can receive a response command and return `would_enforce` / `unsupported` / `executed=false`.
- Response decision and result are persisted and queryable.
- No test enables destructive enforcement by default.

### Suggested Tests

```bash
go test ./...
make -C test e2e-response-all
make -C test e2e-postgres-response-write
make -C test e2e-postgres-response-query
make -C test e2e-agent-gateway-response-command
```

## 9. Phase 3: Incident / Evidence / Graph Foundation

Status: partial implementation started.

### Goal

把 MVP analytics 拆成真正的 graph/evidence/incident 包结构,并迁到 worker + OpenSearch/Postgres 分层。

### Current Implementation Slice

已落地:

- `internal/analytics/graph` 提供最小 graph builder,支持 `KHop` 和 `ShortestPath`。
- `internal/analytics/evidence` 已通过 graph API 生成 `EvidenceSubgraph`。
- manager 可查询 incident evidence subgraph,可 attach evidence,可 merge incidents。
- Incident proto 已有最小 lifecycle status 字段。
- `internal/analytics/converge` / `correlate` / `incident` 已拆出基础包边界。
- `internal/analytics/rarity` 已提供 count-based、workload-aware baseline 和 baseline maintenance primitive。
- `make -C test e2e-graph-all` 当前聚合 graph/incident gate。

仍需调整:

- analytics 不能长期作为 manager ingest 同步路径里的重计算逻辑。
- incident metadata/lifecycle 应进入 Postgres。
- evidence docs、timeline、entity docs、searchable signals/events 应进入 OpenSearch。
- rarity baseline 更新应由 Kafka worker 维护,Postgres 只保存 metadata/summary 或 compact state。

### Deliverables

Workers:

```text
normalizer-worker
endpoint-detection-worker
cloud-correlation-worker
incident-builder-worker
evidence-worker
rarity-baseline-worker
opensearch-indexer
```

Storage contract:

- Postgres: incident metadata, lifecycle, owner, status, evidence references.
- OpenSearch: evidence documents, incident timeline, entity documents, searchable event/signal/findings.

### Acceptance Criteria

- Existing MVP incidents are produced through converge/incident package boundary.
- Evidence subgraph is produced by graph/evidence APIs.
- CLI/API can query incident evidence path as JSON.
- Incident evidence can be attached and survives incident update.
- Incidents can be merged by explicit id without losing target lifecycle state.
- OpenSearch-backed evidence/timeline query path exists for containerized e2e.

### Suggested Tests

```bash
go test ./...
make -C test e2e-graph-all
make -C test e2e-opensearch-evidence
make -C test e2e-kafka-incident-pipeline
```

## 10. Phase 4: Postgres Control / State Store

Status: foundation implementation started, architecture target changed.

### Goal

把 file-backed MVP store 彻底移出正式路径,实现 Postgres-only control/state/audit store。

### Current Implementation Slice

已落地:

- `internal/store/migrations` 已定义 Postgres schema v1,覆盖 agents、agent_health、rules、policies、policy_assignments、policy_audit、operator_role_bindings、events、signals、incidents、incident_events、evidence、response_audit、evidence_pullbacks、agent_gateway_sessions、rarity_baseline、metrics 和基础查询索引。
- `internal/store/postgres` 已提供基于 `database/sql` 的 migration runner。
- 当前 manager 已有 `--store-backend file|memory|postgres` 过渡入口。
- Postgres snapshot adapter 已开始接入逐表读写路径。
- response audit 和 policy control 已有第一组 mutation write path。
- `make -C test e2e-postgres-all` 当前聚合 Postgres foundation gate。

架构调整:

- 删除 file store 产品路径。
- 删除 Postgres backend 对 `sysarmor_state` snapshot 的在线依赖。
- Postgres 成为 manager / Agent Gateway / workers 的唯一平台权威状态库。
- memory/mock store 仅保留给单测,不作为部署 backend。
- 原 `agent_gateway_sessions` 后续改名为 `agent_gateway_sessions`;原 AgentGateway cursor 改名为 Agent Gateway cursor/checkpoint。

### Postgres Tables

Control and state:

```text
tenants
agents
agent_inventory
agent_health_latest
agent_gateway_sessions
agent_gateway_cursors
policies
policy_versions
policy_assignments
policy_audit
rules_metadata
operator_role_bindings
response_commands
response_approvals
response_acks
response_audit
incidents
incident_status
incident_assignees
evidence_metadata
evidence_references
rarity_baseline_metadata
jobs
schema_migrations
```

Postgres may keep compact telemetry summary tables, but raw high-volume telemetry should flow through Kafka/OpenSearch instead of unbounded relational tables.

### Acceptance Criteria

- Manager starts only with Postgres platform backend in production mode.
- file backend is removed or marked unsupported for production.
- Policy, response, incident, evidence metadata, Agent Gateway sessions/cursors survive restart through Postgres tables.
- No online API path depends on `sysarmor_state`.
- Postgres migration and schema version are observable.

### Suggested Tests

```bash
go test ./...
make -C test e2e-postgres-all
make -C test e2e-postgres-live
make -C test e2e-postgres-no-snapshot
make -C test e2e-postgres-manager-api-table-path
```

## 11. Phase 5: Agent Gateway / Agent Channel

Status: AgentGateway foundation implementation started; architecture term changed.

### Goal

把当前 AgentGateway stream 过渡为 **Agent Gateway / Agent Channel**:统一 agent 接入、上传、ack、resume、downlink、health、response ack、evidence pullback。

### Current Implementation Slice

已落地:

- 当前 `internal/agentgateway` 已定义 upload、health、ack、evidence pullback、downlink frame、stream session、resume cursor 的最小 contract。
- gRPC bidi stream 已可下发 policy/response/evidence pullback frame,并接收 upload/health/ack/evidence pullback result/error frame。
- agent 支持 `manager.transport: stream`。
- `make -C test e2e-agent-gateway-stream-all` 当前聚合 stream foundation gate。

架构调整:

- 文档和产品术语使用 Agent Gateway / Agent Channel。
- 代码边界收敛为 `internal/agentgateway`,旧 Link1 命名和兼容接口从产品路径删除。
- Agent Gateway 不直接做复杂 analytics,只做 validation、durable Kafka append、hot state update、downlink dispatch、cursor checkpoint。
- session live state 写 Redis;durable open/close/cursor checkpoint 写 Postgres。

### Deliverables

- Agent Gateway API / gRPC stream.
- Agent identity and auth.
- Upload batch validation.
- Kafka append with idempotency key.
- Redis live session and hot cursor cache.
- Postgres durable cursor checkpoint.
- Downlink cache and dispatch.
- Backpressure/rate-limit behavior.
- Resume after gateway crash.

### Acceptance Criteria

- Agent can upload batches through Agent Gateway stream.
- Agent ack is returned only after durable Kafka append.
- Agent can reconnect and resume without duplicate amplification.
- Redis stores online/hot state;Postgres stores durable checkpoint.
- Manager can send policy update, observe-only response command, and evidence pullback through gateway.
- Existing unary compatibility remains only as temporary transition path.

### Suggested Tests

```bash
go test ./...
make -C test e2e-agent-gateway-stream-all
make -C test e2e-agent-gateway-kafka-ack
make -C test e2e-agent-gateway-redis-hot-state
make -C test e2e-agent-gateway-postgres-cursor
```

## 12. Phase 6: Kafka Ingest Pipeline

Status: planned.

### Goal

引入 Kafka 作为高频 telemetry durable log,解耦 Agent Gateway、detection、indexing、incident builder 和 external fanout。

### Topics

```text
sysarmor.agent.upload.raw
sysarmor.endpoint.events
sysarmor.endpoint.signals
sysarmor.agent.health
sysarmor.response.acks
sysarmor.evidence.results
sysarmor.detection.findings
sysarmor.incident.timeline
```

### Deliverables

- Kafka producer in Agent Gateway.
- Kafka consumers for workers.
- Message envelope with tenant_id, agent_id, host_id, batch_id, event_id/signal_id, observed_at, schema_version.
- Idempotency and replay contract.
- Dead-letter topic.
- Local containerized Kafka for e2e.

### Acceptance Criteria

- Upload batch is acked after Kafka append.
- Worker can consume raw upload and emit normalized events/signals.
- Duplicate batch replay does not amplify OpenSearch/Postgres state.
- Kafka outage returns explicit backpressure/retry behavior to agent.

### Suggested Tests

```bash
make -C test e2e-kafka-ingest
make -C test e2e-kafka-replay-idempotency
make -C test e2e-kafka-outage-backpressure
```

## 13. Phase 7: Redis Hot State

Status: planned.

### Goal

引入 Redis 保存高频、短生命周期、可重建的热状态,避免 Postgres 承担 heartbeat/last_seen/downlink fanout 等高频更新。

### Keys

```text
agent:{tenant}:{agent}:online
agent:{tenant}:{agent}:last_seen
agent:{tenant}:{agent}:health_latest
agent:{tenant}:{agent}:gateway_owner
agent:{tenant}:{agent}:hot_cursor
downlink:{tenant}:{agent}:policy
downlink:{tenant}:{agent}:responses
downlink:{tenant}:{agent}:evidence_pullbacks
ratelimit:{tenant}:{agent}
lease:{name}
```

### Acceptance Criteria

- Agent online/latest health query can use Redis with Postgres fallback.
- Gateway stream owner is visible in Redis.
- Pending downlink can be cached in Redis and rebuilt from Postgres.
- Redis loss does not lose policy, response, incident, or cursor authority.

### Suggested Tests

```bash
make -C test e2e-redis-agent-online
make -C test e2e-redis-downlink-cache
make -C test e2e-redis-rebuild-from-postgres
```

## 14. Phase 8: OpenSearch Evidence / Search Layer

Status: planned.

### Goal

把 searchable telemetry、signals、findings、evidence、incident timeline、entity docs 从 Postgres/file-store 思路中剥离,进入 OpenSearch。

### Indexes

```text
sysarmor-events-YYYY.MM.DD
sysarmor-signals-YYYY.MM.DD
sysarmor-findings-YYYY.MM.DD
sysarmor-evidence-YYYY.MM
sysarmor-incident-timeline
sysarmor-entities
sysarmor-hunting
```

### Deliverables

- OpenSearch client and bulk indexer.
- Index templates / mappings.
- Retention policy.
- Search APIs in manager facade.
- Evidence/timeline query APIs.
- Containerized OpenSearch e2e.

### Acceptance Criteria

- Endpoint events and signals are searchable in OpenSearch.
- Incident evidence can be queried by incident_id/entity/time.
- Incident timeline can be reconstructed from OpenSearch docs plus Postgres metadata.
- OpenSearch outage does not block agent ack after Kafka append;indexing retries or DLQ are explicit.

### Suggested Tests

```bash
make -C test e2e-opensearch-index
make -C test e2e-opensearch-evidence
make -C test e2e-opensearch-incident-timeline
```

## 15. Phase 9: Containerized Platform Environment

Status: planned.

### Goal

提供可一键启动的全套平台环境,方便本地开发、CI、VM/container agent e2e 和资源占用测试。

### Services

```text
postgres
kafka
redis
opensearch
manager-api
agent-gateway
normalizer-worker
detection-worker
incident-worker
opensearch-indexer
```

### Required Targets

```bash
make -C test platform-up
make -C test platform-down
make -C test platform-reset
make -C test platform-status
make -C test e2e-platform-smoke
make -C test e2e-platform-container-agent
make -C test e2e-platform-vm-agent
```

### Acceptance Criteria

- A clean machine can start the full platform with containers.
- Manager, Agent Gateway, workers, Postgres, Kafka, Redis, OpenSearch become healthy.
- Containerized agent can upload through Agent Gateway into Kafka and produce searchable OpenSearch docs.
- VM agent can use the same platform endpoint.
- Resource tests can compare baseline, EDR idle, EDR business workload, detection workload, and soak windows.

## 16. Cross-cutting Requirements

### Compatibility

- v2 `e2e-agent-runtime-all` remains the runtime regression gate.
- Existing policy/response/incident behavior should remain stable while storage and ingest backend change.
- Old AgentGateway HTTP/gRPC compatibility may remain temporarily, but product docs should move to Agent Gateway.

### Security

- Agent Gateway requires agent identity/auth.
- Policy and response APIs must be tenant-scoped.
- Response commands must be auditable.
- Enforce mode must be disabled by default.
- Dev token support can remain for local e2e, but production auth design should not depend on it.

### Observability

- Platform service health is queryable.
- Kafka lag, worker errors, Redis hot-state health, OpenSearch indexing failures, and Postgres migration version are visible.
- Agent active policy version appears in health.
- Response decisions and results are queryable.
- Agent Gateway session/cursor state is visible.

### Resource / Performance

- Container deployment measures EDR cost from the host perspective: sensor/agent container CPU and memory, plus workload container impact.
- VM deployment measures EDR cost inside the protected VM: `sysarmor-agent`, `tetragon`, `tetra`, and business process CPU/RSS.
- Platform tests also measure Postgres/Kafka/Redis/OpenSearch resource footprint.
- Early gates may warn only; release gates should enforce thresholds on fixed runners or fixed VM specs.

### Testing

Recommended v3 aggregate gates:

```bash
go test ./...
make -C test e2e-agent-runtime-all
make -C test e2e-policy-all
make -C test e2e-response-all
make -C test e2e-graph-all
make -C test e2e-postgres-all
make -C test e2e-agent-gateway-stream-all
make -C test e2e-kafka-ingest
make -C test e2e-redis-agent-online
make -C test e2e-opensearch-index
make -C test e2e-platform-smoke
make -C test perf-resource TOPO=container SCENARIO=edr-idle DUR=60
make -C test perf-resource TOPO=vm SCENARIO=edr-idle DUR=60
make -C test perf-resource-all DUR=60
```
