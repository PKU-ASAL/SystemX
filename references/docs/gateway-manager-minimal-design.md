# Gateway/Manager 最小骨架设计

## 结论

在端侧 Agent 不变的前提下，服务端先采用一个轻量三层抽象：

```text
Agent    端侧运行时，负责采集、检测、执行响应，是高可信端点遥测来源
Gateway  Agent 接入层，负责协议、身份、会话、控制流、数据接收和队列交接
Manager  控制平面和查询审计层，负责策略、指令、审计、查询和运营接口
```

服务端按拆分模式作为主设计。第一阶段可以少实现能力，但进程边界和代码边界从一开始就分清：

```text
cmd/sysarmor-gateway
  启动 Agent-facing gRPC data/control plane

cmd/sysarmor-manager
  启动 Operator-facing HTTP/API

internal/gateway
  Agent-facing gRPC 数据面/控制面

internal/managerapi
  Operator-facing HTTP/API

internal/workers/ingest
  数据落库、索引、后续分析触发

internal/store
  策略、指令、会话、健康、审计、事件、信号的事实来源
```

这样做的目标是：先打通 Agent 到 Gateway 到 Worker 到 Manager 的控制通路和数据通路，同时避免把 Manager 做成 Agent 接入层，也避免把 Gateway 做成重分析系统。

## 目标

- Agent 端侧行为保持不变，现有 telemetry batch、control stream、local watch 逻辑不动。
- Gateway 成为生产环境唯一 Agent 接入口。
- Manager 聚焦控制平面、查询、审计和运营 API。
- 当前只承接 `event`、`signal` 两类核心遥测。
- 控制流支持策略、内容、响应指令、证据拉取、ack/result。
- 为未来接入 syslog、Fluent Bit、OTLP logs、auditd、Tracee 等外部格式预留 adapter 边界。

## 非目标

- 不在端侧重新引入普通 event/signal 的 durable spool/WAL。
- 不在 Gateway 做重关联、重分析、事件生命周期管理。
- `sysarmor-gateway` 作为独立 Agent 接入口，不再由 `sysarmor-manager` 承载 agent-facing gRPC。
- 第一阶段不实现完整 evidence bundle 独立传输协议。
- 外部日志不默认伪装成高可信 endpoint `event`。

## 组件边界

### Agent

Agent 仍然只负责端侧能力：

- 管理 sensor 生命周期；
- 应用 collection/detection policy；
- 规范化端点事件；
- 生成端点信号；
- 批量发送轻量 telemetry；
- 通过本地 Unix socket 支持 `sysarmorctl watch/debug`；
- 执行授权后的响应动作。

Agent 当前原生输出：

```text
event
signal
```

后续 evidence 可以作为独立 frame 或独立对象补充，不阻塞第一阶段骨架。

### Gateway

Gateway 是 Agent 接入中间层，定位类似 OTel gateway 或 Elkeid Agent Center，但语义上面向 EDR。

Gateway 负责：

- Agent gRPC 数据面和控制面 listener；
- mTLS/token 身份校验；
- tenant/agent 绑定；
- `DataBatch` 合同校验；
- telemetry handoff；
- control stream 会话维护；
- agent health/session 热状态；
- 基础 rate limit、size limit、schema limit；
- 未来外部日志 adapter。

Gateway 不负责：

- 分析师查询 API；
- incident 生命周期；
- 重关联分析；
- policy authoring；
- response approval workflow；
- UI 直接消费的产品语义。

### Manager

Manager 是控制平面和运营接口：

- policy/content 生命周期；
- 控制指令创建、审批、审计；
- response approval 和审计；
- evidence pullback 编排；
- agent inventory/health 查询；
- event/signal/incident 查询；
- RBAC/operator authorization。

Manager 不应把 Kafka、Redis、OpenSearch 或 Gateway 内部热状态直接暴露成产品 API。

## 数据通路

端点原生遥测链路：

```text
Agent
  -> Gateway AppendBatch(DataBatch)
  -> Gateway 身份绑定和合同校验
  -> server-side handoff
  -> ingest worker
  -> event/signal store 和索引
  -> analytics/correlation workers
  -> Manager query API
```

第一阶段保留两种 handoff 模式：

```text
local-ingest processor     开发和 e2e 使用，Gateway 进程内处理
Kafka raw DataBatch topic  生产形态异步交接，由 Worker 消费
```

ACK 语义必须明确：

```text
Gateway 只有在 DataBatch 被服务端 handoff point 接受后，才向 Agent 返回 accepted。
```

在 `local-ingest` 模式下，进程内 ingest processor 就是 handoff point。

## 控制通路

控制链路：

```text
Manager API
  -> 持久化 policy assignment / control command / content update
  -> Gateway 查找 control stream
  -> AgentControlPlaneService.Connect 下发
  -> Agent apply/reject
  -> Agent ack/result frame
  -> Gateway 记录 ack/result
  -> Manager audit/query API 可见
```

单 Gateway 阶段可以由 Gateway 直接读取 pending commands。后续多 Gateway 时，再增加热路由表：

```text
tenant_id
agent_id
gateway_instance_id
last_seen_at
control_stream_route
```

Redis 适合承载这类热路由表，Postgres 仍然作为审计和最终事实来源。

## 外部 Adapter 模型

Gateway 从一开始就应按 adapter 思路组织：

```text
Gateway
  sysarmor-agent-grpc adapter
  agentless-http adapter       later
  syslog adapter               later
  fluent-bit/http adapter      later
  otlp-logs adapter            later
  auditd/tracee adapter        later
```

所有 adapter 先归一到内部 envelope：

```text
TelemetryEnvelope
  tenant_id
  source_id
  source_type
  observed_at
  labels
  payload_type
  payload
  raw_ref
```

建议 payload type：

```text
event
signal
evidence
external_observation
raw_log
```

关键原则：外部日志默认进入 `external_observation` 或 `raw_log`。只有经过明确 normalizer 或 analytics 推导后，才可以产生 signal。不要静默提升为 endpoint `event`，否则会混淆可信度和溯源语义。

## 最小接口

Gateway 不应该依赖完整 Manager API server，只依赖窄接口。

```go
type TelemetrySink interface {
    AppendDataBatch(ctx context.Context, batch *dataplanev1.DataBatch, meta AppendMeta) (AppendResult, error)
}

type ControlStore interface {
    EffectivePolicy(...)
    PendingControlCommands(...)
    MarkControlCommandSent(...)
    AckControlCommand(...)
    UpsertAgentHealth(...)
    RecordControlSessionOpen(...)
    RecordDataBatchAppend(...)
}

type SessionRegistry interface {
    OpenControlSession(...)
    TouchData(...)
    TouchControl(...)
    Close(...)
}
```

Manager API 直接调用 store/domain services。Gateway 不通过 HTTP Manager API 承接原生 Agent 数据路径。

## 运行模式

### 开发/e2e 拆分模式

```text
sysarmor-gateway --local-ingest
sysarmor-manager
```

- Gateway 接收 Agent gRPC 数据和控制流。
- DataBatch 进程内处理。
- Manager API 立即可查询 event/signal/health。
- 用于单测、集成测试、VM e2e。

### 生产拆分模式

```text
sysarmor-gateway --kafka-brokers ...
sysarmor-worker
sysarmor-manager
```

- Gateway 写 raw DataBatch 到 Kafka。
- ingest worker 消费、校验、落库、索引。
- Manager API 查询持久化后的状态。

### 扩展模式

```text
sysarmor-gateway
sysarmor-manager
sysarmor-worker
```

- Gateway 只暴露 Agent-facing 端口。
- Manager 只暴露 Operator-facing API。
- Worker 可以按 ingest、analytics、correlation 进一步拆分扩容。

## 测试设计

### 1. Gateway Local Ingest

验证链路：

```text
DataBatch -> Gateway -> local ingest -> Manager query
```

验收点：

- 合法 batch 返回 accepted；
- event 可查询；
- signal 可查询；
- 非法身份被拒绝；
- batch 校验错误显式返回。

### 2. Gateway Control Stream

验证链路：

```text
Agent hello/health/capability -> Gateway -> store
Manager creates command -> Gateway downlink -> Agent ack
```

验收点：

- session open 被记录；
- health 可通过 Manager API 查询；
- pending command 能被下发；
- ack/result 能更新审计状态。

### 3. Endpoint Integrated VM

验证链路：

```text
fresh vm-endpoint
agent connects Gateway
apt-fileless-c2-local
Manager query sees expected events/signals
```

验收点：

- `download_by_lolbin`；
- `payload_dropped`；
- `reverse_shell_pattern`；
- `suspicious_exec_connect`；
- `payload_lifecycle`；
- 预期 terminal signal 包含 evidence/response intent。

### 4. Adapter Smoke

后续验证链路：

```text
agentless HTTP upload -> Gateway external_observation/raw_log -> queue
```

验收点：

- 外部日志不会自动变成 endpoint event；
- adapter 能写入统一 handoff；
- analytics 可以基于外部观察产生 signal。

## 分阶段实施

### Phase 1：拆出 Gateway 入口

- 增加 Gateway/Manager 架构文档和 package 边界。
- 新增 `cmd/sysarmor-gateway`。
- Agent 不变。
- `cmd/sysarmor-manager` 不再启动 agent-facing gRPC。

### Phase 2：抽取 Gateway 后端接口

- 将 Agent-facing backend interfaces 从 `managerapi.Server` 中抽出。
- Gateway 只依赖 `TelemetrySink`、`ControlStore`、`SessionRegistry`。
- Manager API 聚焦 operator HTTP/API。

### Phase 3：明确 Telemetry Handoff

- 定义服务端 telemetry handoff interface。
- e2e 继续使用 local-ingest。
- 生产形态支持 Kafka raw DataBatch append。
- 明确 DataAck 语义：accepted 表示服务端 handoff accepted。

### Phase 4：控制流硬化

- 明确 command delivery、ack、result、audit 状态机。
- 增加 session lifecycle 测试。
- 增加 reconnect/resume 测试。
- 维持 telemetry best-effort，控制指令 durable/auditable。

### Phase 5：Adapter Ready Gateway

- 增加内部 adapter interface。
- 第一阶段只实现 native SysArmor agent adapter。
- 后续再接 agentless HTTP、OTLP、syslog、Fluent Bit 等输入。

## 风险

- 如果 Gateway 继续依赖完整 Manager API server，边界会再次混在一起。
- 如果外部日志过早提升为 endpoint event，信任语义会失真。
- 如果 Gateway 做重分析，agent ack latency 会和后端分析延迟耦合。
- 如果控制命令审计只存在热状态，后续 response/policy 变更很难解释。

## 短期建议

短期按下面的代码方向推进：

```text
internal/gateway      agent-facing data/control
internal/managerapi   operator-facing API
internal/workers      async ingest/analytics
internal/store        source of truth
```

先用拆分进程证明 Gateway/Manager/Worker 边界和测试闭环。等接口稳定、测试跑通后，再补 Kafka、Redis 热路由、多 Gateway 扩容和更完整的 adapter。
