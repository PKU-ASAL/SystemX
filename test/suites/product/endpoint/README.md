# Product Endpoint

结论：product endpoint 验证单台被保护主机上的 agent/sensor/local control 产品能力。它不验证 manager 侧 incident/query，也不作为正式性能 benchmark。

## System Under Test

| 对象 | 说明 |
|---|---|
| `sysarmor-agent` | endpoint agent 主进程 |
| owned sensor | agent 托管的 sensor/Tetragon runtime |
| local control | `sysarmorctl --socket` 本地 Unix socket API |
| local telemetry | agent 本地 event/signal watch |
| health/recover | agent/sensor restart、recover、drop/parse health |

## 推荐入口

```bash
make -C test product-endpoint
```

性能 benchmark 使用：

```bash
make -C test performance-endpoint \
  SYSARMOR_BENCH_PROFILE=medium \
  SYSARMOR_BENCH_WORKLOAD=business-normal \
  SYSARMOR_BENCH_SCENARIO=apt-fileless-c2-local \
  SYSARMOR_BENCH_POLICIES='test/data/policies/collection-balanced.json'
```

## 适用环境

| 环境 | 用途 |
|---|---|
| `vm-endpoint` | endpoint 产品功能和性能 benchmark 的主环境 |
| `container` | 轻量兼容/冒烟路径 |

正式端侧 CPU/RSS 结论应使用 `performance-endpoint`，不是直接使用 product e2e 脚本。

## 不覆盖

- manager cloud signal；
- incident/query；
- manager/gateway/worker 资源占用；
- 多节点 attacker C2 拓扑；
- 平台控制面下发语义。
