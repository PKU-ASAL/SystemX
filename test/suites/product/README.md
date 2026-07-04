# Product Suite

结论：product suite 回答“系统通不通”。它验证 agent、gateway、manager、worker、store、control、response 等产品功能和链路，不负责长窗口性能结论，也不替代 detection effectiveness matrix。

| 子目录 | Scope | 主要验证 |
|---|---|---|
| `endpoint/` | 单 endpoint | agent、owned sensor、本地 socket、event/signal、health、restart/recover |
| `platform/` | 平台能力 | manager/gateway/worker/store/policy/response/control 合约 |
| `topology/` | 产品链路 | agent 通过 gateway/worker/manager 形成真实链路，含 VM/container 场景 |

## 运行入口

```bash
make -C test product-endpoint
make -C test product-platform
make -C test product-platform-full
make -C test product-topology
```

## 边界

| 目录 | 做什么 | 不做什么 |
|---|---|---|
| `endpoint/` | 单 endpoint 行为和本地 agent/sensor 能力 | manager incident/query、平台性能 |
| `platform/` | manager/gateway/worker/control/store 合约 | 真实攻击效果矩阵、端侧 CPU/RSS 结论 |
| `topology/` | 多节点产品路径、C2 场景、agent 接入链路 | 单 endpoint 性能基线、整个平台资源基线 |

如果一个测试需要输出 CPU/RSS、phase、matrix 或 truth-label effectiveness 报告，应优先放到 `suites/performance/` 或 `suites/effectiveness/`。
