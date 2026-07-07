# Product Suite

结论：product suite 回答“系统通不通”。它验证 agent、gateway、manager、worker、store、control、response 等产品功能和链路，不负责长窗口性能结论，也不替代 detection effectiveness matrix。

| 子目录 | Scope | 主要验证 |
|---|---|---|
| `endpoint/` | 单 endpoint | agent、owned sensor、本地 socket、event/signal、health、restart/recover |
| `platform/` | 平台能力 | manager/gateway/worker/store/policy/response/control 合约和本地 smoke |
| `topology/` | 产品链路 | VM signed artifact/channel/enrollment 安装、CSR 证书签发、mTLS 接入、gateway/manager 查询链路 |

## 运行入口

```bash
make -C test product-endpoint
make -C test product-platform
make -C test product-platform-smoke
make -C test product-platform-full
make -C test product-topology
```

## Smoke 标注

| 入口 | Smoke | Sensor | 说明 |
|---|---|---|---|
| `product-endpoint` | 否 | owned real Tetragon VM | 单 VM agent/sensor/local control 产品路径。 |
| `product-platform` | 是 | 构造数据 / fake 输入 | 本地合约和平台能力快速回归。 |
| `product-platform-smoke` | 是 | 构造数据 / fake 输入 | `product-platform` 的显式别名。 |
| `product-topology` | 否 | manager 分发真实 agent | 三 VM 产品链路，验证 signed artifact registry、channel、enrollment、CSR 证书签发、mTLS、systemd agent、gateway/manager 接入。 |
| `product-platform-full` | 否 | Tetragon container | container 场景链路，覆盖 event/signal/incident 查询。 |

## 边界

| 目录 | 做什么 | 不做什么 |
|---|---|---|
| `endpoint/` | 单 endpoint 行为和本地 agent/sensor 能力 | manager incident/query、平台性能 |
| `platform/` | manager/gateway/worker/control/store 合约 | 真实攻击效果矩阵、端侧 CPU/RSS 结论 |
| `topology/` | 多节点产品路径、agent 分发和接入链路 | 单 endpoint 性能基线、整个平台资源基线；`product-topology` 不证明真实 Tetragon 检测效果 |

如果一个测试需要输出 CPU/RSS、phase、matrix 或 truth-label effectiveness 报告，应优先放到 `suites/performance/` 或 `suites/effectiveness/`。
