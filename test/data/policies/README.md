# PolicyEnvelope 契约样例

本目录的 PolicyEnvelope / endpoint policy 文件是 sysarmor agent / manager 控制面的目标契约样例。

当前状态:

- agent-managed runtime 已经存在,很多 e2e 脚本会临时生成最小 `policy.yaml` 并交给 agent 使用。
- 本目录这些完整策略文件还没有形成“manager 下发 -> agent 拉取/应用 -> 版本化/启停 -> audit”的完整闭环。
- 因此它们现在主要用于设计对齐、后续 e2e 目标和手工调试,不是当前所有脚本的唯一策略来源。

## 与 TracingPolicy 的区别

| | TracingPolicy (environments/resources/syscall-capture.yaml) | PolicyEnvelope (本目录) |
|---|---|---|
| 谁读 | tetragon,`tetra tracingpolicy add` | sysarmor-agent / manager 控制面 |
| 现在能执行吗 | 能,用于 replay/debug/perf 兼容路径 | 部分能;完整下发/版本化/启停闭环未完成 |
| 性质 | 传感器采集配置 | 检测/收敛/资源/上行/响应配置 |

## 文件说明

| 文件 | 控制什么 |
|---|---|
| collection.yaml | 采集哪些事件种类 (EXEC/OPEN/CONNECT...),编译出 sensor 的 CollectionIntent |
| collection-minimal.json | 最小常开面,只保留高置信 exec、IOC 外联、payload/persistence 写入 |
| collection-balanced.json | 长期运行的 EDR baseline,只常开高价值行为:可疑 exec、IOC 外联、payload/persistence 写入 |
| collection-deep.json | 调查窗口/高风险触发策略,短时间打开 fork/exit/credential read 等高可见性采集面 |
| detection.yaml | 检测规则 + 收敛参数 (rarity_structural, top_k=8, cms_width=4096) |
| detection-additive.yaml | 对照档:把收敛切为 additive_threshold (反模式,仅 benign-ci-noise 对照实验用) |
| resource.yaml | 端侧资源上限 (RSS 512MB, lineage TTL 1min, ringbuffer 64MiB...) |
| telemetry.yaml | 上行批处理 (batch=256, flush=1s, 优先级: CONNECT/EXEC) |
| response.yaml | 响应模式 (MVP 固定 OBSERVE,只记 intent 不实发) |

## 何时完全生效

完整 policy/rule content 控制面落地后,这些文件应作为 manager 可管理、可分配、可版本化的策略内容:
- collection.yaml → agent 编译出 TracingPolicy 下发给 tetragon
- collection-balanced.json → agent local manager 路径可配合 `test/data/content/context-*.json` 和 `test/data/content/ioc-*.json` 展开 refs,作为长期运行默认面
- collection-deep.json → 由本地 manager / sysarmorctl 在告警、调查或高风险窗口中动态 apply,窗口结束后回到 balanced
- collection-minimal.json → 用于 `bench-collection-vm` 默认矩阵,代表低成本常开下界
- detection.yaml → agent 检测引擎加载收敛参数和规则引用
- detection-additive.yaml → 对照实验:替换 detection.yaml 的 converge.mode
- resource.yaml → agent 运行时守资源上限,超则降级
- telemetry.yaml → agent 上行模块配置批处理/重试
- response.yaml → agent 响应模块 (MVP 只观察,不阻断)

当前 e2e 主路径已经会验证 agent 托管 sensor、apply runtime policy、health 上报等能力;但策略内容的下发、版本化、启停和审计仍是后续测试缺口。

## 对照实验用法

detection-additive.yaml 仅在 benign-ci-noise 对照实验中使用:
正常模式 (detection.yaml, rarity_structural) → Incident=0 (不误报)
对照模式 (detection-additive.yaml, additive_threshold) → Incident≥1 (裸加误报)

这证明了"裸加是反模式,罕见度+结构收敛才能抗误报"。
