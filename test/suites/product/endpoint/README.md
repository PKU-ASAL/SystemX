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

`product-endpoint` 不是 smoke：它在单 VM 上使用 agent 托管的真实 Tetragon，验证端侧产品路径。

## Smoke 子脚本

endpoint 目录里还保留一组 runtime smoke，用于快速验证 agent 生命周期、健康状态和托管 sensor 行为。它们不等价于真实检测效果测试。

| 子脚本类型 | Smoke | Sensor | 说明 |
|---|---|---|---|
| `e2e-daemon*`、`e2e-sensor-*`、`e2e-capability*`、`e2e-parse-health*`、`e2e-dropped-health*` | 是 | 构造数据 / fake 输入 | 本地 agent runtime、health、异常语义。 |
| `e2e-managed-*` | 是 | fake Tetragon bundle | 验证 agent 托管 sensor 的启动、重启、恢复。 |
| `e2e-real-tetragon-owned-*` | 否 | owned real Tetragon | 验证真实 Tetragon owned sensor 路径。 |

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
