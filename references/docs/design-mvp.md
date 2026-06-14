# SysArmor MVP 设计

> 抽象与主线见 design-essentials.md，完整工程设计见 design.md。
> 本文用最小代价跑通主线、证明它成立，并**把贯穿数据面到控制面的 schema 定义清楚**，作为后续技术细节设计的地基。
>
> 四个部分：**一、设计思路** · **二、达成目标** · **三、目录设计** · **四、Schema 设计**。

---

## 一、设计思路

### 1.1 主线

```text
Sensor(看见/阻断) → Agent(打标 lineage + 快路径 Signal + 产 terminal/证据)
   → 上云(Event 流 + Signal 流 + 证据包)
   → 云端建图 → 云端 Signal 规则 → 罕见度加权 + 结构收敛 → Incident + 证据子图
   → 调查界面看到一条可解释的攻击链
```

一句话：**端侧在源头打标、跑有界快路径、广撒带实体键的种子；云端建溯源图、用罕见度加权 + 结构收敛裁决。**

### 1.2 MVP 阶段必须坚持的设计决断

这些是从 design-essentials 五条原理推导出的硬约束，MVP 再简也不能违背，否则证明的就不是这条主线：

| # | 决断 | 为什么不能让步 |
|---|---|---|
| D1 | 事实三层：Event → Signal → Incident | Signal 吸收"中性积木"和"检测发现"，risk 只是属性 |
| D2 | lineage 是贯穿三层的横轴索引，exec 时继承谱系根 | 源头打标最便宜（P3），云端建图免重建 |
| D3 | 端侧只维护 O(活跃实体) 的三张表 + 两跳，**不建图** | 端侧成本可封顶（P4） |
| D4 | 端侧 Signal 必须吐出 `entities`（实体 join key） | 否则云端无法跨 lineage / 跨主机缝合 |
| D5 | `terminal` 由规则声明（高置信），端侧**不算累计 score** | 端侧加性必溢出；全局判定留云端 |
| D6 | 云端收敛 = 罕见度加权 + 结构收敛，**禁止裸加阈值** | 裸加是反模式：忙/久的良性实体必然撑爆任何阈值 |
| D7 | Sensor / Edge-Cloud / DetectionPolicy 三个契约面先冻结 | 上层不依赖 Tetragon / 图算法 / 具体规则实现 |

### 1.3 MVP 对"收敛"的务实实现（穷人版，但不是裸加）

完整版是 NODLINK（行为嵌入异常分 + 在线 STP）。MVP 用可落地的近似，**数据模型已为完整版预留字段**（见 Signal.rarity / NodeRisk）：

```text
MVP 收敛 = 罕见度加权(Count-Min Sketch 频率 → IDF 权重)
         + 相关族去重(同一源动作派生的 Signal 只算一次)
         + 因果路径约束(必须在同一祖先-后代路径上，不是同连通块就算)
         + top-k 独立异常 + 最短路证据树
正式版 = 行为嵌入异常分 + Personalized PageRank 局部 push + KMB/在线 Steiner Tree
```

### 1.4 端云分工与传输

```text
端侧(Agent)  毫秒级、有界    打标 / 快路径 Signal / 本地罕见度+去重 / terminal+证据包
云端(Analytics) 秒~分钟、弹性  建图 / 云端 Signal / 全局罕见度 / 结构收敛 / 裁决

Link1  Agent ↔ Gateway   原生 gRPC + 自有 protobuf 契约（MVP：直连 manager 内置 gateway）
Link2  Manager → 外部     OTel Collector 扇出 SIEM/SOAR（MVP：Out，仅预留）
```

MVP 把 Gateway / Manager / Analytics 塌缩成一个 binary，但**按契约切好接缝**，后续按规模拆分不返工。

---

## 二、达成目标

### 2.1 唯一目标

证明主线端到端成立，且每一段可观测、可度量。

### 2.2 验收场景

`lifecycle-smoke` 是**前置冒烟**（必须先过，不是成功判据）；两个 apt 场景是**成功判据**：

| 场景 | 验证什么 | 通过标准 |
|---|---|---|
| `lifecycle-smoke`（前置） | 安装→下发静态策略→采集→可见事件/Signal→优雅卸载 | 全流程无崩溃、无资源失控 |
| `apt-fileless-c2` | **"响"的攻击**：端侧能高置信产 terminal，云端缝合 | 1 个 Incident，证据子图含 `web→shell→落盘→外联`，不被同节点运维噪声淹没 |
| `apt-staged-drop` | **云端图的立论**：跨 lineage 共享实体，端侧任一条 lineage 都不成案 | 仅当云端按"共享文件实体"缝合两条独立 lineage 才产出 1 个 Incident |

`apt-staged-drop` 是关键：它故意让落盘进程和执行进程分属**不同 lineage**（如一个写入、稍后另一个 exec 该文件），端侧两跳和单 lineage 规则都连不起来，**只有云端图按共享文件实体缝合**才能成案——这才真正验证"为什么需要云端图"。

### 2.3 范围裁剪：In / Out

| 能力 | MVP | 说明 |
|---|---|---|
| Tetragon 托管 + GetEvents 消费 | ✅ In | 主线起点 |
| SensorEvent → CanonicalEvent 归一 + 稳定进程 ID | ✅ In | L0 |
| lineage 打标 | ✅ In | 横轴索引 |
| 进程上下文表 + 谱系规则状态 + touch cache | ✅ In | 端侧三张表 |
| 端侧 Signal 引擎（六模块）+ 规则（YAML） | ✅ In | 证明 L1，含 entities/terminal |
| 本地罕见度（CMS）+ 相关族去重 | ◻ 最小 | MVP 可固定权重起步，schema 必须就位 |
| terminal + 证据包 | ✅ In | 至少 reverse_shell 一条 |
| Event/Signal/证据 上云（Link1 原生 gRPC） | ✅ In | 端云契约最小实现 |
| 云端建图（单 host、内存图）+ 共享实体缝合 | ✅ In | 证明 L2 + apt-staged-drop |
| 云端 Signal 规则（图模式，3 条） | ✅ In | 证明跨实体检测 |
| 收敛（罕见度+去重+因果路径 top-k）→ Incident | ✅ In | **非裸加** |
| 证据子图裁剪 + sysarmorctl 调查视图 | ✅ In | 证明可解释 |
| GetEvents 压测基线 | ✅ In | Phase 1 硬性出口 |
| 响应（kill/block/quarantine） | ❌ Out | observe-only，只记 response intent |
| Native Sensor | ❌ Out | 压测数据出来再决定 |
| 跨主机缝合 / Incident 合并 | ❌ Out | 单 host 即可证明主线 |
| 完整 NODLINK STP + 行为嵌入异常分 | ❌ Out | 穷人版收敛先替代 |
| 策略签名 / 灰度 / 回滚 | ❌ Out | 静态 YAML，但走 PolicyEnvelope 结构 |
| Ingestion Adapter（agentless 源） | ❌ Out | 契约预留 source_capability |
| Link2 OTel Collector → SIEM | ❌ Out | 仅预留出口 |
| Kafka / ClickHouse / 对象存储 | ❌ Out | 内存图 + SQLite |
| 多租户 / mTLS enrollment | ❌ Out | 单租户 + 静态 token |

裁剪原则：**凡不影响"主线是否成立"判断的全部后置；但所有跨层 schema 字段先定义好（哪怕 MVP 不填），避免后续返工。**

### 2.4 技术选型

| 组件 | 选型 | 理由 |
|---|---|---|
| 语言 | Go | 对齐 Tetragon / Elkeid 生态，eBPF/gRPC 工具链成熟 |
| Sensor | Tetragon | Day 1 复用，不自研 |
| Link1 传输 | gRPC + protobuf（自有契约） | 端云强类型契约，重试用 gRPC backoff，断连用本地 spool |
| 云端图 | 进程内内存图（Go map 邻接表，按 host 分区） | 单 host MVP 够用，避免引图数据库 |
| 云端存储 | SQLite（Incident / 证据 / health / 罕见度计数快照） | 零运维，后续换 ClickHouse |
| 规则 | YAML 规则 + Go 解释执行（谓词/序列/两跳/图模式子集） | 热更新内容，不发版 |
| 罕见度 | Count-Min Sketch（端侧本机 + 云端全局各一份） | 定长内存，O(1) 更新 |
| 调查视图 | sysarmorctl CLI + Mermaid/JSON 导出 | 不做完整前端 |
| 部署 | systemd + K8s DaemonSet 两种 manifest | 覆盖裸机与 K8s |

---

## 三、目录设计

```text
sysarmor/
├── cmd/
│   ├── sysarmor-agent/         # 端侧：sensor 托管 + 打标 + 快路径 + 上传
│   ├── sysarmor-manager/       # 云端单 binary（MVP 内含 gateway+analytics+control+store）
│   └── sysarmorctl/            # 调查 CLI
│
├── api/                        # ★ 所有 schema 的单一事实源（先定义、跨端云共享）
│   ├── proto/
│   │   ├── sensor/v1/          # Sensor Contract（capability/event/enforce/health）
│   │   ├── event/v1/           # CanonicalEvent
│   │   ├── signal/v1/          # Signal + EvidenceBundle
│   │   ├── analytics/v1/       # Edge-Cloud Contract（uplink/downlink streams）
│   │   ├── incident/v1/        # Incident + EvidenceSubgraph
│   │   └── policy/v1/          # PolicyEnvelope + 5 类 policy spec
│   └── schema/                 # 规则 DSL 的 JSON Schema（rule 校验用）
│
├── internal/
│   ├── sensor/
│   │   ├── contract/           # Sensor Contract 的 Go 接口（Capability/Subscribe/Enforce）
│   │   └── tetragon/           # 实现：runtime 托管 + GetEvents + policycompiler + mapper
│   │
│   ├── endpoint/
│   │   ├── context/            # ★ 三张表：proctable / lineage / touchcache
│   │   │   ├── proctable.go
│   │   │   ├── lineage.go
│   │   │   └── touchcache.go
│   │   ├── normalize/          # SensorEvent → CanonicalEvent（打 stable_id + lineage_id）
│   │   ├── fastpath/           # ★ 端侧 Signal 引擎（六模块，见 §4.6）
│   │   │   ├── match/          #   ① 无状态谓词
│   │   │   ├── sequence/       #   ② lineage 内窗口/序列状态机
│   │   │   ├── join/           #   ③ touch cache 两跳关联
│   │   │   ├── weight/         #   ④ 本地罕见度加权（CMS）
│   │   │   ├── dedup/          #   ⑤ 相关族去重
│   │   │   └── emit/           #   ⑥ 组装 Signal + terminal + 证据包
│   │   ├── evidence/           # 证据包切片（lineage 子树 + 两跳 + raw ref）
│   │   ├── ringbuffer/         # 原始事件环形缓冲（供回拉）
│   │   └── uploader/           # Link1 上行 + spool（断连续传）
│   │
│   ├── analytics/
│   │   ├── ingest/             # 上行流接收 + （预留）Ingestion Adapter
│   │   ├── graph/              # 内存溯源图（节点/边/合并/TTL）
│   │   ├── rarity/             # 全局罕见度（CMS + IDF 权重）
│   │   ├── rules/              # 云端 Signal 引擎（图模式）
│   │   ├── converge/           # ★ 收敛：去重 + 因果路径 + top-k（→ 后续 PPR/STP）
│   │   ├── incident/           # Incident 组装
│   │   └── evidence/           # 证据子图裁剪
│   │
│   ├── control/
│   │   ├── policy/             # PolicyEnvelope 加载/校验/下发（MVP 读静态文件）
│   │   └── registry/           # agent 注册（MVP 静态 token）
│   │
│   ├── transport/
│   │   ├── link1/              # Agent↔Gateway gRPC server/client
│   │   └── link2/              # （预留）OTel Collector 出口
│   │
│   └── store/                  # SQLite：incident / evidence / health / rarity 快照
│
├── configs/
│   ├── policies/               # ★ PolicyEnvelope 实例（YAML）
│   │   ├── collection/         #   CollectionPolicy（编译成 TracingPolicy）
│   │   ├── resource/           #   ResourcePolicy（端侧预算/TTL/降级）
│   │   ├── detection/          #   DetectionPolicy（规则包，含 where 路由）
│   │   ├── response/           #   ResponsePolicy（MVP：observe-only）
│   │   └── telemetry/          #   TelemetryPolicy（批/spool/优先级）
│   └── rules/                  # 规则内容（被 DetectionPolicy 引用）
│       ├── endpoint/           #   端侧规则（5 条）
│       └── cloud/              #   云端规则（3 条）
│
├── deployments/
│   ├── systemd/
│   └── helm/
│
└── test/
    └── scenarios/              # apt-fileless-c2 / apt-staged-drop / lifecycle-smoke
```

设计要点：

- **`api/` 是 schema 的单一事实源**：proto 同时生成端侧和云侧的类型，保证契约不漂移。MVP 即便单 binary 也走这套类型。
- **`endpoint/context` 三张表独立成包**：它们是端侧的全部"记忆"，边界清晰、可单独压测。
- **`fastpath` 六个子包对应六模块**：可插拔、可单独关闭（weight/dedup 在资源压力下降级）。
- **`analytics/converge` 独立**：MVP 是穷人版，正式版换 PPR/STP 时只动这个包，规则与建图不受影响。

---

## 四、Schema 设计

自底向上、从数据面到控制面。记号：`type name` + `// 注释`；`[MVP]` 必填，`[later]` 预留字段（结构在、MVP 可空）。

### 数据面

```text
┌ 控制面 ─ PolicyEnvelope（Collection/Resource/Detection/Response/Telemetry）+ Rule
├ 数据面 ─ Incident / EvidenceSubgraph        （云端裁决产物）
│         GraphNode / GraphEdge / NodeRisk     （云端图）
│         Edge-Cloud Contract（uplink/downlink）（端云传输）
│         Signal / EvidenceBundle              （端侧派生事实 + 证据）
│         ProcessContext / Lineage / TouchEntry（端侧三张表）
│         CanonicalEvent                       （归一事实 L0）
└ 底层 ── Sensor Contract（Capability/Event/Enforce/Health）（sensor→agent）
```

---

### 4.1 Sensor Contract（sensor → agent，最底层）

定义"任意 sensor 必须能提供什么"，让 Tetragon 可被 Native Sensor 替换。

```protobuf
// 能力探测：agent 启动时问 sensor "这台机器能采集/阻断什么"
message SensorCapability {
  string   sensor_kind      // "tetragon" | "native" ...        [MVP]
  string   sensor_version                                       [MVP]
  string   kernel_version                                       [MVP]
  bool     has_btf                                              [MVP]  // CO-RE 前提
  repeated string lsm_hooks // 可用 LSM 钩子（bpf/selinux...）    [MVP]
  repeated EventKind supported_events                           [MVP]
  repeated EnforceKind supported_enforce                        [MVP]  // kill/block/...
  bool     supports_enforcement                                 [MVP]
}

// sensor 中立的内核观测（尚未打 stable_id / lineage_id）
message SensorEvent {
  string   sensor_event_id                                      [MVP]
  uint64   mono_ns        // sensor 单调时钟，用于排序           [MVP]
  EventKind kind          // exec/exit/connect/open/write/...    [MVP]
  RawProcess proc         // pid, tid, ppid, binary, argv, uid, start_time, cgroup [MVP]
  RawObject  object       // path | dst_ip:port | target_pid     [MVP]
  bytes      attrs        // kind 专属字段（flags/mode/...）      [MVP]
  string     container_id // sensor 能给则给                     [MVP]
}

enum EventKind  { EXEC; EXIT; FORK; OPEN; WRITE; CHMOD; CONNECT; ACCEPT;
                  SETUID; LOAD_MODULE; MOUNT; PTRACE; ... }       [MVP 子集]
enum EnforceKind { KILL; BLOCK_CONNECT; BLOCK_FILE; QUARANTINE; } [later]

// 阻断（MVP：仅定义，observe-only 不实发）
message EnforcementCmd { string id; EnforceKind kind; Selector target; } [later]
message EnforcementAck { string id; bool applied; string error; }        [later]

// sensor 健康（丢失率是主线数据完整性的关键指标）
message SensorHealth {
  double   events_per_sec                                       [MVP]
  uint64   dropped_events    // ★ GetEvents 丢失计数             [MVP]
  uint64   queue_depth                                          [MVP]
  double   cpu_pct; uint64 rss_bytes;                           [MVP]
}
```

Go 侧契约接口（`internal/sensor/contract`）：

```go
type Sensor interface {
    Capability(ctx) (SensorCapability, error)
    Subscribe(ctx, CollectionIntent) (<-chan SensorEvent, error) // 采集意图由 CollectionPolicy 编译
    Enforce(ctx, EnforcementCmd) (EnforcementAck, error)         // MVP 返回 unsupported
    Health(ctx) (SensorHealth, error)
}
```

---

### 4.2 CanonicalEvent（L0：归一后的客观事实）

agent 的 `normalize` 把 `SensorEvent` + 打标 → `CanonicalEvent`。这是云端建图的原子记录。

```protobuf
message CanonicalEvent {
  string    id            // ULID                                [MVP]
  uint64    seq           // per-agent 单调，丢失检测用            [MVP]
  int64     time_ns       // 墙钟（mono 校准后）                  [MVP]
  EventKind kind                                                 [MVP]

  // ★ 主体进程：稳定身份（agent 打）
  ProcessRef subject_proc                                        [MVP]
  // 客体：三选一
  ObjectRef  object       // file_path | socket(5tuple) | target_proc [MVP]

  // ★ 打标结果（agent 加，sensor 给不了）
  string    parent_stable_id                                     [MVP]
  string    lineage_id    // exec 时继承谱系根                    [MVP]

  // 环境
  string    host_id                                              [MVP]
  string    container_id                                         [MVP]
  string    pod_uid                                              [later]
  string    raw_ref       // 指向 ringbuffer，供云端回拉          [MVP]

  string    source_capability  // "full"(原生 agent) | "ingest_only"(agentless) [later]
}

message ProcessRef {
  string  stable_id   // ★ = hash(host_id, pid, start_time_ns)，跨 PID 复用稳定 [MVP]
  uint32  pid                                                    [MVP]
  string  binary                                                 [MVP]
  repeated string argv                                           [MVP]
  uint32  uid                                                    [MVP]
}

message ObjectRef {
  oneof target {
    string  file_path                                            [MVP]
    Socket  socket      // src/dst ip+port, proto                [MVP]
    string  target_stable_id                                     [MVP]
  }
}
```

`stable_id` 是整套系统的进程身份基石：**云端图节点身份直接用它**，所以它必须在丢事件、PID 复用下仍稳定（启动扫 `/proc` 重建基线）。

---

### 4.3 端侧三张表（O(活跃实体)，端侧全部"记忆"）

```protobuf
// 表1：进程上下文表（∝ 活跃进程）
message ProcessContext {
  string  stable_id                                              [MVP]
  string  parent_stable_id                                       [MVP]
  string  lineage_id                                             [MVP]
  string  binary; repeated string argv; uint32 uid;             [MVP]
  int64   start_time_ns                                          [MVP]
  string  container_id; string pod_uid;                         [MVP/later]
  int64   last_seen_ns    // 用于 TTL 淘汰                       [MVP]
}

// 表2：谱系规则状态（∝ 活跃 lineage）——端侧序列/窗口引擎按此分区
message LineageState {
  string  lineage_id                                             [MVP]
  string  root_stable_id                                         [MVP]
  // 每条有状态规则在该 lineage 上的运行态
  map<string, SeqMachineState> seq_states  // rule_id → 状态机   [MVP]
  map<string, WindowCounter>   windows     // rule_id → 计数窗口 [MVP]
  int64   budget_used     // ★ per-lineage 预算，超则熔断该规则  [MVP]
  int64   last_seen_ns                                           [MVP]
}

// 表3：实体接触缓存（固定大小 LRU）——两跳关联的唯一依据
message TouchEntry {
  oneof entity { string file_path; Socket socket; }              [MVP]
  string  last_writer_stable_id   // "谁刚写了这个文件"          [MVP]
  string  last_actor_lineage_id                                  [MVP]
  EventKind last_op                                              [MVP]
  int64   ts_ns                                                  [MVP]
}
```

---

### 4.4 Signal（L1：规则派生的新事实）—— 跨端云的核心记录

这是 D4/D5 落地处：**必带 `entities`，`terminal` 由规则声明，`rarity` 为收敛预留。**

```protobuf
message Signal {
  string   name           // 规则名，如 "reverse_shell_pattern"  [MVP]
  string   rule_version                                          [MVP]
  Where    where          // ENDPOINT | CLOUD                    [MVP]
  int64    time_ns                                               [MVP]

  // 风险：基础分 + 本地罕见度权重（云端再叠全局罕见度）
  uint32   base_risk      // 0~100，规则声明                     [MVP]
  float    local_rarity   // [0,1] 本机/workload 视角的罕见度    [MVP，可先恒为1]

  // ★ 索引与缝合
  string   lineage_id                                            [MVP]
  string   subject_stable_id                                     [MVP]
  repeated EntityRef entities   // ★★ 云端 join key，必填        [MVP]

  // 引用与组合（Signal 可引用 Event 或下层 Signal）
  repeated string event_refs    // CanonicalEvent.id             [MVP]
  repeated string signal_refs   // 组合规则引用的下层 Signal      [MVP]

  // ★ terminal：规则级高置信锚点
  bool          terminal                                         [MVP]
  EvidenceBundle evidence       // 仅 terminal=true 时附带        [MVP]

  ResponseIntent response       // 仅端侧高风险 Signal，MVP 只记不发 [later]
  map<string,string> fields     // 规则产出的命名字段             [MVP]
}

enum Where { ENDPOINT; CLOUD; }

// 缝合键：云端据此把跨 lineage / 跨主机的 Signal 连到同一节点
message EntityRef {
  EntityKind kind         // PROCESS | FILE | SOCKET | TOKEN | USER | CONTAINER | POD
  string     key          // 规范化键：stable_id / 绝对路径+inode / dst_ip:port / token摘要
  string     role         // "subject" | "object" | "writer" | "reader" ...
}
```

**为什么 `entities` 必填**：`apt-staged-drop` 中，落盘 Signal 与执行 Signal 分属不同 lineage，唯一能缝起它们的就是共享的 `FILE` 实体键。没有它，云端图建不出这条边。

---

### 4.5 EvidenceBundle（端侧证据包，仅随 terminal 上行）

```protobuf
message EvidenceBundle {
  string   terminal_stable_id      // 锚点进程                   [MVP]
  // lineage 子树切片：从谱系根（或 N 级祖先）到 terminal 的链 + 直接子节点
  repeated ProcessContext lineage_slice                          [MVP]
  // 两跳邻居：terminal 经 touch cache 关联到的文件/socket
  repeated EntityRef two_hop_neighbors                           [MVP]
  repeated string raw_refs         // ringbuffer 引用，供云端按需回拉 [MVP]
  repeated Signal  contributing_signals  // 该 lineage 上触发的 Signal [MVP]
}
```

低风险 Signal **不**带 bundle（省带宽），只带 `entities` + `event_refs` 作云端候选种子。

---

### 4.6 端侧 Signal 引擎（六模块，`fastpath/`）

引擎的数据流与每个模块的输入/输出：

```text
CanonicalEvent
  │
  ├─① match    : 单事件谓词（binary/path/uid/argv/kind）         → AtomMatch
  ├─② sequence : 按 lineage_id 分区的窗口/序列状态机（读写 LineageState） → SeqMatch
  ├─③ join     : 查 TouchEntry 做两跳（写后执行 / 落盘后外联）    → JoinMatch
  ├─④ weight   : 本机罕见度 CMS，算 local_rarity（可关，降级=1.0）
  ├─⑤ dedup    : 同一源动作派生的多个匹配折叠（窗口内 hashset）
  └─⑥ emit     : 组装 Signal（填 entities/terminal/evidence/refs）
```

模块边界（决定"合理"的关键）：

```text
能在端侧做：  ①②③ 产 Signal · ④ 本机罕见度 · ⑤ 去重 · 规则级 terminal 判定
不在端侧做：  全局罕见度 · 任意跳图扩展 · 扩散/STP · 累计 score 裁决  → 全在云端
```

`emit` 输出契约（与 §4.4 Signal 对齐）：**每个 Signal 必含 `entities`；规则声明 `terminal` 的才调 `evidence` 模块切证据包。** 端侧不计算"谁是攻击"，只回答"这个点对本机而言反常吗 + 它涉及哪些实体"。

---

### 4.7 Edge-Cloud Analytics Contract（Link1，双向流）

```protobuf
// ── 上行（agent → gateway）──
message UplinkEnvelope {
  string agent_id; string tenant_id; uint64 stream_seq;          [MVP]
  oneof payload {
    CanonicalEvent event;                                        [MVP]
    Signal         signal;                                       [MVP]
    EvidenceBundle evidence;   // 也可内嵌在 terminal Signal     [MVP]
    AgentHealth    health;     // 含 SensorHealth + spool 深度    [MVP]
  }
}

// ── 下行（gateway → agent）──
message DownlinkEnvelope {
  oneof payload {
    PolicyEnvelope policy;        // 策略下发                     [MVP：静态/启动期]
    ExpandedRequest expand;       // 回拉 raw_ref 细节            [later]
    EnhancedCollection enhance;   // 临时提升某 lineage 采集粒度  [later]
    ResponseIntent response;      // 响应指令                     [later]
    HealthAck ack;                                               [MVP]
  }
}

message ExpandedRequest { repeated string raw_refs; string lineage_id; } [later]
```

MVP 上行 event/signal/evidence/health 四类；下行只需 policy 下发 + health ack。重试靠 gRPC backoff，断连靠 agent 本地 spool。

---

### 4.8 云端图与收敛产物

```protobuf
message GraphNode {
  string  node_id         // 进程=stable_id；文件/socket=规范化实体键  [MVP]
  EntityKind kind                                                [MVP]
  map<string,string> attrs                                       [MVP]
  repeated string signal_ids   // 挂在此节点的 Signal             [MVP]
  NodeRisk risk                                                  [MVP]
  int64   first_seen_ns; int64 last_seen_ns; int64 ttl_ns;       [MVP]
}

message GraphEdge {
  string  src_id; string dst_id; EdgeKind kind;  // fork/exec/write/connect/... [MVP]
  int64   first_seen_ns; uint64 merge_count;     // 边合并去重     [MVP]
}

// ★ 收敛用的节点风险——不是简单累加
message NodeRisk {
  float   anomaly_score   // 罕见度加权后的异常分（MVP: base_risk × global_rarity） [MVP]
  float   global_rarity   // 全局 CMS/IDF 给出                    [MVP]
  uint32  signal_count                                           [MVP]
  bool    is_terminal     // 端侧声明 或 云端罕见度浮出           [MVP]
  // 正式版：行为嵌入向量 / PPR 扩散得分                          [later]
}

message Incident {
  string  id; string summary; uint32 severity; repeated string mitre; [MVP]
  repeated string lineage_ids;                                   [MVP]
  repeated string terminals;     // stable_id                    [MVP]
  EvidenceSubgraph evidence;                                     [MVP]
  repeated Signal contributing_signals;                          [MVP]
  repeated string raw_refs;                                      [MVP]
  ConvergeTrace converge;        // 如何收敛出来的（可解释）      [MVP]
}

message EvidenceSubgraph { repeated GraphNode nodes; repeated GraphEdge edges; } [MVP]

// 可解释性：记录收敛依据，便于调参与复盘
message ConvergeTrace {
  repeated string seed_terminals;      // 起点                    [MVP]
  string  method;   // "rarity+causal-topk"(MVP) | "ppr+stp"(later) [MVP]
  float   score;    repeated string steiner_path_node_ids;       [MVP]
}
```

收敛逻辑（`analytics/converge`，对应 D6）：

```text
1. 每个 Signal → 给对应实体 GraphNode 叠 anomaly_score = base_risk × global_rarity
2. 相关族去重：同一 (源事件/源动作) 派生的多 Signal 只取 max
3. 取 anomaly_score top-k 节点作 seed
4. 仅当 seed 间存在【因果路径】（祖先-后代 / 共享实体边）才连
5. 抽最短路证据树 → Incident（method="rarity+causal-topk"）
   —— 绝不做"连通子图 Σrisk ≥ 阈值"
```

---

### 控制面

### 4.9 PolicyEnvelope（统一信封）+ 五类 Policy

所有策略共用信封，靠 `type` 区分 spec。MVP 从静态 YAML 读，但走完整结构，为签名/灰度预留。

```protobuf
message PolicyEnvelope {
  PolicyMeta   meta;                                             [MVP]
  PolicyTarget target;                                           [MVP]
  RolloutSpec  rollout;                                          [later]
  oneof spec {
    CollectionPolicy collection;                                 [MVP]
    ResourcePolicy   resource;                                   [MVP]
    DetectionPolicy  detection;                                  [MVP]
    ResponsePolicy   response;                                   [MVP：observe-only]
    TelemetryPolicy  telemetry;                                  [MVP]
  }
}

message PolicyMeta { string id; PolicyType type; uint64 version; string signature; } [sig: later]
message PolicyTarget {
  string scope;                 // "fleet" | "host" | "workload"
  map<string,string> selectors; // 标签选择
  string source_capability;     // "full" | "ingest_only"  ★ 能力门控 [later]
}
message RolloutSpec { string mode; uint32 percent; uint64 rollback_to; }  [later]
```

五类 spec 的关键旋钮（调参面）：

```protobuf
// ① 采集：编译成 sensor 的 CollectionIntent / TracingPolicy
message CollectionPolicy {
  repeated EventKind kinds;          // 采哪些 syscall            [MVP]
  repeated Selector  include;        // 进程/容器/路径范围         [MVP]
  repeated Selector  exclude;                                    [MVP]
  RateLimit rate_limit;              // 每类事件限速              [MVP]
  float     sample_rate;                                         [later]
}

// ② 资源：端侧成本封顶与降级（守 P4 / D3）
message ResourcePolicy {
  uint64 max_proctable_entries;                                  [MVP]
  uint64 max_lineage_states;                                     [MVP]
  uint64 touchcache_size;                                        [MVP]
  int64  lineage_budget;             // per-lineage 规则预算       [MVP]
  int64  proc_ttl_ns; int64 lineage_ttl_ns;                      [MVP]
  uint64 ringbuffer_bytes;                                       [MVP]
  uint64 spool_max_bytes;                                        [MVP]
  DegradeThresholds degrade;         // CPU/RSS 超限时关 weight/降采样 [MVP]
}

// ③ 检测：规则内容包，靠 where 路由到端/云
message DetectionPolicy {
  repeated Rule rules;                                           [MVP]
  ConvergeParams converge;           // top-k / 路径跳数上限 / 阈值 [MVP]
  RarityParams   rarity;             // CMS 宽深 / IDF 基线窗口    [MVP]
}

// ④ 响应：MVP 只授权 observe（记 intent 不实发）
message ResponsePolicy {
  enum Mode { OBSERVE; ENFORCE; }
  Mode   mode;                       // MVP 固定 OBSERVE          [MVP]
  repeated EnforceKind allowed;                                  [later]
  repeated Selector    guardrail_protect;  // 禁止误伤的关键进程  [later]
}

// ⑤ 遥测：上行行为
message TelemetryPolicy {
  uint32 batch_size; int64 flush_interval_ns;                    [MVP]
  uint32 retry_max;  int64 backoff_base_ns;                      [MVP]
  repeated EventKind priority_kinds; // 拥塞时优先上行            [MVP]
}
```

---

### 4.10 Rule（规则 schema，DetectionPolicy 的核心）

端侧规则与云端规则同一信封，靠 `where` 路由；端侧规则映射到六模块。

```yaml
# 端侧规则（where: endpoint）—— 编译进 fastpath 六模块
rule:
  id: reverse_shell_pattern
  version: "1.0"
  where: endpoint
  match:                                   # → ① match
    on: connect
    when: "proc.is_interactive_shell && proc.stdio_redirected_to_socket"
  sequence:                                # → ② sequence（可选）
    after: [web_runtime_spawns_shell, payload_dropped]
    within: 60s
    by: lineage_id
  join:                                    # → ③ join（可选）
    touch: { entity: file, op: write_then_exec }
  emit:                                    # → ⑤⑥ emit
    name: reverse_shell_pattern
    base_risk: 70
    terminal: true                         # 高置信 → 切证据包
    entities:                              # ★ 必填：云端 join key
      - { kind: process, key: "$proc.stable_id", role: subject }
      - { kind: socket,  key: "$conn.dst_ip:port", role: object }
      - { kind: file,    key: "$payload.file",     role: writer }
    event_refs: ["$event.id"]
    response: { action: kill, mode: observe }   # MVP 只记
```

```yaml
# 云端规则（where: cloud）—— 在图上匹配，可跨 lineage
rule:
  id: dropped_payload_executed_and_connects
  version: "1.0"
  where: cloud
  graph_match:                             # 图模式：进程→文件→socket，≤2 跳
    pattern: |
      (p:process)-[:exec]->(f:file {dropped:true}),
      (p)-[:connect]->(s:socket {private:false})
    cross_lineage: true                    # ★ 允许 f 由别的 lineage 写入 → apt-staged-drop
  emit:
    name: dropped_payload_executed_and_connects
    base_risk: 80
    entities:
      - { kind: process, key: "$p.stable_id", role: subject }
      - { kind: file,    key: "$f.key",       role: object }
      - { kind: socket,  key: "$s.key",       role: object }
```

字段语义总览：

| 字段 | 端侧 | 云端 | 说明 |
|---|---|---|---|
| `where` | endpoint | cloud | 路由到哪个引擎 |
| `match` / `graph_match` | 单事件谓词 | 图模式 | 端侧无图，云端有图 |
| `sequence` / `join` | ✅ | — | 端侧 lineage 内时序 + 两跳 |
| `cross_lineage` | — | ✅ | 云端可跨 lineage 按共享实体缝 |
| `emit.entities` | ★必填 | ★必填 | 云端缝合 join key |
| `emit.terminal` | 规则声明 | 罕见度浮出 | 高置信锚点 |
| `emit.base_risk` | ✅ | ✅ | 与 rarity 相乘，**不直接累加** |

---

## 五、里程碑（4 个迭代）

```text
M0  地基（端到端空管道）
    - Sensor Contract Go 接口 + tetragon 实现；GetEvents → SensorEvent
    - normalize → CanonicalEvent；proctable + lineage 打标
    - Link1 gRPC 上行 Event；云端落 SQLite；sysarmorctl 看到事件
    出口: 一条 exec 事件带正确 stable_id + lineage_id 出现在云端

M1  端侧检测（六模块 + 三张表）
    - fastpath ①②③⑥ + 5 条端侧规则（YAML）
    - touchcache 两跳；emit 带 entities；reverse_shell 产 terminal + 证据包
    - ④weight/⑤dedup 最小实现（可恒权重起步）
    出口: 复现落盘+反连，端侧产出带 entities 的 Signal 与 1 个 terminal 证据包

M2  云端图与收敛
    - 内存图按 host 建图 + 边合并 + 共享实体缝合
    - 全局 rarity（CMS）；3 条云端规则（含 cross_lineage）
    - converge: 罕见度加权 + 去重 + 因果路径 top-k → Incident + 证据子图 + ConvergeTrace
    出口: apt-fileless-c2 与 apt-staged-drop 各产 1 个 Incident，证据链完整

M3  收口与度量
    - lifecycle-smoke 全流程
    - GetEvents 压测基线（事件率 vs CPU/RSS/丢失率）
    - 噪声评估：带正常运维负载（含 CI 类）节点跑，确认收敛不溢出、误报可数
    出口: 三场景通过 + 压测基线图 + 误报计数 + 一次"裸加会误报、罕见度不会"的对比验证
```

---

## 六、关键风险与对策

| 风险 | 影响 | MVP 对策 |
|---|---|---|
| 内核兼容：Tetragon 需较新内核(BTF/CO-RE) | 老内核跑不起来 | 锁定 5.15+ 验证，SensorCapability 暴露能力，兼容矩阵后置 |
| GetEvents 吞吐/丢失 | 主线数据不全 | M3 必出压测基线；SensorHealth.dropped_events 作为 health 可见 |
| stable_id 在丢事件/PID 复用下断链 | 图谱错位 | hash(host,pid,start_time) + 启动扫 /proc 重建；丢失率监控 |
| 罕见度冷启动（基线未建立） | 早期权重不准 | MVP 可恒权重起步，schema 就位；基线随运行积累，必要时导入先验白名单 |
| 内存图随长跑膨胀 | 云端 OOM | NodeRisk.ttl + 节点上限淘汰（单 host 风险低） |
| 误报淹没 Incident | 主线"可用性"存疑 | converge 用罕见度+结构而非裸加；M3 在 CI 类脏基线节点专门验证不溢出 |
| 收敛穷人版 vs 正式版差距 | 后期重写风险 | converge 独立成包；NodeRisk 预留 anomaly 向量字段，换 PPR/STP 不动建图与规则 |

---

## 七、一句话总结

MVP = **一个 Tetragon + 一个"打标 lineage、六模块快路径、产 terminal/证据包"的 Agent + 一个"内存建图、全局罕见度、罕见度加权+因果路径收敛、3 条云端规则"的 Manager + 一个看 Incident 的 CLI**。用 `apt-fileless-c2`（响）和 `apt-staged-drop`（跨 lineage，验证云端图立论）两个场景证明主线，并以一张 GetEvents 压测基线为后续 Native Sensor 决策留依据。所有跨层 schema（Sensor Contract → CanonicalEvent → 三张表 → Signal/证据 → Edge-Cloud Contract → 图/Incident → PolicyEnvelope/Rule）先冻结，**收敛坚持罕见度+结构、绝不裸加**，响应/跨主机/STP/签名灰度在主线被证明后再加。
