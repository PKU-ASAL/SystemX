# Product Topology

结论：product topology 验证多节点产品路径。当前 `product-topology` 是 VM smoke：使用 fake sensor 验证真实 VM 部署、mTLS、systemd agent、streaming dataplane 和 manager event query；它不证明 Tetragon 真实采集或检测效果。

## VM Topology

```text
attacker -> node-a agent/sensor -> gateway -> Kafka -> worker -> manager/store
                         \-> local event/signal watch
```

| VM | 角色 |
|---|---|
| `mgr` | manager、gateway、worker、Postgres、Kafka、Redis、OpenSearch |
| `node-a` | sysarmor-agent，被保护主机；`product-topology` 使用 fake sensor smoke |
| `attacker` | C2/恶意脚本/攻击辅助 |

## Smoke 入口

```bash
make -C test product-topology
make -C test product-topology-smoke
```

这两个入口等价，都会运行 `e2e-systemd-vm.sh`：

```text
sensor: fake
expected output: health.json, metrics.json, events.json, health-after-restart.json, systemd.txt, journal.txt
expected event: rawRef=fake-startup, behavior=process.exec
expected signals/incidents: none
```

该 smoke 证明：

```text
node-a fake event -> agent batcher/sender -> gateway streaming dataplane -> manager events API
```

它不证明：

```text
Tetragon kernel telemetry
attack detection recall/precision
signal/incident quality
```

## 部署缓存

`vm-topology` 的部署输入缓存位于：

```text
test/environments/vm-topology/deploy/
```

- `platform/` 保存轻量平台部署包；
- `images/` 保存可复用 Docker 镜像包和 manifest；
- `test/.results/` 只保存本次测试输出。

## 真实场景入口

```bash
make -C test product-platform-full
```

`product-platform-full` 使用 container topology 和 Tetragon 场景，验证 event/signal/incident 查询链路。

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

`effectiveness-topology` 的主要价值是证明真实检测效果：

```text
node-a event/signal -> gateway -> worker -> manager incident/query
```

如果要评估 manager/gateway/worker/Kafka/Postgres 的资源，需要新增 platform recorder。

## 不覆盖

- 纯端侧性能基线，使用 `performance-endpoint`；
- 平台组件 microbenchmark，使用 `product-platform` 或模块 benchmark；
- manager/gateway/worker 整体资源曲线，目前尚未纳入 topology matrix。
