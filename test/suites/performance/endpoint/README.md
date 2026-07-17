# Performance Endpoint

结论：`performance-endpoint` 是端侧 CPU/RSS 结论的主入口。它运行在单 VM `vm-endpoint` 上，只关注 `node-a` 上 agent/sensor 的资源占用、phase timeline 和必要的 profiling 根因。

## 采样对象

| 指标 | 来源 |
|---|---|
| agent CPU/RSS | `node-a` 上 `sysarmor-agent` 进程 |
| sensor CPU/RSS | `node-a` 上 sensor/Tetragon 进程 |
| EDR CPU/RSS | agent + sensor 汇总 |
| event/signal | `node-a` 本地 agent socket watch |
| phase | benchmark markers + lifecycle report |

## 推荐命令

```bash
make -C test performance-endpoint \
  SYSARMOR_BENCH_PROFILE=medium \
  SYSARMOR_BENCH_WORKLOAD=business-normal \
  SYSARMOR_BENCH_SCENARIO=apt-fileless-c2-local \
  SYSARMOR_BENCH_POLICIES='test/data/policies/collection-balanced.json'
```

## Profile

| Profile | 用途 |
|---|---|
| `quick` | 冒烟和明显回归 |
| `medium` | 日常检测 + 性能关联 |
| `long` | 正式端侧 CPU/RSS 结论 |

## 输出

```text
test/.results/performance-endpoint/<run-id>/
  manifest.json
  matrix.csv

test/.results/recordings/performance-endpoint/<run-id>/<policy>/
  timeline.csv
  markers.ndjson
  events.ndjson
  signals.ndjson
  summary.json
  raw/
  profiles/
```

profiling 默认关闭。需要解释某个阶段 CPU 高时再显式开启 `SYSARMOR_BENCH_PROFILE_AGENT=1`。
