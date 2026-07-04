# Test Workloads

结论：workload 是稳定背景压力，不带安全断言；scenario 是攻击/良性安全行为，带 expected labels。两者分开，才能同时判断“正常业务下资源占用”和“攻击场景下检测有效性”。

## Workload 与 Scenario

| 类型 | 目录 | 作用 | 是否有安全断言 |
|---|---|---|---|
| workload | `test/data/workloads/` | 制造正常业务或系统活动压力 | 否 |
| scenario | `test/data/scenarios/` | 触发攻击/良性安全行为 | 是 |

## 当前 Workloads

| Workload | 目的 |
|---|---|
| `business-normal` | 常规业务噪声，适合日常 benchmark |
| `host-activity-heavy` | 主机进程和普通文件活动压力 |
| `edr-activity-heavy` | EDR 关注面活动压力，例如 exec/file/network |

## 目录约定

```text
test/data/workloads/
  vm/<workload>/run.sh
  vm/<workload>/labels.yaml
```

`run.sh` 约定：

```bash
DURATION=60 REPEAT=0 CONCURRENCY=1 ./run.sh
```

脚本只输出 stdout/stderr，不直接写 benchmark summary。Recorder 负责采样，benchmark 负责按 phase 汇总。

## Benchmark 用法

```bash
make -C test performance-endpoint \
  SYSARMOR_BENCH_PROFILE=medium \
  SYSARMOR_BENCH_WORKLOAD=business-normal

make -C test effectiveness-topology \
  WORKLOADS='business-normal' \
  SCENARIOS='apt-fileless-c2'
```
