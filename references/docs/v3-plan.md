# SysArmor v3 Implementation Plan

Date: 2026-06-15
Status: proposed plan

## 1. Goal

v3 的目标是把 v2 已经完成的 **EDR endpoint runtime MVP** 往上推进一层,形成可运营的 **EDR platform MVP**。

v2 已经证明:

```text
agent daemon + sensor runtime + scope contract + policy apply + health + spool + retry
```

v3 要补的是:

```text
policy/control plane + rule content + response contract
  + incident/evidence/graph foundation
  + durable store + reliable Link1 control/data channel
```

换句话说,v3 不是重新做采集,也不是直接扩成 XDR。v3 要解决的是:检测内容怎么运营、策略怎么下发、响应怎么授权和审计、incident 证据怎么从图里组织、数据如何可靠持久化、agent 与 manager 之间如何形成双向控制链路。

## 2. Product Positioning

v3 的产品定位是 **EDR platform foundation**。

它仍然是 endpoint-first:

- endpoint runtime 已由 v2 打底。
- policy/control plane 是 v3 的第一优先级。
- response/enforce 先做可审计骨架,不急着生产级阻断。
- graph/evidence 先做包结构和最小可用查询,复杂 rarity/STP 后续迭代。
- Postgres 与 Link1 stream 是为了支撑 policy、incident、evidence、health 和控制链路,不是为了过早做大型数据平台。

v3 完成后,系统应该从:

```text
能采集、能检出、能长期运行
```

推进到:

```text
能运营规则和策略、能审计响应意图、能沉淀 incident/evidence、能用 durable store 和可靠链路支撑平台化
```

## 3. Non-goals

v3 不做以下内容:

- 完整 XDR 多源 ingestion。
- 完整 NODLINK/STP/rarity 生产算法。
- 生产级 kill/block/quarantine 默认启用。
- 完整 UI 产品。
- 完整 SIEM/SOAR/数据湖出口。
- 完整多租户商业化权限系统。
- 大规模消息平台强依赖。

v3 可以保留接口、schema 和扩展点,但不要把 cloud audit、identity、network、K8s audit、CI/CD ingestion 全部拉进主线。

## 4. Guiding Principles

1. **先控制面,后响应面**
   - response 必须被 policy 授权。
   - 没有 policy assignment / version / audit 时,不要直接做真实阻断。

2. **先规则内容化,后复杂 DSL**
   - v3 不需要一步到位做完整 CEP/DSL。
   - 先把硬编码规则迁成可版本化、可启停、可下发的 content pack。

3. **先 evidence/graph 包结构,后复杂算法**
   - 先让 incident 能从 graph/evidence API 拿路径和实体关系。
   - rarity/STP 可以先留接口和最小实现。

4. **Postgres 优先于 MQ/Redis**
   - policy、rule versions、incident lifecycle、evidence、agent health 都需要 durable store。
   - MQ/Redis 根据吞吐、fanout 和解耦压力再引入。

5. **Link1 stream 先定义语义,再迁移协议**
   - session、ack cursor、resume、downlink、response command 的语义比传输形态更重要。
   - HTTP/gRPC unary 可作为兼容路径保留。

## 5. Architecture Target

v3 目标架构:

```text
sysarmor-agent
  -> sensor runtime
  -> endpoint rule engine
  -> local spool
  -> Link1 stream client
  <- policy downlink
  <- response command

sysarmor-manager
  -> policy registry
  -> rule content registry
  -> agent assignment
  -> response audit
  -> incident lifecycle API
  -> Postgres store

analytics
  -> ingest
  -> entity normalization
  -> graph
  -> evidence
  -> correlate
  -> converge
  -> incident

Link1
  agent upload stream
  health heartbeat
  policy downlink
  response command
  ack cursor / resume
  evidence pullback
```

## 6. Phase 1: Policy + Rule Content Minimum Loop

Status: partial implementation started.

### Goal

把当前硬编码检测能力推进到可运营的最小控制面。

### Current Implementation Slice

已落地的第一刀:

- `internal/policy` 定义 rule content、policy、assignment 的最小模型。
- `configs/rules/{endpoint,cloud}/` 和 `configs/policies/default-edr-policy.json` 提供默认 content pack。
- manager store 可持久化 rules、policies、assignments。
- manager 可配置静态 `operator-token`,对 policy / response / incident / evidence pullback 等控制面写操作做最小门禁;在配置 token 后,写操作需要最小角色授权:已支持 actor -> roles 绑定,`admin` 全通,`policy_admin` 管 policy,`responder` 管 response,`incident_admin` 管 incident/evidence;未配置绑定的 actor 仍保留 `X-SysArmor-Role` 兼容路径。
- `sysarmorctl` 可通过 `SYSARMOR_OPERATOR_TOKEN`、`SYSARMOR_ROLE` 和 `SYSARMOR_ACTOR` 传递操作者 token、角色与审计主体。
- manager `GET|POST /api/v1/operator-role-bindings` 与 `sysarmorctl operator-role-bindings` 可创建/查询 actor role binding,并随 store state 持久化。
- manager HTTP API 支持:
  - `GET /api/v1/rules`
  - `GET|POST /api/v1/policies`
  - `POST /api/v1/policy-publish`
  - `GET /api/v1/policy-audit`
  - `GET|POST /api/v1/operator-role-bindings`
  - `GET|POST /api/v1/policy-assignments`
  - `GET /api/v1/effective-policy`
- `sysarmorctl` 支持查询 rules、policies、policy-assignments、effective-policy、policy-audit,并可通过 `policy-publish` 发布/取消发布 policy version。
- analytics 会按 effective policy 的 cloud rule references 启停 cloud convergence rule。
- agent 启动时会通过 HTTP 拉取 effective policy,并用 endpoint rule references 初始化 endpoint rule engine。
- agent 会按 `policy.refresh_interval` 周期性刷新 effective policy,并在 policy/rule references 变化时切换 endpoint rule engine。
- agent health 会报告实际生效 policy id/version/mode。
- `make -C test e2e-policy-endpoint-disable` 验证 agent 拉取 assigned policy 后禁用 endpoint rule,对应 endpoint signal 不再生成。
- `make -C test e2e-policy-agent-refresh` 验证无需重启 agent 即可刷新 endpoint policy。
- `make -C test e2e-policy-cloud-disable` 验证 manager policy assignment 禁用 cloud rule 后不再收敛 incident。
- `make -C test e2e-policy-publish` 验证 draft policy 不能分配和生效,发布后才可进入 effective policy,且 upsert/publish/assign 会进入 policy audit。
- HTTP 单测验证配置 operator token 后控制面写操作必须带 operator token 和匹配 role,错误 role 返回 forbidden,actor 可从 `X-SysArmor-Actor` 进入 policy audit,且 actor role binding 会优先于 header role 参与授权。
- `make -C test e2e-operator-role-bindings` 验证 actor role binding 可授权控制面写操作,CLI 可创建/查询 binding,并随 store state 持久化。
- daemon 单测验证禁用 endpoint rule 后对应 endpoint signal 不再生成。

仍未完成:

- 生产级 manager API 认证/RBAC 与更完整审计语义;当前已有静态 token + actor role binding 的最小门禁,还没有真实身份、租户级权限模型、session/JWT、审计签名或集中权限管理。

### Deliverables

- Rule content model:
  - `rule_id`
  - `version`
  - `enabled`
  - `where`: endpoint | cloud
  - `severity`
  - `tags`
  - `mitre`
  - `response_intent`
- Policy model:
  - `policy_id`
  - `version`
  - `tenant_id`
  - `scope`
  - endpoint rule references
  - cloud rule references
  - observe/enforce mode
- Content pack layout:

```text
configs/rules/endpoint/
configs/rules/cloud/
configs/policies/
```

- Manager policy API:
  - create/update/list/get policy
  - publish policy version
  - assign policy to agent/scope
  - get effective policy
- Agent policy fetch/apply:
  - fetch policy at startup
  - refresh policy on interval or downlink signal
  - apply endpoint rule enable/disable
  - expose policy version in health
- Cloud rule enable/disable:
  - manager/analytics honors cloud rule state
  - recompute tests can use real policy state instead of ad hoc flags

### Acceptance Criteria

- A policy can disable an endpoint rule and e2e proves the corresponding signal disappears.
- A policy can disable a cloud rule and e2e proves the corresponding incident does not converge.
- Agent health reports active policy id/version.
- Manager can query policy assignment by agent/scope.
- v1/v2 detection default behavior remains unchanged when default policy is enabled.

### Suggested Tests

```bash
go test ./...
make -C test e2e-policy-endpoint-disable
make -C test e2e-policy-cloud-disable
make -C test e2e-policy-agent-refresh
make -C test e2e-policy-publish
make -C test e2e-operator-role-bindings
```

## 7. Phase 2: Response / Enforce Observe-only Skeleton

Status: partial implementation started.

### Goal

建立可审计的 response/enforce 控制链路,但默认不做生产级真实阻断。

### Current Implementation Slice

已落地的第一刀:

- `internal/response` 定义 observe-only response command / ack / audit record。
- manager store 可持久化 response commands 和 agent acks。
- manager HTTP API 支持:
  - `GET|POST /api/v1/responses`
  - `POST /api/v1/response-acks`
- `sysarmorctl responses` 可查询 response audit。
- agent 在 health loop 中轮询 pending response command,调用 sensor `Enforce`,并强制以 observe-only / executed=false 上报 ack。
- `make -C test e2e-response-observe-only` 验证 command -> agent observe-only ack -> manager audit 查询闭环。
- `internal/response` 提供最小 response policy contract: allowed actions、allowed modes、approval requirement、destructive action 显式开关;默认 policy 只允许 observe + collect/noop。
- `configs/policies/default-edr-policy.json` 已包含默认 observe-only response policy。
- manager 创建 response command 时会读取 effective policy 的 `response_policy`,可由 policy 自动要求 `pending_approval`;HTTP 单测覆盖 policy-driven approval requirement。
- manager 默认 response policy 拒绝 destructive action 并持久化 denied audit。
- `make -C test e2e-response-policy-deny` 验证 destructive action 默认拒绝且不会进入 pending。
- manager 会用 agent health runtime scope 校验显式 response command scope,错 scope 会 denied 并留下 audit。
- `make -C test e2e-response-scope-deny` 验证 response command 不能越过 agent runtime scope 边界。
- Signal proto 已有原生 `response_intent` 字段,endpoint terminal signal 可写入结构化响应意图。
- manager `POST /api/v1/response-decisions` 可将 signal response intent 转为 observe-only response command。
- `sysarmorctl response-decision` 可从 terminal signal 创建 response decision。
- `make -C test e2e-response-audit` 验证 signal intent -> response decision -> audit 查询闭环。
- manager `POST /api/v1/response-approvals` 与 `sysarmorctl response-approval` 可把 `approval_required` command 从 `pending_approval` 转为 `pending` 或 `denied`。
- `make -C test e2e-response-approval` 验证待审批 response 不会进入 agent pending,审批通过后才会进入 pending。
- response policy 支持最小多级审批合约:`approval_threshold` 和 `approval_roles`;response command 会记录 approval history,达到阈值后才会从 `pending_approval` 进入 `pending`。
- `make -C test e2e-response-multi-approval` 验证错误审批角色不能通过,单次审批只能进入 partial,满足阈值后才会进入 agent pending。

仍未完成:

- 生产身份认证和完整 RBAC;当前已有静态 operator token、actor role binding 与 response 多级审批合约,但还没有真实身份、租户级角色绑定、审批组、审批策略继承或审计签名。

### Deliverables

- Response intent in Signal:
  - `response_intent`
  - `recommended_action`
  - `confidence`
  - `reason`
- Response policy:
  - allowed actions
  - allowed scopes
  - observe-only / enforce mode
  - approval requirement
  - approval threshold and approver roles
- Response command model:
  - `response_id`
  - `policy_id`
  - `agent_id`
  - `scope`
  - `action`: kill | block | quarantine | collect | noop
  - `mode`: observe | enforce
- Agent response executor:
  - accepts command
  - validates policy and scope
  - calls sensor `Enforce`
  - returns observe-only ack by default
- Response audit store:
  - decision
  - command
  - result
  - actor
  - timestamps

### Acceptance Criteria

- Endpoint terminal signal can produce a response intent.
- Manager can convert response intent into an observe-only response decision.
- Agent can receive a response command and return `would_enforce` / `unsupported` / `executed=false`.
- Response decision and result are persisted and queryable.
- No test enables destructive enforcement by default.

### Suggested Tests

```bash
go test ./...
make -C test e2e-response-observe-only
make -C test e2e-response-policy-deny
make -C test e2e-response-scope-deny
make -C test e2e-response-audit
make -C test e2e-response-approval
```

## 8. Phase 3: Incident / Evidence / Graph Foundation

Status: partial implementation started.

### Goal

把 MVP analytics 拆成真正的 graph/evidence/incident 包结构,为后续 rarity/STP/XDR 做地基。

### Deliverables

### Current Implementation Slice

已落地的第一刀:

- `internal/analytics/graph` 提供最小 graph builder,可从 signals 构建 evidence nodes/edges。
- `internal/analytics/evidence` 已通过 graph API 生成 `EvidenceSubgraph`,不再直接 ad hoc 拼 nodes。
- manager `GET /api/v1/incident-evidence` 可按 incident id 或 scenario 查询 incident evidence subgraph。
- `sysarmorctl incident-evidence` 可直接输出 evidence graph JSON。
- `make -C test e2e-graph-evidence` 验证 staged-drop 共享 file 节点能形成 evidence graph,并能查询 file -> socket connect edge。
- Incident proto 已有最小 lifecycle status 字段: `open` / `closed` / `suppressed`。
- manager `POST /api/v1/incident-lifecycle` 与 `sysarmorctl incident-lifecycle` 可更新 incident 状态、原因和操作者。
- `make -C test e2e-incident-lifecycle` 验证 incident 可 suppress / close / reopen 并可查询。
- manager `POST /api/v1/incident-evidence` 与 `sysarmorctl incident-evidence-attach` 可向既有 incident 追加 evidence subgraph,并在 incident 重新派生/upsert 时保留已追加证据。
- `make -C test e2e-incident-attach-evidence` 验证 incident 可追加 evidence node/edge 并通过 evidence graph 查询。
- manager `POST /api/v1/incident-merge` 与 `sysarmorctl incident-merge` 可按显式 incident id 合并两个 incident,合并 evidence、lineage、terminal、MITRE 和 contributing signals,并删除 source incident。
- `make -C test e2e-incident-merge` 验证 incident merge 后 target 保留、source 移除、source evidence 进入 target。
- `internal/analytics/converge` 提供最小 converge decision 边界,从 ingest 中拆出 terminal / cross-lineage / additive threshold 成案判断。
- `internal/analytics/correlate` 提供最小 signal correlation view,从 ingest 中拆出 signal 分组、scenario 选择、terminal 判断和 entity 聚合。
- `internal/analytics/incident` 提供 incident builder,从 ingest 中拆出 Incident 构造、evidence 绑定、lineage/terminal 提取和默认 lifecycle status。
- `internal/analytics/rarity` 提供最小 scorer interface 和当前兼容的 risk * global rarity scorer。
- `internal/analytics/rarity` 已提供 count-based MVP scorer,同一 incident 内重复 signal name 会按出现次数降权。
- `internal/analytics/rarity` 已提供 workload-aware baseline scorer,可按 workload/signal historical count 对常见信号降权,并可被 incident builder 注入使用。
- `internal/analytics/rarity` 已提供 baseline maintenance primitive,可从 signal 流自动累计 workload/global signal counts,并支持 snapshot / merge。
- manager ingest 主路径会从 store 读取 rarity baseline 注入 analytics scorer,并在成功接收新 endpoint signals 后更新 baseline;baseline 已进入 store state,可随 file/Postgres snapshot backend 持久化。
- manager `GET /api/v1/rarity-baseline` 与 `sysarmorctl rarity-baseline` 可查询当前 baseline,也可按 workload/signal 查询 count。
- `make -C test e2e-rarity-baseline` 验证 endpoint signal 上传会更新 workload/global rarity baseline,重复 batch 不放大,且 baseline count 可查询。
- `internal/analytics/graph` 已支持最小 `KHop` 和 `ShortestPath` 查询。
- manager `GET /api/v1/incident-evidence` 与 `sysarmorctl incident-evidence` 支持 `seed/hops` 和 `path_from/path_to` 查询。

仍未完成:

- 生产级 rarity baseline 自动维护:当前已有 store-backed baseline observe/snapshot/merge 和 manager ingest 主路径接入,仍缺 CMS / IDF、窗口/TTL、baseline 训练策略和逐表 Postgres adapter。

Package split:

```text
internal/analytics/ingest
internal/analytics/entity
internal/analytics/graph
internal/analytics/evidence
internal/analytics/correlate
internal/analytics/converge
internal/analytics/incident
internal/analytics/rarity
```

Graph foundation:

- node model:
  - process
  - file
  - socket/ip
  - host
  - container
  - pod
  - user
- edge model:
  - fork
  - exec
  - read
  - write
  - connect
  - load
  - owns / belongs_to
- indexes:
  - lineage index
  - entity key index
  - signal-to-entity index
- query:
  - k-hop neighborhood
  - shortest path between entities
  - incident evidence subgraph extraction

Incident lifecycle minimum:

- create
- update
- merge
- close
- suppress
- attach evidence

Rarity interface:

- define interface
- provide no-op or simple count-based MVP implementation
- provide workload baseline observe/snapshot/query contract
- leave CMS/IDF/windowed baseline for later

### Acceptance Criteria

- Existing MVP incidents are produced through the new converge/incident package boundary.
- Evidence subgraph is produced by graph/evidence APIs, not ad hoc assembly only.
- CLI can query incident evidence path as JSON.
- Incident evidence can be attached and survives incident upsert/recompute.
- Incidents can be merged by explicit id without losing target lifecycle state.
- Existing `apt-fileless-c2`, `apt-staged-drop`, and `benign-ci-noise` semantics remain stable.

### Suggested Tests

```bash
go test ./...
make -C test e2e-graph-evidence
make -C test e2e-incident-lifecycle
make -C test e2e-incident-attach-evidence
make -C test e2e-incident-merge
make -C test e2e-rarity-baseline
make -C test e2e TOPO=container SCENARIO=apt-staged-drop DUR=12
```

## 9. Phase 4: Postgres Store

Status: foundation implementation started.

### Goal

把 file-backed MVP store 推进到 durable platform store。

### Current Implementation Slice

已落地的第一刀:

- `internal/store/migrations` 定义 Postgres schema v1,覆盖 agents、agent_health、rules、policies、policy_assignments、events、signals、incidents、incident_events、evidence、response_audit、metrics 和基础查询索引。
- `internal/store/postgres` 提供基于标准库 `database/sql` 的 migration runner,可对 live Postgres 执行 schema v1。
- manager 已提供 `--store-backend file|memory|postgres`、`--postgres-driver`、`--postgres-dsn` 配置入口;postgres 分支会先执行 migration runner,再打开 JSON snapshot-backed store 作为逐表 adapter 前的过渡路径。
- `internal/transport/link1` 已提取 `ManagerStore` 接口,manager/Link1 transport 不再直接绑定具体 file store 类型,为 Postgres adapter 接入预留稳定 contract。
- file store 已抽出 `ExportState` / `ImportState` 状态序列化边界,Postgres snapshot adapter 复用同一套 proto/json state contract 持久化完整 manager state,后续可逐步落表。
- file/memory store 已暴露 backend metadata: backend type、state version、migration version、Postgres schema version。
- store 已提供 backend metadata/save hook,Postgres snapshot adapter 会将 `Info().Backend` 暴露为 `postgres` 并把 `Save()` 写入 `sysarmor_state`。
- Postgres snapshot adapter 保存 snapshot 时会同步投影 agents 到 `agents` 表、agent health 到 `agent_health` 表,response command/ack 到 `response_audit` 表,policy versions 到 `policies` 表,policy assignments 到 `policy_assignments` 表,作为逐表 adapter 的第一组运营表路径。
- manager `/healthz` 会返回 store backend 信息。
- manager `GET /api/v1/store-status` 与 `sysarmorctl store-status` 可查询 store backend 和 migration/schema version。
- manager `events` / `signals` / `incidents` 查询 API 与 `sysarmorctl` 已支持 `limit` / `offset` 分页参数。
- `make -C test e2e-store-status` 验证 manager file backend 和 Postgres schema version 可观测。
- `make -C test e2e-query-pagination` 验证 query pagination contract。
- `make -C test e2e-postgres-store` 验证 Postgres backend 可运行 migration、打开 snapshot store,并跨 reopen 保留 response audit。
- `make -C test e2e-postgres-idempotency` 验证 snapshot-backed Postgres backend 保持重复 ingest 的幂等性。
- `make -C test e2e-postgres-policy-persistence` 验证 snapshot-backed Postgres backend 跨 reopen 保留 policy publish/assignment/audit 和 incident lifecycle 状态。
- `make -C test e2e-postgres-manager-api` 验证 snapshot-backed Postgres backend 可支撑 manager ingest/query/policy/incident lifecycle API,并跨 reopen 保留 API 写入状态。
- `make -C test e2e-postgres-agent-projection` 验证 Postgres backend 保存 snapshot 时会 upsert `agents` 和 `agent_health` 表投影。
- `make -C test e2e-postgres-response-projection` 验证 Postgres backend 保存 snapshot 时会 upsert `response_audit` 表投影,包含 command 与 ack。
- `make -C test e2e-postgres-policy-projection` 验证 Postgres backend 保存 snapshot 时会 upsert `policies` 和 `policy_assignments` 表投影。
- `make -C test e2e-postgres-all` 当前聚合 Postgres foundation gate。

仍未完成:

- live Postgres migration e2e。
- 逐表 Postgres adapter:当前只开始投影 `agents` / `agent_health` / `response_audit` / `policies` / `policy_assignments`,尚未把 incident/query 主路径迁到逐表读写。
- ingest/query/policy/incident e2e 已有 snapshot-backed Postgres manager API 门禁;仍缺完整逐表 Postgres adapter 路径上的同类 e2e。

### Deliverables

- Postgres schema:
  - agents
  - agent_health
  - policies
  - policy_versions
  - policy_assignments
  - rules
  - rule_versions
  - events
  - signals
  - incidents
  - incident_events
  - evidence
  - response_audit
  - metrics
- Migration system.
- Store interface split:
  - MVP file store remains for tests/dev.
  - Postgres store becomes platform path.
- Query pagination.
- Basic indexes:
  - tenant_id
  - agent_id
  - host_id
  - scope
  - lineage_id
  - entity_key
  - incident_id
  - observed_at
- Idempotent upsert semantics preserved.

### Acceptance Criteria

- Manager can run with Postgres store.
- Existing ingest/query tests pass against Postgres.
- Duplicate batch idempotency works against Postgres.
- Policy and incident APIs persist across manager restart.
- File store remains available for lightweight dev/e2e unless explicitly removed later.

### Suggested Tests

```bash
go test ./...
make -C test e2e-postgres-store
make -C test e2e-postgres-idempotency
make -C test e2e-postgres-policy-persistence
make -C test e2e-store-status
make -C test e2e-postgres-agent-projection
make -C test e2e-postgres-response-projection
make -C test e2e-postgres-policy-projection
```

## 10. Phase 5: Link1 Bidirectional Stream

Status: foundation implementation started.

### Goal

把 Link1 从 unary upload 推进到可靠双向控制/数据通道。

### Current Implementation Slice

已落地的第一刀:

- store 已定义 `Link1Session`,包含 session id、agent id、tenant id、start time、last seen、closed at、last ack cursor、transport 和 status。
- HTTP/gRPC unary upload 成功后会更新 Link1 session,`last_ack_cursor` 使用当前 accepted `batch_id`。
- manager `GET /api/v1/link1-sessions` 与 `sysarmorctl link1-sessions` 可查询 Link1 session state。
- manager `GET /api/v1/link1-resume` 与 `sysarmorctl link1-resume` 可按 agent 查询 resume cursor。
- agent spool 已支持 `AckThrough(cursor)`,可在拿到 manager resume cursor 后删除 cursor 及之前的已确认本地 batch。
- agent 启动 HTTP upload worker 时会调用 Link1 resume API,并用 `AckThrough(cursor)` 清理 manager 已确认的本地 spool batch;resume API 不可用时 fail-soft,避免阻断 endpoint runtime。
- `internal/transport/link1` 已定义最小 downlink frame contract:`policy_update`、`response_command` 和 `evidence_pullback`。
- manager `GET /api/v1/link1-downlink` 与 `sysarmorctl link1-downlink` 可按 agent 查询 policy update frame、pending response command frames 和 pending evidence pullback frames。
- manager `GET|POST /api/v1/evidence-pullbacks` 与 `sysarmorctl evidence-pullbacks` 可创建/查询 evidence pullback request,并随 file/memory store 状态持久化。
- `internal/transport/link1` 已定义最小 uplink frame contract:`upload` / `health` / `ack` / `evidence_pullback_result` / `error`。
- manager `POST /api/v1/link1-frames` 与 `sysarmorctl link1-frames --file` 可通过 HTTP 兼容路径提交 uplink frames;upload frame 会复用 ingest,health frame 会更新 agent health,ack frame 会持久化 response ack,evidence pullback result frame 会完成/失败 pullback request 并可附加 incident evidence,error frame 会返回可确认结果。
- Link1 gRPC 已提供最小 bidirectional `Stream` RPC:agent/client 发送 `hello` 后 manager 返回当前 downlink frames,随后同一 stream 可提交 upload/health/ack/evidence_pullback_result/error uplink frames 并收到逐帧结果。
- Link1 gRPC stream `hello` 会记录 session open,后续 stream frame 会刷新 last_seen,stream close/EOF 会记录 session closed,为后续常驻连接 keepalive/reconnect 语义提供状态底座。
- agent upload worker 已支持 `manager.transport: stream`,可通过 Link1 gRPC `Stream` RPC 上传 spool batch 并推进 manager session cursor。
- agent stream uploader 会显式处理服务端 `error` frame,返回可诊断失败,避免把协议拒绝误当作成功 ack。
- agent 在 `manager.transport: stream` 下会通过 Link1 stream `health` frame 上报 health heartbeat;HTTP health 上报保留给显式 `transport: http` 兼容路径。
- Link1 stream `hello` downlink 已包含 resume cursor frame,agent stream transport 启动时可据此删除 cursor 及之前的本地 spool batch。
- agent 已支持通过 Link1 stream `hello` downlink 拉取 effective policy update,并将 endpoint rule references 应用到 runtime fastpath。
- agent 已支持通过 Link1 stream downlink 拉取 pending response command,执行 observe-only `Enforce`,并通过 stream `ack` frame 回写 response audit。
- agent 已支持通过 Link1 stream downlink 拉取 pending evidence pullback request,回传最小 target evidence subgraph,result 被 manager 完成并附加到 incident evidence。
- agent 配置默认 `manager.transport` 已切换为 `stream`;示例配置默认使用 Link1 stream,显式 `transport: http` / `grpc` 仍作为兼容路径保留。
- agent/upload worker 单测覆盖 resume cursor 清理本地 spool、空 cursor no-op、resume source 失败不删除 batch。
- `make -C test e2e-link1-session` 验证同一 agent 连续上传会推进 session cursor。
- `make -C test e2e-link1-downlink` 验证 downlink frame 包含 effective policy、pending response command 和 pending evidence pullback request。
- `make -C test e2e-link1-frames` 验证 upload / health / ack / evidence pullback result / error uplink frame contract。
- `make -C test e2e-link1-grpc-stream` 验证 gRPC bidi stream 能下发 policy/response/evidence pullback frame 并接收 upload frame。
- `make -C test e2e-link1-stream-lifecycle` 验证 gRPC stream session 会记录 open/seen/closed lifecycle,且 close 不丢失已确认 cursor。
- `make -C test e2e-link1-stream-health` 验证 gRPC stream 可接收 agent health heartbeat frame 并更新 manager agent health。
- `make -C test e2e-link1-stream-upload` 验证 agent uploader 能通过 Link1 gRPC stream 上传 batch 并推进 session cursor,且服务端 error frame 会被客户端识别为失败。
- `make -C test e2e-link1-stream-reconnect` 验证 agent stream uploader 重新连接后重复上传同一 batch 不会放大 event/signal。
- `make -C test e2e-link1-stream-resume` 验证 agent 能通过 Link1 stream 获取 resume cursor 并清理本地已确认 spool batch。
- `make -C test e2e-link1-policy-downlink` 验证 agent 能通过 Link1 stream downlink 拉取 effective policy 并获得 endpoint rule references。
- `make -C test e2e-link1-response-command` 验证 agent 能通过 Link1 stream 下发 response command 并回写 observe-only ack。
- `make -C test e2e-link1-evidence-pullback` 验证 agent 能通过 Link1 stream 下发 evidence pullback request 并回写可附加到 incident 的 evidence subgraph。
- `make -C test e2e-link1-stream-all` 当前聚合 Link1 session/cursor foundation gate。

仍未完成:

- stream 长连接/reconnect 的生产级可靠性语义;当前已有 stream session open/seen/closed 状态与 heartbeat frame,但 agent 侧仍是按 health tick 建立短 stream,不是生产级常驻长连接 keepalive。

### Deliverables

- Link1 session model:
  - session id
  - agent id
  - tenant id
  - start time
  - last seen / closed at
  - last ack cursor
  - status
- Stream upload:
  - event/signal batch frame
  - health frame
  - ack frame
  - error frame
- Resume:
  - agent resumes from cursor
  - manager acks durable cursor
  - agent keeps local spool until ack
- Downlink:
  - policy update notification
  - response command
  - evidence pullback request
- Compatibility:
  - unary HTTP/gRPC upload remains until stream path is stable.

### Acceptance Criteria

- Agent can upload batches through stream.
- Manager can ack cursor and agent can delete acked spool entries.
- Agent can reconnect and resume without duplicate amplification.
- Manager can send policy update notification over stream.
- Manager can send observe-only response command over stream.
- Existing unary upload e2e still passes.

### Suggested Tests

```bash
go test ./...
make -C test e2e-link1-stream-upload
make -C test e2e-link1-grpc-stream
make -C test e2e-link1-stream-lifecycle
make -C test e2e-link1-stream-resume
make -C test e2e-link1-policy-downlink
make -C test e2e-link1-response-command
make -C test e2e-link1-evidence-pullback
```

## 11. Phase 6: Redis / MQ Evaluation

### Goal

只在明确需要时引入 Redis 或 MQ,避免过早增加运维复杂度。

### Redis Candidates

- latest agent heartbeat cache
- policy assignment cache
- rate limit
- distributed lease
- short-lived response command state

### MQ Candidates

- ingest -> analytics decoupling
- high-volume event buffering
- async incident convergence
- fanout to external sinks
- XDR adapter ingestion

### Decision Criteria

Introduce Redis when:

- latest health queries become store-heavy
- policy fanout needs low-latency cache
- manager becomes horizontally scaled and needs leases

Introduce MQ when:

- ingest throughput blocks synchronous analytics
- analytics needs async workers
- XDR adapters produce independent high-volume streams
- external sink fanout becomes required

### Acceptance Criteria

- No Redis/MQ hard dependency is introduced without a measured bottleneck or architectural need.
- If introduced, local dev and e2e have reproducible startup targets.
- Failure behavior is explicit: degraded, retry, or fallback.

## 12. Cross-cutting Requirements

### Compatibility

- v1 replay/stream remains usable.
- v2 `e2e-agent-runtime-all` remains the runtime regression gate.
- Default detection semantics for existing scenarios remain stable.

### Security

- Policy and response APIs must be tenant-scoped.
- Response commands must be auditable.
- Enforce mode must be disabled by default.
- Dev token support can remain, but production auth design should not be blocked by it.

### Observability

- Policy version appears in agent health.
- Response decisions are queryable.
- Link1 session state is visible.
- Store backend type and migration version are visible.

### Resource / Performance

- Container deployment measures EDR cost from the host perspective: sensor/agent container CPU and memory, plus workload container impact.
- VM deployment measures EDR cost inside the protected VM: `sysarmor-agent`, `tetragon`, `tetra`, and business process CPU/RSS.
- Resource tests compare baseline, EDR idle, EDR business workload, EDR detection workload, and soak windows.
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
make -C test e2e-link1-stream-all
make -C test perf-resource TOPO=container SCENARIO=edr-idle DUR=60
make -C test perf-resource TOPO=vm SCENARIO=edr-idle DUR=60
make -C test perf-resource-all DUR=60
```

## 13. Success Criteria

v3 is complete when:

- Rule content is versioned and can be enabled/disabled through policy.
- Agent receives and applies effective policy.
- Manager can assign policy by agent/scope.
- Response intent, response decision, agent command ack, and audit trail exist.
- Default response path is observe-only and non-destructive.
- MVP analytics run through graph/evidence/incident package boundaries.
- Incident evidence can be queried as graph/path JSON.
- Manager can run on Postgres with migrations and idempotent ingest.
- Link1 stream supports upload, ack cursor, resume, policy downlink, and observe-only response command.
- Container/VM resource gates can prove normal EDR operation stays within agreed CPU/memory and business-impact thresholds.
- v1/v2 regression gates continue to pass.
- v3 aggregate gates pass.

## 14. Recommended Implementation Order

1. Policy schema and content pack files.
2. Manager policy registry and assignment APIs.
3. Agent fetch/apply effective policy.
4. Endpoint/cloud rule enable-disable e2e.
5. Response intent and observe-only response audit.
6. Link1 downlink model for policy/response, initially over simple polling if needed.
7. Analytics package split and graph/evidence MVP.
8. Incident lifecycle minimum.
9. Postgres schema and store adapter.
10. Link1 bidirectional stream and resume.
11. Redis/MQ evaluation based on measured bottlenecks.

This order keeps the system useful after every phase: policy makes rules operable, response becomes safe because it is policy-bound, graph/evidence makes incidents explainable, Postgres makes state durable, and Link1 stream turns the runtime into a true control channel.
