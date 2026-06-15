# SysArmor MVP 状态与缺口

> 本文不再重复完整产品设计细节,而是记录当前 MVP 已经实现的框架,以及从 MVP 走向 EDR/XDR 平台原型的主要缺口。第一性原理和长期架构背景见 `design-essentials.md`;测试场景见 `design-test-cases.md`;v2 计划见 `v2-implementation-plan.md`。

## 零、SysArmor Next 的定位

SysArmor Next 的长期定位是 **EDR/XDR 平台原型**,不是 Tetragon 日志转发器,也不是只服务几个测试场景的检测 demo。

当前路线可以这样理解:

```text
v1: EDR detection path MVP
  Tetragon/replay event -> agent normalize/fastpath -> manager analytics -> incident

v2: EDR endpoint runtime MVP
  agent daemon + sensor runtime + policy apply + health + spool + retry

中期: EDR platform
  长期驻留 agent、端侧检测/响应、策略控制、取证回拉、incident lifecycle

长期: XDR platform
  endpoint + workload + cloud audit + identity + network + CI/CD 等多源遥测
  跨域实体图、攻击链收敛、风险裁决、响应编排
```

因此,本文里的 MVP 不是最终产品形态,而是对长期路线的第一段验证:

- v1 验证 **端点事实 -> Signal -> Incident** 这条检测链路成立。
- v2 要补齐 **agent daemon + sensor runtime** 这块 EDR 端点地基。
- 后续 EDR/XDR 工作会继续补齐 policy/control plane、reliable transport、durable store、graph analytics、rule content、incident lifecycle、response plane 和多源 ingestion。

Tetragon 在当前阶段是最现实的 Linux sensor backend,但不是 SysArmor Next 的产品边界。项目真正要守住的边界是:agent 侧统一采集/归一/打标,manager/cloud 侧统一建图/收敛/裁决,控制面统一策略/响应/调查。

## 一、当前已经实现的 MVP 框架

当前仓库已经实现了一条端到端 SysArmor v1 MVP 链路。它对应的是上面路线里的 **EDR detection path MVP**:

```text
Tetragon JSONL / replay SensorEvent JSONL
  -> sysarmor-agent
  -> normalize + endpoint fastpath
  -> Link1 upload
  -> sysarmor-manager
  -> MVP analytics + store
  -> sysarmorctl JSON query
  -> test harness assert/report
```

### 1.1 代码与组件

已实现三个 Go binary:

| Binary | 当前职责 |
|---|---|
| `sysarmor-agent` | 读取 replay JSONL 或 Tetragon JSONL,归一化事件,运行 endpoint fastpath,批量上传 manager |
| `sysarmor-manager` | 提供 HTTP/gRPC Link1 ingest,执行 MVP analytics,持久化 store,暴露查询与 recompute API |
| `sysarmorctl` | 查询 events / signals / incidents / metrics / recompute,作为测试和调查的 JSON 边界 |

核心目录:

| 目录 | 当前内容 |
|---|---|
| `api/proto` | sensor/event/signal/analytics/incident/policy proto,Go 代码由 `protoc` 生成 |
| `internal/sensor/tetragon` | Tetragon JSONL adapter |
| `internal/endpoint` | context、normalize、fastpath、ringbuffer、uploader |
| `internal/analytics` | entity normalization、evidence assembly、MVP ingest/converge |
| `internal/transport/link1` | HTTP ingest 与 generated gRPC service |
| `internal/store` | file-backed MVP store,支持 events/signals/incidents/metrics 与幂等 upsert |
| `test` | container / VM 双拓扑 e2e harness |

### 1.2 数据契约

MVP 已经冻结并使用这些 proto 契约:

| 契约类型 | 用途 |
|---|---|
| `SensorEvent` | sensor 中立原始观测,用于 replay 和 Tetragon adapter 输出 |
| `CanonicalEvent` | agent 归一后的事实,包含 `stable_id`、`lineage_id`、`raw_ref` |
| `Signal` | endpoint/cloud 派生事实,包含 `entities`、`event_refs`、`terminal`、`evidence` |
| `UploadBatch` / `UploadAck` | Link1 上行批次 |
| `Incident` | manager 收敛后的攻击故事,包含 evidence subgraph 与 `ConvergeTrace` |
| `DetectionPolicy` | 当前用于 recompute control 的最小策略面 |

跨组件数据都走 `api/proto`,没有让 manager 直接消费 raw Tetragon JSON。raw JSON 只在 agent adapter 中转成 contract 对象。

### 1.3 Endpoint 能力

agent 当前支持:

- 从 `.sensor.jsonl` 读取 replay `SensorEvent`。
- 从 `tetra getevents -o json` 读取 Tetragon raw JSONL。
- 归一化为 `CanonicalEvent`:
  - `stable_id = hash(host_id, pid/start_time 或 sensor exec_id)`
  - `lineage_id` 继承父进程谱系
  - `seq`、`agent_id`、`host_id`、`raw_ref`
- 使用 ringbuffer 保存 raw refs。
- 使用 HTTP 或 gRPC Link1 上传 batch。
- 支持 streaming batch:
  - `--stream-jsonl`
  - `--batch-size`
  - `--flush-interval`

endpoint fastpath 目前是 Go 内编译规则,已覆盖 MVP 场景需要的 signals:

```text
web_runtime_spawns_shell
download_by_lolbin
payload_dropped
reverse_shell_pattern
sensitive_cred_read
suspicious_exec_connect
```

其中 `reverse_shell_pattern` 可作为 terminal signal,并携带最小 evidence bundle。

### 1.4 Manager 与 Analytics 能力

manager 当前提供:

```text
/healthz
/api/v1/reset
/api/v1/upload
/api/v1/recompute
/api/v1/agents
/api/v1/events
/api/v1/signals
/api/v1/incidents
/api/v1/metrics
```

MVP analytics 当前实现:

- 接收 endpoint events/signals。
- 按 scenario 汇总 endpoint signals。
- 产出 cloud signals:
  - `dropped_payload_executed_and_connects`
  - `web_shell_chain`
- 支持 cross-lineage stitching:
  - 通过共享 file/socket/process entities 连接不同 lineage。
- 产出 incident:
  - `ConvergeTrace.method = rarity+causal-topk`
  - evidence subgraph 从 contributing signals/entities 组装。
- 支持 recompute control:
  - `disable=cloud.cross_lineage`
  - `mode=additive_threshold`
- streaming 上传后会按 touched scenario 重新收敛,避免 batch 切分导致跨批次链路丢失。
- store 对重复 event/signal/incident 做幂等 upsert,避免断连重传放大结果。

### 1.5 测试与验证

MVP 已接入 container 和 VM 双拓扑:

| 拓扑 | 当前实现 |
|---|---|
| container | `mgr` 容器运行 manager;`tetragon` 容器采集宿主内核事件并运行 agent stream;capture 阶段按 `node-a` Docker id 过滤 Tetragon 事件 |
| VM | `mgr` VM 运行 manager/ctl;`node-a` VM 运行 Tetragon 与 agent stream |

已验证场景:

| 场景 | 预期 | 当前结果 |
|---|---|---|
| `apt-fileless-c2` | endpoint terminal + cloud chain + 1 incident | 通过 |
| `apt-staged-drop` | endpoint 无 terminal,cloud cross-lineage stitch + 1 incident | 通过 |
| `benign-ci-noise` | 正常模式 0 incident,additive control 可误报 | 通过 |
| `lifecycle-smoke` | agent 注册、policy 加载、EXEC 可见 | 通过 |
| `perf-getevents` | 短窗口性能 smoke | 通过 |

最近的 stream 结果摘要:

```text
container apt-fileless-c2-stream   events=46  endpoint=12 cloud=2 incidents=1
container apt-staged-drop-stream   events=22  endpoint=8  cloud=1 incidents=1
container benign-ci-noise-stream   events=76  endpoint=12 cloud=0 incidents=0

vm apt-fileless-c2-stream          events=43  endpoint=9  cloud=2 incidents=1
vm apt-staged-drop-stream          events=24  endpoint=7  cloud=1 incidents=1
vm benign-ci-noise-stream          events=55  endpoint=10 cloud=0 incidents=0
```

当前可用验证命令:

```bash
make api
make test
make build

cd test
make e2e TOPO=container SCENARIO=apt-fileless-c2 DUR=12
make e2e TOPO=container SCENARIO=apt-staged-drop DUR=12
make e2e TOPO=container SCENARIO=benign-ci-noise DUR=12

make e2e TOPO=vm SCENARIO=apt-fileless-c2 DUR=12
make e2e TOPO=vm SCENARIO=apt-staged-drop DUR=12
make e2e TOPO=vm SCENARIO=benign-ci-noise DUR=12

make report
```

### 1.6 当前 MVP 的判断

当前项目已经完成"架构同构 MVP":

- 端到端链路是真的。
- proto 契约是真的。
- agent / manager / ctl 边界是真的。
- container 与 VM 都能跑同一套产品路径。
- 三个核心立论场景都能证明:
  - "响"的攻击能检出。
  - 跨 lineage 需要云端缝合。
  - 良性 CI 噪音不应靠裸加阈值误报。

但当前仍是 MVP,不是完整 EDR/XDR 产品。它证明了检测路径和抽象边界,还没有补齐端点长期运行、控制面、可靠传输、持久化、图分析、响应编排和多源遥测接入。

## 二、v2 已经开始补齐的端点运行时

在 v1 MVP 之后,当前仓库已经进入 v2: **EDR endpoint runtime MVP**。这些工作还没有让 v2 完整完成,但已经把 agent 从纯 replay/stream 工具推进到可长期运行、可观测、可恢复的 runtime 雏形:

| v2 增量 | 当前状态 |
|---|---|
| agent config | 已有 `internal/agent/config`,支持示例配置和 `sysarmor-agent run --config ... --dry-run` 校验 |
| daemon run shape | 已有 `sysarmor-agent run --config ...`,可通过 fake sensor 跑 daemon 主路径 |
| sensor contract | 已有 `internal/sensor/contract`,定义 capability/subscribe/enforce/health 等边界 |
| sensor runtime skeleton | 已有 `internal/sensor/runtime`,支持 fake backend 生命周期测试 |
| policy apply path | 已有 `internal/agent/policy` 将最小 collection policy 映射为 `CollectionIntent` |
| Tetragon backend | 已有 `internal/sensor/tetragon`,支持 JSONL/stdin dev source、本地 bundle verify/install、managed Tetragon/tetra 进程订阅和 health 汇总 |
| local spool | 已有 `internal/agent/spool`,支持 file-backed append/list/load/ack/stats 和 stable batch id |
| upload drain | 已有 `internal/agent/uploadworker`,支持 oldest-first drain、成功 ack、失败保留、后台 retry loop 和 request timeout |
| queue backpressure | 已有 spool `max_bytes` 限制、backpressure/drop 统计,daemon health 输出队列状态 |
| agent health | 已有 health reporter、manager health ingest/query、`sysarmorctl agents` / `agent-health` |
| dev auth | 已有静态 dev token 校验,覆盖 HTTP/gRPC upload 与 health report |
| sensor restart/tamper | 已有 process supervisor restart、managed Tetragon restart 配置、degraded/tamper signal 生成和上传测试 |
| systemd | 已有 `deployments/systemd/sysarmor-agent.service` 与 daemon 示例配置 |
| v2 smoke | 已有本机 daemon/health/spool/sensor-restart 聚合 e2e、container fake/managed smoke、VM fake-sensor systemd smoke、container 三个核心场景的 agent-managed detection smoke,以及 container 主 capture/assert managed daemon 路径 |

这说明 v2 已经从"可测试骨架"进入"端点 runtime 收口"阶段。当前剩余重点不是再搭一套基础设施,而是补 VM 真实 Tetragon systemd 验收、收口 policy ownership,并继续压实长期运行语义。

## 三、走向完整项目的主要缺口

从长期定位看,缺口可以分成两类:

- **EDR 底座缺口**: agent daemon、sensor runtime、spool/retry、health、policy apply、registration/auth、systemd、response skeleton。
- **XDR 平台缺口**: 多源 ingestion adapter、跨域实体模型、全局图/罕见度、incident lifecycle、响应编排、SIEM/SOAR/数据湖出口。

v2 应优先补 EDR 底座,让当前检测链路变成能长期运行的 endpoint runtime。XDR 能力应建立在稳定的 EDR 事实模型和图收敛之上,不要在 agent/runtime 尚未稳定前过早扩散。

### 3.1 Sensor Runtime 还未完全成为主路径

当前状态:

- v1 replay/stream 仍可消费 `tetra getevents -o json` 输出。
- v2 已有 Sensor contract、fake backend、runtime skeleton。
- v2 已有 Tetragon backend,支持 JSONL/stdin dev source、本地 bundle verify/install、managed Tetragon/tetra 进程和 health 汇总。
- process supervisor 已支持 restart delay、max restarts、stop cancellation、duplicate start/restart rejection。
- managed Tetragon 已接入 restart 配置和 health 状态。
- sensor tamper/blindness 已能作为 endpoint signal 写入 spool 并上传。
- container 三个核心场景已有 agent-managed `tetra getevents` 专项 smoke,但 Tetragon 主进程和 TracingPolicy 仍主要由测试 topology/harness 启动和加载。

主要缺口:

- capability 探测仍是最小骨架,不是完整主机能力探测。
- dropped events / parse errors / restart window / degraded 状态还需要继续细化阈值和验收。
- CollectionPolicy 到 Tetragon policy 的编译/安装链路仍是最小实现。
- container/VM 主 e2e 已默认迁移到 agent-managed sensor,不再由 harness pipe `tetra getevents` 给 agent。
- Enforce 目前应保持 observe-only/unsupported skeleton,尚不是完整阻断能力。
- 没有 Native Sensor,当前只支持 Tetragon adapter。

目标形态:

```go
type Sensor interface {
    Capability(ctx) (SensorCapability, error)
    Subscribe(ctx, CollectionIntent) (<-chan SensorEvent, error)
    Enforce(ctx, EnforcementCmd) (EnforcementAck, error)
    Health(ctx) (SensorHealth, error)
}
```

### 3.2 Agent daemon 已成型,但还需长期运行验收

当前状态:

- v1 支持 replay、stream、HTTP/gRPC 上传。
- v2 已有 `run --config` daemon 命令形态、配置校验、fake sensor daemon 路径。
- v2 已有 file-backed spool、oldest-first drain、后台 retry loop、队列上限和 backpressure/drop accounting。
- `upload.request_timeout` 已贯穿 uploader。
- manager 已有 agent health ingest/query,CLI 已能查询 latest health。
- static dev token/auth 和 tenant/agent identity 已进入 upload/health 主链路。
- 已有 systemd unit 和 example config。
- 已有本机 daemon、health CLI、spool recovery、sensor restart/tamper e2e smoke。
- 已有 container topology 的 fake/managed daemon smoke、managed restart/tamper smoke、三场景 managed detection smoke。
- 已有 VM fake-sensor systemd lifecycle smoke。

主要缺口:

- graceful shutdown flush 语义还需要明确测试。
- retry/backoff 还需要更长时间 soak 和失败恢复验证。
- container/VM 主检测场景已迁移为 daemon-managed sensor 主路径。
- VM 真实 Tetragon + systemd 主路径仍需补齐。
- tenant/token 仍是开发形态,不是生产 enrollment/RBAC。

继续收口:

```text
graceful shutdown + long-run soak
VM real Tetragon systemd smoke
container/VM agent-managed sensor e2e
retry/idempotency integration
tenant/agent identity consistency
```

### 3.3 Endpoint fastpath 还是硬编码规则

当前状态:

- endpoint rules 写在 Go 代码里。
- 能覆盖 MVP 三个场景。

主要缺口:

- 没有规则 DSL。
- 没有 `configs/rules/endpoint` 内容包。
- 没有 DetectionPolicy 控制 rule enable/disable。
- 没有 rule metadata:
  - rule id
  - version
  - severity
  - MITRE tags
  - response intent
- 没有真正的 CEP rule compiler。
- 没有系统化 dedup/throttle。
- 规则语义仍偏宽:
  - `web_runtime_spawns_shell` 会在 benign/staged 中出现。
  - `reverse_shell_pattern` 在 staged 中会产非 terminal signal,命名偏强。

下一步应把规则从 Go 代码迁到内容包,Go 侧只保留执行引擎。

### 3.4 Analytics 还不是完整 EDR/XDR 图分析系统

当前状态:

- analytics 以 endpoint signals 为输入。
- 用最小规则产 cloud signals。
- 用 scenario-level recompute 产 incident。
- evidence subgraph 从 signal entities 组装。

主要缺口:

- 没有正式 `analytics/graph` 包:
  - node/edge
  - TTL
  - lineage/entity index
  - shortest path
  - k-hop query
- 没有 `analytics/rarity`:
  - CMS
  - IDF
  - workload baseline
  - per-tenant/per-host baseline
- 没有 `analytics/rules`:
  - cloud graph rules
  - graph pattern matching
- 没有 `analytics/correlate`:
  - signal dedup
  - related-family folding
  - repeated terminal suppression
- 没有独立 `analytics/converge` 接口。
- 没有 incident lifecycle:
  - create
  - update
  - close
  - merge
  - suppress
- evidence 还不是真正从图中裁剪路径。
- 没有 XDR 数据域的实体模型和 adapter:
  - cloud principal / IAM role
  - K8s object / workload
  - network flow
  - identity event
  - CI job / artifact / registry image
  - SaaS / cloud audit event

完整结构建议:

```text
analytics/ingest
analytics/entity
analytics/graph
analytics/rarity
analytics/rules
analytics/correlate
analytics/converge
analytics/incident
analytics/evidence
```

### 3.5 Policy / Control Plane 基本还未成型

当前状态:

- proto 中有 policy。
- manager 有 recompute control,用于测试:
  - disable cross-lineage
  - additive threshold mode

主要缺口:

- 没有 policy loader。
- 没有 policy validation。
- 没有 PolicyEnvelope。
- 没有 endpoint/cloud rule references。
- 没有 agent policy assignment。
- 没有 policy rollout/versioning。
- 没有 observe/enforce mode 管控。
- 没有 registry/auth/token。
- 没有 manager 到 agent 的正式下行策略链路。

下一步最小闭环:

```text
static policy file
manager policy endpoint
agent fetch policy
endpoint/cloud rule enable-disable
observe-only response mode
```

### 3.6 Store 仍是 MVP 文件存储

当前状态:

- file-backed JSON store。
- 支持 agents/events/signals/incidents/metrics。
- 支持 scenario reset 和幂等 upsert。

主要缺口:

- 没有 SQLite/Postgres schema。
- 没有 migration。
- 没有索引。
- 没有分页。
- 没有 TTL。
- 没有 incident timeline。
- 没有 raw evidence blob 分层存储。
- 没有并发写入压力验证。
- 没有审计日志。

下一阶段建议先切 SQLite:

```text
events table
signals table
incidents table
evidence table
agents table
metrics table
migrations
indexes
pagination
```

### 3.7 Link1 可靠传输还很薄

当前状态:

- HTTP upload 可用。
- unary gRPC upload 可用。
- batch ack 很简单。
- agent 侧已有 file-backed spool 和 drain worker,但协议层 ack/resume 还没有完整化。

主要缺口:

- 没有双向 stream。
- 没有 agent session。
- 没有 ack cursor。
- 没有 resend/resume protocol。
- 没有 compression。
- 没有 TLS/mTLS。
- 没有 authn/authz。
- 没有 server-side backpressure。
- 没有 schema version negotiation。
- 没有 raw evidence pullback。

完整 Link1 应该支持:

```text
agent session
stream upload
ack cursor
resume from cursor
policy downlink
raw evidence pullback
health heartbeat
```

### 3.8 CLI 还只是测试查询边界

当前状态:

- `sysarmorctl` 可输出 JSON。
- 满足 harness 查询和断言。

主要缺口:

- 没有人类可读 incident detail。
- 没有 evidence path 展示。
- 没有 agent status/detail。
- 没有 policy list/apply。
- 没有 auth config。
- 没有输出 schema version。
- 没有分页/过滤的完整体验。

完整项目中 `sysarmorctl` 应是调查入口,不只是测试工具。

### 3.9 测试还需要从场景通过扩展到长期稳定

当前状态:

- container/VM e2e 场景已经通过。
- Go unit tests 覆盖核心 MVP 包。
- stream smoke 能证明真实 Tetragon -> agent -> manager 通路。
- v2 已有 config、sensor runtime、spool、uploadworker、daemon fake path、process supervisor、tamper signal 的测试。
- v2 已有本机 daemon/health/spool/sensor-restart 聚合 smoke。
- v2 已有 container topology fake/managed daemon smoke、managed restart/tamper smoke 和三场景 managed detection smoke。
- v2 已有 VM fake-sensor systemd smoke 和 VM managed fake bundle smoke。

主要缺口:

- agent-managed Tetragon process VM systemd 主路径 e2e。
- VM 真实 Tetragon systemd smoke。
- 更长时间的 manager outage/spool recovery soak。
- graph path tests。
- rarity baseline tests。
- converge edge-case tests。
- retry/idempotency integration tests。
- high-volume soak。
- dropped-event tests。
- multi-agent tests。
- multi-scenario isolation tests。
- gRPC transport e2e。
- security/auth tests。

当前测试证明 MVP 立论,但还不能证明长期运行稳定性。

### 3.10 部署与运维还未产品化

主要缺口:

- 已有 systemd unit 和 VM fake-sensor systemd smoke,但还缺 VM 真实 Tetragon systemd smoke 和安装脚本。
- 没有 Helm/DaemonSet。
- 没有 packaging/release。
- 已有 config examples,但还缺生产默认值、升级兼容和安全配置说明。
- 没有日志规范。
- 没有 Prometheus 格式 metrics。
- 没有 tracing。
- 没有 upgrade path。
- 没有版本注入。
- 没有安全默认配置。

### 3.11 推荐的下一阶段顺序

建议按下面顺序推进,先把 v2 的 EDR endpoint runtime 地基夯实,再扩到更完整的 EDR/XDR 平台能力,避免过早投入复杂算法或多源接入:

1. **迁移 agent-managed sensor 主路径**
   - container daemon e2e 已从 fake backend 推进到 managed Tetragon/tetra,并成为默认 capture/assert 主路径
   - VM daemon capture/assert 已不再依赖 harness pipe
   - 保留 replay/stream debug path

2. **补 VM 真实 Tetragon/systemd smoke**
   - 复用已落地的 VM systemd agent lifecycle smoke
   - 将 fake sensor 替换为真实 Tetragon/tetra 或真实可验收 bundle
   - manager 侧断言 recent health、policy_loaded、sensor running/degraded
   - agent 退出后仍由 systemd 拉起

3. **压实长期运行语义**
   - graceful shutdown flush 验收
   - retry/backoff soak
   - agent restart 后 unacked batch 不放大结果
   - degraded/recovered health 状态机

4. **Policy/control 最小闭环**
   - static policy loader
   - manager policy endpoint
   - agent fetch/apply policy
   - rule enable/disable

5. **Store 切 SQLite**
   - schema/migrations/indexes
   - query pagination
   - incident/evidence tables

6. **Analytics 包结构补齐**
   - graph
   - rarity interface
   - cloud rules
   - converge interface
   - incident lifecycle

7. **规则内容化**
   - `configs/rules/endpoint`
   - `configs/rules/cloud`
   - default MVP content pack
   - rule metadata and MITRE tags

8. **Link1 stream 化**
   - bidirectional gRPC stream
   - session
   - ack cursor
   - resume
   - policy downlink

9. **EDR investigation / response plane**
   - incident detail
   - evidence path
   - raw evidence pullback
   - observe-only response skeleton
   - kill/block/quarantine 的授权模型

10. **XDR ingestion adapter**
   - k8s audit
   - cloud audit
   - identity events
   - network flow
   - CI/CD and registry events
   - canonical entity mapping

## 四、结论

当前实现已经完成"能证明 EDR detection path 架构成立"的 v1 MVP:

```text
container/VM 双拓扑可跑
端到端链路可跑
核心攻击/跨 lineage/负例场景可验证
proto/agent/manager/ctl 边界已建立
```

但走向 EDR/XDR 平台原型的关键工作还在:

```text
daemon lifecycle
sensor runtime
policy/control plane
graph analytics
durable store
reliable Link1
rule content system
investigation/response plane
multi-source XDR ingestion
deployment/operations
```

下一步最值得做的是 **VM 真实 Tetragon/systemd smoke + policy 最小闭环 + 长跑可靠性**。agent daemon、sensor contract、spool、health、dev auth、restart/tamper 和 container/VM managed detection/capture 的地基已经立起来了,现在要把它们继续推进到 systemd 托管和 policy ownership。在这个端点 runtime 稳定后,再逐步补 investigation/response plane 和多源 ingestion,把 EDR 图扩展成 XDR 图。
