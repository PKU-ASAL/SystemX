# Effectiveness Topology

结论：`effectiveness-topology` 在三节点 `vm-topology` 下评估检测效果。它组合 workload、scenario、policy，验证 node-a 端侧 event/signal 是否产生，并验证链路能否形成 manager incident/query。

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
make -C test effectiveness-topology \
  ENV=vm-topology \
  POLICIES='test/data/policies/collection-balanced.json' \
  WORKLOADS='business-normal' \
  SCENARIOS='apt-fileless-c2'
```

## 结果口径

| 输出 | 含义 |
|---|---|
| `test/.results/effectiveness-topology/<run-id>/matrix.csv` | topology case 汇总，含 node-a 端侧资源观察 |
| `test/.results/effectiveness/<run-id>/matrix.csv` | truth label effectiveness 指标 |
| `test/.results/effectiveness/<run-id>/truth_steps.csv` | 每个 expected label 的匹配明细 |

CPU/RSS 当前只来自 `node-a` 上 agent/sensor，不是 `mgr` 平台资源，也不是三台 VM 总资源。

## 不覆盖

- 端侧长期性能基线，使用 `performance-endpoint`；
- manager/gateway/worker 平台资源曲线，未来应放 `suites/performance/platform`；
- 单模块性能，使用 `performance-rule-engine` 或 `performance-matcher`。
