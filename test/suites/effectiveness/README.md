# Effectiveness Suite

结论：effectiveness suite 回答“抓得准不准”。`topology` 默认从 manager 查询 event/signal/incident 做评分；本地 recorder 输出只作为端侧诊断和性能辅助。

## 子目录

| 目录 | 作用 |
|---|---|
| `topology/` | 在 `vm-topology` 下组合 workload/scenario/policy，验证真实链路检测效果 |
| `endpoint/` | 预留，未来可放 endpoint-local truth label 检测 |

## 推荐命令

```bash
make -C test effectiveness-topology
```

默认组合：

```text
policies:  collection-balanced, collection-deep
workloads: business-normal
scenarios: apt-fileless-c2, apt-staged-drop, benign-ci-noise
```

`collection-minimal` 是窄采集策略，适合做降级、成本或覆盖边界观察；它不是完整检测效果的默认 gate。

## 结果口径

`effectiveness-topology` 的主要输出：

```text
test/.results/effectiveness-topology/<run-id>/matrix.csv
test/.results/effectiveness/<run-id>/matrix.csv
test/.results/effectiveness/<run-id>/truth_steps.csv
```

每个 policy case 下会同时保留：

```text
manager.events.ndjson      manager 查询到的评分 event 输入
manager.signals.ndjson     manager 查询到的评分 signal 输入
manager.incidents.ndjson   manager 查询到的 incident 输入
events.scope.ndjson        node-a 本地 recorder 诊断流
signals.scope.ndjson       node-a 本地 recorder 诊断流
```

CPU/RSS 指标当前仍来自 `node-a` 上 agent/sensor，是检测效果过程中的端侧资源观察，不是整个平台资源报告。

## 场景口径

| 场景 | 主要预期 |
|---|---|
| `apt-fileless-c2` | 同 lineage 内 download/write/exec/connect 形成 `payload_lifecycle` 和 terminal `reverse_shell_pattern` |
| `apt-staged-drop` | staged helper 跨 lineage 行为形成 `suspicious_exec_connect`，不要求 terminal `reverse_shell_pattern` |
| `benign-ci-noise` | 正常业务噪声不应产生恶意 signal/incident |
