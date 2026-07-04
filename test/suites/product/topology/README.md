# Product Topology

结论：product topology 验证多节点产品路径。VM 形态下是 `mgr + node-a + attacker`，其中 `node-a` 是被保护端，`mgr` 承载 manager/gateway/worker/infra，`attacker` 提供 C2/攻击辅助。

## VM Topology

```text
attacker -> node-a agent/sensor -> gateway -> Kafka -> worker -> manager/store
                         \-> local event/signal watch
```

| VM | 角色 |
|---|---|
| `mgr` | manager、gateway、worker、Postgres、Kafka、Redis、OpenSearch |
| `node-a` | sysarmor-agent、sensor/Tetragon，被保护主机 |
| `attacker` | C2/恶意脚本/攻击辅助 |

## 部署缓存

`vm-topology` 的部署输入缓存位于：

```text
test/environments/vm-topology/deploy/
```

- `platform/` 保存轻量平台部署包；
- `images/` 保存可复用 Docker 镜像包和 manifest；
- `test/.results/` 只保存本次测试输出。

## 推荐入口

```bash
make -C test product-topology
```

完整 benchmark 使用：

```bash
make -C test effectiveness-topology \
  ENV=vm-topology \
  POLICIES='test/data/policies/collection-balanced.json' \
  WORKLOADS='business-normal' \
  SCENARIOS='apt-fileless-c2'
```

## 结果口径

`effectiveness-topology` 的 CPU/RSS 指标当前来自 `node-a` 上的 agent/sensor，不是 `mgr` 上 manager/gateway/worker/infra 的资源，也不是三台 VM 的总资源。

`effectiveness-topology` 的主要价值是证明：

```text
node-a event/signal -> gateway -> worker -> manager incident/query
```

如果要评估 manager/gateway/worker/Kafka/Postgres 的资源，需要新增 platform recorder。

## 不覆盖

- 纯端侧性能基线，使用 `performance-endpoint`；
- 平台组件 microbenchmark，使用 `product-platform` 或模块 benchmark；
- manager/gateway/worker 整体资源曲线，目前尚未纳入 topology matrix。
