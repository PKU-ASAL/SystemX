# Platform E2E

结论：platform e2e 验证 manager、gateway、worker、store、policy、response 和 control-plane 合约。它关注平台能力是否正确，不负责端侧 CPU/RSS 性能结论。

## System Under Test

| 对象 | 说明 |
|---|---|
| manager | operator-facing HTTP API、policy、audit、query |
| gateway | agent-facing gRPC data/control endpoint |
| worker | Kafka ingest、incident projection、indexing |
| store | Postgres/OpenSearch/Redis/Kafka 相关合约 |
| control | policy/control downlink、ack、response approval/deny |

## 推荐入口

本地轻量合约：

```bash
make -C test product-platform
```

容器产品路径：

```bash
make -C test product-platform-full
```

## 与 topology 的区别

| Suite | 重点 |
|---|---|
| `platform` | 平台组件和 API/控制/存储合约 |
| `topology` | 三节点真实产品链路和 C2 场景 |

如果要验证 agent 通过 mTLS 接入 gateway 并形成 incident，优先跑 `product-topology` 或 `effectiveness-topology`。

## 不覆盖

- 单 endpoint 长窗口 CPU/RSS；
- sensor/BPF reload 性能；
- 攻击场景 effectiveness matrix；
- VM fresh benchmark 生命周期。
