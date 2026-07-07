# SysArmor 测试体系细节

结论：本文是 `test/README.md` 的细节补充，说明测试目录结构、suite 边界、环境模型、输出位置和报告口径。

## 顶层结构

```text
test/
  Makefile
  README.md

  suites/
    performance/
    effectiveness/
    product/

  data/
    policies/
    scenarios/
    workloads/
    content/

  environments/
    container/
    vm-endpoint/
    vm-topology/

  shared/
    harness/
    recorder/
    reports/
    diagnostics/
    vm/
```

## 三类 Suite

| Suite | 核心问题 | 主环境 | 主输出 |
|---|---|---|---|
| `performance` | agent/sensor/platform 的资源成本是多少 | `vm-endpoint` 为主 | CPU/RSS、phase、pprof、drops |
| `effectiveness` | 攻击/良性场景是否检测正确 | `vm-topology` 为主 | manager-sourced recall、precision、truth steps、incident linkage |
| `product` | 产品功能和链路是否工作 | local/container/VM | pass/fail、API response、health、ack、incident query |

## 环境模型

| ENV | 形态 | 用途 | 性能口径 |
|---|---|---|---|
| `container` | Docker compose 轻量平台 | 快速平台/产品功能检查 | 不做端侧性能结论 |
| `vm-endpoint` | 单 VM：`node-a` | 端侧检测、CPU/RSS、profiling、soak | 端侧性能基线 |
| `vm-topology` | 三 VM：`mgr`、`node-a`、`attacker` | 完整产品链路和 C2 场景 | 当前只采 `node-a` 端侧资源 |

`vm-endpoint` 只安装 endpoint agent/sensor，agent 使用本地模式，目标是隔离 manager/gateway 噪声。

`vm-topology` 在 `mgr` 上启动 deployment-shaped platform，包括 manager、gateway、worker、Postgres、Kafka、Redis、OpenSearch；在 `node-a` 上安装 agent/sensor，并通过 agent-plane mTLS 接入 gateway；`attacker` 提供 C2/攻击辅助。

`vm-topology` 的部署输入缓存位于 `test/environments/vm-topology/deploy/`：

- `platform/`：轻量平台部署包，每次按当前仓库和本地二进制再生成；
- `images/`：Docker 镜像包和 manifest，未变化时复用，避免每轮上传 2G 级镜像包。

测试输出仍统一写入 `test/.results/`。

## 常用命令

环境生命周期：

```bash
make -C test up ENV=vm-endpoint
make -C test up ENV=vm-topology
make -C test status ENV=vm-topology
make -C test down ENV=vm-topology
make -C test down-all
```

显式别名：

```bash
make -C test up-container
make -C test up-vm-endpoint
make -C test up-vm-topology
make -C test down-container
make -C test down-vm-endpoint
make -C test down-vm-topology
```

产品功能：

```bash
make -C test product-endpoint
make -C test product-platform
make -C test product-platform-smoke
make -C test product-platform-full
make -C test product-topology
```

端侧性能：

```bash
make -C test performance-endpoint \
  SYSARMOR_BENCH_PROFILE=quick \
  SYSARMOR_BENCH_WORKLOAD=business-normal

make -C test performance-endpoint \
  SYSARMOR_BENCH_PROFILE=medium \
  SYSARMOR_BENCH_WORKLOAD=business-normal \
  SYSARMOR_BENCH_SCENARIO=apt-fileless-c2-local \
  SYSARMOR_BENCH_POLICIES='test/data/policies/collection-balanced.json'

make -C test performance-endpoint \
  SYSARMOR_BENCH_PROFILE=long \
  SYSARMOR_BENCH_WORKLOAD=business-normal \
  SYSARMOR_BENCH_POLICIES='test/data/policies/collection-balanced.json'
```

检测效果：

```bash
make -C test effectiveness-topology
```

模块性能：

```bash
make -C test performance-rule-engine
make -C test performance-matcher
```

## Make Targets 清单

所有命令从仓库根目录通过 `make -C test <target>` 执行。

### Environment Lifecycle

| Target | Purpose |
|---|---|
| `up ENV=container` | 启动容器平台环境。 |
| `up ENV=vm-endpoint` | 启动单 endpoint VM：`node-a`。 |
| `up ENV=vm-topology` | 启动三节点 VM：`mgr`、`node-a`、`attacker`，并在 `mgr` 启动 deployment stack。 |
| `up-container` / `up-vm-endpoint` / `up-vm-topology` | 上述环境的显式启动别名。 |
| `down ENV=...` | 停止指定环境。 |
| `down-container` / `down-vm-endpoint` / `down-vm-topology` | 上述环境的显式停止别名。 |
| `down-all` | Best-effort 停止 container、vm-topology、vm-endpoint，适合测试后统一清理。 |
| `status ENV=...` | 查看指定环境状态。 |
| `provision ENV=vm-endpoint|vm-topology` | 对 `node-a` 执行 rsync/provision。 |
| `clean ENV=...` | 停止环境并清理生成结果。 |

### Product Suite

| Target | Purpose |
|---|---|
| `product-endpoint` | 验证单 VM agent 和 owned real Tetragon 基础能力；不是 smoke。 |
| `product-platform` | 验证 manager/gateway/store/policy/response/control 本地合约和轻量 smoke。 |
| `product-platform-smoke` | `product-platform` 的显式 smoke 别名。 |
| `product-platform-full` | 使用 container 环境验证 gateway/worker/manager/Kafka/store 产品路径。 |
| `product-topology` | 三 VM 产品链路；manager 上传 signed agent artifact、绑定 channel、创建 enrollment，`node-a` 从 manager 下载验签安装真实 agent，并通过 CSR 自动签发证书接入 gateway/manager。 |

### Performance Suite

| Target | Purpose |
|---|---|
| `performance-endpoint SYSARMOR_BENCH_PROFILE=quick SYSARMOR_BENCH_WORKLOAD=...` | 单 VM endpoint 冒烟性能 benchmark。 |
| `performance-endpoint SYSARMOR_BENCH_PROFILE=medium SYSARMOR_BENCH_WORKLOAD=... SYSARMOR_BENCH_SCENARIO=apt-fileless-c2-local` | 单 VM endpoint 检测 + 性能关联。 |
| `performance-endpoint SYSARMOR_BENCH_PROFILE=long SYSARMOR_BENCH_WORKLOAD=...` | 单 VM endpoint 长窗口 CPU/RSS benchmark。 |
| `performance-modules` | 运行本地模块 microbenchmark。 |
| `performance-rule-engine` | 运行 endpoint detection engine microbenchmark。 |
| `performance-matcher` | 运行 matcher microbenchmark。 |

### Effectiveness Suite

| Target | Purpose |
|---|---|
| `effectiveness-topology POLICIES=... WORKLOADS=... SCENARIOS=...` | 三节点检测效果 benchmark；CPU/RSS 只采 `node-a` agent/sensor。 |
| `effectiveness-report RUN_ID=...` | 基于 topology benchmark 输出重新生成 effectiveness metrics。 |

### Diagnostics And Reports

| Target | Purpose |
|---|---|
| `diag-endpoint SYSARMOR_BENCH_WORKLOAD=...` | 在 endpoint workload 下采诊断信息。 |
| `recorder-start RUN_ID=...` | 启动 endpoint VM timeline recorder。 |
| `recorder-mark RUN_ID=... PHASE=... DETAIL=...` | 写入 recorder marker。 |
| `recorder-stop RUN_ID=...` | 停止 recorder 并拉回 timeline artifacts。 |
| `recorder-report RUN_ID=...` | 生成 recorder summary。 |
| `report` | 运行共享报告聚合入口。 |

### Capture

| Target | Purpose |
|---|---|
| `capture-endpoint SCENARIO=...` | 在 `vm-endpoint` 捕获一个场景。 |
| `capture-topology SCENARIO=...` | 在 `vm-topology` 的 `node-a` 捕获一个场景。 |

## Data Sets

| Name | Kind | Scope |
|---|---|---|
| `apt-fileless-c2` | malicious scenario | effectiveness/topology |
| `apt-fileless-c2-local` | endpoint-local malicious scenario | performance/endpoint |
| `apt-staged-drop` | malicious scenario | effectiveness/product |
| `benign-ci-noise` | benign scenario | effectiveness/product |
| `business-normal` | workload | performance/effectiveness |
| `host-activity-heavy` | workload | performance |
| `edr-activity-heavy` | workload | performance |

## Performance Suite

`suites/performance/endpoint` 是端侧 CPU/RSS 结论的主入口。它运行在 `vm-endpoint` 单机环境，采样对象是 `node-a` 上的 `sysarmor-agent` 和 sensor。

输出位置：

```text
test/.results/performance-endpoint/<run-id>/
  manifest.json
  matrix.csv
  <policy>/
    timeline.csv
    markers.ndjson
    events.ndjson
    events.scope.ndjson
    signals.ndjson
    signals.scope.ndjson
    summary.json
    raw/
    raw.tar
    profiles/
```

`suites/performance/modules` 是本地 Go microbenchmark，不启动 VM/container。

## Effectiveness Suite

`suites/effectiveness/topology` 运行在 `vm-topology` 三节点环境。它会组合 workload、scenario、policy，并默认从 manager API 查询 event/signal/incident 作为 truth label 评分输入。

默认完整检测 gate 使用：

```text
policies:  collection-balanced, collection-deep
workloads: business-normal
scenarios: apt-fileless-c2, apt-staged-drop, benign-ci-noise
```

`collection-minimal` 是窄采集策略，适合观察降级覆盖、策略边界或资源成本；它不作为默认完整检测准确性的必过策略。

关键 truth label 口径：

| 场景 | 预期 |
|---|---|
| `apt-fileless-c2` | 同 lineage 内 download/write/exec/connect 关联，产生 `payload_lifecycle` 和 terminal `reverse_shell_pattern` |
| `apt-staged-drop` | staged helper 跨 lineage 行为产生 `suspicious_exec_connect`，不要求 terminal `reverse_shell_pattern` |
| `benign-ci-noise` | 正常业务噪声不产生恶意 signal/incident |

它也会复用 endpoint recorder，因此资源指标含义是：

```text
node-a 上 agent/sensor 的 CPU/RSS
```

本地 recorder 的 `events.scope.ndjson` / `signals.scope.ndjson` 保留为诊断输入；topology 默认评分输入是每个 policy case 下的 `manager.events.ndjson`、`manager.signals.ndjson`、`manager.incidents.ndjson`。

不是：

```text
mgr + node-a + attacker 的总资源
mgr 上 manager/gateway/worker/Kafka/Postgres 的资源
```

输出位置：

```text
test/.results/effectiveness-topology/<run-id>/
  manifest.json
  matrix.csv
  cases/

test/.results/effectiveness/<run-id>/
  matrix.csv
  truth_steps.csv
```

## Product Suite

`suites/product` 验证产品功能和链路：

| 子目录 | 主要验证 |
|---|---|
| `endpoint` | agent、owned sensor、本地 socket、event/signal、health、restart/recover |
| `platform` | manager/gateway/worker/store/policy/response/control 合约 |
| `topology` | agent 通过 gateway/worker/manager 形成真实产品链路 |

产品功能测试可以短、快、确定性强；它不承担正式长窗口性能评价。

## 标准阶段

performance/effectiveness 报告统一使用：

| Phase | 含义 | 是否适合长期性能结论 |
|---|---|---|
| `startup` | fresh VM、agent/sensor readiness、policy apply、settle | 否，主要排查启动/策略应用峰值 |
| `steady` | policy 已启用，无 workload，无 scenario | 是 |
| `workload` | 只有背景业务负载 | 是 |
| `activity` | scenario 执行窗口 | 是，用于攻击活动期间成本 |
| `persistence` | scenario 后观察窗口 | 是，用于 signal/incident 延迟和后续归因 |
| `overall` | recorder 生命周期整体 | 只做总览，不能替代分阶段结论 |

正式性能结论优先使用 `vm-endpoint + medium/long`，并以 `steady/workload/activity/persistence` 为主。`quick` 和 `effectiveness-topology` 结果适合证明方向、链路和明显回归。

## Shared Reports

组合报告工具放在 `shared/reports/`：

| 文件 | 用途 |
|---|---|
| `lifecycle_report.py` | marker/phase 派生、CPU/RSS、events/signals delta |
| `effectiveness_report.py` | truth labels、recall、precision、incident/evidence 指标 |
| `local_perf_report.py` | 旧本地性能摘要辅助 |
| `local_signal_report.py` | endpoint-local signal linkage 辅助 |

这样 performance 和 effectiveness 可以共享 phase/lifecycle 能力，但入口仍保持各自语义。

## 判断原则

1. 要判断 agent/sensor 端侧性能，用 `performance-endpoint`。
2. 要判断检测效果和真实链路下的 truth label 命中，用 `effectiveness-topology`。
3. 要判断产品功能、API、控制面、部署链路是否工作，用 `product-*`。
4. 要判断 manager/gateway/worker 自身性能，需要新增 `suites/performance/platform`；当前 `effectiveness-topology` 还不采 `mgr` 侧 CPU/RSS。
5. 要解释 CPU 高的根因，再开 profiling；不要把 profiling run 当正式资源结论。
