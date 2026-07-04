# Shared Test Utilities

结论：`test/shared/` 只放跨 suite 复用的工具。测试断言放在 `test/suites/<suite>/`，performance/effectiveness 编排放在对应 suite，具体产品语义不要塞进 shared。

## 目录职责

| 目录 | 职责 |
|---|---|
| `assertions/` | 复用断言和本地 capture 检查 |
| `diagnostics/` | perf/pprof/strace/Tetragon 诊断辅助 |
| `fixtures/` | 合成事件、场景 fixture 生成 |
| `harness/` | container/VM 生命周期和通用 shell glue |
| `recorder/` | VM timeline recorder，采 CPU/RSS、events、signals、markers |
| `reports/` | 报告聚合和本地 effectiveness 辅助 |
| `vm/` | VM 维护，例如同步当前 agent/ctl 二进制 |

## 使用原则

1. shared helper 应保持通用，不绑定某个 scenario 的 pass/fail 语义。
2. 环境启动/停止/等待/采集放 `harness/`。
3. 端侧性能采样放 `recorder/`。
4. suite 级断言放 `test/suites/<suite>/`。
5. phase/effectiveness 等共享报告工具放 `test/shared/reports/`。

## 常用入口

```bash
make -C test up ENV=vm-endpoint
make -C test up ENV=vm-topology
make -C test recorder-start RUN_ID=my-run
make -C test recorder-mark RUN_ID=my-run PHASE=workload_start DETAIL=business-normal
make -C test recorder-stop RUN_ID=my-run
make -C test recorder-report RUN_ID=my-run
```

## Recorder 口径

`recorder-vm.sh` 当前通过 `vagrant ssh node-a` 采样。因此：

| 环境 | Recorder 采样对象 |
|---|---|
| `vm-endpoint` | `node-a` 上 agent/sensor |
| `vm-topology` | `node-a` 上 agent/sensor |

它不采 `mgr` 上 manager/gateway/worker/Kafka/Postgres 的资源。如果需要平台资源曲线，应新增 platform recorder，而不是扩展 endpoint recorder 的语义。

## Module Benchmarks

本地模块 benchmark 不需要 container 或 VM：

```bash
make -C test performance-rule-engine
make -C test performance-matcher
BENCHTIME=1s COUNT=3 make -C test performance-rule-engine
```

输出写入：

```text
test/.results/rule-engine/<run-id>/
```
