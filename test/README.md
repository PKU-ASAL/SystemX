# SysArmor Test Framework

`test/` 是 SysArmor Next 的端到端测试、场景契约、采集效果评估和性能基准目录。它的目标不是只证明某条脚本能跑通,而是回答同一个 agent 在不同拓扑、策略档位和工作负载下:

- 采到了哪些事件;
- 产出了哪些 endpoint signal / cloud signal / incident;
- 是否存在漏采、误报、drop 或 parse error;
- agent 和 sensor 的 CPU/RSS/EPS 成本是多少;
- 成本来自采集策略、业务负载、sensor runtime 还是后端处理链路。

设计原则见 `references/docs/testing-benchmark.md`:功能 E2E、workload、recorder、benchmark、diagnostic 分开维护,不要把性能采样、synthetic workload 或 perf/pprof 逻辑塞进功能断言脚本。

## Topology

同一套场景契约会在容器和 VM 两种拓扑下运行,用于验证事件采集和检测结果能跨 namespace、跨运行环境保持一致。

```text
Container topology (docker compose)       VM topology (Vagrant + libvirt)
┌──────────────────────────────┐          ┌──────────┐ ┌──────────┐
│ host kernel                   │          │ attacker │ │  node-a  │
│  └─ Docker: sysarmor-net      │          │  VM(C2)  │ │ +agent   │
│      ├─ attacker  .99         │          │          │ │ +sensor  │
│      ├─ node-a    .11         │          └──────────┘ └──────────┘
│      ├─ mgr       .10         │
│      └─ tetragon sensor       │          agent-owned Tetragon path
└──────────────────────────────┘
```

当前 endpoint refinement 的主路径是 VM 内 agent 托管真实 Tetragon:

```text
sysarmor-agent run --config ...
  -> agent-owned Tetragon + tetra getevents
  -> normalize + endpoint detection engine
  -> durable spool WAL
  -> sysarmorctl --agent-sock /var/run/sysarmor/agent.sock
  -> harness/assert-vm-local.sh
```

容器拓扑和部分平台兼容测试仍保留 manager/data-plane 路径:

```text
sysarmor-agent
  -> durable spool + data batch dispatcher
  -> sysarmor-manager AgentDataPlaneService AppendBatch(DataBatch) / analytics / store
  -> sysarmorctl JSON query
  -> harness/assert.py
```

Data append has a single transport: gRPC `AgentDataPlaneService.AppendBatch(DataBatch)`. Test fixtures use `sysarmor-databatch-append` to submit DataBatch payloads through the same data-plane service; HTTP remains only for manager query/control APIs.

`sysarmorctl --agent-sock` is a local-only side channel over Unix socket gRPC. Its watch/get tests observe the agent spool/WAL and do not exercise the cloud manager data plane. Cloud manager control behavior is covered by `AgentControlPlaneService.Connect` contract tests.

## Agent mTLS Identity

Production agent-to-manager gRPC should run with mTLS enabled on the manager:

```text
--grpc-tls-cert manager.pem
--grpc-tls-key manager-key.pem
--grpc-client-ca ca.pem
--grpc-require-client-cert
```

The agent certificate identity is bound to `tenant_id` and `agent_id`. The preferred certificate subject identity is a URI SAN:

```text
spiffe://sysarmor.local/tenant/<tenant_id>/agent/<agent_id>
```

The manager verifies that this certificate identity matches the `DataBatch.header.tenant_id/agent_id` and the `ControlFrame.context.tenant_id/agent_id`. It also records the presented certificate principal in the agent registry; the same `tenant_id/agent_id` cannot later present a different mTLS principal. Common Name formats such as `<tenant_id>/<agent_id>` are accepted only as a compatibility fallback.

`DataAck` semantics:

| Status | Cursor effect | Retry |
|---|---|---|
| `STATUS_ACCEPTED` | `committed_cursor` is safe to remove from local spool | no |
| `STATUS_DUPLICATE` | already committed; local spool can remove it | no |
| `STATUS_RETRYABLE` | keep local spool entry | yes, optionally after `retry_after_ms` |
| `STATUS_REJECTED` | terminal reject unless `retryable=true`; local worker may drop with health error | no |

Stable DataAck error classes are intentionally small. Invalid payloads return `STATUS_REJECTED` with `reason_code=invalid_data_batch` and `retryable=false`; server capacity, durability, timeout, and transient internal failures return `STATUS_RETRYABLE` with `reason_code=retryable_server_error` and a non-zero `retry_after_ms`. Authentication and mTLS identity failures remain gRPC status errors because the agent is not yet authorized to participate in the data-plane contract.

`AgentControlPlaneService.Connect` frames use `contract_version=1` and require `request_id`. Agent-to-manager sequence numbers are per stream, start at `1`, and must strictly increase. Replays return a rejected ack with `ControlError.code=AlreadyExists`; sequence gaps return `ControlError.code=FailedPrecondition`. If an agent retries the same `request_id` with a new valid sequence, the manager returns the previous response without re-running the command. Server-to-agent frames also carry per-stream monotonically increasing `sequence` values. Rejected frames carry both a `ControlAck(status="rejected")` and structured `ControlError{code,message,retryable,retry_after_ms}`.

Local `sysarmorctl --agent-sock` is intentionally separate from cloud control. It is a local Unix socket operator/debug path and can watch/query the local spool/WAL as a read-only side channel. Production manager traffic remains `AgentDataPlaneService` for data flow and `AgentControlPlaneService.Connect` for control flow.

The table-form contract is maintained in `references/docs/agent-manager-contract.md`.

Development certificates can be generated with:

```bash
tools/pki/gen-agent-plane-mtls.sh test/.results/pki default agent-a localhost
```

The mTLS smoke test covers successful append, control-plane connect/health over mTLS, forged `agent_id` rejection, missing client certificate rejection, and untrusted client CA rejection:

```bash
make -C test e2e-agent-mtls
```

## Directory Map

```text
test/
├── Makefile                  test entrypoints
├── README.md                 this document
├── SCENARIOS.md              scenario input/output contracts
│
├── env/                      topology and shared environment input
│   ├── container/
│   │   ├── compose.yaml
│   │   └── images/
│   ├── vm/
│   │   ├── Vagrantfile
│   │   └── provision/
│   └── resources/
│       ├── syscall-capture.yaml
│       └── registry-token
│
├── scenarios/                functional/security scenarios
│   ├── container/<scenario>/
│   │   ├── attack.sh
│   │   └── expected.yaml
│   └── vm/<scenario>/
│       ├── attack.sh
│       └── expected.yaml
│
├── workloads/                repeatable pressure sources, no security assertions
│   └── vm/<workload>/run.sh
│
├── policies/                 collection/detection/resource/telemetry/response samples
├── content/                  IOC/context/rulepack content used by policies
│
├── harness/                  functional orchestration and assertions
│   ├── start-*.sh
│   ├── stop-*.sh
│   ├── capture-*.sh
│   ├── assert.py
│   ├── assert-vm-local.sh
│   └── e2e-*.sh
│
├── tools/
│   ├── recorder/             long-running timeline sampler
│   ├── benchmarks/           policy/workload matrix runners and reports
│   └── diagnostics/          perf/pprof/strace helpers
│
└── .results/                 generated outputs
```

记忆方式:

| 目录 | 角色 | 说明 |
|---|---|---|
| `env/` | 环境输入 | Docker/Vagrant 拓扑、镜像、provision、共享资源 |
| `scenarios/` | 功能输入 | 攻击/良性场景脚本 + `expected.yaml` 契约 |
| `workloads/` | 压力输入 | exec/file/network/mixed/business 负载,不做安全断言 |
| `policies/` | 策略输入 | 采集、检测、资源、上行、响应策略样例 |
| `content/` | 内容输入 | IOC feed、路径上下文、endpoint rulepack |
| `harness/` | 测试代码 | 启停、采集、断言、专项 e2e |
| `tools/recorder/` | 性能采样 | CPU/RSS/EPS/drop/signal timeline |
| `tools/benchmarks/` | 矩阵评估 | policy x workload x phase 汇总 |
| `tools/diagnostics/` | 热点诊断 | perf/pprof/strace,用于解释成本 |
| `.results/` | 输出 | event/signal ndjson、summary、matrix、日志 |

## Scenario Contracts

每个 scenario 是一份功能契约:给定输入动作,系统必须或不得产出指定的 Event、Signal、Incident、Evidence 或 Response。

| 场景 | 核心命题 | 期望 |
|---|---|---|
| `apt-fileless-c2` | 单 lineage 内形成完整攻击链 | endpoint terminal signal, incident=1 |
| `apt-staged-drop` | 跨 lineage 分阶段落盘和执行 | endpoint 不应单独 terminal, cloud incident=1 |
| `benign-ci-noise` | 良性 CI 行为与攻击相似但不应成案 | incident=0 |
| `lifecycle-smoke` | agent 生命周期和基础事件可见性 | agent registered, events visible |

详见 `SCENARIOS.md`。

## Workloads

`workloads/` 是性能压力输入,不负责安全断言。每个 `run.sh` 接受类似参数:

```bash
DURATION=60 REPEAT=10 CONCURRENCY=1 ./run.sh
```

当前 VM workload:

| Workload | 用途 |
|---|---|
| `exec-storm` | 放大 process exec/fork/exit 路径 |
| `file-read-storm` | 放大 credential/secret read 路径 |
| `file-write-storm` | 放大 payload/persistence write/chmod 路径 |
| `network-connect-storm` | 放大 socket connect 路径 |
| `mixed-edr-storm` | 混合 exec/file/network,默认 sensor benchmark |
| `benign-business` | 正常构建/校验/文件活动,评估业务干扰和误报 |

## Policy And Content Inputs

`policies/` 中最常用于采集评估的是 collection policy 档位:

| Policy | 用途 |
|---|---|
| `collection-minimal-high-signal.json` | 最小常开面,低成本高置信 |
| `collection-edr-balanced.json` | 长期运行 EDR baseline |
| `collection-incident-deep.json` | 调查窗口/高风险窗口增强采集 |
| `collection-debug-wide.json` | 调试和能力边界探索,高可见性高成本 |

辅助策略:

| 文件 | 用途 |
|---|---|
| `detection.yaml` | 默认检测与收敛参数 |
| `detection-additive.yaml` | benign 对照实验,证明裸加阈值容易误报 |
| `resource.yaml` | 端侧资源预算样例 |
| `telemetry.yaml` | 上行批处理/重试策略样例 |
| `response.yaml` | 响应策略样例 |

`content/` 提供这些 policy 引用的 IOC 和上下文:

```text
context-credential-path-prefixes.json
context-payload-path-prefixes.json
context-persistence-path-prefixes.json
context-secret-volume-prefixes.json
ioc-c2-ip-feed.json
ioc-c2-port-feed.json
rulepack-cep-endpoint.json
```

## Outputs

所有生成物默认写入 `test/.results/`。

功能 E2E 常见输出:

```text
test/.results/
├── <topology>.<scenario>.json
├── <topology>.<scenario>.events.ndjson
├── <topology>.<scenario>.signals.ndjson
├── <topology>.<scenario>.local.json
└── <topology>.<scenario>.linked.json
```

Recorder 输出:

```text
test/.results/recordings/<run-id>/
├── timeline.csv       sampled CPU/RSS/EPS/drop/health counters
├── markers.ndjson     phase markers
├── events.ndjson      scoped event frames
├── signals.ndjson     scoped signal frames
├── recorder.log
└── summary.json       phase summary
```

Collection benchmark 输出:

```text
test/.results/bench-collection-vm/<run-id>/
├── matrix.csv
├── matrix.json
├── content.<name>.apply.json
└── <policy-name>/
    ├── timeline.csv
    ├── markers.ndjson
    ├── events.ndjson
    ├── signals.ndjson
    ├── summary.json
    ├── collection-apply.json
    ├── workload.out
    └── workload.err
```

Lifecycle benchmark 输出:

```text
test/.results/bench-edr-lifecycle-vm/<run-id>/
├── timeline.csv
├── markers.ndjson
├── summary.json
├── collection-apply.json
├── workload.out
└── workload.err
```

`timeline.csv` 记录 agent CPU/RSS、sensor CPU/RSS、EDR 总 CPU/RSS、events、signals、drops、parse errors、active policy 等字段。`summary.json` 会按 marker 切出 baseline、policy_apply、settle、steady、workload 等阶段。

## Common Commands

从仓库根目录运行:

```bash
# 功能 E2E: container topology
make -C test e2e TOPO=container SCENARIO=apt-fileless-c2
make -C test e2e TOPO=container SCENARIO=apt-staged-drop
make -C test e2e TOPO=container SCENARIO=benign-ci-noise

# 功能 E2E: VM topology, agent-owned sensor/local ctl path
make -C test e2e TOPO=vm SCENARIO=apt-fileless-c2
make -C test e2e TOPO=vm SCENARIO=apt-staged-drop
make -C test e2e TOPO=vm SCENARIO=benign-ci-noise

# Real Tetragon ownership smoke
make -C test e2e-agent-real-tetragon-owned-vm
make -C test e2e-agent-real-tetragon-owned-container

# Agent reliability and runtime smoke
make -C test e2e-agent-shutdown
make -C test e2e-agent-backpressure
make -C test e2e-agent-runtime-all

# Policy / response / graph / store gates
make -C test e2e-policy-all
make -C test e2e-response-all
make -C test e2e-graph-all
make -C test e2e-postgres-all

# Short performance smoke
make -C test perf TOPO=vm DUR=10
make -C test perf-resource TOPO=vm SCENARIO=idle DUR=30

# VM recorder: wrap any manual workload or E2E
make -C test recorder-vm-start RUN_ID=my-run
make -C test recorder-vm-mark RUN_ID=my-run PHASE=workload_start DETAIL=mixed-edr-storm
make -C test recorder-vm-stop RUN_ID=my-run
make -C test recorder-vm-report RUN_ID=my-run

# VM collection policy benchmark
make -C test bench-collection-vm DIAG_SCENARIO=benign-business
make -C test bench-collection-vm DIAG_SCENARIO=mixed-edr-storm
make -C test bench-collection-vm DIAG_SCENARIO=apt-fileless-c2

# Fixed policy x workload/scenario benchmark matrix, then effectiveness report
make -C test bench-matrix-vm

# Build an effectiveness report from existing scenario/benchmark outputs
make -C test effectiveness-report TOPO=vm RUN_ID=manual

# EDR lifecycle benchmark
make -C test bench-edr-lifecycle-vm DIAG_SCENARIO=mixed-edr-storm

# Run a functional E2E with recorder around it
make -C test bench-e2e-vm SCENARIO=apt-fileless-c2

# Tetragon hotspot diagnostic, not an official resource conclusion
make -C test diag-tetragon-vm
make -C test diag-tetragon-vm-workload DIAG_SCENARIO=mixed-edr-storm

# Aggregate simple reports and clean generated outputs
make -C test report
make -C test clean
```

## Evaluation Model

当前框架已经能稳定回答效率问题:

| 指标 | 来源 |
|---|---|
| agent/sensor/EDR CPU | recorder `timeline.csv` / `summary.json` |
| agent/sensor/EDR RSS | recorder `timeline.csv` / `summary.json` |
| EPS | recorder scoped event frames or event counters |
| signal count | recorder scoped signal frames |
| drops / parse errors | agent health sampled by recorder |
| policy apply spike | `policy_apply` phase |
| steady-state cost | `steady` phase |
| workload cost | `workload` phase |

效果评估当前主要由 scenario `expected.yaml` 和专项 e2e 断言完成。后续建议把效果指标也矩阵化:

```text
required_event_hit_rate
event_recall_by_kind
signal_hit_rate
terminal_signal_latency_ms
incident_hit_rate
incident_latency_ms
false_positive_count
drop_rate
parse_error_rate
cost_per_1k_events_cpu
```

当前可用入口:

```bash
make -C test bench-matrix-vm
make -C test effectiveness-report TOPO=vm RUN_ID=manual
```

`bench-matrix-vm` 固定跑:

```text
policies:
collection-minimal-high-signal
collection-edr-balanced
collection-incident-deep
collection-debug-wide

workloads:
benign-business
exec-storm
file-read-storm
file-write-storm
network-connect-storm
mixed-edr-storm

scenarios:
apt-fileless-c2
apt-staged-drop
benign-ci-noise
```

它会输出:

```text
test/.results/bench-matrix-vm/<run-id>/matrix.csv
test/.results/bench-matrix-vm/<run-id>/matrix.json
test/.results/effectiveness/<run-id>/summary.json
test/.results/effectiveness/<run-id>/matrix.csv
test/.results/effectiveness/<run-id>/policy_comparison.csv
test/.results/effectiveness/<run-id>/policy_comparison.json
```

这样可以把 policy 档位、业务工作负载和攻击场景放到同一张 matrix 中比较:采得准不准、全不全、快不快、贵不贵。

`policy_comparison.csv` 的综合分模型:

```text
overall_score = 0.6 * effectiveness_score
              + 0.3 * resource_score
              + 0.1 * stability_score

resource_score = 0.75 * inverse_cpu_score
               + 0.25 * inverse_rss_score
```

其中 `effectiveness_score` 来自 `expected.yaml` 的结构化命中结果,`resource_score` 来自 workload 阶段的 EDR CPU/RSS,`stability_score` 在 drop 和 parse error 都为 0 时为 1。
