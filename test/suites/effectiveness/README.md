# Effectiveness Suite

结论：effectiveness suite 回答“抓得准不准”。它基于 scenario truth labels 评估 event、signal、incident、recall、precision 和 false positive。

## 子目录

| 目录 | 作用 |
|---|---|
| `topology/` | 在 `vm-topology` 下组合 workload/scenario/policy，验证真实链路检测效果 |
| `endpoint/` | 预留，未来可放 endpoint-local truth label 检测 |

## 推荐命令

```bash
make -C test effectiveness-topology \
  ENV=vm-topology \
  POLICIES='test/data/policies/collection-balanced.json' \
  WORKLOADS='business-normal' \
  SCENARIOS='apt-fileless-c2'
```

## 结果口径

`effectiveness-topology` 的主要输出：

```text
test/.results/effectiveness-topology/<run-id>/matrix.csv
test/.results/effectiveness/<run-id>/matrix.csv
test/.results/effectiveness/<run-id>/truth_steps.csv
```

CPU/RSS 指标当前来自 `node-a` 上 agent/sensor，是检测效果过程中的端侧资源观察，不是整个平台资源报告。
