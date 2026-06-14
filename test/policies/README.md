# PolicyEnvelope 契约

6 个 PolicyEnvelope 是 sysarmor agent 的配置契约。当前 agent 未构建,**不执行任何东西**。

## 与 TracingPolicy 的区别

| | TracingPolicy (env/resources/syscall-capture.yaml) | PolicyEnvelope (本目录) |
|---|---|---|
| 谁读 | tetragon,`tetra tracingpolicy add` | sysarmor-agent (未构建) |
| 现在能执行吗 | 能,正在跑 | 不能 |
| 性质 | 传感器采集配置 | 检测/收敛/资源/上行/响应配置 |

## 文件说明

| 文件 | 控制什么 |
|---|---|
| collection.yaml | 采集哪些事件种类 (EXEC/OPEN/CONNECT...),编译出 sensor 的 CollectionIntent |
| detection.yaml | 检测规则 + 收敛参数 (rarity_structural, top_k=8, cms_width=4096) |
| detection-additive.yaml | 对照档:把收敛切为 additive_threshold (反模式,仅 benign-ci-noise 对照实验用) |
| resource.yaml | 端侧资源上限 (RSS 512MB, lineage TTL 1min, ringbuffer 64MiB...) |
| telemetry.yaml | 上行批处理 (batch=256, flush=1s, 优先级: CONNECT/EXEC) |
| response.yaml | 响应模式 (MVP 固定 OBSERVE,只记 intent 不实发) |

## 何时生效

agent 二进制构建后,这些文件作为 agent 启动配置加载:
- collection.yaml → agent 编译出 TracingPolicy 下发给 tetragon
- detection.yaml → agent 检测引擎加载收敛参数和规则引用
- detection-additive.yaml → 对照实验:替换 detection.yaml 的 converge.mode
- resource.yaml → agent 运行时守资源上限,超则降级
- telemetry.yaml → agent 上行模块配置批处理/重试
- response.yaml → agent 响应模块 (MVP 只观察,不阻断)

## 对照实验用法

detection-additive.yaml 仅在 benign-ci-noise 对照实验中使用:
正常模式 (detection.yaml, rarity_structural) → Incident=0 (不误报)
对照模式 (detection-additive.yaml, additive_threshold) → Incident≥1 (裸加误报)

这证明了"裸加是反模式,罕见度+结构收敛才能抗误报"。
