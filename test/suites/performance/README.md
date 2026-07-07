# Performance Suite

结论：performance suite 回答“成本多少”。它把端侧成本和平台侧成本分开测，不负责证明完整产品功能。

## 子目录

| 目录 | 作用 |
|---|---|
| `endpoint/` | 单 VM endpoint benchmark，端侧 CPU/RSS 结论主入口 |
| `modules/` | 本地 Go microbenchmark，例如 rule engine、matcher |
| `platform/` | 三 VM topology 中采 manager/gateway/worker/infra 资源 |

## 推荐命令

```bash
make -C test performance-endpoint \
  SYSARMOR_BENCH_PROFILE=medium \
  SYSARMOR_BENCH_WORKLOAD=business-normal \
  SYSARMOR_BENCH_SCENARIO=apt-fileless-c2-local \
  SYSARMOR_BENCH_POLICIES='test/data/policies/collection-balanced.json'

make -C test performance-platform \
  SYSARMOR_PLATFORM_PERF_DURATION=120 \
  SYSARMOR_PLATFORM_PERF_INTERVAL=5

make -C test performance-rule-engine
make -C test performance-matcher
```

## 结论口径

正式端侧性能结论优先使用：

```text
vm-endpoint + medium/long + steady/workload/activity/persistence
```

`quick` 只做冒烟。profiling 只做根因诊断，不作为低扰动性能结论。

正式平台侧性能结论使用：

```text
vm-topology + performance-platform
```

平台侧采集对象是 `mgr` 上的 manager、gateway、worker、Kafka、Postgres、Redis、OpenSearch。它回答平台接入和分析链路自身的资源成本，不回答 `node-a` 上 agent/sensor 的端侧成本。
