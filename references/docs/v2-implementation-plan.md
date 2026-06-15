# SysArmor v2 Implementation Plan

Date: 2026-06-15
Status: living plan, updated after daemon/spool/health/restart/tamper, agent-owned runtime policy, container policy-preload removal, VM default policy-preload removal, VM real Tetragon systemd smoke, VM owned-process smoke, and container owned-process smoke commits.

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

对容器场景,这里还要再加一条路线约束: **v2 的默认方向不是把 agent 装进业务容器本体,而是把容器视为一个 workload scope,由独立的 agent+tetragon sensor runtime 去观测它。** 换句话说,容器主线不是“容器里也跑一套 VM 流程”,而是“一个受控 sensor runtime 保护一个 scope”。

为了避免范围漂移,这个成功标准最好再拆成四个可验收问题:

1. **ownership**: agent 是否真正拥有 sensor process / event subscription / runtime policy apply 的生命周期。
2. **reliability**: manager outage、agent restart、sensor restart、graceful shutdown 后,数据和检测结果是否可恢复且不放大。
3. **observability**: manager/ctl 是否能看见 agent、sensor、queue、upload 的 recent/degraded/recovered 状态。
4. **compatibility**: v1 replay/debug 路径和现有 detection 行为是否保持稳定。

## 1.2 Runtime Scope Model

为了让 VM、容器和后续 K8s workload 能落到同一条 EDR/XDR 路线上,中期应把 Sensor Runtime 显式抽象成带 scope 的模型:

```text
Sensor Runtime
  scope:
    type: host | container | cgroup | namespace | pod
    selector: ...
```

在这套模型下:

- VM / 裸机 = `host scope`
- 单个容器 = `container` 或 `cgroup scope`
- K8s workload = `pod` 或 `namespace scope`

这意味着容器方案的推荐部署形态是:

- 运行一个独立的 privileged `sysarmor-agent + tetragon` sensor container
- 由它拥有 sensor process / policy / health / spool / upload 生命周期
- 通过 scope selector 限定只观测目标 workload,selector 可以是 container id、cgroup path、namespace inode、pod identity 或 label selector

而不是把 agent 直接内嵌进业务容器镜像,把容器硬解释成“迷你 VM”。

因此 v2 后续实施要按下面的路线收敛:

1. 配置和 policy 表达以 `sensor.scope.type + sensor.scope.selector` 为主,`scope_type/scope_selector` 只作为扁平兼容字段,`container_id_prefix` 只作为 legacy alias。
2. container harness/e2e 应模拟“sensor container 观测 workload container”,而不是把业务容器改造成 VM。
3. K8s 方向不另起一套 agent 模型,而是在同一 contract 下把 selector 扩展到 pod/namespace。
4. capability、health、spool、policy apply、upload retry 都归属于 sensor runtime 实例,其 scope 是 runtime identity 的一部分。
5. manager/ctl 的健康与调查视角应表达“agent runtime 正在保护哪个 scope”,而不是只表达“agent 运行在哪台机器或哪个容器里”。

v2 配置层可以先保留扁平字段,但语义上要等价于下面的对象:

```yaml
sensor:
  backend: tetragon
  mode: managed
  scope:
    type: container
    selector: "<container-id-or-cgroup-selector>"
```

短期兼容:

```yaml
sensor:
  scope_type: container
  scope_selector: "<container-id-prefix>"
  container_id_prefix: "<legacy-alias>"
```

其中 `container_id_prefix` 只能映射到 `scope_type=container` 的兼容入口,不能再出现在新的验收标准和产品叙述中心。

## 1.1 Current Priority

按当前实现进度,v2 接下来的优先级不应该再平均铺开,而应该集中在下面三件事:

1. **继续收口 ownership**
   - container 默认主路径已经去掉预加载 TracingPolicy。
   - VM provision 默认主路径已不再 preload policy；兼容性 preload 需显式降级为 replay/debug/perf 专用。
   - container/VM 主路径都应继续朝 agent 完整拥有 Tetragon process 和 policy lifecycle 收口。
   - container 路线要继续从 legacy `container_id_prefix` 兼容入口,收口到正式的 runtime scope contract: `sensor.scope.type + sensor.scope.selector`。
   - 测试拓扑要表达“独立 sensor container + workload scope”,不要再把业务容器内安装 agent 作为默认目标。
2. **把 reliability 做成主路径证据**
   - manager outage drain、graceful shutdown flush、retry/backoff、agent restart 恢复都要继续用 e2e 证明,而不是只停留在局部单测或一次性 smoke。
3. **避免把真实订阅误当成完整 ownership**
   - agent-managed `tetra getevents`、agent-owned runtime policy、systemd smoke 都是重要进展。
   - 但它们不自动等于“完整 Tetragon lifecycle ownership 已完成”,后续文档和验收要持续区分这两层成熟度。

这也是当前 plan 最需要强调的地方: v2 的剩余工作已经主要是 **ownership 收口 + 可靠性收口**,而不是再补一轮新的骨架模块。

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

v1 已经具备的是 **EDR detection path MVP**，也就是从端点事实到 manager 侧 signal/incident 的检测闭环：

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

v2 已经落地的内容已经超过“骨架”阶段，当前可分成三类。

### 2.1 已经基本落地的 v2 能力

- `internal/agent/config`:
  - agent/manager/sensor/spool/upload/health 配置结构。
  - `configs/agent.example.yaml` 和 `configs/agent.fake.yaml`。
  - `sysarmor-agent run --config ... --dry-run` 配置校验。
- `internal/sensor/contract`:
  - Sensor interface。
  - Capability / CollectionIntent / EventEnvelope / Health。
  - CollectionIntent 已有 runtime scope 归一和校验入口。
  - observe-only/unsupported Enforce 边界。
- `internal/sensor/runtime`:
  - fake backend lifecycle 测试骨架。
  - Probe / Apply / Subscribe / Stop 的最小路径。
  - `Apply` 已下沉调用 backend apply,不再只是 runtime 内部记录 intent。
- `internal/agent/policy`:
  - 最小 collection policy 解析。
  - policy 到 `CollectionIntent` 的初步转换。
- `internal/sensor/tetragon`:
  - Tetragon backend skeleton。
  - 从 JSONL/stdin 读取事件。
  - policy file 存在性校验。
  - 根据 `CollectionIntent` 生成最小 Tetragon TracingPolicy,并在 managed `tetra` 路径执行 `tetra tracingpolicy add` 后用 `tetra tracingpolicy list` 验证 `sysarmor-runtime-collection` 已加载。
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
  - request timeout 已贯穿 uploader。
  - agent restart 后 unacked batch 恢复的单测已补齐。
- `internal/agent/daemon`:
  - fake sensor daemon 路径。
  - policy apply。
  - event normalize/fastpath。
  - spool write。
  - `--drain-once`。
  - 后台 upload drain/retry loop。
  - health 输出 sensor + queue 状态。
  - health payload 已包含 queue/upload/sensor 的主要状态。
- `internal/agent/health`:
  - agent health model。
  - health payload 已包含 runtime `scope` identity。
  - agent health reporter。
- `internal/transport/link1` / `internal/store` / `cmd/sysarmorctl`:
  - manager 已支持 latest agent health ingest/query。
  - `sysarmorctl agent-health` 已可查询 agent health。
- dev registration/auth:
  - manager 支持静态 dev token 校验。
  - HTTP upload、health report 和 gRPC upload 已进入 token check。
  - agent/uploader 能携带 token。
- Tetragon local bundle:
  - bundle `manifest.json` 校验。
  - `bin/tetragon`、`bin/tetra` sha256 校验。
  - local bundle install 到 `install_dir/tetragon/<version>`。
  - `current` 指针维护和幂等安装。
- Tetragon managed process:
  - basic process supervisor 支持 start/stop/status。
  - Tetragon backend 可在配置了 `TetragonPath` 时启动本地 `tetragon` 进程。
  - Tetragon backend 可在没有 `EventSource` 且配置了 `TetraPath` 时启动 `tetra getevents -o json` 并解析 stdout。
  - sensor health 已能合并 managed process 的 restart/exit/error 状态。

### 2.2 v2 仍未完成的关键缺口

- Sensor/runtime:
  - capability 探测已从最小骨架推进到 kernel release、BTF、bpffs、配置二进制可执行性检查；缺失 BTF 的 degraded health smoke 已补。后续仍需在真实 VM/container 主路径上继续验证权限矩阵。
  - policy compile/apply 已有 backend apply、generated TracingPolicy 最小路径和 backend 内部 `tracingpolicy list` 验证；VM real Tetragon systemd smoke 和 container/VM 通用 capture 主路径已验证 agent-owned runtime policy。container topology 和 VM provision 默认主路径都已不再预加载 TracingPolicy；VM 仅保留显式兼容模式 preload,限定在 replay/debug/perf。
  - dropped events / parse errors / degraded 状态已有阈值配置、health 暴露和本机 e2e 验收；后续重点转向真实 Tetragon 主路径中的 dropped counter 对齐。
  - process supervisor restart policy 已落地；container 三个核心场景已有真实 `tetra getevents` agent-managed detection smoke，已通过 `e2e-agent-detection-container-all` 聚合验证；VM 真实 Tetragon systemd detection smoke 已补齐。
  - sensor kill/restart 和带 runtime scope entity 的 tamper/blindness signal 已有本机 smoke；container 已有 managed fake Tetragon restart/tamper smoke；container/VM 已有 managed fake Tetragon bundle smoke。
  - container 已有 `apt-fileless-c2`、`apt-staged-drop`、`benign-ci-noise` agent-managed detection smoke 和 `e2e-agent-detection-container-all` 聚合入口；真实订阅已通过正式 `sensor.scope.type=container` + `sensor.scope.selector=<container id prefix>` 收紧到 node-a workload scope,并完成聚合验证。container/VM `make e2e TOPO=...` 主路径已默认走 agent-managed sensor。`scope_type/scope_selector` 仅作为扁平兼容入口,`container_id_prefix` 仅作为 legacy config alias 保留。
  - `Enforce` 仍应保持 observe-only/unsupported skeleton。
  - native sensor 不在 v2 完整实现范围内。

- Agent daemon:
  - 后台 upload loop、spool recovery、request timeout、ack batch_id 校验、manager outage 多 batch drain soak、retry/backoff 多 batch 503 soak、agent restart 后 unacked batch 恢复 e2e、manager ingest 幂等计数和重复 batch 不放大 e2e 已有；后续重点转向真实主路径可靠性验收。
  - graceful shutdown flush 已有本机 e2e,并断言 SIGTERM 后 shutdown drain、spool 清空、manager 侧 final degraded health 中 `queued_batches=0` / `remaining_batches=0`；后续仍需更长窗口 soak。
  - systemd VM fake-sensor lifecycle smoke、真实 Tetragon systemd detection smoke、VM agent-owned real Tetragon process smoke 以及 container agent-owned real Tetragon process smoke 已有,但 container/VM 主路径的完整 Tetragon process ownership 仍需继续收口。
- health API、CLI 查询、本机 e2e、`e2e-agent-all` 本机聚合、container/VM managed degraded→recovered smoke、container/VM real owned Tetragon 主路径 degraded/recovered health 断言、parse/drop 阈值 degraded smoke 以及 required BTF 缺失 degraded smoke 已落地；后续重点转向更长窗口的 reliability soak 与真实权限矩阵验收。
  - tenant/agent identity 已进入 upload、health 和 store 主链路；manager 已对 upload agent/host/tenant identity 做最小校验,但还不是完整 RBAC/enrollment。

当前最值得优先收口的,已经不是“再搭新骨架”,而是两件事:

- **主路径 ownership 收口**: 把 container/VM 主路径中残留的 topology/provision 预置 Tetragon process/policy 继续迁出或严格限定在 replay/debug/perf。
- **长期运行语义收口**: 用更明确的测试证据覆盖 manager outage drain、graceful shutdown flush、retry/backoff soak、degraded/recovered health。

对于容器,还要同步守住一条架构边界:

- 默认部署形态应是独立 sensor container 观测 workload。
- `scope` 才是容器与 VM 的统一抽象,而不是“容器拓扑特殊分支”。
- 后续 K8s 方向应在这套 `host | container | cgroup | namespace | pod` scope 模型上自然外延。

如果再说得更直接一点,当前 plan 最容易让人误读的地方有两个:

- **“managed” 不是一个单层状态**:
  - 第一层是 agent 托管订阅、上传、health、runtime policy。
  - 第二层才是 agent 完整拥有 Tetragon 主进程和 policy lifecycle。
- **“已有 smoke” 也不等于“已经完成”**:
  - smoke 证明路径能通。
  - 完成标准则要求 container/VM 主路径在失败、恢复、重启、断连场景下仍然稳定。

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

### 2.3 当前计划需要避免的误读

- v2 不是“重新实现 v1 检测”，而是把 v1 检测链路放进可长期运行的 endpoint runtime。
- v2 不是“只做 Tetragon supervisor”，Tetragon 是第一个 backend,抽象边界仍是 Sensor Runtime。
- v2 也不是 XDR 阶段。XDR 要求多源 ingestion 和跨域实体图,但这些应该等 endpoint runtime 稳定后再进入主线。
- v2 的容器路线不是“把 agent 塞进业务容器里”，而是“独立 sensor runtime 观测 workload scope”。
- 当前文档里的 `agent-managed` 已经覆盖两种成熟度:
  - 已落地: agent 托管 `tetra getevents` 订阅、daemon/spool/upload/health/systemd 路径。
  - 待收口: agent 完整拥有 Tetragon 主进程、TracingPolicy apply/verify 和失败恢复。
  后续验收必须明确自己验证的是哪一层,避免把真实订阅 smoke 误判为完整 sensor lifecycle ownership。

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
  scope
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
- `CollectionIntent` 携带 runtime scope,并能把 `host` / `container` / `cgroup` / `namespace` / `pod` 的合法性校验清楚。
- `host` scope 不要求 selector,也不应用 container 过滤；非 host scope 必须有 selector。
- runtime contract 层必须对 scope 做同样校验,不能只依赖 config 层拦截。
- legacy `container_id_prefix` 只能映射到 `scope.type=container`,不能成为新的 backend contract。

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
- health、policy_loaded、events_seen、events_dropped、restart_count 都应能归属到当前 runtime scope。

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
  scope:
    type: host
    selector: ""
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
- `sensor.scope` 是新配置的主表达;旧的 `scope_type/scope_selector` 可作为扁平兼容字段,`container_id_prefix` 只作为 legacy alias。
- 非 host scope 缺失 selector 时 dry-run 失败;host scope 携带 container-only alias 时给出明确错误或兼容告警。

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
  scope
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
scope_type
scope_selector
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

状态：基本完成，后续只做兼容性维护和配置字段补齐。

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

状态：基本完成。restart/tamper 已有本机 smoke,后续重点是更真实的 Tetragon integration 和 VM/container 主路径验收。

任务：

- 增加 `internal/sensor/contract`。
- 增加 fake sensor backend。
- 增加 Sensor Runtime Manager。
- 定义 capability、intent、health、enforce 类型。

退出标准：

- daemon 能通过 fake sensor 消费事件。
- lifecycle 单测覆盖 probe/apply/subscribe/stop/restart。

### Phase 2: Policy Compile / Apply Chain

状态：大部分完成。已有 `CollectionIntent`、runtime backend apply 调用和 Tetragon generated TracingPolicy apply 最小路径；VM real Tetragon systemd smoke 和 container/VM 通用 capture 主路径已不再依赖 harness/provision 预加载 TracingPolicy,并断言 agent-owned `sysarmor-runtime-collection` 已应用；container topology 与 VM provision 默认启动路径都已去掉预加载 TracingPolicy，VM 仅保留显式兼容模式用于 replay/debug/perf。

任务：

- 定义 `CollectionIntent`。
- 增加 Tetragon policy template/static policy。
- 实现 intent 到 backend policy 的最小映射。
- health 反映 policy apply 状态。

退出标准：

- policy apply 成功/失败都有明确测试。
- container/VM e2e policy 由 agent/runtime 管理。
- VM 默认主路径不再依赖 provision 预加载 policy；兼容 preload 必须显式开关控制并明确标注为 replay/debug/perf 专用。

### Phase 3: Tetragon Managed Backend

状态：进行中。bundle verify/install、capability probe、process supervisor restart、managed Tetragon/tetra stdout subscribe、restart health、tamper signal、generated TracingPolicy apply、本机 restart smoke、container managed fake restart/tamper smoke、container/VM managed fake bundle smoke、required BTF 缺失 degraded smoke、container 三个核心场景的真实 `tetra getevents` agent-managed detection smoke、正式 `sensor.scope` 配置下的聚合验证、container/VM 主 capture/assert 路径迁移、VM 真实 Tetragon systemd detection smoke、VM agent-owned real Tetragon process smoke 以及 container agent-owned real Tetragon process smoke 已落地；container/VM 主路径级别的完整 Tetragon process ownership 和更长时间可靠性仍未完成。

注意：已通过的真实 Tetragon detection smoke 证明的是 agent 以 daemon/systemd 形态订阅真实 `tetra getevents`、应用 agent-owned generated TracingPolicy 并完成检测上传；新增 VM/container owned-process smoke 进一步证明 agent 可拥有真实 `tetragon` 进程并跑通检测。Phase 3 完成标准仍然是 agent/runtime 能在 container/VM 主路径上独立安装/校验、启动/停止、apply/verify policy 并恢复 Tetragon backend。

任务：

- 实现 local bundle install。
- 实现 binary/checksum/policy 校验。
- agent 启动/停止 Tetragon。
- agent 启动 `tetra getevents` 或等价事件订阅进程。
- 复用现有 Tetragon JSON adapter。
- 记录 stderr、exit code、parse errors。
- 保持 process supervisor restart policy:
  - restart delay。
  - max restart count。
  - stop cancellation。
  - duplicate start/restart rejection。
- 明确 managed mode 和 dev JSONL mode:
  - managed mode: agent 托管 Tetragon 进程。
  - dev JSONL mode: 仅用于本地开发和 v1 回归,不作为 v2 主路径。

退出标准：

- container/VM daemon 主路径不再由 harness pipe `tetra getevents` 给 agent。
- VM real Tetragon smoke 能证明 systemd agent + real `tetra getevents` 订阅 + detection + agent restart。
- VM owned-process smoke 能证明 systemd agent + real `tetragon` process ownership + real `tetra getevents` + detection + agent restart。
- container owned-process smoke 能证明 agent + real `tetragon` process ownership + real `tetra getevents` + detection + agent restart。
- agent/runtime 拥有 Tetragon process lifecycle,不依赖 topology 预先启动 Tetragon 主进程。
- agent/runtime 拥有 policy apply/verify lifecycle,不依赖 harness 预先加载 TracingPolicy。
- agent kill/restart Tetragon 的行为可由单测或 integration test 稳定覆盖。

### Phase 4: Sensor Health, Restart, Tamper Signal

状态：大部分完成。health ingest/query、sensor process health、restart policy、tamper/blindness endpoint signal、本机 restart smoke、本机 degraded→recovered smoke、container managed degraded→recovered smoke、VM managed degraded→recovered smoke、parse/drop 阈值 degraded smoke 以及 required BTF 缺失 degraded smoke 已落地；剩余重点是 ownership/更真实主路径验收。

任务：

- 实现 dropped/parse error 统计。
- 保持 restart policy。
- 保持 restart window/max restarts。
- sensor 异常退出进入 health。
- 多次失败产生 tamper/blindness signal,incident 收敛保持可选。
- tamper/blindness 优先作为 endpoint `Signal` 通过现有 Link1 上行；是否进一步收敛成 `Incident` 保持可选。

退出标准：

- e2e kill sensor 后 agent 能自动拉起。
- 连续失败后 manager 能看到 degraded health。
- 连续失败后 manager 能看到 tamper/blindness signal 或 incident。

### Phase 5: Agent Daemon, Spool, Retry

状态：大部分已完成。file-backed spool、oldest-first drain、后台 upload loop、request timeout、backpressure/drop health、manager outage 多 batch drain soak、retry/backoff 多 batch 503 soak、agent restart 后 unacked batch 恢复、shutdown final health 验收以及 `e2e-agent-all` 本机聚合已落地；剩余重点是真实主路径可靠性验收。

任务：

- 完成 daemon run loop 的长跑验证。
- 收口 retry/backoff 的配置化和测试：已有本机多 batch 503 soak,覆盖 capped backoff 窗口、失败期间 batch 保留、恢复后多个 batch 全部 drain。
- 验证 manager outage 后恢复 drain：已有本机多 batch soak,覆盖 outage 期间累计 5 个 batch、manager 恢复后全部 drain、health queue/upload 归零。
- 验证 agent restart 后 unacked batches 恢复。
- 保持 queue limit/backpressure/drop accounting 纳入 health payload。
- 保持 `upload.request_timeout` 贯穿 HTTP/gRPC uploader。
- graceful shutdown flush 已进入 spool 的数据。

退出标准：

- manager unavailable 时 agent 能持续排队。
- manager 恢复后队列 drain。
- agent restart 后 unacked batches 不丢。
- upload worker 不 busy loop,失败重试有可测试 backoff。
- daemon health 能暴露 queued batches、queued bytes、dropped batches、dropped bytes、last upload error。
- v1 replay/stream debug path 仍然不经过 spool，或有明确开关选择是否经过 spool。

### Phase 6: Registration/Auth, Heartbeat, Health API

状态：基本完成。dev token 校验、upload identity validation、agent health ingest/query、`sysarmorctl agent-health`、本机 degraded→recovered health smoke、container 主路径 degraded→recovered health smoke 以及 VM 主路径 degraded→recovered health smoke 已落地；剩余重点是 ownership 更真实验收和后续 RBAC/enrollment。

任务：

- 保持 config 中 agent/tenant/token 为 daemon 主路径必需身份。
- 保持 upload/health payload 带 identity。
- manager upload 对 agent_id / host_id / tenant_id 做最小校验,store 按 tenant_id + agent_id 维度保存 agent/health。
- 保持 manager dev auth 支持静态 token 校验,并可在测试中关闭或固定。
- 补齐 health recent/degraded/recovered 的 e2e 断言。
- 确认 `sysarmorctl agents` 和 `sysarmorctl agent-health` 的输出满足测试与排障需要。

退出标准：

- CLI 能看到 heartbeat、sensor、queue、upload health。
- e2e 能断言 health recent。

### Phase 7: Systemd And Harness Migration

状态：部分完成。systemd unit、example config、VM fake-sensor systemd smoke、VM real Tetragon systemd smoke、container/VM managed fake bundle smoke、container `apt-fileless-c2` / `apt-staged-drop` / `benign-ci-noise` agent-managed detection smoke、container detection 聚合验证以及 container/VM 完整 capture/assert 主路径已落地；剩余重点是完整 Tetragon process ownership、policy ownership 和长跑可靠性。

任务：

- 保持 systemd unit。
- 保持 VM harness 可用 systemd 启动 agent。
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
make -C test e2e-agent-daemon
make -C test e2e-agent-health
make -C test e2e-agent-spool
make -C test e2e-agent-sensor-restart
make -C test e2e-agent-all
make -C test e2e-agent-daemon-container
make -C test e2e-agent-managed-container
make -C test e2e-agent-managed-restart-container
make -C test e2e-agent-systemd-vm
make -C test e2e-agent-managed-vm
make -C test e2e-agent-real-tetragon-vm
make -C test e2e-agent-apt-container
make -C test e2e-agent-staged-container
make -C test e2e-agent-benign-container
make -C test e2e-agent-detection-container-all
```

`e2e-agent-detection-container-all` 应验证：

1. 三个 container 核心场景都能通过 agent-managed `tetra getevents` 路径产出事件。
2. `apt-fileless-c2` 能产生 endpoint signal、cloud signal、incident 和 rarity/causal-topk evidence。
3. `apt-staged-drop` 能覆盖 payload drop、exec/connect 跨事件链路和 cloud cross-lineage 收敛。
4. `benign-ci-noise` 默认不产生 terminal signal/incident，并能用 additive_threshold 对照证明裸加分会误报。
5. 聚合 target 是 container managed detection smoke 集合；container/VM `make e2e TOPO=...` 已默认复用 managed daemon 主路径。

`e2e-agent-sensor-restart` 应验证：

1. agent 启动并管理 fake Tetragon/tetra bundle。
2. 测试杀掉 fake sensor 进程。
3. agent health 记录 sensor exit/restart。
4. agent 自动重启 sensor。
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
- runtime scope contract 已成为主路径: `scope.type` 支持 `host | container | cgroup | namespace | pod`,非 host scope 必须有 selector,host scope 不应带 selector。
- agent/manager/ctl 文档和验收都以 runtime scope 描述保护对象;`container_id_prefix` 不再作为主线 contract。
- agent 能从 local bundle 安装/校验 Tetragon binary 和 policy。
- agent 能启动、停止、重启 Tetragon。
- agent 能持续订阅 Tetragon 事件。
- container/VM 主 e2e 不再由 harness pipe `tetra getevents` 给 agent。
- container 主 e2e 体现独立 sensor container 观测 workload scope;业务容器内安装 agent 只作为兼容/调试模式。
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

建议把这些完成标准再按四类验收理解,避免“测试很多但闭环不清楚”:

- `ownership`: agent 拥有 sensor process / subscription / policy lifecycle。
- `reliability`: outage、restart、shutdown 后可恢复且不放大。
- `observability`: manager/ctl 能看到 recent/degraded/recovered、queue、upload、sensor 状态。
- `compatibility`: v1 replay/debug 和现有 detection 行为保持稳定。
- `scope`: VM、容器、K8s workload 都落到统一 Sensor Runtime scope contract;容器路线默认是独立 sensor runtime 观测 workload。

## 9. Plan Review Notes

当前计划整体方向正确：它抓住了 v1 最大缺口，也就是 agent/sensor 还不是长期运行 runtime；也守住了长期目标，即 v2 做 EDR endpoint runtime,不提前扩成 XDR ingestion。真正需要改进的是计划表达和实现排序。

当前计划需要修正或持续注意的地方主要有这些：

1. **阶段状态必须随实现滚动更新**：config、sensor contract、spool、upload drain、backpressure、health API、dev token、bundle verify/install、restart/tamper 和 container managed detection 聚合已经不是纯待办,后续计划应写成"收口/验证/迁移",不要重复实现。
2. **Tetragon managed backend 仍是最大风险项**：基础安装、checksum、进程监督、事件订阅已经有了,但真实 Tetragon 权限、policy ownership、container/VM harness 迁移仍应继续拆成独立可提交的小步。
3. **当前最关键的未闭环已经变成 ownership 和长跑可靠性**：本机和 container 专项 smoke、container/VM `make e2e TOPO=...` 主路径、VM real Tetragon systemd smoke 证明了 daemon、spool、restart、tamper、detection 的局部闭环,下一步要证明 agent 能完整拥有 Tetragon process/policy lifecycle,并在更长失败/恢复窗口内保持可靠。
4. **health 是依赖轴,不是附属功能**：tamper、restart、spool backpressure、upload error、agent liveness 都要靠 health 被 manager 看见。health API 已经落地,后续不要再新增本地-only 的并行状态面。
5. **spool 正确性不只在 agent**：agent 有 durable queue 以后,manager ingest 的幂等/upsert 和 batch ack 语义就是可靠传输的一半。计划需要持续把 batch id、ack、retry、manager idempotency 放在同一个验收面里。
6. **e2e 主路径迁移已经覆盖 container/VM capture/assert 和 VM real Tetragon systemd smoke**：下一步应把 VM real Tetragon smoke 从“订阅真实 tetra”继续推进到“agent 拥有 Tetragon process/policy lifecycle”。
7. **当前 plan 容易混淆两种“managed”**：真实 container detection smoke 已由 agent 托管 `tetra getevents` 订阅，但 Tetragon 主进程和 TracingPolicy 仍由拓扑/harness 提前准备；完整 v2 主路径必须把 policy apply 和 sensor lifecycle ownership 继续收口到 agent/runtime。
8. **VM real Tetragon systemd smoke 是关键增量,但不是终点**：它把真实 `tetra getevents` 订阅、systemd agent restart、检测上传放进同一条 VM 链路；下一步不要再重复做类似 smoke,而应直接推进 process ownership、policy ownership 和长跑恢复。
9. **成功标准还应更操作化**：当前文档已经有大量“已有/缺口”描述,但真正决定 v2 是否完成的应是 ownership、reliability、observability、compatibility 这四类验收,每个阶段最好显式挂靠到这四类之一,避免做了很多 smoke 却仍然不知道哪里没闭环。
10. **容器路线要避免回到“容器=迷你 VM”**：默认架构应是独立 privileged sensor container 观测 workload scope。业务容器内安装 agent 可以保留为特殊环境兼容,但不能成为计划主线,否则后续 K8s/pod/namespace scope 会继续长成拓扑特判。
11. **scope 要成为运行时 identity 的一部分**：后续新增 health、policy、spool、upload、tamper、e2e 断言时,都应能回答“这个状态属于哪个 runtime scope”。否则 container/K8s 场景会很快退化成按拓扑猜测。

### 9.1 范围控制

v2 应聚焦 EDR endpoint runtime。XDR 的方向要在接口和 identity 上留口，但不要在 v2 直接实现 cloud audit、identity、network flow、CI/CD ingestion。否则主线会从“把 agent 做稳”发散成“同时做平台数据湖”。

### 9.2 Health 与 Tamper 的依赖

tamper/blindness 是安全信号，不只是日志。但它要被 manager 看见，需要 health ingest/query 或 Signal 上行先打通。因此实现顺序应避免先写一套只能本地打印的 tamper 逻辑。

推荐策略：

```text
agent health model -> manager health ingest/query -> sensor restart health
  -> tamper/blindness endpoint signal -> container/VM/systemd smoke
  -> optional incident convergence
```

### 9.3 Spool 与 Link1 Ack 语义

file-backed spool 不是简单写文件。它需要和 Link1 ack 语义对齐：

- batch id 必须稳定。
- manager ingest 必须幂等。
- ack cursor 必须能表达“哪些 batch 已被 durable 接收”。
- agent restart 后不能重复放大 signal/incident。
- store upsert/idempotency 是 spool 正确性的另一半。

v2 可以继续使用 unary HTTP/gRPC upload，但要把 batch id、ack、retry、delete 的语义写清楚。

当前代码已经有 agent-side stable batch id、upload payload 显式 `batch_id`、HTTP/gRPC `UploadAck.batch_id`、manager store upsert,以及 worker 侧 ack id 校验:

- spool append 生成稳定 batch id,并写入 `UploadBatch.batch_id`。
- manager ack 返回 durable accepted 的 `batch_id`。
- agent 只在收到成功 ack 后删除 batch；如果 ack batch id 与本地 entry 不一致,保留 batch 并进入 upload error。
- retry 重传依赖 store upsert/idempotency 避免放大 event/signal/incident。
- health 已暴露 queue depth、last upload error、drop/backpressure counters。

后续如果 Link1 从 unary 演进到 streaming,应保留同一个 batch/cursor 语义,不要退回“HTTP 200 即删除本地文件”的隐式确认。

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
- daemon config 可带可选 `agent.scenario`,仅用于测试/开发环境复用 manager scenario reset/query 和 e2e 断言；生产语义仍以 tenant/agent/host identity 为主。
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

为了避免 v2 变成难以 review 的大块改动,近期可以按下面顺序提交。前面的 health/token/bundle/spool/restart/tamper 基础已经基本完成,后续重点应放在 harness 主路径迁移、systemd 和长跑可靠性上：

1. 固化 container managed detection 聚合:
   - 保持正式 `sensor.scope.type=container` + `sensor.scope.selector` 过滤，避免 host 噪音淹没真实 container 场景事件；`scope_type/scope_selector` 只作为兼容入口测试。
   - 新增配置和测试时优先使用 `sensor.scope.type/sensor.scope.selector`,扁平字段只作为兼容入口。
   - 保留 `make -C test e2e-agent-detection-container-all` 作为三场景 smoke。
   - 保持 `make e2e TOPO=container ...` 作为正式 capture/assert 主路径，两者互为补充。
2. 迁移 container capture/assert 主路径:
   - `make e2e TOPO=container SCENARIO=...` 已默认走 agent-managed sensor。
   - 保留 replay/stream debug path,避免丢掉 v1 回归入口。
3. 评估真实 Tetragon 权限/policy apply:
   - 复用已落地的 container/VM managed fake bundle harness。
   - VM systemd 已有真实 `tetra getevents` detection smoke,后续逐步替换 fake bundle 为真实 Tetragon process ownership。
   - 明确 BTF/bpffs/capability 缺失时的 degraded health。
4. 收口 policy apply 主路径:
   - static collection intent。
   - Tetragon policy template/static file。
   - health.policy_loaded / apply error。
   - container/VM managed capture/assert 主路径不再依赖 harness 预先加载 policy。
5. 补长期运行可靠性:
   - graceful shutdown flush 已补 final health 验收,覆盖 SIGTERM 后 shutdown drain、spool 清空和 manager health 队列状态归零。
   - retry/backoff soak 已补本机多 batch 503 验收,覆盖 capped backoff 窗口、失败 batch 重试、恢复后全部 drain。
   - agent restart 后 unacked batch 恢复已补本机 e2e,覆盖 mismatched ack 不删 batch、agent restart 后以相同 batch_id 重传并在 valid ack 后 drain。
   - manager outage 后 queued batches drain 已补本机多 batch soak,覆盖 outage 期间排队、恢复后全部 drain、manager metrics/events 与 health 归零。
   - manager ingest 幂等不放大 event/signal/incident 已补本机 e2e,覆盖重复 Link1 batch 的 ack accepted counts、metrics、events、signals、incidents 不放大。
6. 迁移 VM 主路径:
   - VM 三个核心场景已默认走 agent-managed capture/assert 主路径。
   - replay/stream debug path 继续保留。
   - 老 v1 场景继续作为回归对照。

这个顺序让已经落地的 health/identity/spool/restart/tamper 直接服务下一步验收,也避免一上来就把真实 Tetragon、systemd、VM 网络和场景脚本耦在一起。

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
