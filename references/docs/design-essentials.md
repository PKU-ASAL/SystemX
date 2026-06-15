# SysArmor Next 设计纲领

> 一句话：**SysArmor Next 是面向 EDR/XDR 的安全平台原型：端侧在源头给每条内核事实盖上因果上下文、拦住最危险的瞬间、并产出带标签的种子；云端把端点、云、身份、网络、工作负载等事实流连成溯源图，用罕见度加权 + 结构收敛裁决告警。**

本文只讲稳定设计：我们要解决什么问题、基于哪些第一性原理、系统应具备哪些能力、分层抽象是什么、组件之间通过什么契约交互、哪些边界不能被实现细节污染。具体实现进展、测试结果和版本历史放在 changelog / plan 文档中。

---

## 一、要解决的问题

现代攻击很少表现为一个孤立的“坏动作”。攻击者更常用一串看起来都合理的操作完成目标：

```text
web runtime spawn shell
  -> curl 下载脚本
  -> chmod / exec
  -> 读 token / key
  -> 外联或横向移动
  -> 云资源 / 身份 / 网络侧继续扩散
```

单看每一步,它们都可能像正常运维。把每个动作都报警会制造误报风暴；把它们都沉默又会漏掉攻击。

SysArmor Next 的核心问题是：

> 如何在海量看似正常的事实里,把少数真正相关的事实连成一条可解释的攻击链,并在攻击造成损害前做出授权范围内的响应？

因此,系统不能只做日志转发,也不能只做单点规则匹配。它必须同时具备：

- 端侧可信观测和快速响应。
- 统一事实模型。
- 云端跨实体、跨工作负载、跨时间、跨安全域的图收敛。
- 可运营的策略、规则、响应和调查闭环。

---

## 二、第一性原理

| | 事实 | 设计推论 |
|---|---|---|
| P1 | 内核是行为的唯一可信来源,用户态可被绕过或伪造 | 关键采集和阻断必须尽量靠近内核,通过 eBPF/LSM/sensor backend 获取事实 |
| P2 | 攻击是因果链,不是孤立事件 | 告警必须来自图上的结构裁决,不是单事件判断 |
| P3 | 上下文在源头打很便宜,事后重建很贵 | 端侧给事实打 lineage、scope、entity 标签,云端再建图 |
| P4 | 端侧资源与业务争抢,云端资源弹性更好 | 端侧只维护 O(活跃实体) 的轻量状态,复杂图计算上移云端 |
| P5 | 检测内容天天变,执行核心变化很少 | 检测靠可版本化、可热更新的规则内容和策略,不靠频繁改代码 |
| P6 | 响应动作有破坏性 | 响应必须由策略授权、可审计、默认 observe-only,不能绕过控制面 |
| P7 | 部署形态会变化 | host、container、cgroup、namespace、pod 都要落到统一 runtime scope,不能让拓扑特判渗透到上层 |

---

## 三、系统应具备的能力

### 3.1 Endpoint Runtime

端侧 agent 应长期运行,并具备：

- 加载配置和身份。
- 管理 sensor runtime。
- 探测能力和健康状态。
- 应用采集策略。
- 订阅内核/工作负载事件。
- 归一化事件并打 lineage / scope / entity 标签。
- 执行端侧快路径规则。
- 生成 Event / Signal / evidence seed。
- 本地 durable spool。
- 断网/重启后续传。
- 上报 health / heartbeat / tamper signal。
- 接收策略、回拉、响应等下行指令。

端侧不是一个“脚本采集器”,而是 EDR/XDR 数据面和响应面的第一跳。

### 3.2 Sensor Runtime

Sensor Runtime 是端点可见性和可选阻断能力的封装层。它屏蔽具体 backend：

- 当前可以由 Tetragon 实现。
- 未来可以替换或并行接入 native sensor。
- 上层只依赖 Sensor Contract,不直接消费 backend raw JSON。

Sensor Runtime 必须支持 scope：

```text
Sensor Runtime
  scope:
    type: host | container | cgroup | namespace | pod
    selector: ...
```

含义：

- VM / 裸机是 `host` scope。
- 单容器或 cgroup workload 是 `container` / `cgroup` scope。
- K8s workload 是 `pod` / `namespace` scope。

容器不是迷你 VM。默认容器架构是运行独立 privileged `sysarmor-agent + sensor` container,由它观测目标 workload scope。业务容器内安装 agent 可以作为调试、受限环境或兼容模式,但不是默认产品架构。

### 3.3 Policy And Rule Content

检测内容必须可运营：

- 规则有 id、version、where、severity、tags、MITRE、response intent。
- 策略能引用规则、启停规则、指定 scope、指定 observe/enforce mode。
- manager 能发布策略版本、灰度、回滚、分配给 agent/scope。
- agent 能获取 effective policy 并应用。
- cloud analytics 能按同一 policy 控制云端规则。

规则内容是产品能力,不应长期硬编码在执行引擎里。

### 3.4 Response / Enforce

响应能力分两层：

- **Response intent**：Signal 表达建议动作,例如 kill、block、quarantine、collect。
- **Response decision**：Control Plane 根据 policy、scope、权限和审批状态决定是否执行。

默认路径应是 observe-only：

```text
Signal.response_intent
  -> policy authorization
  -> response command
  -> agent validate scope/policy
  -> sensor Enforce
  -> response ack/audit
```

任何真实阻断都必须有：

- 策略授权。
- 租户/agent/scope 约束。
- 审计记录。
- 结果回传。
- 回滚或降级策略。

### 3.5 Cloud Analytics

云端负责把端侧和外部数据源产出的事实连接成图,再裁决告警。它应具备：

- entity normalization。
- provenance graph。
- lineage/entity index。
- cloud signal rules。
- rarity baseline。
- correlation / dedup / suppression。
- converge / incident decision。
- evidence subgraph extraction。
- incident lifecycle。

云端图不是为了“好看”,而是为了把单点无法判断的弱信号连成攻击故事。

### 3.6 Incident And Evidence

Incident 是唯一面向人的告警单元。它不是一堆 signal 的列表,而是一条攻击故事：

- terminals：攻击锚点。
- contributing signals：贡献事实。
- evidence graph：实体和关系。
- timeline：关键时间线。
- converge trace：为何裁决为 incident。
- response history：响应意图、授权、执行结果。
- lifecycle：create / update / merge / suppress / close。

调查入口应该能回答：

- 攻击从哪里开始？
- 经过了哪些进程、文件、socket、账号、容器、pod、主机或云资源？
- 哪些点是罕见的？
- 哪些点是结构上必要的？
- 哪些动作已经被端侧响应？

### 3.7 Durable Store

平台状态必须可恢复、可查询、可审计：

- agents
- agent health
- policies / policy versions / assignments
- rules / rule versions
- events
- signals
- incidents
- evidence
- response audit
- metrics

文件存储适合原型和轻量测试；平台化需要数据库、迁移、索引、分页、TTL、幂等写入和审计。

### 3.8 Link1 Transport

Agent 与云端之间需要一条可靠的安全控制/数据通道：

- 上行 Event / Signal / health / evidence seed。
- 下行 policy / response command / evidence pullback request。
- batch id / ack cursor / resume。
- agent session。
- backpressure。
- authn/authz。
- schema version negotiation。

HTTP/gRPC unary 可以作为简单上传路径；长期应演进为双向 stream。OTel 适合作为云端向 SIEM/SOAR/数据湖的出口,不应替代 Agent ↔ Gateway 的原生安全通道。

### 3.9 XDR Ingestion

XDR 的关键不是多接几个日志源,而是把更多安全域归一成同一套事实模型：

- cloud audit
- identity
- network flow
- K8s audit
- CI/CD
- registry
- SaaS audit

原生 agent 是一等公民,因为它能在源头打 lineage。agentless adapter 是二等公民,因为它只能在云端尽力补齐上下文。两者都必须输出 CanonicalEvent / Signal / Entity,进入同一张图。

---

## 四、核心抽象

### 4.1 Event / Signal / Incident

SysArmor 的事实流只有三层。

```text
Incident   攻击故事。图上风险收敛后的裁决,面向人的告警单元。
   ▲
Signal     规则从事实派生出的新事实。可以是低风险积木,也可以是高风险检测发现。
   ▲
Event      sensor 采集并归一后的客观内核/工作负载行为,不含检测逻辑。
```

三层的职责边界：

- Event 是观测事实。
- Signal 是规则派生事实。
- Incident 是图上裁决结果。

Signal 的关键字段：

| 字段 | 作用 |
|---|---|
| `risk` | 表示该事实对攻击判断的贡献,不是简单累加分 |
| `where` | endpoint 或 cloud,决定规则运行位置 |
| `entities` | 进程、文件、socket、IP、token、user、container、pod 等 join key |
| `terminal` | 是否是高置信攻击锚点 |
| `response` | 是否带响应意图 |
| `event_refs` | 指向原始事件或证据 |

告警只应该在 Incident 层对外呈现。单个 Signal 默认不直接告警,除非它是端侧高置信 terminal 并触发授权范围内的快路径响应。

### 4.2 Lineage

lineage 是贯穿 Event / Signal / Incident 的索引坐标,回答“这条事实属于哪条执行谱系”。

每个进程在 exec 时获得 lineage id。它继承父进程谱系,并被盖到该谱系产生的每条事实上：

```text
Incident ┐        ← 按 lineage + entity graph 收敛
Signal   │        ← 继承所属 Event 的 lineage
Event    ┘        ← exec 时盖 lineage_id
──────────────────► lineage
```

lineage 的作用：

- 端侧规则按谱系做轻量状态。
- 云端图按谱系聚簇。
- 调查时能拉出行为线。

端侧持有的是 lineage 标签和短期状态,不是长期记忆。跨天、跨主机、跨 workload 的记忆在云端图里。

### 4.3 Entity Graph

云端图由实体和关系组成：

```text
nodes:
  process, file, socket, ip, host, container, pod, namespace,
  user, token, cloud principal, IAM role, CI job, registry artifact

edges:
  fork, exec, read, write, connect, load, mount,
  owns, belongs_to, authenticates_as, assumes_role, deploys
```

lineage 负责同一执行谱系内的事实索引；entity graph 负责跨谱系、跨主机、跨工作负载、跨云资源的连接。

### 4.4 Runtime Scope

runtime scope 是被保护对象的边界：

```text
type RuntimeScope struct {
    Type     string // host | container | cgroup | namespace | pod
    Selector string
}
```

所有能力都应能归属到 scope：

- policy assignment
- sensor health
- event collection
- response authorization
- evidence query
- incident scope
- agent status

这样 manager 看到的不是“某个容器里装了 agent”,而是“某个 agent runtime 正在保护某个 scope”。

---

## 五、系统分层

```text
┌─────────────────────────────────────────────────────────┐
│ Control Plane                                           │
│ 策略、规则、灰度、回滚、响应授权、调查入口、租户权限      │
├─────────────────────────────────────────────────────────┤
│ Cloud Analytics                                         │
│ 建图、云端规则、罕见度、结构收敛、Incident、Evidence     │
├─────────────────────────────────────────────────────────┤
│ Gateway / Link1                                         │
│ Agent 连接终结、上行摄入、ack/resume、下行策略/响应      │
├─────────────────────────────────────────────────────────┤
│ Endpoint Core                                           │
│ lineage、normalize、端侧规则、证据种子、spool、upload    │
├─────────────────────────────────────────────────────────┤
│ Sensor Runtime                                          │
│ Tetragon / Native Sensor, capability, health, enforce    │
├─────────────────────────────────────────────────────────┤
│ Kernel / Workload                                       │
│ Linux kernel, process, file, network, container, K8s     │
└─────────────────────────────────────────────────────────┘
```

分层原则：

- 上层依赖下层契约,不依赖下层实现。
- manager/cloud 不直接消费 raw sensor JSON。
- agent pipeline 不直接绑定某个 sensor backend。
- control plane 不绕过 policy 直接发 response。
- external export 不替代内部 Link1 协议。

---

## 六、稳定契约与协议边界

### 6.1 Sensor Contract

Sensor Runtime ↔ Endpoint Core 的契约。

```go
type Sensor interface {
    Capability(ctx context.Context) (Capability, error)
    Apply(ctx context.Context, intent CollectionIntent) error
    Subscribe(ctx context.Context, intent CollectionIntent) (<-chan EventEnvelope, error)
    Enforce(ctx context.Context, cmd EnforcementCmd) (EnforcementAck, error)
    Health(ctx context.Context) (Health, error)
    Stop(ctx context.Context) error
}
```

关键对象：

- `Capability`: backend、version、event support、enforce support、kernel/BTF/bpffs 等。
- `CollectionIntent`: runtime scope、event kinds、file/socket filter、observe-only。
- `EventEnvelope`: sensor-neutral event、raw ref、received_at。
- `Health`: running、installed、policy_loaded、events_seen、events_dropped、parse_errors、restart_count。
- `EnforcementCmd/Ack`: 阻断命令和结果,默认可以是 unsupported 或 observe-only。

Tetragon 是实现,不是边界。

### 6.2 Endpoint-Core Contract

Endpoint Core 的输出必须是 sensor-neutral：

- `CanonicalEvent`
- `Signal`
- `EvidenceSeed`
- `AgentHealth`
- `UploadBatch`

它负责：

- raw event -> canonical event。
- process context -> lineage。
- local rule state -> endpoint signal。
- raw refs -> evidence seed。
- durable queue -> upload batch。

### 6.3 Link1 Agent-Gateway Protocol

Link1 是 Agent ↔ Gateway 的原生安全通道。

上行：

- agent hello / session。
- event batch。
- signal batch。
- evidence seed。
- health heartbeat。
- response result。

下行：

- policy update。
- response command。
- evidence pullback。
- enhanced collection intent。

可靠性语义：

- batch id。
- ack cursor。
- resume。
- idempotent ingest。
- server-side backpressure。
- schema/version negotiation。

### 6.4 Policy / Rule Contract

规则内容与策略控制面之间的契约。

Rule:

```text
rule_id
version
where: endpoint | cloud
enabled
severity
risk
tags
mitre
response_intent
inputs
outputs
```

Policy:

```text
policy_id
version
tenant_id
scope selector
endpoint rule refs
cloud rule refs
response policy
observe/enforce mode
rollout state
```

Policy 的职责是回答：

- 哪些规则对哪个 scope 生效？
- 哪些规则在端侧跑,哪些在云端跑？
- 哪些响应动作被允许？
- 当前生效版本是什么？

### 6.5 Response Contract

响应链路必须显式可审计：

```text
Signal response_intent
  -> ResponseDecision
  -> ResponseCommand
  -> EnforcementAck
  -> ResponseAudit
```

任何执行结果都必须回写：

- command accepted / denied。
- would_enforce / executed / unsupported。
- reason。
- actor / policy / scope。
- timestamp。

### 6.6 Analytics Contract

Cloud Analytics 的内部边界：

```text
ingest -> entity -> graph -> rules -> correlate -> converge -> incident -> evidence
```

每层职责：

- `ingest`: 接收 CanonicalEvent / Signal。
- `entity`: 归一化实体键。
- `graph`: 保存节点和边。
- `rules`: 生成 cloud signal。
- `correlate`: 去重、折叠、抑制。
- `converge`: 结构收敛和风险裁决。
- `incident`: 生命周期。
- `evidence`: 证据子图和路径。

算法可以替换,但输出契约应稳定。

### 6.7 Ingestion Adapter Contract

外部数据源进入 XDR 图时必须归一：

```text
raw source event
  -> CanonicalEvent
  -> EntityRef
  -> optional Signal
  -> graph ingest
```

Adapter 必须声明：

- source type。
- tenant。
- timestamp。
- entity mapping。
- confidence。
- lineage quality：native / inferred / unavailable。

---

## 七、端侧与云端分工

### 7.1 端侧为什么轻

端侧不建全局图,只维护有限状态：

```text
进程上下文表      活跃进程身份、父链、lineage_id
谱系规则状态      每条 lineage 的计数器和短序列
实体接触缓存      最近文件/socket/token 接触关系
原始事件环形缓冲  云端回拉证据
上传队列          断网续传
```

端侧可以做：

- 本机上下文打标。
- lineage 维护。
- 两跳关联。
- 高置信快路径 detection。
- observe/enforce response execution。

端侧不应该做：

- 全局图。
- 跨主机裁决。
- 长跨度全局罕见度。
- 大规模搜索。
- 累计加分式告警。

### 7.2 云端为什么建图

云端天然汇聚多 agent、多 workload、多时间窗口、多安全域的事实。它负责：

- 跨 lineage stitching。
- 跨 host / container / pod / namespace 关联。
- 跨 identity / cloud / network 关联。
- 全局或局部 rarity baseline。
- 结构收敛。
- incident lifecycle。
- evidence path。

一句话：端侧负责“把事实说清楚、拦住瞬间”,云端负责“把事实连成故事、裁决告警”。

---

## 八、收敛原则

### 8.1 为什么不能用加法

简单风险加法会奖励“忙碌”：

- CI runner 动作多,容易累计高分。
- 特权 agent 本来就读很多文件、连很多网络。
- 长寿进程天然事件多。

所以“分数累加超过阈值”会同时造成误报和漏报。

### 8.2 正确收敛

SysArmor 的裁决应基于两个正交机制：

1. **罕见度加权**
   - 行为对其所属实体/workload 越罕见,权重越高。
   - 惯常行为权重趋零。

2. **结构收敛**
   - 多个独立罕见点是否在因果上紧凑相连。
   - 目标是找最小攻击子图,不是累积分数。

工程上可以采用：

- Count-Min Sketch / IDF / workload baseline。
- Personalized PageRank 局部扩散。
- Steiner Tree 近似。
- 相关族去重。
- terminal anchor。

算法可迭代,原则不能退回加法。

---

## 九、部署和运行边界

### 9.1 Agent 部署形态

支持多种形态：

- host/VM agent。
- privileged sensor container。
- K8s DaemonSet。
- 特殊环境下的 in-container debug/compat agent。

但产品语义统一为：

```text
agent runtime protects scope
```

而不是：

```text
agent installed in some machine/container
```

### 9.2 Gateway / Manager / Analytics

云端可以拆成：

- **Gateway**：连接终结、摄入、ack/resume、策略下发、响应下发。
- **Manager**：策略、注册、租户、调查、响应授权。
- **Analytics**：建图、规则、收敛、incident/evidence。

小规模部署可以合进一个进程；架构边界仍要保留。

### 9.3 Storage

存储层必须支持：

- 幂等 ingest。
- 迁移。
- 索引。
- 分页。
- TTL。
- 审计。
- evidence blob 分层。

### 9.4 External Integration

外部集成分两类：

- **Ingestion**：外部遥测进入 SysArmor 图,必须通过 adapter 归一成 CanonicalEvent / Entity。
- **Export**：SysArmor 的 incident / signal / audit 输出到 SIEM/SOAR/数据湖,可以使用 OTel Collector 或其他标准出口。

不要用外部出口协议替代内部安全控制通道。

---

## 十、诚实边界

这套架构不是银弹：

1. **依赖云端图的检测有延迟**
   - 低慢攻击可接受秒级到分钟级收敛。
   - 端侧断网时,复杂跨域关联会缺位。
   - 快路径只兜住高置信、单机可判断的危险瞬间。

2. **模型质量决定上限**
   - 罕见度 baseline、规则内容、实体归一化和收敛参数决定检测质量。
   - 架构让问题可解,不自动让检测变好。

3. **完全同构合法行为无法被可靠区分**
   - 如果攻击行为与合法行为在观测维度上完全同构,任何系统都无法无条件检出。
   - 只能增加观测维度、扩大上下文或依靠外部情报。

4. **响应必须保守**
   - kill/block/quarantine 对业务有破坏性。
   - 默认 observe-only,逐步授权,强审计。

---

## 十一、能力完成度的判断标准

一项能力不能只看“有没有代码”,而要看是否具备闭环：

```text
contract
  -> implementation
  -> config/policy expression
  -> health/observability
  -> failure/recovery behavior
  -> e2e proof
  -> compatibility story
```

例如：

- Sensor 不只是能读事件,还要能表达 scope、capability、health、policy apply、restart、enforce ack。
- Policy 不只是一个 proto,还要能 version、assign、fetch、apply、audit、rollback。
- Incident 不只是一个 JSON,还要有 evidence graph、timeline、lifecycle、query。
- Link1 不只是 HTTP 200,还要有 ack cursor、resume、idempotency、downlink。

这条标准是后续设计和评审的共同尺子。
