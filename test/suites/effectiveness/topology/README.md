# Effectiveness Topology

结论：`effectiveness-topology` 在三节点 `vm-topology` 下评估真实检测效果。它默认从 manager API 查询 event/signal/incident 作为评分输入，本地 recorder 流只用于诊断和端侧资源观察。

## 拓扑

```text
attacker -> node-a agent/sensor -> gateway -> Kafka -> worker -> manager/store
```

| VM | 角色 |
|---|---|
| `mgr` | manager、gateway、worker、Postgres、Kafka、Redis、OpenSearch |
| `node-a` | 被保护主机，运行 agent/sensor |
| `attacker` | C2/攻击辅助 |

## 推荐命令

```bash
make -C test effectiveness-topology
```

需要观察窄策略时显式指定：

```bash
make -C test effectiveness-topology \
  POLICIES='test/data/policies/collection-minimal.json'
```

`collection-minimal` 不作为默认完整检测 gate。它可能缺少部分 truth labels，这是预期的策略覆盖边界，不代表 balanced/deep 检测链路失败。

## 结果口径

| 输出 | 含义 |
|---|---|
| `test/.results/effectiveness-topology/<run-id>/matrix.csv` | topology case 汇总，含 node-a 端侧资源观察 |
| `test/.results/effectiveness/<run-id>/matrix.csv` | truth label effectiveness 指标 |
| `test/.results/effectiveness/<run-id>/truth_steps.csv` | 每个 expected label 的匹配明细 |

每个 policy case 会生成两类 telemetry：

| 文件 | 用途 |
|---|---|
| `manager.events.ndjson` / `manager.signals.ndjson` / `manager.incidents.ndjson` | 默认评分输入，来自 manager API |
| `events.scope.ndjson` / `signals.scope.ndjson` | node-a 本地 recorder 诊断输入，不作为 topology 默认评分源 |

CPU/RSS 当前只来自 `node-a` 上 agent/sensor，不是 `mgr` 平台资源，也不是三台 VM 总资源。

## Truth Labels

| 场景 | 默认预期 |
|---|---|
| `apt-fileless-c2` | payload 下载、落盘、执行、反连在同 lineage 中关联，预期 `payload_lifecycle` 和 terminal `reverse_shell_pattern` |
| `apt-staged-drop` | helper 落盘后执行并连接 C2，预期跨 lineage 的 `suspicious_exec_connect`；不要求 terminal `reverse_shell_pattern` |
| `benign-ci-noise` | 预期无恶意 signal/incident |

## 不覆盖

- 端侧长期性能基线，使用 `performance-endpoint`；
- manager/gateway/worker 平台资源曲线，未来应放 `suites/performance/platform`；
- 单模块性能，使用 `performance-rule-engine` 或 `performance-matcher`。
