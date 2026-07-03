# SysArmor Agent 性能实验与调优复盘

## 结论摘要

本轮调优的核心结论是：SysArmor agent 之前的 CPU 高占用主要不是检测规则本身造成的，而是端侧数据平面把普通 telemetry 事件走成了偏 durable 的文件队列路径，导致 `spool` 文件读写、cursor/fsync、data batch drain 在运行热路径中持续消耗 CPU。

优化后，agent 端热路径已经切换为轻量 telemetry 风格：事件和 signal 先进入内存 `TelemetryBus`，再由 batcher/sender 批量发送；本地 watch 直接从 bus 读取；旧的默认 spool/dataflow drain 热路径被移除。最新 VM topology quick 实测显示，在 `business-normal` 背景负载下执行 `apt-fileless-c2` 场景，检测闭环正常，常态和攻击相关阶段 EDR CPU 平均值保持在低个位数区间：

| 阶段 | EDR CPU avg/max | Agent CPU avg/max | Sensor CPU avg/max | EDR RSS avg/max | events | signals |
|---|---:|---:|---:|---:|---:|---:|
| steady | 1.75% / 3% | 1.00% / 2% | 0.75% / 1% | 133.03 / 133.03 MB | 0 | 0 |
| workload | 3.12% / 4% | 1.62% / 2% | 1.50% / 2% | 128.10 / 133.03 MB | 52 | 0 |
| activity | 5.50% / 6% | 2.00% / 3% | 3.50% / 4% | 117.56 / 132.32 MB | 145 | 0 |
| persistence | 4.88% / 8% | 2.25% / 4% | 2.62% / 4% | 113.05 / 114.02 MB | 179 | 4 |
| overall | 10.71% / 112% | 1.48% / 4% | 9.23% / 112% | 124.63 / 143.78 MB | 1039 | 4 |

`overall` 的 `112%` max 主要来自 `startup/policy_apply` 期间 sensor BPF reload/policy apply 的瞬时峰值，不代表 steady/workload/activity/persistence 的持续运行成本。因此，评估端侧长期 CPU/RSS 时应优先看 `vm-endpoint` 的 `medium/long`，并重点比较标准阶段，而不是只看整轮 `overall max`。

## 实验目标

本轮实验要同时回答三个问题：

| 问题 | 评价对象 | 需要的证据 |
|---|---|---|
| 端侧 agent 运行成本是否可接受 | agent + sensor 的 CPU/RSS 生命周期曲线 | `/proc` 低扰动采样、phase 汇总、长窗口均值 |
| 检测是否真实有效 | event、signal、incident 是否按预期产生 | scenario truth labels、events/signals、manager 查询结果 |
| CPU 高时到底高在哪里 | agent 内部函数级归因 | pprof/perf/strace 等 diagnostic artifacts |

第一性原理上，性能 benchmark 不能只给一个总平均值。EDR 的资源消耗具有明显生命周期：启动、策略下发、空闲保护、业务背景负载、攻击活动、攻击后观察期的成本不同。合理报告应当给出随时间变化的 CPU/RSS 曲线，并且按阶段输出平均值、峰值和事件量；当某个阶段异常升高时，再用 profiling 做根因归因。

## 实验场景设计

### 环境分层

| 环境 | 形态 | 主要用途 | 是否适合做端侧资源基线 |
|---|---|---|---|
| `vm-endpoint` | 单 VM，部署 agent/sensor | 端侧检测、CPU/RSS、profiling、soak | 是 |
| `vm-topology` | `mgr` + `node-a` + `attacker` | agent-gateway-manager-C2 真实产品链路 | 否，适合验证闭环 |
| `container` | compose/local process | manager/gateway/platform 快速集成测试 | 否 |

`vm-endpoint` 用来回答“端侧 agent 本身消耗多少”。它避免 manager、gateway、attacker、Kafka、Postgres 等拓扑噪声，适合做 long profile 和 profiling。

`vm-topology` 用来回答“真实接入 manager/gateway 后，事件、signal、incident 是否能走完整链路”。它的资源数据有参考价值，但不作为端侧资源基线。

### VM 生命周期

benchmark 默认采用 fresh 模式：

```bash
vagrant destroy -f
vagrant up
provision
sync-agent
run benchmark
```

这样每轮实验都从新的 VM 状态开始，避免历史事件、旧 policy、残留进程、缓存状态污染结果。为了调试效率，脚本保留了 reuse/debug 开关，但正式实验默认 fresh。

### Profile 长度

| profile | 目标 | 单 policy 预期耗时 | 使用建议 |
|---|---|---:|---|
| `quick` | 快速冒烟，验证 wiring 和明显回归 | 约 1 分钟，不含 fresh VM 启动成本 | PR 前快速检查 |
| `medium` | 日常检测 + 性能关联 | 约 10 分钟，不含 fresh VM 启动成本 | 对比 steady/workload/activity/persistence |
| `long` | 端侧 CPU/RSS 结论 | 约 83 分钟，不含 fresh VM 启动成本 | 正式资源结论、soak 前置 |

注意：fresh VM 的 destroy/up/provision/sync-agent 时间不属于 agent runtime 成本，但属于整轮 benchmark wall time。

### 标准 profiling phase

| phase | 含义 | 主要回答的问题 |
|---|---|---|
| `startup` | fresh VM、agent/sensor readiness、policy apply、settle | 启动和策略应用是否有异常峰值 |
| `steady` | policy 已启用，无 benchmark workload，无 attack scenario | 空闲保护成本 |
| `workload` | 只有业务背景负载 | 正常业务下的检测成本 |
| `activity` | scenario 执行窗口 | 攻击活动发生时的检测成本 |
| `persistence` | scenario 后观察窗口 | signal/incident 延迟和后续归因成本 |
| `overall` | recorder 生命周期整体 | 粗略总览，不适合作为唯一结论 |

底层 raw marker 可以更细，例如 `policy_apply_start`、`scenario_start`、`scenario_observe_start`、`profile_activity_finish_start`。报告层统一派生为上述标准 phase，减少分析口径分裂。

### Recorder 采样逻辑

Recorder 采用低扰动主采样：

| 数据 | 采样方式 | 目的 |
|---|---|---|
| CPU/RSS | 每秒从 `/proc` 读取 agent/sensor 进程 | 形成低扰动资源时间线 |
| marker | benchmark 在阶段边界写入 `markers.ndjson` | 对齐 phase |
| events/signals | 持续 watch，写入 `events.ndjson` / `signals.ndjson` | 保留检测流原始数据 |
| scope 派生 | 离线按本轮 cursor/label 生成 `events.scope.ndjson` / `signals.scope.ndjson` | 便于本轮统计 |
| health/raw | 低频 semantic 快照和原始文件 | 诊断 drop、parse error、batcher 状态 |

性能结论以 `/proc` CPU/RSS 时间线为主。pprof/runtime profile 属于诊断层，只有在需要解释“为什么高”时开启，因为 profiling 本身会改变被测 workload。

## 调优过程

### 初始症状

早期 quick/medium 实验中，`persistence` 等阶段曾出现明显高于预期的 EDR CPU，例如用户观测到过 `12.97% / 24%` 一类结果。由于该阶段没有大量复杂规则计算，直觉上不应持续高 CPU，因此需要做 agent 内部归因，而不是直接优化规则。

### Profiling 发现

`20260701T073320Z` 的 agent pprof 结果显示，CPU 累计时间集中在旧数据平面：

| 阶段 | 主要热点 | pprof 证据 |
|---|---|---|
| `activity` | `databatchworker.DrainOnce/DrainWithRetry`、`spool.LoadDataBatch`、`spool.Ack`、`spool.listLocked`、`spool.writeCursorLocked` | drain 约 40%，LoadDataBatch 约 28%，Ack 约 24%，listLocked 约 20%，writeCursor/writeFileSync 约 16% |
| `persistence` | `databatchworker.DrainOnce/DrainWithRetry`、`spool.AppendDataBatch`、`spool.LoadDataBatch`、`spool.Ack`、`spool.listLocked`、`spool.writeCursorLocked`、`spool.writeFileSync`、`spool.syncDir` | drain 约 38.35%，AppendDataBatch/listLocked 约 24.44%，LoadDataBatch 约 20.30%，Ack 约 19.92%，writeCursor 约 13.16%，writeFileSync 约 12.03% |

根因判断：

1. 普通 telemetry events 被放入 durable spool，导致每批事件都伴随文件写入、目录扫描、cursor 更新和 fsync。
2. dataflow drain worker 反复 list/load/ack batch，本质上把内存流式 telemetry 变成了文件队列轮询。
3. local watch 也依赖 spool 读取，测试观察本身会触发额外文件 IO。
4. cursor 的使用场景从“恢复/续传状态”扩展到了高频 telemetry 热路径，语义过重。

### 架构优化

优化原则是把 agent 定位回轻量实时 telemetry agent：普通 event/signal 走内存队列和批量发送，控制流和强一致确认不混入热路径。

| 优化项 | 旧逻辑 | 新逻辑 | 预期收益 |
|---|---|---|---|
| telemetry 接入 | 事件先落 spool | 事件进入 `TelemetryBus` | 减少文件写入和 fsync |
| 批量发送 | dataflow worker list/load/ack spool batch | `TelemetryBatcher` 聚合，sender 发送 | 减少目录扫描和 batch 反复读写 |
| local watch | 从 spool/cursor 派生 | 从 bus 订阅 | 观察路径不放大 IO |
| health | 旧 spool/worker 指标 | bus/batcher/drop/send 指标 | 能看到队列压力和 drop |
| 默认可靠性模型 | 普通事件 durable retry | 普通 telemetry 轻量批量，允许后续再扩展 priority/evidence | 降低端侧热路径成本 |

这次没有把 evidence 独立 frame/proto 一起做完。当前语义先简化为 `event/signal/evidence` 三层，其中 `event` 和 `signal` 的传输已走轻量 telemetry 通道；后续如果 evidence bundle 需要更强可靠性，可以单独设计 priority/evidence 通道，而不是让普通 event 重新背上 durable spool 成本。

## Benchmark 结果

### 最新真实 topology quick

实验配置：

| 项 | 值 |
|---|---|
| run id | `20260702T233121Z` |
| benchmark | `bench-topology` + endpoint case |
| policy | `collection-balanced` / `balanced-linux` v2 |
| workload | `business-normal` |
| scenario | `apt-fileless-c2` |
| VM | `mgr` + `node-a` + `attacker` |
| 数据文件 | `test/.results/bench-topology/20260702T233121Z/matrix.csv` |

资源结果：

| phase | duration | EDR CPU avg/max | Agent CPU avg/max | Sensor CPU avg/max | Agent RSS avg/max | Sensor RSS avg/max | events_delta | signals_delta |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| startup | 43s | 15.40% / 112% | 1.47% / 4% | 13.93% / 112% | 27.59 / 29.39 MB | 99.87 / 115.50 MB | 606 | 0 |
| steady | 4s | 1.75% / 3% | 1.00% / 2% | 0.75% / 1% | 28.55 / 28.55 MB | 104.48 / 104.48 MB | 0 | 0 |
| workload | 5s | 3.12% / 4% | 1.62% / 2% | 1.50% / 2% | 28.73 / 29.99 MB | 99.37 / 104.48 MB | 52 | 0 |
| activity | 4s | 5.50% / 6% | 2.00% / 3% | 3.50% / 4% | 28.42 / 28.61 MB | 89.14 / 104.48 MB | 145 | 0 |
| persistence | 5s | 4.88% / 8% | 2.25% / 4% | 2.62% / 4% | 29.02 / 29.99 MB | 84.03 / 84.03 MB | 179 | 4 |
| overall | 58s | 10.71% / 112% | 1.48% / 4% | 9.23% / 112% | 28.17 / 29.99 MB | 96.47 / 115.50 MB | 1039 | 4 |

解释：

1. `steady/workload/activity/persistence` 的 EDR CPU avg 分别为 `1.75%`、`3.12%`、`5.50%`、`4.88%`，说明热路径从文件 IO 重负载下降到低个位数。
2. `startup/overall max=112%` 由 sensor 贡献，主要发生在 policy apply/BPF reload，不应和长期运行成本混为一谈。
3. agent 进程自身在所有标准阶段 max 不超过 `4%`，说明 spool/dataflow 重构后 agent 用户态热路径明显变轻。
4. RSS 在 quick 窗口内稳定，agent RSS 约 `28-30 MB`，sensor RSS 约 `84-115 MB`。

### 检测有效性

同一 run 的 effectiveness 结果：

| 指标 | 值 |
|---|---:|
| `alert_score` | 1.0 |
| `alert_event_recall` | 1.0 |
| `alert_signal_recall` | 1.0 |
| `alert_terminal_recall` | 1.0 |
| `evidence_score` | 0.8875 |
| `evidence_event_recall` | 1.0 |
| `evidence_signal_recall` | 0.75 |
| `observed_events` | 376 |
| `observed_events_total` | 1066 |
| `observed_signals` | 4 |
| `observed_signals_total` | 4 |
| `drop_rate` | 0.0 |
| `parse_error_rate` | 0.0 |
| `signal_precision` | 1.0 |
| `signal_event_link_rate` | 1.0 |

匹配到的关键 event：

| label | 是否 required | 是否 matched |
|---|---:|---:|
| `payload_download` | true | true |
| `payload_write` | true | true |
| `payload_exec` | true | true |
| `reverse_c2` | true | true |

匹配到的关键 signal：

| signal | 目标 | 是否 matched |
|---|---|---:|
| `download_by_lolbin` | evidence | true |
| `payload_dropped` | evidence | true |
| `reverse_shell_pattern` | alert/evidence | true |
| `payload_dropped_beacon` | supplemental | true |
| `payload_lifecycle` | evidence | false |

`payload_lifecycle` 未匹配导致 evidence signal recall 为 `0.75`。这不影响 terminal alert 的成功，因为 `reverse_shell_pattern` 已产生并被 manager 查询到；但它说明 lifecycle 关联规则仍需要继续校准。

### Manager/Gateway 闭环

本轮 topology 修复并验证了完整产品路径：

| 链路 | 结果 |
|---|---|
| agent -> gateway | telemetry 可进入 gateway，mTLS health 为 true |
| gateway -> Kafka/worker | Kafka consumer group `sysarmor-ingest-worker` lag 为 0 |
| worker -> manager store | manager 可查询到 incident |
| incident 内容 | 包含 `download_by_lolbin`、`payload_dropped`、`reverse_shell_pattern`、cloud signal `dropped_payload_executed_and_connects` |

同时，为了让 topology 更接近 deployment 形态，已避免依赖 `gateway --local-ingest` 的单进程视角，改为通过部署栈打通共享持久层。

## 优化前后对照

| 维度 | 优化前 | 优化后 |
|---|---|---|
| telemetry 热路径 | spool 文件队列 + cursor/fsync + dataflow drain | bus + batcher + sender |
| watch 数据源 | 依赖 spool/cursor | 持续 watch bus stream |
| CPU 根因 | 文件 IO、目录扫描、batch 反复读写 | 主要剩 sensor policy apply/BPF reload 峰值 |
| agent 阶段 CPU | `persistence` 曾出现约 `12.97% / 24%` | 最新 topology quick 中 `persistence agent CPU 2.25% / 4%` |
| EDR 常态 CPU | 容易被 spool drain 放大 | `steady 1.75%`、`workload 3.12%`、`activity 5.50%`、`persistence 4.88%` |
| 检测流 | 能产生 event/signal，但观察链路和发送链路耦合 | event/signal 可持续 watch，manager incident 可查询 |

严格来说，上表不是同一 commit、同一 profile 长度下的 A/B 实验；它是“profiling 根因证据 + 重构后实跑观测”的工程复盘。若要形成正式性能报告，应在同一机器、同一 VM profile、同一 workload/scenario/policy 下保留优化前分支和优化后分支，各跑 `medium/long` 三次取均值和置信区间。

## 当前结果如何解读

### 可以确认的结论

1. 文件 IO 瓶颈判断成立。pprof 明确显示旧 `spool`、`databatchworker`、`writeCursor`、`writeFileSync`、`syncDir` 是主要累计 CPU 消耗来源。
2. 轻量 telemetry 数据平面方向正确。重构后 agent 用户态 CPU 在 quick topology 各标准阶段保持低位，且 event/signal/incident 链路仍然可用。
3. phase 拆分是必要的。若只看 `overall max=112%`，会误判端侧长期成本；拆开后可以看到峰值来自 `startup/policy_apply`，而 steady/workload/activity/persistence 均较低。
4. benchmark 已能同时回答性能和检测有效性问题。同一 run 中既有 CPU/RSS phase 表，也有 event/signal/effectiveness/incident 结果。

### 还不能过度推断的点

1. quick 结果不能作为最终资源结论。quick 阶段窗口只有秒级，用于 smoke 和趋势判断；正式结论应使用 `medium/long`。
2. topology 结果不能替代 endpoint 基线。topology 包含 manager/gateway/worker/infra/C2，适合验证产品链路，不适合作为纯端侧资源基线。
3. `startup` 峰值仍需单独优化。当前主要由 sensor policy apply/BPF reload 贡献，和已解决的 agent spool IO 是不同问题。
4. `payload_lifecycle` 未匹配需要继续分析规则语义和 evidence 关联窗口。

## 后续建议

优先级建议如下：

| 优先级 | 工作 | 验收标准 |
|---|---|---|
| P0 | 跑 `vm-endpoint medium`，固定 `business-normal + apt-fileless-c2 + balanced` | 输出 10 分钟窗口的 phase CPU/RSS 和 effectiveness matrix |
| P0 | 跑 `vm-endpoint long`，不启用 profiling | 形成端侧 CPU/RSS 结论基线 |
| P1 | 单独 profile `startup/policy_apply` 的 sensor 峰值 | 明确 BPF reload、policy compile、sensor restart 哪个贡献最大 |
| P1 | 修正或重新定义 `payload_lifecycle` 规则 | effectiveness signal recall 回到 1.0，或 truth label 明确不再 required |
| P1 | benchmark 自动导出 manager-side incident/Kafka lag/gateway mTLS health | topology 报告中直接看到端到端闭环指标 |
| P2 | 对优化前后做严格 A/B medium/long 对比 | 同 profile、同 workload、同 policy、同 VM fresh，多轮均值 |

## 数据索引

| 数据 | 路径 |
|---|---|
| 最新 topology matrix | `test/.results/bench-topology/20260702T233121Z/matrix.csv` |
| 最新 endpoint summary | `test/.results/bench-endpoint/20260702T233121Z/cases/workload=business-normal__scenario=apt-fileless-c2/collection-balanced/summary.json` |
| 最新 effectiveness matrix | `test/.results/effectiveness/20260702T233121Z/matrix.csv` |
| 最新 truth steps | `test/.results/effectiveness/20260702T233121Z/truth_steps.csv` |
| profiling activity top | `test/.results/bench-endpoint/20260701T073320Z/collection-balanced/profiles/activity.cpu.top.txt` |
| profiling persistence top | `test/.results/bench-endpoint/20260701T073320Z/collection-balanced/profiles/persistence.cpu.top.txt` |
| benchmark 体系说明 | `references/docs/testing-benchmark.md` |
