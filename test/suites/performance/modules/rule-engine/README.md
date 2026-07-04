# Module Benchmarks

结论：`test/suites/performance/modules/` 放本地模块级 microbenchmark，不启动 container 或 VM。它用于解释单个实现模块的性能，不替代 endpoint/topology 产品 benchmark。

## 适用对象

| 对象 | 示例 |
|---|---|
| detection engine | rule engine processing |
| matcher | IOC/context matcher |
| parser | telemetry/event parser |
| projection | store/index projection |

## 运行命令

```bash
make -C test performance-rule-engine
make -C test performance-matcher
BENCHTIME=1s COUNT=3 make -C test performance-rule-engine
```

## 与产品 Benchmark 的区别

| Benchmark | 环境 | 回答的问题 |
|---|---|---|
| module | 本地 Go benchmark | 某个模块实现是否快 |
| endpoint | `vm-endpoint` | 单 endpoint agent/sensor 真实成本 |
| topology | `vm-topology` | 真实产品链路和 effectiveness 是否成立 |

产品级矩阵请使用：

```bash
make -C test performance-endpoint SYSARMOR_BENCH_PROFILE=medium SYSARMOR_BENCH_WORKLOAD=business-normal
make -C test effectiveness-topology WORKLOADS='business-normal' SCENARIOS='apt-fileless-c2'
```

输出写入：

```text
test/.results/rule-engine/<run-id>/
```
