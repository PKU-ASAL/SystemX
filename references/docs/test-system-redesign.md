# Test System Redesign

本文档定义 SysArmor Next 下一阶段测试体系的重组方案，以及 benchmark 如何扩展到真实 `agent -> gateway -> manager` 产品链路。

## 目标

结论：测试体系应该围绕“要回答的问题”组织，而不是围绕历史脚本形态组织。

重构后的测试体系需要同时满足四个目标：

1. 关注点分离：端侧功能、平台通路、真实拓扑、性能 benchmark 不互相污染。
2. 默认验证轻量：日常开发默认跑本地进程和 memory/local fixture，不强依赖 Docker、Kafka、OpenSearch 或 VM。
3. 真实链路可验证：保留完整 `agent -> gateway -> Kafka -> worker -> manager` 的产品路径测试。
4. 性能结论可信：端侧 CPU/RSS 仍以单 VM fresh 环境为准；接入 manager 的 benchmark 用于产品链路效果和延迟分析。

## 设计原则

### 第一性原则

每一类测试只回答一个主问题：

| 范围 | 主问题 | 默认环境 |
|---|---|---|
| `unit` | 本地实现是否正确？ | 无 |
| `endpoint` | 单端点 agent/sensor 是否工作，资源成本是多少？ | `vm-endpoint` |
| `platform` | gateway、manager、worker、store、API、控制流契约是否正确？ | 本地进程或 container |
| `topology` | 多节点真实产品路径和攻击场景是否成立？ | `vm-topology` |

端侧 CPU/RSS 结论必须来自 `vm-endpoint`，因为它隔离了 manager、gateway、attacker、Kafka、OpenSearch 等拓扑噪声。

接入 manager 后的测试主要回答：

- signal/incident/evidence 是否能在 manager 查询侧看到；
- 数据链路和控制链路是否闭环；
- activity 到 manager visible 的端到端延迟是多少；
- 这些产品链路动作对应的端侧资源变化是多少。

## 目录目标形态

建议将 `test/` 收敛为：

```text
test/
  Makefile
  README.md
  TEST_ITEMS.md
  data/
    content/
    policies/
    scenarios/
    workloads/
  environments/
    container/
    vm-endpoint/
    vm-topology/
  e2e/
    endpoint/
    platform/
    topology/
  benchmarks/
    endpoint/
    topology/
    matrix/
  shared/
    harness/
    recorder/
    diagnostics/
    reports/
```

### `e2e/endpoint`

定位：单端点功能验证，不关心 manager。

由旧 `e2e/local-agent` 和 `e2e/agent-runtime` 合并而来。

保留内容：

- agent daemon 启停；
- fake/tetragon sensor owned 模式；
- sensor restart/recover；
- capability 检测；
- local control socket；
- endpoint signal smoke。

删除或迁移内容：

- 与 manager、gateway、policy publish、response approval 相关的脚本迁到 `e2e/platform`。
- capture 类脚本迁到 `shared/diagnostics`，不作为标准 e2e。

### `e2e/platform`

定位：平台控制面、数据面和查询面的契约测试。

建议包含：

```text
e2e/platform/
  e2e-control-contract.sh
  e2e-manager-api-policy.sh
  e2e-manager-api-response.sh
  e2e-store-status.sh
  e2e-agent-gateway-manager-local.sh
  e2e-control-roundtrip-local.sh
  e2e-agent-gateway-worker-manager-container.sh
```

其中：

- local 测试使用本地进程、memory store、`--local-ingest`，进入默认 `test-platform`。
- container full 测试使用 Docker Compose、Postgres、Kafka、Redis、OpenSearch，进入 `test-platform-full`。

### `e2e/topology`

定位：多 VM 真实产品路径。

建议从现有 `test-topology`、`bench-topology` 和 VM 场景脚本中整理出：

- manager/gateway/agent/attacker 多节点 smoke；
- C2 场景；
- staged drop 场景；
- benign 场景；
- topology capture 诊断入口。

`topology` 不用于端侧干净 CPU/RSS 结论，只用于产品路径真实性。

### `benchmarks/endpoint`

定位：端侧性能和端侧检测效果。

保持现有原则：

- fresh `vm-endpoint`；
- `vagrant destroy -f + up + provision + sync-agent`；
- 低扰动 `/proc` CPU/RSS timeline；
- continuous events/signals watch；
- `quick|medium|long` profile；
- 标准 phase：`startup`、`steady`、`workload`、`activity`、`persistence`、`overall`。

不建议让 `bench-endpoint` 依赖 manager/gateway，否则会污染端侧成本结论。

### `benchmarks/topology`

定位：真实接入 manager 后的产品链路 benchmark。

这个 benchmark 不替代 endpoint benchmark，而是补充回答：

- manager 查询侧是否出现 expected signals；
- manager 查询侧是否生成 expected incidents；
- evidence 是否完整；
- agent/gateway/worker/manager 每段链路延迟是多少；
- 接入产品链路时端侧资源是否明显变化。

## 推荐 Make 入口

保留少量稳定入口：

```text
test-unit
test-endpoint
test-platform
test-platform-full
test-topology
test-all

bench-endpoint
bench-topology
diag-endpoint

up
down
status
clean
```

### 默认套件

`test-platform` 默认只跑快测试：

```text
control contract
manager policy API
manager response API
store status
agent -> gateway -> manager local
manager -> gateway -> agent control roundtrip local
```

`test-platform-full` 跑完整平台依赖：

```text
agent -> gateway -> Kafka -> worker -> store/opensearch -> manager
```

## 清理策略

### 建议合并

| 现状 | 目标 |
|---|---|
| 旧 `e2e/local-agent` | `e2e/endpoint` |
| 旧 `e2e/agent-runtime` | `e2e/endpoint` |
| 旧 `e2e/manager-cloud` | `e2e/platform` 和 `e2e/topology` |
| 旧 `e2e/control-plane` | `e2e/platform` |
| 旧 `e2e/storage` | `e2e/platform` |

### 建议迁移

| 现状 | 目标 |
|---|---|
| `capture-*` | `shared/diagnostics` |
| 旧 `benchmarks/perf` | 删除，资源采样以 recorder 和 endpoint benchmark 为准 |
| 历史 manager-cloud scenario 脚本 | `e2e/topology` 或 `benchmarks/topology` |

### 建议删除

删除标准：

- 已被 `bench-endpoint` recorder/profiles 覆盖的旧 resource sampler；
- 不再匹配 gateway/manager 拆分架构的 manager 直连 agent 测试；
- 与 Make 标准入口重复的手写 all runner；
- 只保留历史命名、不再被 Make 或文档引用的脚本。

删除前需要做一次引用扫描：

```bash
rg "script-name|directory-name" test references docs Makefile
```

## 新增平台通路测试

### 1. `e2e-agent-gateway-manager-local`

目的：快速验证 agent、gateway、manager 三者最小正向链路。

链路：

```text
sysarmor-agent fake sensor
  -> sysarmor-gateway gRPC
  -> local-ingest processor
  -> shared memory store
  -> sysarmor-manager HTTP query
```

断言：

- gateway `/healthz` 返回 ok；
- gateway `/metrics.accepted_batches > 0`；
- gateway `/metrics.accepted_events > 0`；
- manager `agents list` 能看到 agent；
- manager `health get` 能看到 agent health；
- manager `metrics.events_ingested > 0`。

归属：默认 `test-platform`。

### 2. `e2e-control-roundtrip-local`

目的：验证 manager 到 agent 的反向控制闭环。

链路：

```text
manager response API
  -> store pending command
  -> gateway control stream
  -> agent receive command
  -> agent ack
  -> manager query acked state
```

断言：

- response command 创建成功；
- agent control stream 收到 command；
- agent ack 成功；
- manager 查询状态变为 sent/acked；
- gateway control session 可观测指标增长。

归属：默认 `test-platform`。

### 3. `e2e-agent-gateway-worker-manager-container`

目的：验证真实拆分数据面。

链路：

```text
agent
  -> gateway gRPC
  -> Kafka raw databatch
  -> worker ingest
  -> store / opensearch
  -> manager query API
```

断言：

- gateway accepted metrics 增长；
- worker 消费成功；
- manager 能查询到 events/signals；
- 若 scenario 触发，manager 能查询到 incident/evidence；
- gateway/worker/manager 日志无明显 error。

归属：`test-platform-full`，不进入默认 quick platform。

## Benchmark 接入 Manager 的设计

### 分工

结论：保留 `bench-endpoint` 的纯端侧定位，新建或强化 `bench-topology` 的产品链路定位。

| Benchmark | 回答的问题 | 环境 |
|---|---|---|
| `bench-endpoint` | agent/sensor 在宿主机上的真实 CPU/RSS，以及端侧 expected signals | `vm-endpoint` |
| `bench-topology` | 真实接入 manager 后 signals/incidents/evidence 是否出现，以及端到端延迟 | `vm-topology` 或 container full |

### `bench-topology` 输出结构

建议输出：

```text
test/.results/bench-topology/<run-id>/
  manifest.json
  matrix.csv
  cases/
    <policy>/<workload>/<scenario>/
      manifest.json
      endpoint-summary.json
      gateway-metrics.json
      worker-metrics.json
      manager-signals.ndjson
      manager-incidents.ndjson
      manager-evidence.ndjson
      timeline.csv
      markers.ndjson
      raw/
```

### 指标分组

#### 端侧资源

- agent CPU/RSS by phase；
- sensor CPU/RSS by phase；
- edr CPU/RSS by phase；
- events delta；
- signals delta；
- dropped events；
- parse errors。

#### 数据链路

- gateway accepted batches/events/signals；
- gateway rejected/duplicate/handoff errors；
- Kafka raw databatch append 成功数；
- worker consumed batches/events/signals；
- manager query visible signals/incidents；
- OpenSearch evidence/query 可见性。

#### 检测效果

- expected signals 命中率；
- expected incidents 命中率；
- expected evidence 是否完整；
- benign scenario false positive；
- signal/incident severity 和 rule id 是否符合预期。

#### 延迟

建议记录链路时间线：

```text
activity_start
agent_signal_observed
gateway_batch_accepted
worker_batch_consumed
manager_signal_visible
manager_incident_visible
manager_evidence_visible
```

这些时间点用于计算：

- endpoint detection latency；
- gateway handoff latency；
- worker ingest latency；
- manager visibility latency；
- incident convergence latency。

### Phase 语义

继续沿用标准 phase：

```text
startup
steady
workload
activity
persistence
overall
```

但 `bench-topology` 需要额外保留产品链路 markers。标准 phase 用于资源汇总，链路 markers 用于延迟分析。

### 数据采集方式

端侧资源仍使用 recorder：

- `/proc` 采样 agent/sensor CPU/RSS；
- low-frequency health snapshots；
- continuous event/signal watch；
- raw markers。

manager 侧采集使用查询导出：

- activity/persistence 后按 run labels 查询 manager signals；
- 查询 manager incidents；
- 查询 evidence；
- 拉取 gateway/worker metrics；
- 保存原始 JSON/NDJSON，不先生成 HTML 报告。

### 标签要求

所有 agent 事件、signal、incident 都应带上：

```text
benchmark_run
workload
scenario
policy_profile
policy_id
policy_version
vm_env
```

`bench-topology` 只基于 label scope 做归因，不依赖全局时间窗口猜测。

## 实施顺序

### 阶段 1：目录和入口收敛

目标：不改测试语义，只改命名、位置和 Make 入口。

验收：

- `make -C test test-platform` 通过；
- `make -C test test-endpoint` 通过；
- `make -C test bench-endpoint SYSARMOR_BENCH_PROFILE=quick ...` 通过；
- `rg "manager-cloud|local-agent|agent-runtime|control-plane|storage" test/README.md test/TEST_ITEMS.md test/Makefile` 不再出现旧分类作为一等入口。

### 阶段 2：补 platform local 通路

新增：

- `e2e-agent-gateway-manager-local`；
- `e2e-control-roundtrip-local`。

验收：

- 默认 `test-platform` 覆盖正向数据链路和反向控制链路；
- 不依赖 Docker、Kafka、OpenSearch；
- 运行时间保持在开发可接受范围。

### 阶段 3：补 platform full 通路

新增：

- `test-platform-full`；
- `e2e-agent-gateway-worker-manager-container`。

验收：

- Docker Compose 拆分拓扑能跑通；
- gateway、worker、manager 三段可观测指标都能导出；
- manager 查询侧能看到由 agent 产生的数据。

### 阶段 4：扩展 topology benchmark

目标：把 manager signals/incidents/evidence 纳入 benchmark 原始数据。

验收：

- `bench-topology` 输出 manager-side raw artifacts；
- matrix 同时包含端侧资源、检测效果、链路延迟；
- 不影响 `bench-endpoint` 的端侧性能结论语义。

## 风险和取舍

### 不把 full platform 放进默认套件

原因：Kafka、OpenSearch、Docker Compose 启动成本高，容易让日常开发验证变慢。

默认套件应优先稳定和快速；full suite 用于合并前、夜间或专项验证。

### 不让 endpoint benchmark 强依赖 manager

原因：端侧 CPU/RSS 是宿主机成本结论，manager 接入会引入网络、后端、控制流和拓扑噪声。

接入 manager 的资源分析应作为 product-path benchmark，而不是 endpoint baseline。

### 保留 raw artifacts 优先

现阶段不优先做 HTML 报告。

所有测试和 benchmark 先保存原始数据：

- JSON；
- NDJSON；
- CSV；
- logs；
- markers。

报告展示可以后续基于这些稳定 artifacts 增量实现。
