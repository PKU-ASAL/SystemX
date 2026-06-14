# SysArmor v2 Implementation Plan

Date: 2026-06-15

## 1. Goal

v2 的目标是把 v1 MVP 从“可跑通的检测上传链路”推进到“可长期运行的 endpoint runtime 基础”。

从 SysArmor Next 的长期定位看，v2 是 **EDR endpoint runtime MVP**，不是完整 EDR 平台，也不是 XDR 阶段。它要补的是后续 EDR/XDR 都依赖的端点地基：

```text
v1: EDR detection path MVP
  endpoint event -> agent -> manager -> signal -> incident

v2: EDR endpoint runtime MVP
  agent daemon + sensor runtime + policy apply + health + spool + retry

later: EDR platform
  investigation, response, incident lifecycle, durable control plane

later: XDR platform
  cloud audit, identity, network, K8s, CI/CD 等多源事实接入同一张图
```

因此 v2 的成功标准不是“功能看起来更多”，而是：agent 能作为一个真实 EDR endpoint runtime 长期运行，sensor 由 agent 托管，数据可缓冲可恢复，健康可观测，策略可应用，v1 detection 行为保持稳定。

核心范围分为两条主线：

1. **Sensor Runtime**
   - 建立完整 Sensor 接口和 runtime 管理。
   - 支持 capability 探测、policy compile/apply、health/dropped events 统计。
   - 把 Tetragon 从“测试里跑命令”升级为 agent 托管的 sensor backend。
   - 为未来 native sensor 和 enforce 能力保留接口边界。

2. **Agent Daemon Runtime**
   - 把 `sysarmor-agent` 从 uploader/streamer 升级为长期运行 daemon。
   - 支持 config、systemd lifecycle、本地 spool、断点续传、upload retry/backoff、队列限流和背压。
   - 支持 agent registration/auth 的最小开发形态、tenant/token 字段、heartbeat/health report。

Tetragon 本地安装和生命周期管理是 v2 的必做内容，但它是完整 Sensor Runtime 的第一种 backend，不应把 v2 窄化成单纯的 Tetragon supervisor。

## 2. Current Baseline

v1 已经具备：

- Go binaries:
  - `sysarmor-agent`
  - `sysarmor-manager`
  - `sysarmorctl`
- Proto contracts under `api/proto`。
- Tetragon JSONL adapter。
- Normalize + endpoint fastpath。
- HTTP/gRPC Link1 upload。
- Manager ingest/query/recompute/metrics/store。
- Container 和 VM e2e harness。

v2 已经落地的骨架：

- `internal/agent/config`:
  - agent/manager/sensor/spool/upload/health 配置结构。
  - `configs/agent.example.yaml` 和 `configs/agent.fake.yaml`。
  - `sysarmor-agent run --config ... --dry-run` 配置校验。
- `internal/sensor/contract`:
  - Sensor interface。
  - Capability / CollectionIntent / EventEnvelope / Health。
  - observe-only/unsupported Enforce 边界。
- `internal/sensor/runtime`:
  - fake backend lifecycle 测试骨架。
  - Probe / Apply / Subscribe / Stop 的最小路径。
- `internal/agent/policy`:
  - 最小 collection policy 解析。
  - policy 到 `CollectionIntent` 的初步转换。
- `internal/sensor/tetragon`:
  - Tetragon backend skeleton。
  - 从 JSONL/stdin 读取事件。
  - policy file 存在性校验。
  - health 计数、parse error、raw ref 记录。
- `internal/agent/spool`:
  - file-backed batch queue。
  - stable batch id。
  - append/list/load/ack/stats。
  - `max_bytes`、backpressure/drop accounting。
- `internal/agent/uploadworker`:
  - oldest-first drain。
  - upload 成功后 ack。
  - upload 失败保留 batch。
- `internal/agent/daemon`:
  - fake sensor daemon 路径。
  - policy apply。
  - event normalize/fastpath。
  - spool write。
  - `--drain-once`。
  - health 输出 sensor + queue 状态。

v1 的关键缺口：

- Sensor 层还不是完整 runtime：
  - 只消费 `tetra getevents -o json` 输出。
  - 没有 Sensor 接口的完整 runtime 管理。
  - 没有 capability 探测。
  - 没有 policy compile/apply 正式控制链路。
  - 没有严肃的 dropped events / sensor health 统计。
  - 没有阻断能力，目前只是 observe。
  - 没有 native sensor，只有 Tetragon adapter。

- Agent 还不是长期运行 daemon：
  - 缺少 systemd service / daemon lifecycle。
  - 已有本地 spool 和 drain-once,但缺少完整后台 retry/backoff loop。
  - 已有队列限流和背压统计,但还未通过 manager health API 暴露。
  - 已有配置文件骨架,但生产路径配置和 systemd 仍未完成。
  - 缺少 agent registration/auth。
  - 缺少 heartbeat / health report。
  - tenant/token 已进入 config,但尚未进入完整 upload/health/store 校验链路。

v2 目标形态：

```text
sysarmor-agent daemon
  -> load config
  -> register/auth with manager in dev form
  -> initialize sensor runtime
  -> probe capability
  -> install/verify Tetragon backend from local bundle
  -> compile/apply policy
  -> subscribe sensor events
  -> normalize + endpoint detection
  -> write batches to durable spool
  -> upload with retry/backoff
  -> report heartbeat and health
  -> restart sensor on failure
  -> emit tamper/blindness signal after repeated sensor failures
```

## 3. Architecture

```text
sysarmor-agent daemon
  |
  +-- config loader
  +-- registration/auth client
  +-- lifecycle supervisor
  |
  +-- sensor runtime
  |     |
  |     +-- Sensor interface
  |     +-- capability probe
  |     +-- policy compiler/applier
  |     +-- sensor health collector
  |     +-- dropped event accounting
  |     +-- backend supervisor
  |           |
  |           +-- Tetragon backend
  |                 +-- local bundle installer
  |                 +-- process/service manager
  |                 +-- tetra event subscription
  |
  +-- endpoint pipeline
  |     |
  |     +-- normalize
  |     +-- fastpath signals
  |     +-- ringbuffer
  |
  +-- local spool queue
  |     |
  |     +-- append batches
  |     +-- ack cursor
  |     +-- retry/resume
  |     +-- backpressure/drop accounting
  |
  +-- uploader
  |     |
  |     +-- HTTP Link1
  |     +-- gRPC Link1
  |
  +-- health and incident reporter
        |
        +-- heartbeat
        +-- agent health
        +-- sensor health
        +-- queue/upload health
        +-- sensor tamper/blindness signal
```

## 4. Non-goals

v2 不做以下内容：

- 完整 graph analytics 重写。
- SQLite/Postgres store migration。
- 完整 rule DSL/content-pack 体系。
- 生产级 mTLS、远程 enrollment、多租户权限系统。
- 在线下载 Tetragon。
- 完整 native sensor。
- 完整阻断控制面。
- XDR 多源 ingestion：
  - cloud audit
  - identity provider logs
  - network flow
  - K8s audit
  - CI/CD and registry events
- 完整 investigation / response 产品面：
  - 人类可读 investigation UI
  - 完整 evidence path 浏览
  - kill / quarantine / isolate 等生产级响应编排

v2 可以定义 `Enforce()` 接口和控制链路骨架，但默认 backend 仍以 observe-only 为主。Tetragon 安装来源先使用 local bundle 或本地指定目录，避免阻塞在供应链和网络分发上。

v2 可以为 EDR/XDR 后续阶段保留 proto 字段、entity identity 和接口边界，但不要把多源接入、全局图重写、生产级响应面塞进 v2 主线。

## 5. Deliverables

### 5.1 Sensor Contract

新增 `internal/sensor/contract`。

最低接口：

```go
type Sensor interface {
    Capability(ctx context.Context) (Capability, error)
    Subscribe(ctx context.Context, intent CollectionIntent) (<-chan EventEnvelope, error)
    Enforce(ctx context.Context, cmd EnforcementCmd) (EnforcementAck, error)
    Health(ctx context.Context) (Health, error)
}
```

最低类型：

```text
Capability
  backend
  version
  supports_exec
  supports_connect
  supports_file
  supports_enforce
  supports_health
  kernel_release
  btf_available
  bpffs_available

CollectionIntent
  event_kinds
  file_prefixes
  socket_families
  observe_only

EventEnvelope
  sensor_event
  raw_ref
  received_at

Health
  backend
  running
  installed
  version
  policy_loaded
  events_seen
  events_dropped
  parse_errors
  restart_count
  last_event_at
  last_exit_reason
  last_error

EnforcementCmd / EnforcementAck
  v2 定义接口和 unsupported/observe-only 响应，不实现完整阻断。
```

验收：

- fake sensor 单测覆盖接口。
- Tetragon backend 实现 `Capability`、`Subscribe`、`Health`。
- `Enforce` 返回明确 unsupported 或 observe-only ack。
- 现有 Tetragon parser 继续复用并保持测试覆盖。

### 5.2 Sensor Runtime Manager

新增 `internal/sensor/runtime` 或 `internal/agent/sensormgr`。

职责：

- 管理 Sensor backend lifecycle。
- 执行 capability probe。
- 接收 collection intent。
- compile/apply policy。
- 订阅事件并向 endpoint pipeline 输出统一 `EventEnvelope`。
- 统计 events seen / dropped / parse errors。
- 采集 health。
- 对 backend failure 执行 restart。
- 将 sensor tamper/blindness 转成 signal 或 incident。

建议接口：

```go
type Runtime interface {
    Probe(ctx context.Context) (Capability, error)
    Apply(ctx context.Context, intent CollectionIntent) error
    Subscribe(ctx context.Context) (<-chan EventEnvelope, error)
    Enforce(ctx context.Context, cmd EnforcementCmd) (EnforcementAck, error)
    Health(ctx context.Context) (Health, error)
    Stop(ctx context.Context) error
}
```

验收：

- runtime 可以用 fake backend 做 lifecycle 测试。
- `Probe -> Apply -> Subscribe -> Stop` 路径可重复执行。
- backend 异常退出会进入 health。
- backend 被杀后 runtime 可以按策略重启。

### 5.3 Policy Compile / Apply

v2 需要把 policy 从“测试脚本里准备好”提升为 agent/runtime 的正式控制链路。

最低实现：

- 定义 `CollectionIntent` 作为 agent 内部策略意图。
- 提供 Tetragon policy template 或静态 policy 文件。
- runtime 将 intent 映射到 backend policy。
- agent 负责安装/更新 policy。
- health 能反映 policy loaded / apply error。

验收：

- policy apply 成功后 health 显示 `policy_loaded=true`。
- policy 文件缺失或格式错误时，agent 启动失败或进入 degraded，并给出明确错误。
- container/VM e2e 使用 agent/runtime apply 的 policy。

### 5.4 Tetragon Managed Backend

Tetragon 是 v2 的第一个托管 backend。

最低实现：

- agent 管理 Tetragon binary/policy 的本地安装。
- agent 启动、停止、重启 Tetragon。
- agent 持续订阅 Tetragon 事件。
- agent 上报 Tetragon health。
- sensor 被杀后 agent 自动拉起。
- sensor 多次失败后上报 degraded health，并产生 tamper/blindness signal 或 incident。
- container/VM e2e 不再由 harness 直接运行 `tetra getevents` 并 pipe 给 agent。

本地 bundle 建议目录：

```text
/opt/sysarmor/sensors/tetragon/<version>/
  bin/tetragon
  bin/tetra
  manifest.json

/opt/sysarmor/sensors/tetragon/current -> <version>
/etc/sysarmor/policies/sysarmor-tetragon.yaml
```

配置建议：

```yaml
sensor:
  backend: tetragon
  mode: managed
  version: "v1.x"
  bundle_dir: /opt/sysarmor/bundles/tetragon
  install_dir: /opt/sysarmor/sensors
  tetra_path: /opt/sysarmor/sensors/tetragon/current/bin/tetra
  tetragon_path: /opt/sysarmor/sensors/tetragon/current/bin/tetragon
  policy_path: /etc/sysarmor/policies/sysarmor-tetragon.yaml
  observe_only: true
  restart: always
  max_restarts: 5
  restart_window: 60s
```

验收：

- Tetragon 未安装时，agent 能从 local bundle 安装。
- 已安装且版本匹配时，安装过程幂等。
- binary 缺失或 checksum 不匹配时，agent 给出明确错误。
- `sysarmor-agent run --config ...` 能独立启动 Tetragon 并订阅事件。
- kill Tetragon 后，agent 自动 restart。
- 连续失败超过阈值后，health degraded，并产生 tamper/blindness signal 或 incident。

### 5.5 Agent Config

新增配置文件加载。

推荐路径：

```text
/etc/sysarmor/agent.yaml
```

最低配置：

```yaml
agent:
  id: node-a
  host_id: node-a
  tenant_id: default
  token: dev-token

manager:
  address: http://10.66.0.10:9443
  transport: http

sensor:
  backend: tetragon
  mode: managed
  bundle_dir: /opt/sysarmor/bundles/tetragon
  install_dir: /opt/sysarmor/sensors
  policy_path: /etc/sysarmor/policies/sysarmor-tetragon.yaml
  observe_only: true
  restart: always

spool:
  path: /var/lib/sysarmor/agent/spool
  max_bytes: 268435456
  batch_size: 256
  flush_interval: 1s

upload:
  retry_initial: 1s
  retry_max: 30s
  request_timeout: 10s

health:
  interval: 10s
```

验收：

- missing config 有明确错误，除非显式启用 dev defaults。
- `sysarmor-agent run --config ... --dry-run` 能校验配置。
- v1 `--input-jsonl` 和 `--stream-jsonl` 仍可运行。

### 5.6 Agent Daemon Lifecycle

新增明确 daemon mode：

```bash
sysarmor-agent run --config /etc/sysarmor/agent.yaml
```

职责：

- 加载配置。
- 执行最小 registration/auth。
- 初始化 sensor runtime。
- 初始化 normalizer、fastpath、ringbuffer。
- 初始化 spool 和 uploader。
- 启动 sensor subscription。
- 持续处理 sensor events。
- 生成 endpoint signals。
- 写入 durable spool。
- 上传 batches。
- 周期性上报 heartbeat/health。
- 处理 `SIGTERM` / `SIGINT` graceful shutdown。

验收：

- daemon 可长期运行。
- `SIGTERM` 后能停止 sensor，并 flush 已进入 spool 的数据。
- systemd 可管理 agent。
- replay/debug CLI 路径不被破坏。

### 5.7 Local Spool, Retry, Backpressure

v2 需要真正的 agent-side 断网恢复上传队列。

建议目录：

```text
/var/lib/sysarmor/agent/spool/
  00000000000000000001.batch.json
  00000000000000000002.batch.json
  cursor.json
```

规则：

- batch 文件原子写入。
- oldest unacked first。
- 上传成功后 advance cursor 并删除已 ack 文件。
- manager 不可用时持续排队，直到达到 `max_bytes`。
- 达到上限时进入 backpressure 或显式 drop，并写入 health counter。
- 不允许静默丢数据。
- upload worker 使用 exponential backoff 和 request timeout。

验收：

- manager down 时 agent 不崩溃。
- manager 恢复后 queued batches 能按顺序 drain。
- agent 重启后能从 unacked batches 恢复。
- health 暴露 queued batches、queued bytes、drop/backpressure 计数、last upload error。

### 5.8 Registration, Auth, Tenant

v2 先实现开发形态，不做生产级 enrollment。

最低实现：

- config 内显式声明 `agent.id`、`host_id`、`tenant_id`、`token`。
- upload 和 health payload 都带 tenant/agent identity。
- manager 校验 token 的最小逻辑可配置启停。
- manager store 按 tenant/agent 维度保存 health 和 incident。

验收：

- 缺少 agent id / tenant / token 时，配置校验失败或进入明确 dev mode。
- manager 查询结果能区分 tenant 和 agent。
- e2e 使用固定 dev token。

边界：

- token 先是 dev shared token 或静态 token，不做远程 enrollment。
- tenant 先作为 identity 维度写入 upload/health/store，不要求完整 RBAC。
- 后续 XDR 多源接入应复用 tenant/entity identity，不在 v2 实现。

### 5.9 Heartbeat And Health Reporting

扩展 proto/API 或增加 manager endpoint，保存每个 agent 最新 health。

最低字段：

```text
AgentHealth
  agent_id
  host_id
  tenant_id
  status
  uptime_seconds
  sensor_health
  queue_health
  upload_health
  observed_at

SensorHealth
  backend
  installed
  running
  version
  policy_loaded
  events_seen
  events_dropped
  parse_errors
  restart_count
  last_event_at
  last_exit_reason
  last_error
```

CLI：

```bash
sysarmorctl --mgr <addr> agents --json
sysarmorctl --mgr <addr> agent-health --agent-id <id> --json
```

验收：

- manager 能查询最新 heartbeat 和 health。
- health 包含 sensor running/degraded、queue depth、last upload error。
- e2e 能断言 health 是 recent。

### 5.10 Sensor Tamper And Blindness Signal

sensor 异常不是普通错误，应该进入安全信号面。

触发条件：

- sensor backend 异常退出。
- Tetragon 被 kill 后需要重启。
- policy 缺失或无法加载。
- 事件流长时间无事件，且 backend 仍宣称 running。
- parse errors 或 dropped events 超过阈值。
- restart 次数超过阈值。

最低 signal：

```text
type: sensor_tamper_or_blindness
severity: high
source: agent_health
agent_id
host_id
tenant_id
sensor_backend
reason
restart_count
last_exit_reason
observed_at
```

验收：

- 单测覆盖 tamper signal 生成。
- e2e kill sensor 后，manager 能看到 health degraded。
- 连续失败后，manager 能看到 incident 或 signal。

实现依赖：

- 需要 5.9 的 health ingest/query 先可用，否则 tamper 只能停留在 agent 本地日志。
- v2 优先产出 `Signal`；是否进一步收敛成 `Incident` 可以作为可选路径，避免在 v2 中重写 analytics。
- signal 应复用现有 Link1/UploadBatch 上行，避免新增一条平行告警通道。

### 5.11 Systemd Packaging

新增：

```text
deployments/systemd/sysarmor-agent.service
configs/agent.example.yaml
```

service 示例：

```ini
[Unit]
Description=SysArmor Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/sysarmor-agent run --config /etc/sysarmor/agent.yaml
Restart=always
RestartSec=5s
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
```

验收：

- VM harness 可选择通过 systemd 启动 agent。
- agent 退出后 systemd 能拉起。
- manager heartbeat missing 可用于发现 agent 整体失联。

## 6. Phased Implementation

阶段原则：

- 先保住 v1 replay/stream 行为，再增加 daemon 主路径。
- 先把 health/identity 的最小数据面打通，再做 tamper/blindness 的安全语义。
- 先做 fake sensor/fake uploader 的可测闭环，再接 Tetragon 进程托管。
- v2 主线只做 EDR endpoint runtime；XDR ingestion 留到 v2 之后。

### Phase 0: Config And Command Shape

任务：

- 增加 config structs 和 YAML loader。
- 增加 `sysarmor-agent run --config ...`。
- 增加 `--dry-run` 配置校验。
- 保留 `--input-jsonl` 和 `--stream-jsonl`。
- 增加 `configs/agent.example.yaml`。

退出标准：

- config 校验可运行。
- v1 replay/stream 测试仍然通过。

### Phase 1: Sensor Contract And Runtime Skeleton

任务：

- 增加 `internal/sensor/contract`。
- 增加 fake sensor backend。
- 增加 Sensor Runtime Manager。
- 定义 capability、intent、health、enforce 类型。

退出标准：

- daemon 能通过 fake sensor 消费事件。
- lifecycle 单测覆盖 probe/apply/subscribe/stop/restart。

### Phase 2: Policy Compile / Apply Chain

任务：

- 定义 `CollectionIntent`。
- 增加 Tetragon policy template/static policy。
- 实现 intent 到 backend policy 的最小映射。
- health 反映 policy apply 状态。

退出标准：

- policy apply 成功/失败都有明确测试。
- container/VM e2e policy 由 agent/runtime 管理。

### Phase 3: Tetragon Managed Backend

任务：

- 实现 local bundle install。
- 实现 binary/checksum/policy 校验。
- agent 启动/停止 Tetragon。
- agent 启动 `tetra getevents` 或等价事件订阅进程。
- 复用现有 Tetragon JSON adapter。
- 记录 stderr、exit code、parse errors。
- 明确 managed mode 和 dev JSONL mode:
  - managed mode: agent 托管 Tetragon 进程。
  - dev JSONL mode: 仅用于本地开发和 v1 回归,不作为 v2 主路径。

退出标准：

- container daemon e2e 不再 pipe `tetra getevents`。
- VM daemon smoke 不再 pipe `tetra getevents`。
- agent kill/restart Tetragon 的行为可由单测或 integration test 稳定覆盖。

### Phase 4: Sensor Health, Restart, Tamper Signal

任务：

- 实现 dropped/parse error 统计。
- 实现 restart policy。
- 实现 restart window/max restarts。
- sensor 异常退出进入 health。
- 多次失败产生 tamper/blindness signal 或 incident。
- 若 manager health API 尚未完成，先将 tamper/blindness 作为 endpoint signal 上行。

退出标准：

- e2e kill sensor 后 agent 能自动拉起。
- 连续失败后 manager 能看到 degraded health。
- 连续失败后 manager 能看到 tamper/blindness signal 或 incident。

### Phase 5: Agent Daemon, Spool, Retry

任务：

- 完成 daemon run loop。
- 收口已存在的 file-backed spool。
- 收口已存在的 upload worker。
- 增加 retry/backoff。
- 将已存在的 queue limit/backpressure/drop accounting 纳入 health payload。
- 将 `upload.request_timeout` 贯穿 HTTP/gRPC uploader。
- graceful shutdown flush 已进入 spool 的数据。

退出标准：

- manager unavailable 时 agent 能持续排队。
- manager 恢复后队列 drain。
- agent restart 后 unacked batches 不丢。
- upload worker 不 busy loop,失败重试有可测试 backoff。
- daemon health 能暴露 queued batches、queued bytes、dropped batches、dropped bytes、last upload error。
- v1 replay/stream debug path 仍然不经过 spool，或有明确开关选择是否经过 spool。

### Phase 6: Registration/Auth, Heartbeat, Health API

任务：

- config 增加 agent/tenant/token。
- upload/health payload 带 identity。
- manager dev auth 支持静态 token 校验,并可在测试中关闭或固定。
- manager 增加 latest health store。
- `sysarmorctl agents` 和 `sysarmorctl agent-health`。

退出标准：

- CLI 能看到 heartbeat、sensor、queue、upload health。
- e2e 能断言 health recent。

### Phase 7: Systemd And Harness Migration

任务：

- 增加 systemd unit。
- 更新 VM harness 可用 systemd 启动 agent。
- 更新 container/VM e2e 主路径使用 agent-managed sensor。
- 保留 replay/stream debug harness。

退出标准：

- container/VM daemon e2e 通过。
- v1 replay/stream e2e 仍然可运行。

## 7. Test Plan

### Unit Tests

- config parsing and validation。
- Sensor contract fake backend。
- Sensor Runtime lifecycle。
- capability probe。
- policy compile/apply success and failure。
- Tetragon bundle install idempotency。
- checksum failure。
- process supervisor restart。
- dropped/parse error accounting。
- tamper/blindness signal generation。
- spool append/reload/ack/delete。
- retry/backoff。
- identity validation。

### Integration Tests

- daemon + fake sensor + fake uploader。
- daemon + fake sensor + failing/recovering uploader。
- sensor runtime + fake process backend restart。
- Tetragon backend install/apply/subscribe smoke。
- manager health ingest/query。
- registration/auth dev-token check。

### E2E Tests

保留 v1 场景：

```bash
cd test
make e2e TOPO=container SCENARIO=apt-fileless-c2 DUR=12
make e2e TOPO=container SCENARIO=apt-staged-drop DUR=12
make e2e TOPO=container SCENARIO=benign-ci-noise DUR=12

make e2e TOPO=vm SCENARIO=apt-fileless-c2 DUR=12
make e2e TOPO=vm SCENARIO=apt-staged-drop DUR=12
make e2e TOPO=vm SCENARIO=benign-ci-noise DUR=12
```

新增 v2 场景：

```bash
make e2e-agent-daemon TOPO=container SCENARIO=apt-fileless-c2 DUR=12
make e2e-agent-daemon TOPO=vm SCENARIO=apt-fileless-c2 DUR=12
make e2e-agent-sensor-restart TOPO=container DUR=20
make e2e-agent-spool TOPO=container SCENARIO=apt-fileless-c2 DUR=12
make e2e-agent-health TOPO=vm DUR=20
```

`e2e-agent-sensor-restart` 应验证：

1. agent 启动并管理 Tetragon。
2. 测试杀掉 Tetragon。
3. agent health 记录 sensor exit。
4. agent 自动重启 Tetragon。
5. manager 收到 degraded/recovered health。
6. 多次失败路径能产生 tamper/blindness signal 或 incident。

`e2e-agent-spool` 应验证：

1. manager 暂停或不可达。
2. agent 继续采集并写入 spool。
3. manager 恢复。
4. queue drain。
5. incident/signal 仍能被查询。

## 8. Acceptance Criteria

v2 完成时必须满足：

- `sysarmor-agent run --config ...` 可以作为 daemon 长期运行。
- Sensor interface 和 Sensor Runtime Manager 已建立。
- runtime 支持 capability、subscribe、health、observe-only enforce skeleton。
- policy compile/apply 有正式链路和测试。
- agent 能从 local bundle 安装/校验 Tetragon binary 和 policy。
- agent 能启动、停止、重启 Tetragon。
- agent 能持续订阅 Tetragon 事件。
- container/VM 主 e2e 不再由 harness pipe `tetra getevents` 给 agent。
- sensor dropped events / parse errors / restart count 进入 health。
- sensor 被 kill 后 agent 能自动拉起。
- sensor 多次失败后 manager 能看到 degraded health。
- sensor 多次失败后 manager 能看到 tamper/blindness signal 或 incident。
- local spool 支持 manager outage 后恢复上传。
- upload retry/backoff 不 busy loop。
- queue limit/backpressure/drop 进入 health。
- agent identity 包含 agent id / host id / tenant / token。
- manager 和 `sysarmorctl` 能查询 heartbeat、agent/sensor/queue/upload health。
- systemd service 和 example config 存在。
- v1 replay/debug 路径仍然保留并可测试。
- v2 没有引入 XDR 多源 ingestion 或 analytics 重写范围膨胀。
- v2 文档能明确说明后续 EDR platform / XDR platform 的衔接点。

## 9. Plan Review Notes

当前计划整体方向正确：它抓住了 v1 最大缺口，也就是 agent/sensor 还不是长期运行 runtime。下面这些点需要在实现时持续守住。

当前计划需要修正的地方主要有四类：

1. **阶段状态要随实现滚动更新**：config、sensor contract skeleton、spool、upload drain、backpressure 已经不是纯待办,后续计划应写成"收口/接入/暴露",不要重复实现。
2. **Tetragon managed backend 是最大风险项**：它涉及安装、checksum、进程监督、policy apply、事件订阅和 e2e harness 迁移,应拆成独立可提交的小步,不要和 health API、systemd 一起塞进一个大提交。
3. **health 是依赖轴,不是附属功能**：tamper、restart、spool backpressure、upload error、agent liveness 都要靠 health 被 manager 看见。Phase 4 和 Phase 6 之间应共享同一套 health model,避免先做一套本地日志再重写。
4. **spool 正确性不只在 agent**：agent 有 durable queue 以后,manager ingest 的幂等/upsert 和 batch ack 语义就是可靠传输的一半。计划需要持续把 batch id、ack、retry、manager idempotency 放在同一个验收面里。

### 9.1 范围控制

v2 应聚焦 EDR endpoint runtime。XDR 的方向要在接口和 identity 上留口，但不要在 v2 直接实现 cloud audit、identity、network flow、CI/CD ingestion。否则主线会从“把 agent 做稳”发散成“同时做平台数据湖”。

### 9.2 Health 与 Tamper 的依赖

tamper/blindness 是安全信号，不只是日志。但它要被 manager 看见，需要 health ingest/query 或 Signal 上行先打通。因此实现顺序应避免先写一套只能本地打印的 tamper 逻辑。

推荐策略：

```text
agent health model -> manager health ingest/query -> sensor restart health
  -> tamper/blindness endpoint signal -> optional incident convergence
```

### 9.3 Spool 与 Link1 Ack 语义

file-backed spool 不是简单写文件。它需要和 Link1 ack 语义对齐：

- batch id 必须稳定。
- manager ingest 必须幂等。
- ack cursor 必须能表达“哪些 batch 已被 durable 接收”。
- agent restart 后不能重复放大 signal/incident。
- store upsert/idempotency 是 spool 正确性的另一半。

v2 可以继续使用 unary HTTP/gRPC upload，但要把 batch id、ack、retry、delete 的语义写清楚。

当前代码已经有 agent-side stable batch id 和 manager store upsert,下一步应补的是：

- upload payload/metadata 显式携带 batch id。
- manager ack 返回 durable accepted 的 batch id 或 cursor。
- agent 只在收到成功 ack 后删除 batch。
- retry 重传不应放大 event/signal/incident。
- health 暴露 queue depth、last upload error、drop/backpressure counters。

### 9.4 Policy Apply 的最小闭环

v2 不需要完整 rule DSL，但需要把 policy 从 harness 移到 agent/runtime。这里最容易过度设计。

建议最小闭环是：

```text
static collection intent -> Tetragon policy template/static file
  -> agent apply/verify -> health.policy_loaded
```

rule content pack、MITRE metadata、cloud rule DSL 都留到后续。

### 9.5 Daemon 与 Debug Path 共存

`sysarmor-agent run --config ...` 应成为 v2 主路径，但 v1 的 `--input-jsonl` / `--stream-jsonl` 仍是回归测试和调试入口。两者共存时要避免 flag 语义混乱：

- daemon mode 使用子命令 `run`。
- replay/stream mode 保持兼容。
- `--dry-run` 只验证 daemon config。
- replay/stream 是否经过 spool 应显式配置，不要隐式改变 v1 结果。

### 9.6 Tetragon 托管不要变成产品边界

Tetragon 是第一个 backend，不是 SysArmor Next 的定位。代码结构应保持：

```text
internal/sensor/contract
internal/sensor/runtime
internal/sensor/tetragon
```

agent pipeline 只依赖 contract/runtime，不直接依赖 Tetragon raw JSON。

### 9.7 建议的近期提交切分

为了避免 v2 变成难以 review 的大块改动,近期可以按下面顺序提交：

1. `upload.request_timeout` 接入 uploader,补单测。
2. daemon 后台 upload retry/backoff loop,保留 `--drain-once` 作为测试/调试入口。
3. agent health model 本地结构化,包含 sensor/queue/upload 三块。
4. manager latest health ingest/query + `sysarmorctl agent-health`。
5. Tetragon local bundle manifest/checksum 校验。
6. Tetragon process supervisor start/stop/restart。
7. sensor restart/degraded/tamper signal。
8. systemd unit + daemon e2e harness 迁移。

这个顺序的好处是每一步都有独立验收,而且 health/identity 会在 sensor restart 和 tamper 之前先变成可查询事实。

## 10. Guardrails

- 不要让 manager 消费 raw Tetragon JSON；raw sensor 适配仍在 agent 侧完成。
- 不要把 Tetragon 当成测试预置依赖；daemon 主路径必须由 agent 管理。
- 不要把 v2 窄化为 Tetragon supervisor；Tetragon 是 Sensor Runtime 的第一个 backend。
- 不要在线下载 sensor；v2 只做 local bundle install。
- 不要把 `Enforce` 扩成完整阻断控制面；v2 保持 observe-only skeleton。
- 不要静默丢 spool 数据；必须通过 health 暴露 backpressure/drop。
- 不要把 v2 扩成 analytics 重写；保持 v1 detection 行为稳定。
- 不要把 v2 扩成 XDR ingestion；多源接入留给 endpoint runtime 稳定之后。
- 不要删除 replay/stream 模式；它们仍然是调试和回归测试的重要路径。
