# SysArmor MVP 设计 (v2)

> **读者**：接手从零搭建 SysArmor 的工程师 / agent。
> **目标**：读完本文你应该清楚**造什么、怎么分块、代码怎么摆、按什么顺序搭**，并且搭出来的东西与生产版架构同构,日后能沿着既有接缝长成完整产品,而不是推倒重来。
> **文档地图**：`design-essentials.md` 讲"为什么这样设计"(第一性原理) · 本文讲"怎么落地" · `design-test-cases.md` 讲"怎么验证它成立"。
>
> **一条总原则**：先把三道契约(接口)冻结,再在契约背后填实现。MVP 可以把所有进程塞进一个 binary、用最土的算法,但接缝必须是真的,这样从 MVP 长到生产版就是"沿契约拆分 + 替换实现",不返工。

---

## 〇、术语速查

| 术语 | 一句话 |
|---|---|
| **Event** | 一次内核行为的客观记录(exec / connect / open ...),不含任何检测逻辑 |
| **Signal** | 规则从 Event(或下层 Signal)派生出的命名事实,带 `risk` / `entities` 等属性 |
| **Incident** | 图上风险收敛后裁决出的"攻击故事",面向人的唯一告警单元 |
| **lineage** | 进程执行谱系的身份标签,exec 时继承谱系根,盖在该谱系产生的每条 Event 上 |
| **entities** | Signal 携带的实体键(进程/文件/IP/token),云端据此跨谱系/跨主机缝合 |
| **terminal** | 端侧高置信判定的攻击锚点,附证据包,作为云端收敛的种子 |
| **stable_id** | 进程的稳定身份 = hash(host, pid, start_time),跨 PID 复用仍唯一 |

---

## 一、要造的东西:一条端到端的线

### 1.1 产品一句话

SysArmor 是云原生内核级 EDR。**端侧**在源头给每条内核事实打上因果标签(lineage)、跑有界的快路径检测、广撒带实体键的种子;**云端**把事实流连成溯源图,用罕见度加权 + 结构收敛裁决出可解释的攻击故事(Incident)。

它解决的核心问题不是"看见一个坏动作",而是"在海量看起来正常的事件里,把少数真正相关的事件连成一条可解释的因果链,并在造成损害前反应"。

### 1.2 端到端数据流(MVP 必须打通的那条线)

```text
内核 syscall (exec / connect / open ...)
   │
   ▼  ┌─────────────────────────────────────────────── 端侧 (sysarmor-agent) ───┐
[Sensor]      Tetragon GetEvents ──► SensorEvent        (sensor 中立的原始观测)
   │
[Normalize]   打 stable_id + lineage_id ──► CanonicalEvent   (归一事实 L0)
   │
[Fastpath]    六模块规则引擎 ──► Signal (低风险种子 / 高风险 terminal + 证据包)
   │
[Uploader]    断连可续传的上行队列
   └──────────────────────────────────────────────────────────────────────────┘
   │  Link1: 原生 gRPC + 自有 protobuf 契约 (双向)
   ▼  ┌─────────────────────────────────────────────── 云端 (sysarmor-manager) ─┐
[Ingest]      接收 Event / Signal / 证据
   │
[Graph]       按实体键建溯源图 (进程→文件→socket,边合并)
   │
[Rarity]      全局罕见度 (Count-Min Sketch + IDF 权重)
   │
[Rules]       云端 Signal 规则 (图模式,可跨 lineage)
   │
[Converge]    罕见度加权 + 相关族去重 + 因果路径 top-k ──► Incident + 证据子图
   │
[Store]       SQLite 落 Incident / 证据 / 健康
   └──────────────────────────────────────────────────────────────────────────┘
   │
   ▼
[sysarmorctl] 调查者看到一条可解释的攻击链
```

**具体走一遍(apt-fileless-c2)**:

1. `java` web 进程 exec `bash`,`bash` exec `curl` 下载 `x.sh`,`bash -i` 反弹到 `10.66.0.99:443`。
2. 每个 exec 都被 Tetragon 抓到 → agent 归一成 `CanonicalEvent`,沿谱系继承同一个 `lineage_id`。
3. fastpath 命中 `reverse_shell_pattern`,声明 `terminal=true`,切一个证据包(谱系切片 + 两跳邻居 + raw 引用)。
4. terminal Signal + 一串低风险 Signal 上云。
5. 云端建图:`bash → 写 → x.sh`、`x.sh → exec → bash -i → connect → C2` 连成子图。
6. 收敛:这些点对 web 运行时罕见度高 → 高异常分 → 因果上紧凑相连 → 裁决出 **1 个 Incident**。
7. `sysarmorctl incident <id>` 打印出 `java → bash → x.sh → 10.66.0.99:443` 的证据链。

### 1.3 MVP 的"完成"长什么样

MVP 不追求覆盖面,只追求**把上面这条线端到端跑通并可度量**。判定标准 = `design-test-cases.md` 的三个场景:

| 场景 | 证明什么 | 通过标准 |
|---|---|---|
| `apt-fileless-c2` | "响"的攻击:端侧能高置信产 terminal,云端缝合 | 1 个 Incident,证据子图含 `web→shell→落盘→外联` |
| `apt-staged-drop` | 云端图的立论:跨 lineage 共享实体才能成案 | 仅当云端按共享文件实体缝合两条独立 lineage 才产 1 个 Incident;关掉缝合则 0 |
| `benign-ci-noise` | 收敛不误报:罕见度而非裸加 | Incident = 0;切成裸加阈值则误报 ≥ 1 |

`apt-staged-drop` 和 `benign-ci-noise` 是两个"立论实验":前者证明"为什么需要云端图",后者证明"为什么收敛不能用加法"。它们通过 = 架构选择被验证。

---

## 二、系统架构:五个组件 + 三道契约

### 2.1 五个组件,各一句职责

```text
┌─────────────────────────────────────────────────────────┐
│  Control Plane   策略下发 · 注册 · 调查 UI                 │
├─────────────────────────────────────────────────────────┤
│  Cloud Analytics 建图 · 云端规则 · 罕见度+结构收敛 · 裁决   │ ← 拼图、讲故事
├─────────────────────────────────────────────────────────┤
│  Endpoint Core   打标(lineage) · 快路径 Signal · 上传      │ ← 贴标签、抢时间
├─────────────────────────────────────────────────────────┤
│  Sensor Runtime  Tetragon 今天 / Native Sensor 以后        │ ← 看见、阻断
├─────────────────────────────────────────────────────────┤
│  Kernel / Workload                                        │
└─────────────────────────────────────────────────────────┘
```

| 组件 | 唯一职责 | MVP 形态 |
|---|---|---|
| **Sensor Runtime** | 在内核看见行为、(以后)阻断 | 托管 Tetragon,消费 GetEvents |
| **Endpoint Core** (agent) | 打 stable_id/lineage、跑快路径、产 Signal+证据、上传 | `sysarmor-agent` binary |
| **Cloud Analytics** | 建溯源图、跑云端规则、收敛裁决 | 折叠进 `sysarmor-manager` |
| **Control Plane** | 策略下发、agent 注册、调查接口 | 折叠进 `sysarmor-manager` |
| **Store** | 持久化 Incident / 证据 / 健康 / 罕见度快照 | SQLite |

### 2.2 三道契约 —— 可扩展性的全部来源

这是整个架构最重要的部分。系统能长期演进、能把 MVP 长成生产版,靠的就是这三道**接缝**。每道契约定义"两侧如何对话",两侧的实现可以各自替换而互不影响。

| 契约 | 位于 | 定义什么 | 让你以后能换掉 |
|---|---|---|---|
| **Sensor Contract** | Sensor ↔ Agent | 一条事件长什么样、采集意图怎么表达、能力怎么探测、怎么阻断 | Tetragon → 自研 Native Sensor,上层无感 |
| **Edge-Cloud Contract** (Link1) | Agent ↔ Cloud | 上行流(Event/Signal/证据/健康) + 下行回路(策略/回拉/响应) | 云端图算法(穷人版 → NODLINK),端侧无感 |
| **DetectionPolicy** | Control → 两侧引擎 | 检测内容包,用统一 DSL,靠 `where` 决定下发端侧还是云端 | 加检测 = 下发签名内容,不发版、不改代码 |

**为什么这能让 MVP "与生产版同构"**:MVP 把 gateway/analytics/control/store 塞进一个 manager binary,但它们之间走的是真契约(proto 类型)。生产版要拆成独立可扩展的 Gateway(无状态摄入) + Manager(控制面) + Analytics(建图收敛),只是"沿契约把进程拆开",不是重写。

**实现纪律**:`api/proto/` 是所有 schema 的单一事实源(见 §4.3)。任何跨组件的数据都先在 proto 里定义类型,再生成端云两侧代码,保证契约永不漂移。

### 2.3 部署形态:从单 binary 到可拆分

```text
MVP                          生产版
┌────────────┐               ┌────────────┐
│ agent      │               │ agent (车队)│
└─────┬──────┘               └─────┬──────┘
      │ Link1 gRPC                 │ Link1 gRPC
┌─────▼──────────────┐      ┌──────▼─────┐   无状态、水平扩展
│ manager (单 binary) │      │ Gateway    │   连接终结 + 摄入 + 策略下发
│  gateway            │      └──────┬─────┘
│  analytics          │   ──►  ┌────▼─────┐  ┌──────────┐
│  control            │      │ Analytics  │  │ Manager  │  控制面/调查
│  store (SQLite)     │      │ (建图收敛) │  └──────────┘
└─────────────────────┘      └────┬───────┘
                                   │ Link2 (OTel) → SIEM/SOAR/数据湖
```

MVP 即便单 binary,内部也**按契约切好包边界**(见 §3),后续按规模拆分不返工。

---

## 三、代码架构:仓库怎么摆

### 3.1 目录树

```text
sysarmor/
├── cmd/                        # 可执行入口(薄,只做装配)
│   ├── sysarmor-agent/         #   端侧
│   ├── sysarmor-manager/       #   云端单 binary(MVP 内含 gateway+analytics+control+store)
│   └── sysarmorctl/            #   调查 CLI
│
├── api/                        # ★ 所有 schema 的单一事实源(先定义,跨端云共享)
│   ├── proto/
│   │   ├── sensor/v1/          #   Sensor Contract
│   │   ├── event/v1/           #   CanonicalEvent
│   │   ├── signal/v1/          #   Signal + EvidenceBundle
│   │   ├── analytics/v1/       #   Edge-Cloud Contract(上行/下行流)
│   │   ├── incident/v1/        #   Incident + EvidenceSubgraph
│   │   └── policy/v1/          #   PolicyEnvelope + 五类 policy
│   └── schema/                 #   规则 DSL 的 JSON Schema(校验用)
│
├── internal/
│   ├── sensor/
│   │   ├── contract/           # Sensor Contract 的 Go 接口(见 §3.2)
│   │   └── tetragon/           # 实现:runtime 托管 + GetEvents + policy 编译 + 事件映射
│   │
│   ├── endpoint/
│   │   ├── context/            # ★ 三张表:proctable / lineage / touchcache
│   │   ├── normalize/          # SensorEvent → CanonicalEvent(打 stable_id + lineage_id)
│   │   ├── fastpath/           # ★ 端侧 Signal 引擎(六模块,见 §5.1)
│   │   │   ├── match/  sequence/  join/  weight/  dedup/  emit/
│   │   ├── evidence/           # 证据包切片
│   │   ├── ringbuffer/         # 原始事件环形缓冲(供云端回拉)
│   │   └── uploader/           # Link1 上行 + spool(断连续传)
│   │
│   ├── analytics/
│   │   ├── ingest/             # 上行流接收 + (预留)异构源 Adapter
│   │   ├── graph/              # 内存溯源图(节点/边/合并/TTL)
│   │   ├── rarity/             # 全局罕见度(CMS + IDF)
│   │   ├── rules/              # 云端 Signal 引擎(图模式)
│   │   ├── converge/           # ★ 收敛:去重 + 因果路径 + top-k(→ 后续 PPR/STP)
│   │   ├── incident/           # Incident 组装
│   │   └── evidence/           # 证据子图裁剪
│   │
│   ├── control/
│   │   ├── policy/             # PolicyEnvelope 加载/校验/下发(MVP 读静态文件)
│   │   └── registry/           # agent 注册(MVP 静态 token)
│   │
│   ├── transport/
│   │   ├── link1/              # Agent↔Gateway gRPC server/client
│   │   └── link2/              # (预留)OTel Collector 出口
│   │
│   └── store/                  # SQLite
│
├── configs/
│   ├── policies/               # PolicyEnvelope 实例(YAML):collection/resource/detection/response/telemetry
│   └── rules/                  # 规则内容(被 DetectionPolicy 引用):endpoint/ + cloud/
│
├── deployments/
│   ├── systemd/                # MVP 主形态
│   └── helm/                   # K8s DaemonSet(预留)
│
└── test/                       # 见 design-test-cases.md(已实现的双拓扑测试环境)
```

### 3.2 关键包的职责与边界

- **`api/`**:proto 同时生成端侧和云侧类型。MVP 即便单 binary 也走这套类型 —— 这是"契约真实"的物理保证。
- **`internal/sensor/contract`**:定义 `Sensor` Go 接口,Tetragon 只是它的一个实现。上层只依赖接口:

  ```go
  type Sensor interface {
      Capability(ctx) (SensorCapability, error)              // 探测这台机器能采/能阻断什么
      Subscribe(ctx, CollectionIntent) (<-chan SensorEvent, error)  // 采集意图由 CollectionPolicy 编译
      Enforce(ctx, EnforcementCmd) (EnforcementAck, error)   // MVP 返回 unsupported
      Health(ctx) (SensorHealth, error)                      // 含 dropped_events 等
  }
  ```

- **`internal/endpoint/context`**:端侧的全部"记忆",三张表(见 §5.1)。边界清晰、可单独压测。
- **`internal/endpoint/fastpath`**:有界 CEP 引擎,三段六模块(detection: match/sequence/join · scoring: weight · output: dedup/emit),可插拔、可在资源压力下单独降级(关 scoring/throttling 段)。
- **`internal/analytics/converge`**:收敛独立成包。MVP 是穷人版,正式版换 PPR/STP 时**只动这个包**,建图与规则不受影响。
- **`cmd/*`**:入口只做依赖装配(wire up),不写业务逻辑。

### 3.3 扩展点(明确告诉你以后从哪长出去)

| 想加什么 | 改哪 | 是否动核心 |
|---|---|---|
| 新传感器(Native Sensor) | 新增 `internal/sensor/<kind>/` 实现 `Sensor` 接口 | 否 |
| 新检测规则 | 往 `configs/rules/` 加 YAML,下发 DetectionPolicy | 否,不发版 |
| 规则规模上去后的性能 | `endpoint/fastpath/match` 加谓词倒排索引 + RETE 式共享 alpha/beta partial match | 仅匹配核心 |
| 生产级图(图数据库/分布式) | 替换 `analytics/graph` + `analytics/converge` | 仅这两包 |
| 异构数据源(auditd/k8s audit/agentless) | `analytics/ingest` 加 Adapter,归一成 CanonicalEvent | 否 |
| 导出到 SIEM/SOAR | 实现 `transport/link2`(OTel Collector) | 否 |
| 拆分 Gateway/Analytics/Manager | 按 `internal/` 既有包边界拆进程 | 否,沿契约拆 |
| 真实响应(kill/block) | 实现 `Sensor.Enforce` + ResponsePolicy 放开 ENFORCE | 否 |

---

## 四、数据模型:一条事实的旅程

### 4.1 五个核心类型,逐级转化

```text
SensorEvent       sensor 中立的原始观测(还没身份)
   │ normalize:打 stable_id + lineage_id
   ▼
CanonicalEvent    归一事实(L0),云端建图的原子记录
   │ fastpath 规则匹配
   ▼
Signal            派生事实(L1),带 entities(缝合键) + 可选 terminal
   │ 云端建图 + 罕见度 + 收敛
   ▼
GraphNode/Edge    溯源图,Signal 挂在节点上,带 NodeRisk
   │ 收敛裁决
   ▼
Incident          攻击故事 + 证据子图,唯一对外告警
```

### 4.2 关键 schema(只列承重字段;完整定义在 `api/proto/`)

> 记号:`[MVP]` 必填,`[later]` 预留(结构在,MVP 可空)。完整字段、注释、enum 取值以 `.proto` 为准,本节只解释**为什么这几个字段是承重的**。

**Sensor Contract(最底层,sensor → agent)**

```protobuf
message SensorEvent {                 // sensor 中立,尚未打身份
  uint64    mono_ns;                  // 单调时钟,排序用            [MVP]
  EventKind kind;                     // EXEC/EXIT/FORK/OPEN/WRITE/CHMOD/CONNECT/...
  RawProcess proc;                    // pid,ppid,binary,argv,uid,start_time,cgroup
  RawObject  object;                  // path | dst_ip:port | target_pid
  string     container_id;
}
message SensorHealth { uint64 dropped_events; double events_per_sec; ... }  // 丢失率是数据完整性命门
```

**CanonicalEvent(L0)** —— 承重在三个打标字段:

```protobuf
message CanonicalEvent {
  string    id;                       // ULID                        [MVP]
  uint64    seq;                      // per-agent 单调,丢失检测      [MVP]
  EventKind kind;
  ProcessRef subject_proc;            // 主体进程(含 stable_id)
  ObjectRef  object;                  // file_path | socket(5元组) | target_proc
  string    parent_stable_id;         // ★ agent 打,sensor 给不了
  string    lineage_id;               // ★ exec 时继承谱系根
  string    raw_ref;                  // 指向 ringbuffer,供云端回拉
}
message ProcessRef {
  string stable_id;                   // ★★ = hash(host_id, pid, start_time_ns)
  uint32 pid; string binary; repeated string argv; uint32 uid;
}
```

`stable_id` 是整套系统的进程身份基石:**云端图节点直接用它做 node_id**,所以它必须在丢事件、PID 复用下仍稳定(启动扫 `/proc` 重建基线)。

**Signal(L1)** —— 承重在 `entities`(缝合键)和 `terminal`:

```protobuf
message Signal {
  string   name;                      // 规则名,如 reverse_shell_pattern
  Where    where;                     // ENDPOINT | CLOUD
  uint32   base_risk;                 // 0~100,规则声明
  float    local_rarity;              // [0,1] 本机视角罕见度(MVP 可恒为 1)
  string   lineage_id;
  repeated EntityRef entities;        // ★★ 云端 join key,必填
  repeated string event_refs;         // 引用 CanonicalEvent
  repeated string signal_refs;        // 可引用下层 Signal(组合)
  bool     terminal;                  // 规则声明的高置信锚点
  EvidenceBundle evidence;            // 仅 terminal=true 时附带
}
message EntityRef {
  EntityKind kind;                    // PROCESS|FILE|SOCKET|TOKEN|USER|CONTAINER|POD
  string     key;                     // 规范化键:stable_id / 绝对路径+inode / dst_ip:port / token摘要
  string     role;                    // subject|object|writer|reader...
}
```

`entities` 为什么必填:`apt-staged-drop` 里落盘 Signal 和执行 Signal 分属不同 lineage,唯一能缝起它们的就是共享的 `FILE` 实体键。没有它,云端图建不出这条边。

**Incident(裁决产物)** —— 承重在证据和可解释性:

```protobuf
message Incident {
  string  summary; uint32 severity; repeated string mitre;
  repeated string lineage_ids;        // 可跨谱系
  repeated string terminals;          // 锚点 stable_id
  EvidenceSubgraph evidence;          // 节点 + 边
  ConvergeTrace converge;             // ★ 如何收敛出来的(可解释:seed/method/path)
}
```

### 4.3 单一事实源:api/proto

所有跨组件类型先在 `api/proto/` 定义,`protoc` 生成 Go 类型供端云共用。**禁止在端侧或云侧私自定义"差不多"的结构** —— 这是契约不漂移的硬纪律。规则 DSL(YAML)用 `api/schema/` 下的 JSON Schema 校验。

---

## 五、核心机制:检测与收敛(系统的 IP)

前面是骨架,这一节是肌肉。SysArmor 的价值全在"端侧怎么便宜地打标 + 云端怎么不误报地收敛"。

### 5.1 端侧:打标 + 流式检测引擎 + 三张表

**这是端侧规则引擎,范式是有界 CEP(复杂事件处理),不是图引擎。** 它消费 `CanonicalEvent` 流、输出 Signal;对齐的是 EQL / Esper / Flink CEP / RETE 这一派**有状态的事件相关引擎**,而不是 Falco / 经典 Sigma 那种无状态单事件匹配。图模式匹配能力在**云端**(§5.2 / §5.3 的 `graph_match`),端侧绝不建图。

**三张表 = 端侧的全部记忆**(O(活跃实体),不建图):

| 表 | 存什么 | 规模 |
|---|---|---|
| 进程上下文表 (proctable) | 每个活跃进程的身份、父链、lineage_id | ∝ 活跃进程数 |
| 谱系规则状态 (lineage) | 每条 lineage 的计数器/序列机状态 | ∝ 活跃 lineage 数 |
| 实体接触缓存 (touchcache) | "谁刚写了这文件/连了这 IP" 的 LRU | 固定大小 |

加上原始事件环形缓冲(供回拉) + 上传队列(断网续传)。touchcache 是"图"在端侧的最小替身,只支持检测要的**两跳关联**(写后执行、落盘后外联),绝不做任意跳图扩展 —— 那是云端的事。这条线让端侧成本可封顶。

**引擎管线明确分三段:detection(匹配)→ scoring(打分)→ output+throttling(输出抑制)。** 前一段是检测逻辑,后两段是富化与管线,不要混为一谈:

```text
CanonicalEvent
│
├── A. detection  匹配核心(对齐 EQL / RETE / Flink CEP)─────────────────
│   ① match     单事件谓词(binary/path/uid/argv/kind)        → AtomMatch   [RETE alpha / Sigma selection]
│   ② sequence  按 lineage_id 分区的窗口/序列状态机           → SeqMatch    [EQL sequence-by-maxspan / Esper `->`]
│   ③ join      查 touchcache 做两跳(写后执行/落盘后外联)     → JoinMatch   [RETE beta / 有界 stream-table join]
│
├── B. scoring   打分富化(不是匹配)──────────────────────────────────────
│   ④ weight    本机罕见度 CMS,算 local_rarity(可关,降级=1.0)         [UEBA / 风险打分,非检测逻辑]
│
└── C. output + throttling  输出与抑制 ───────────────────────────────────
    ⑤ dedup     同源动作派生的多匹配折叠(窗口内 hashset)               [告警抑制,运维管线]
    ⑥ emit      组装 Signal(填 entities/terminal/evidence/refs)         [Drools RHS / 产出动作]
```

**为什么是 CEP 而非单事件匹配**:我们要判的是"web 起 shell → 落盘 → 60s 内同一 lineage 外联"这类**跨事件时序**模式,单事件谓词(Falco/经典 Sigma)做不到,必须靠 ② 的窗口状态机。`by lineage_id` 分区是关键约束:它把状态机数量绑定到"活跃 lineage 数",天然有界(对齐 I3),避免通用 CEP 因 group-by 基数爆炸而 OOM。③ 的 touchcache join 则是 Flink/Kafka Streams 做有界 join 的标准手法(状态表 + 两跳上限)。

**多规则共享(扩展点,非 MVP 必做)**:当前可"每条规则各跑一遍",MVP 5 条规则无所谓。规则增多后应引入 **RETE 式共享** —— 按 `EventKind`/`binary` 建谓词倒排索引,事件只触发相关规则、共享 alpha/beta 的 partial match,避免重复求值。DSL 设计时给 `sequence` 预留 `until`(否定/缺失模式,如"X 发生但 Y 在窗口内未发生",对齐 EQL `until`)可省后续返工。详见 §3.3。

端侧的本分:**翻译事实、拦住最危险的瞬间、广撒带实体键的种子**;它回答"这个点对本机而言反常吗 + 涉及哪些实体",而**不**回答"这是不是攻击"(那是云端的全局判断)。

### 5.2 云端:建图 + 罕见度 + 结构收敛(为什么不能裸加)

云端把 Signal 流按实体连成 **Provenance Graph**(节点:进程/文件/IP/容器...;边:fork/exec/read/write/connect...;每个节点挂 Signal 和风险)。图在云端而不在端侧,因为图的价值恰恰来自**跨边界**(跨进程、跨容器、跨主机、跨小时),单机看不全。

**收敛绝不能用加法**。最直觉的做法"子图内风险分累加超阈值就告警"是反模式:忙碌或长寿的良性实体(CI runner、特权 agent)做的每件事都有正分,攒够数必然撑爆任何阈值。同样的 `落盘→执行→外联`,在 CI 节点是日常、在 nginx worker 是攻击,加法看不到"对谁而言"。

正确收敛靠两个正交机制:

1. **罕见度加权**:一个行为对所属 workload 越罕见,权重越高;惯常行为权重趋零。`同一动作在 CI 上 rarity≈0、在 nginx 上 rarity≈1`。良性忙碌从构造上就没有可累加的料。
2. **结构收敛**:判定从"分数和超没超"换成"几个**各自独立罕见**的点是否在因果上**紧凑相连**"。忙碌能产生量,产生不了"多个独立罕见点构成的紧凑因果结构"。

**MVP 的穷人版收敛**(可落地,但不是裸加;数据模型已为正式版预留 `NodeRisk` 字段):

```text
1. 每个 Signal → 给对应实体节点叠 anomaly_score = base_risk × global_rarity
2. 相关族去重:同一源动作派生的多 Signal 只取 max
3. 取 anomaly_score top-k 节点作 seed
4. 仅当 seed 间存在【因果路径】(祖先-后代 / 共享实体边)才连
5. 抽最短路证据树 → Incident,method="rarity+causal-topk"
   —— 绝不做"连通子图 Σrisk ≥ 阈值"
正式版:行为嵌入异常分 + Personalized PageRank 局部 push + KMB/在线 Steiner Tree
```

> 实现细节提醒(影响正确性,但不是架构):terminal 由规则声明、端侧不算累计 score;全局罕见度基线和扩散/STP 都需要全局视角,只能在云端。端侧若偷偷算累计分,就会掉回端侧加性溢出。

### 5.3 规则 DSL(DetectionPolicy 的核心)

端侧规则与云端规则同一信封,靠 `where` 路由。端侧规则映射到六模块;云端规则在图上匹配,可跨 lineage。

```yaml
# 端侧规则 → 编译进 fastpath 六模块
rule:
  id: reverse_shell_pattern
  where: endpoint
  match: { on: connect, when: "proc.is_interactive_shell && proc.stdio_redirected_to_socket" }
  sequence: { after: [web_runtime_spawns_shell, payload_dropped], within: 60s, by: lineage_id }
  emit:
    name: reverse_shell_pattern
    base_risk: 70
    terminal: true                          # 高置信 → 切证据包
    entities:                               # ★ 必填,云端 join key
      - { kind: process, key: "$proc.stable_id", role: subject }
      - { kind: socket,  key: "$conn.dst_ip:port", role: object }
    response: { action: kill, mode: observe }   # MVP 只记不发
```

```yaml
# 云端规则 → 在图上匹配,可跨 lineage(apt-staged-drop 的关键)
rule:
  id: dropped_payload_executed_and_connects
  where: cloud
  graph_match:
    pattern: |
      (p:process)-[:exec]->(f:file {dropped:true}),
      (p)-[:connect]->(s:socket {private:false})
    cross_lineage: true                     # ★ 允许 f 由别的 lineage 写入
  emit:
    name: dropped_payload_executed_and_connects
    base_risk: 80
    entities: [ {kind: process, key: "$p.stable_id"}, {kind: file, key: "$f.key"}, {kind: socket, key: "$s.key"} ]
```

| 字段 | 端侧 | 云端 |
|---|---|---|
| `match` / `graph_match` | 单事件谓词 | 图模式 |
| `sequence` / `join` | ✅ lineage 内时序 + 两跳 | — |
| `cross_lineage` | — | ✅ 按共享实体缝 |
| `emit.entities` | ★必填 | ★必填 |
| `emit.base_risk` | 与 rarity 相乘,**不直接累加** | 同 |

---

## 六、构建计划:四个里程碑

每个里程碑有明确出口,出口达成才进下一程。

```text
M0  地基:端到端空管道
    范围: Sensor Contract Go 接口 + tetragon 实现;GetEvents → SensorEvent
          normalize → CanonicalEvent;proctable + lineage 打标
          Link1 gRPC 上行 Event;云端落 SQLite;sysarmorctl 看到事件
    出口: 一条 exec 事件带【正确的 stable_id + lineage_id】出现在云端
    验证: lifecycle-smoke 的 TC-LC-03(采集可见)

M1  端侧检测:六模块 + 三张表
    范围: fastpath ①②③⑥ + 5 条端侧规则(YAML);touchcache 两跳
          emit 带 entities;reverse_shell 产 terminal + 证据包
          ④weight/⑤dedup 最小实现(可恒权重起步)
    出口: 复现 apt-fileless-c2,端侧产出【带 entities 的 Signal + 1 个 terminal 证据包】

M2  云端图与收敛
    范围: 内存图按 host 建图 + 边合并 + 共享实体缝合
          全局 rarity(CMS);3 条云端规则(含 cross_lineage)
          converge: 罕见度加权 + 去重 + 因果路径 top-k → Incident + 证据子图 + ConvergeTrace
    出口: apt-fileless-c2 与 apt-staged-drop 各产【1 个 Incident,证据链完整】
          apt-staged-drop 关掉 cross_lineage → 0(立论验证)

M3  收口与度量
    范围: lifecycle-smoke 全流程;GetEvents 压测基线(事件率 vs CPU/RSS/丢失率)
          噪声评估:带 CI 类负载节点跑,确认收敛不溢出
    出口: 三场景全过 + 压测基线曲线 + 一次"裸加会误报、罕见度不会"的对比验证
          (benign-ci-noise:正常模式 0,切裸加 ≥1)
```

每个里程碑都映射到 `design-test-cases.md` 的具体用例和断言,做完即可验证。

---

## 七、范围边界:In / Out

裁剪原则:**凡不影响"主线是否成立"判断的全部后置;但所有跨层 schema 字段先定义好(哪怕 MVP 不填),避免返工。**

| 能力 | MVP | 说明 |
|---|---|---|
| Tetragon 托管 + GetEvents 消费 | ✅ | 主线起点 |
| SensorEvent → CanonicalEvent 归一 + 稳定进程 ID | ✅ | L0 |
| lineage 打标 | ✅ | 横轴索引 |
| 端侧三张表 + 六模块引擎 + 5 条规则 | ✅ | 证明 L1,含 entities/terminal |
| 本地罕见度(CMS)+ 相关族去重 | ◻ 最小 | 可恒权重起步,schema 必须就位 |
| terminal + 证据包 | ✅ | 至少 reverse_shell 一条 |
| Link1 原生 gRPC 上行 | ✅ | 端云契约最小实现 |
| 云端建图(单 host 内存图)+ 共享实体缝合 | ✅ | 证明 L2 + apt-staged-drop |
| 云端规则(图模式,3 条) | ✅ | 证明跨实体检测 |
| 收敛(罕见度+去重+因果 top-k)→ Incident | ✅ | **非裸加** |
| 证据子图裁剪 + sysarmorctl 调查视图 | ✅ | 证明可解释 |
| GetEvents 压测基线 | ✅ | M3 硬出口 |
| 响应(kill/block/quarantine) | ❌ | observe-only,只记 intent |
| Native Sensor | ❌ | 压测数据出来再决定 |
| 跨主机缝合 / Incident 合并 | ❌ | 单 host 即可证明主线 |
| 完整 NODLINK(STP + 行为嵌入) | ❌ | 穷人版收敛先替代 |
| 策略签名 / 灰度 / 回滚 | ❌ | 静态 YAML,但走 PolicyEnvelope 结构 |
| 异构源 Adapter / Link2 OTel | ❌ | 契约预留 |
| Kafka / ClickHouse / 对象存储 | ❌ | 内存图 + SQLite |
| 多租户 / mTLS enrollment | ❌ | 单租户 + 静态 token |

**技术选型**:Go(对齐 Tetragon/Elkeid 生态) · Tetragon(Day 1 复用) · gRPC+protobuf(端云强类型契约) · 内存图(Go map 邻接表) · SQLite(零运维) · YAML 规则 + Go 解释执行 · Count-Min Sketch(罕见度) · sysarmorctl CLI(调查) · systemd + K8s DaemonSet(部署)。

---

## 八、关键风险与对策

| 风险 | 影响 | 对策 |
|---|---|---|
| Tetragon 需较新内核(BTF/CO-RE) | 老内核跑不起来 | 锁定 5.15+,SensorCapability 暴露能力,兼容矩阵后置 |
| GetEvents 吞吐/丢失 | 主线数据不全 | M3 必出压测基线;SensorHealth.dropped_events 可见 |
| stable_id 在丢事件/PID 复用下断链 | 图谱错位 | hash(host,pid,start_time) + 启动扫 /proc 重建;丢失率监控 |
| 罕见度冷启动(基线未建) | 早期权重不准 | MVP 可恒权重起步,schema 就位;基线随运行积累,可导入先验白名单 |
| 内存图随长跑膨胀 | 云端 OOM | NodeRisk.ttl + 节点上限淘汰(单 host 风险低) |
| 误报淹没 Incident | 主线可用性存疑 | converge 用罕见度+结构而非裸加;M3 在 CI 类脏基线节点专门验证 |
| 穷人版收敛 vs 正式版差距 | 后期重写风险 | converge 独立成包;NodeRisk 预留 anomaly 向量字段,换 PPR/STP 不动建图与规则 |

---

## 九、不可让步的不变量(invariants)

最后,把"如果搞错就等于搭了另一个(更差的)系统"的几条钉死。它们不是细节,是地基:

| # | 不变量 | 破坏的后果 |
|---|---|---|
| I1 | 事实严格三层 Event → Signal → Incident,risk 只是 Signal 的属性 | 否则"中性积木"和"检测发现"分裂成两套引擎 |
| I2 | lineage 在 exec 时继承谱系根,盖在每条 Event 上 | 否则云端要从乱序事件重建谱系,贵且易错 |
| I3 | 端侧只维护三张表 + 两跳,**绝不建图、绝不算累计 score** | 否则端侧成本不可封顶 + 加性溢出误报 |
| I4 | 端侧每个 Signal 必带 `entities` | 否则云端无法跨 lineage / 跨主机缝合(apt-staged-drop 失败) |
| I5 | 云端收敛 = 罕见度加权 + 结构收敛,**禁止裸加阈值** | 否则忙碌良性实体撑爆阈值(benign-ci-noise 失败) |
| I6 | 三道契约(Sensor / Edge-Cloud / DetectionPolicy)先冻结再填实现 | 否则上层耦合 Tetragon / 图算法 / 具体规则,长不出生产版 |
| I7 | 所有跨层类型以 `api/proto/` 为单一事实源 | 否则端云契约漂移 |

---

## 十、一句话总结

MVP = **一个 Tetragon + 一个"打标 lineage、六模块快路径、产 terminal/证据包"的 Agent + 一个"内存建图、全局罕见度、罕见度加权+因果路径收敛、3 条云端规则"的 Manager + 一个看 Incident 的 CLI**。先冻结三道契约(Sensor / Edge-Cloud / DetectionPolicy),再在背后填最土的实现;用 `apt-fileless-c2`(响)、`apt-staged-drop`(云端图立论)、`benign-ci-noise`(收敛不裸加)三个场景证明主线端到端成立。所有跨层 schema 先定义好,收敛坚持罕见度+结构、绝不裸加 —— 这样从 MVP 到生产版,是沿契约长出去,不是推倒重来。
