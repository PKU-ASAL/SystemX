# SysArmor Test Framework

`test/` 是 SysArmor Next 的端到端测试、场景契约、采集效果评估和性能基准目录。它的目标不是只证明某条脚本能跑通,而是回答同一个 agent 在不同拓扑、策略档位和工作负载下:

- 采到了哪些事件;
- 产出了哪些 endpoint signal / cloud signal / incident;
- 是否存在漏采、误报、drop 或 parse error;
- agent 和 sensor 的 CPU/RSS/EPS 成本是多少;
- 成本来自采集策略、业务负载、sensor runtime 还是后端处理链路。

设计原则见 `references/docs/testing-benchmark.md` 和 `test/ARCHITECTURE.md`:功能 E2E、workload、recorder、benchmark、diagnostic 分开维护,不要把性能采样、synthetic workload 或 perf/pprof 逻辑塞进功能断言脚本。

`suite` 和 `harness` 是两层概念:

- `suites/` 定义测什么、SUT 是谁、评估边界是什么、哪些指标算分;
- `harness/` 提供怎么跑的通用能力,例如启动/停止拓扑、等待服务和清理环境;
- `tools/` 放 recorder、report、benchmark、diagnostic 这类可复用工具。

当前稳定边界是:产品 E2E 入口在 `suites/`,可复用断言/报告/benchmark/fixture 在 `tools/`,通用执行胶水在 `harness/`。

效果评估分两条线:功能断言继续使用 `expected.yaml`; benchmark effectiveness 使用 `labels.yaml` 作为 ground truth,将 workload 窗口内 observed events/signals 与标签匹配,输出 event/signal precision/recall。

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
  -> sysarmorctl --socket /var/run/sysarmor/agent.sock
  -> tools/assertions/assert-vm-local.sh
```

容器拓扑和部分平台兼容测试仍保留 manager/data-plane 路径:

```text
sysarmor-agent
  -> durable spool + data batch dispatcher
  -> sysarmor-manager AgentDataPlaneService AppendBatch(DataBatch) / analytics / store
  -> sysarmorctl manager ... JSON query
  -> tools/assertions/assert.py
```

Data append has a single transport: gRPC `AgentDataPlaneService.AppendBatch(DataBatch)`. Test fixtures use `sysarmor-databatch-append` to submit DataBatch payloads through the same data-plane service; HTTP remains only for manager query/control APIs.

`sysarmorctl --socket` is a local-only side channel over Unix socket gRPC. Its watch/get tests observe the agent spool/WAL and do not exercise the cloud manager data plane. Cloud manager control behavior is covered by `AgentControlPlaneService.Connect` contract tests.

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

Stable DataAck error classes are intentionally small. Duplicate batches return `STATUS_DUPLICATE` with `reason_code=duplicate` and are treated as committed. Invalid payloads return `STATUS_REJECTED` with `reason_code=invalid_data_batch` and `retryable=false`; server capacity, durability, timeout, and transient internal failures return `STATUS_RETRYABLE` with `reason_code=retryable_server_error` and a non-zero `retry_after_ms`; non-retryable server-side rejections return `STATUS_REJECTED` with `reason_code=server_error`. Authentication and mTLS identity failures remain gRPC status errors because the agent is not yet authorized to participate in the data-plane contract.

`AgentControlPlaneService.Connect` frames use `contract_version=1` and require `request_id`. Agent-to-manager sequence numbers are per stream, start at `1`, and must strictly increase. Replays return a rejected ack with `ControlError.code=AlreadyExists`; sequence gaps return `ControlError.code=FailedPrecondition`. If an agent retries the same `request_id` with a new valid sequence, the manager returns the previous response without re-running the command. Server-to-agent frames also carry per-stream monotonically increasing `sequence` values. Rejected frames carry both a `ControlAck(status="rejected")` and structured `ControlError{code,message,retryable,retry_after_ms}`.

Local `sysarmorctl --socket` is intentionally separate from cloud control. It is a local Unix socket operator/debug path and can watch/query the local spool/WAL as a read-only side channel. Production manager traffic remains `AgentDataPlaneService` for data flow and `AgentControlPlaneService.Connect` for control flow.

`sysarmorctl` keeps local agent operations at the top level (`agent`, `policy`, `content`, `event`, `signal`). Manager HTTP administration is explicit under `manager`, for example `sysarmorctl manager policies assign --agent agent-a --policy-id edr-balanced --version 3 --downlink` or `sysarmorctl manager control-commands create content --agent agent-a --file ioc.json`. Policy publish/assignment APIs represent desired state; a persisted `ControlCommand` is created only when an operator explicitly asks for downlink delivery.

Common manager query examples:

```bash
sysarmorctl --manager-url http://127.0.0.1:9443 --json manager agents list
sysarmorctl --manager-url http://127.0.0.1:9443 --json manager health get --agent-id agent-a --tenant-id default
sysarmorctl --manager-url http://127.0.0.1:9443 --json manager signals list --label scenario=apt-fileless-c2 --layer endpoint
sysarmorctl --manager-url http://127.0.0.1:9443 --json manager control-commands cancel --command-id ctrl-a --agent agent-a --reason "bad rollout"
sysarmorctl --manager-url http://127.0.0.1:9443 --json manager roles upsert --actor alice --roles policy_admin,control_admin
```

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
├── ARCHITECTURE.md           suite/harness/tool boundaries
│
├── suites/                   stable high-level test suites
│   ├── local-agent/          local endpoint collection/detection/cost
│   │   └── capture-vm.sh
│   ├── manager-cloud/        manager ingest/query/cloud signal/incident
│   │   └── capture-container.sh
│   ├── control-plane/        AgentControlPlaneService/mTLS/command contract
│   ├── reliability/          spool/outage/restart/backpressure
│   └── storage/              Postgres/store projection and query
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
│   │   ├── expected.yaml
│   │   └── labels.yaml
│   └── vm/<scenario>/
│       ├── attack.sh
│       ├── expected.yaml
│       └── labels.yaml
│
├── workloads/                repeatable pressure sources, no security assertions
│   └── vm/<workload>/
│       ├── run.sh
│       └── labels.yaml
│
├── policies/                 collection/detection/resource/telemetry/response samples
├── content/                  IOC/context/rulepack content used by policies
│
├── harness/                  shared execution glue
│   ├── lib/                  shared wait/query/build/cleanup helpers
│   ├── start-*.sh
│   ├── stop-*.sh
│   └── cleanup.sh
│
├── tools/
│   ├── assertions/           expected.yaml and local capture assertions
│   ├── recorder/             long-running timeline sampler
│   ├── benchmarks/           policy/workload matrix runners and reports
│   ├── diagnostics/          perf/pprof/strace helpers
│   ├── fixtures/             synthetic event/scenario fixture generators
│   └── reports/              result summarizers
│
└── .results/                 generated outputs
```

记忆方式:

| 目录 | 角色 | 说明 |
|---|---|---|
| `env/` | 环境输入 | Docker/Vagrant 拓扑、镜像、provision、共享资源 |
| `scenarios/` | 功能输入 | 攻击/良性场景脚本 + `expected.yaml` 功能断言 + `labels.yaml` 效果标签 |
| `workloads/` | 压力输入 | exec/file/network/mixed/business 负载 + benign `labels.yaml` |
| `policies/` | 策略输入 | 采集、检测、资源、上行、响应策略样例 |
| `content/` | 内容输入 | IOC feed、路径上下文、endpoint rulepack |
| `suites/` | 高层测试入口 | 按 SUT/evaluation scope 组织 local-agent、manager-cloud、control-plane 等 |
| `harness/` | 执行胶水 | 启停、等待、清理和通用 shell helper |
| `tools/assertions/` | 断言工具 | `expected.yaml` 与本地 capture 结果断言 |
| `tools/recorder/` | 性能采样 | CPU/RSS/EPS/drop/signal timeline |
| `tools/benchmarks/` | 矩阵评估 | policy x workload x phase 汇总 |
| `tools/diagnostics/` | 热点诊断 | perf/pprof/strace,用于解释成本 |
| `tools/fixtures/` | 合成输入 | replay scenario/event fixture 生成 |
| `tools/reports/` | 报告工具 | summary、matrix、本地 signal 关联报告 |
| `.results/` | 输出 | event/signal ndjson、summary、matrix、日志 |

## Scenario Contracts

每个 scenario 是一份功能契约:给定输入动作,系统必须或不得产出指定的 Event、Signal、Incident、Evidence 或 Response。

| 场景 | 核心命题 | 期望 |
|---|---|---|
| `apt-fileless-c2` | 单 lineage 内形成完整攻击链 | endpoint terminal signal, incident=1 |
| `apt-staged-drop` | 跨 lineage 分阶段落盘和执行 | endpoint 不应单独 terminal, cloud incident=1 |
| `benign-ci-noise` | 本地 CI/cache/artifact 噪声,不触碰 IoC/攻击路径/敏感凭据 | incident=0, attack signal=0 |
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
| `business-normal` | 正常构建/缓存/校验业务,评估业务干扰和误报 |
| `host-activity-heavy` | 主机进程和普通文件活动很重,覆盖 exec/read/write |
| `edr-activity-heavy` | EDR 关注面活动很重,覆盖 exec/file/local-network |

## Policy And Content Inputs

`policies/` 中最常用于采集评估的是 collection policy 档位:

| Policy | 用途 |
|---|---|
| `collection-minimal-high-signal.json` | 最小常开面,低成本高置信 |
| `collection-edr-balanced.json` | 长期运行 EDR baseline |
| `collection-incident-deep.json` | 调查窗口/高风险窗口增强采集 |

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
make -C test recorder-vm-mark RUN_ID=my-run PHASE=workload_start DETAIL=edr-activity-heavy
make -C test recorder-vm-stop RUN_ID=my-run
make -C test recorder-vm-report RUN_ID=my-run

# VM collection policy benchmark
make -C test bench-collection-vm DIAG_SCENARIO=business-normal
make -C test bench-collection-vm DIAG_SCENARIO=edr-activity-heavy
make -C test bench-collection-vm DIAG_SCENARIO=apt-fileless-c2

# Fixed policy x workload/scenario benchmark matrix, then effectiveness report
make -C test bench-matrix-vm

# Full policy x workload x scenario matrix
make -C test bench-matrix-vm

# Optional baseline-only modes
make -C test bench-matrix-vm MATRIX_MODE=workload
make -C test bench-matrix-vm MATRIX_MODE=scenario
make -C test bench-matrix-vm MATRIX_MODE=all

# Build an effectiveness report from existing scenario/benchmark outputs
make -C test effectiveness-report TOPO=vm RUN_ID=manual

# EDR lifecycle benchmark
make -C test bench-edr-lifecycle-vm DIAG_SCENARIO=edr-activity-heavy

# Run a functional E2E with recorder around it
make -C test bench-e2e-vm SCENARIO=apt-fileless-c2

# Tetragon hotspot diagnostic, not an official resource conclusion
make -C test diag-tetragon-vm
make -C test diag-tetragon-vm-workload DIAG_SCENARIO=edr-activity-heavy

# Aggregate simple reports and clean generated outputs
make -C test report
make -C test clean
```

## Runnable Test Index

All entries below are Make targets under `test/`. Run them from the repository
root as `make -C test <target>`.

### Topology And Scenario Flow

| Target | Purpose |
|---|---|
| `up` | Start the selected topology, controlled by `TOPO=container|vm`. |
| `down` | Stop the selected topology. |
| `status` | Show selected topology status. |
| `provision` | VM rsync and provision. |
| `capture` | Run one scenario capture. `TOPO=container` uses `suites/manager-cloud/capture-container.sh`; `TOPO=vm` uses `suites/local-agent/capture-vm.sh`. |
| `assert` | Assert captured scenario output with `tools/assertions`. |
| `e2e` | `up + capture + assert`. |
| `report` | Aggregate simple result matrix. |
| `clean` | Remove generated results and samples. |

Useful variables:

| Variable | Default | Meaning |
|---|---|---|
| `TOPO` | `container` | `container` or `vm`. |
| `SCENARIO` | `apt-fileless-c2` | Scenario name. Common values: `apt-fileless-c2`, `apt-staged-drop`, `benign-ci-noise`. |
| `DUR` | `30` | Capture or sampling duration in seconds. |
| `DIAG_SCENARIO` | `edr-activity-heavy` | Workload/scenario used by diagnostic and benchmark targets. |
| `EVALUATION_SCOPE` | `full` | Scope used by `effectiveness-report`. |
| `RUN_ID` | `manual` where applicable | Result directory id for recorder/effectiveness outputs. |

### Suite Entrypoints

| Target | Suite | What It Runs |
|---|---|---|
| `test-local-agent` | `local-agent` | VM real Tetragon owned-path endpoint smoke. |
| `bench-local-agent` | `local-agent` | VM policy x workload/scenario benchmark matrix. |
| `test-agent-runtime` | `agent-runtime` | Local fake-sensor daemon, sensor restart/recover, capability, parse/drop health, and session smoke tests. |
| `test-manager-cloud` | `manager-cloud` | Manager idempotency, agent health, scenario container tests, policy, response, and incident tests. |
| `test-control-plane` | `control-plane` | Control stream contract and mTLS identity. |
| `test-reliability` | `reliability` | Spool, outage, shutdown, and backpressure reliability. |
| `test-storage` | `storage` | Store status, query pagination, and Postgres/store contract. |
| `diagnostics-vm` | `diagnostics` | VM diagnostic capture for agent-owned Tetragon. |

### Agent And Runtime Tests

| Target | Purpose |
|---|---|
| `e2e-agent-manager-contract` | Agent manager contract: DataBatch append, ControlStream sequence/replay, request id, DataAck error classes, and daemon control loop tests. |
| `e2e-manager-idempotency` | Repeated DataBatch does not amplify ingest. |
| `e2e-agent-mtls` | gRPC mTLS client cert identity binding and rejection cases. |
| `e2e-agent-daemon` | Local fake sensor daemon to health/data-plane smoke. |
| `e2e-agent-sensor-restart` | Managed fake Tetragon restart, degraded health, and tamper signal. |
| `e2e-agent-sensor-recover` | Managed fake Tetragon degrades once then recovers. |
| `e2e-agent-spool` | Manager outage, durable spool, and recovery drain. |
| `e2e-agent-outage-soak` | Manager outage queues multiple batches and drains all. |
| `e2e-agent-shutdown` | Graceful shutdown drains queued batch. |
| `e2e-agent-reliability-soak` | Longer outage/retry/restart/shutdown reliability soak. |
| `e2e-agent-capability` | Startup bundle/checksum capability failure gives degraded health. |
| `e2e-agent-capability-btf` | Required BTF capability failure gives degraded health. |
| `e2e-agent-capability-bpffs` | Required bpffs capability failure gives degraded health. |
| `e2e-agent-parse-health` | Sensor parse errors degrade health and emit tamper signal. |
| `e2e-agent-dropped-health` | Sensor dropped events degrade health and emit tamper signal. |
| `e2e-agent-backpressure` | Spool `max_bytes` backpressure/drop degrades health. |
| `e2e-agent-health` | `sysarmorctl manager` agents/health/metrics smoke. |
| `e2e-agent-session` | Data append updates agent session cursor and resume cursor. |
| `e2e-agent-control-plane-contract` | Long-lived `AgentControlPlaneService.Connect` daemon contract. |
| `e2e-agent-control-plane-all` | Session plus control-plane contract tests. |
| `e2e-agent-all` | Common local/manager agent smoke bundle. |
| `e2e-agent-runtime-all` | Agent smoke bundle plus container detection and real owned-path runtime gates. |

### Agent Runtime Topology Tests

| Target | Purpose |
|---|---|
| `e2e-agent-daemon-container` | Container topology fake daemon smoke. |
| `e2e-agent-managed-container` | Container managed fake Tetragon bundle smoke. |
| `e2e-agent-managed-recover-container` | Container managed degraded-to-recovered smoke. |
| `e2e-agent-managed-restart-container` | Container managed fake Tetragon restart/tamper smoke. |
| `e2e-agent-systemd-vm` | VM systemd agent daemon health/restart smoke. |
| `e2e-agent-managed-vm` | VM systemd managed fake Tetragon bundle smoke. |
| `e2e-agent-managed-recover-vm` | VM systemd managed degraded-to-recovered smoke. |
| `e2e-agent-real-tetragon-owned-vm` | VM systemd agent-owned real Tetragon to local ctl events/signals. |
| `e2e-agent-real-tetragon-owned-container` | Container agent-owned real Tetragon detection smoke. |

### Scenario Detection Tests

| Target | Purpose |
|---|---|
| `e2e-agent-apt-container` | Container `apt-fileless-c2` via agent-managed Tetra subscription. |
| `e2e-agent-staged-container` | Container `apt-staged-drop` via agent-managed Tetra subscription. |
| `e2e-agent-benign-container` | Container `benign-ci-noise` via agent-managed Tetra subscription. |
| `e2e-agent-detection-container-all` | All three container managed detection smokes. |

### Policy And Response Tests

| Target | Purpose |
|---|---|
| `e2e-policy-endpoint-disable` | Effective policy disables an endpoint rule. |
| `e2e-policy-agent-refresh` | Agent refreshes effective policy without restart. |
| `e2e-policy-cloud-disable` | Manager policy assignment disables a cloud rule. |
| `e2e-policy-publish` | Draft policy must be published before assignment/effective use and is audited. |
| `e2e-operator-role-bindings` | Operator role bindings authorize control-plane writes. |
| `e2e-policy-all` | All policy/control-plane gates. |
| `e2e-response-observe-only` | Observe-only response command and audit. |
| `e2e-response-policy-deny` | Response policy denies destructive actions by default. |
| `e2e-response-scope-deny` | Response command must match agent runtime scope. |
| `e2e-response-audit` | Response intent converts into auditable observe decision. |
| `e2e-response-approval` | Approval-required response waits before control delivery. |
| `e2e-response-multi-approval` | Response policy can require multi-approval threshold and roles. |
| `e2e-response-all` | All response/enforce observe-only gates. |

### Incident, Graph, And Evidence Tests

| Target | Purpose |
|---|---|
| `e2e-graph-evidence` | Incident evidence is queryable as graph JSON. |
| `e2e-incident-lifecycle` | Incident lifecycle status can be updated and queried. |
| `e2e-incident-attach-evidence` | Incident evidence can be attached and queried. |
| `e2e-incident-merge` | Incidents can be merged by explicit id. |
| `e2e-rarity-baseline` | Rarity baseline is updated by ingest without duplicate amplification. |
| `e2e-graph-all` | Graph, evidence, incident, and rarity gates. |

### Storage And Postgres Tests

| Target | Purpose |
|---|---|
| `e2e-store-status` | Store backend and migration status are queryable. |
| `e2e-query-pagination` | Query APIs support limit/offset pagination. |
| `e2e-postgres-store` | Postgres snapshot adapter migration, persistence, and close behavior. |
| `e2e-postgres-idempotency` | Postgres duplicate ingest idempotency across reopen. |
| `e2e-postgres-policy-persistence` | Postgres persists policy assignment, audit, and incident state. |
| `e2e-postgres-manager-api` | Postgres backs manager ingest/query/policy/incident APIs. |
| `e2e-postgres-agent-projection` | Postgres projects agents and health into table paths. |
| `e2e-postgres-ingest-projection` | Postgres projects events and signals into table paths. |
| `e2e-postgres-event-query` | Postgres queries events through table path. |
| `e2e-postgres-signal-query` | Postgres queries signals through table path. |
| `e2e-postgres-incident-query` | Postgres queries incidents through table path. |
| `e2e-postgres-control-projection` | Postgres projects rules and evidence pullbacks into table paths. |
| `e2e-postgres-control-audit-projection` | Postgres projects policy audit and operator roles into table paths. |
| `e2e-postgres-response-projection` | Postgres projects response audit into table path. |
| `e2e-postgres-response-write` | Postgres writes response audit through table path. |
| `e2e-postgres-response-query` | Postgres queries response audit through table path. |
| `e2e-postgres-policy-projection` | Postgres projects policies and assignments into table paths. |
| `e2e-postgres-policy-write` | Postgres writes policy control tables directly. |
| `e2e-postgres-policy-query` | Postgres queries policy control tables. |
| `e2e-postgres-policy-get` | Postgres gets a policy through table path. |
| `e2e-postgres-effective-policy-query` | Postgres resolves effective policy through table paths. |
| `e2e-postgres-incident-projection` | Postgres projects incidents and evidence into table paths. |
| `e2e-postgres-observability-projection` | Postgres projects incident events and metrics into table paths. |
| `e2e-postgres-agent-session-projection` | Postgres projects agent sessions into table path. |
| `e2e-postgres-rarity-projection` | Postgres projects rarity baseline into table path. |
| `e2e-postgres-all` | All current Postgres/store foundation gates. |

### Performance, Benchmark, And Diagnostics

| Target | Purpose |
|---|---|
| `perf` | Short getevents performance baseline for selected topology. |
| `perf-resource` | Short EDR resource usage time series. |
| `perf-resource-container` | Container idle EDR resource sampling. |
| `perf-resource-vm` | VM idle EDR resource sampling. |
| `perf-resource-all` | Container and VM idle EDR resource sampling. |
| `sync-vm-agent` | Upload current `bin/sysarmor-agent` and `bin/sysarmorctl` into the VM and verify the local socket. |
| `recorder-vm-start` | Start VM long-running performance recorder. |
| `recorder-vm-mark` | Add a recorder marker with `PHASE=<name>` and optional `DETAIL=...`. |
| `recorder-vm-stop` | Stop VM recorder and pull timeline. |
| `recorder-vm-report` | Generate recorder summary from timeline and markers. |
| `diag-tetragon-vm` | Capture VM Tetragon diagnostics. |
| `diag-tetragon-vm-workload` | Capture VM Tetragon diagnostics while running `DIAG_SCENARIO`. |
| `bench-collection-vm` | Run VM real-path resource/EPS benchmark for collection policies. |
| `bench-matrix-vm` | Run fixed VM policy x workload/scenario matrix and effectiveness report. |
| `bench-edr-lifecycle-vm` | Capture VM EDR baseline/start/apply/steady/workload lifecycle curve. |
| `bench-e2e-vm` | Wrap a VM functional E2E with recorder. |
| `effectiveness-report` | Build event/signal precision-recall effectiveness report from `labels.yaml` and benchmark outputs. |

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

效果评估使用 `labels.yaml` 作为 ground truth。每个 scenario/workload 声明真实应覆盖的 event label 和 signal label; evaluator 将 workload 窗口内 observed events/signals 归一化为 canonical entities 后计算:

```text
event_recall
signal_recall
terminal_recall
signal_precision
event_noise_ratio
signal_event_link_rate
false_positive_signals
drop_rate
parse_error_rate
cost_per_1k_events_cpu
```

当前可用入口:

```bash
make -C test sync-vm-agent
make -C test bench-matrix-vm
make -C test effectiveness-report TOPO=vm RUN_ID=manual
```

`bench-matrix-vm` 固定跑:

```text
policies:
collection-minimal-high-signal
collection-edr-balanced
collection-incident-deep

workloads:
business-normal

scenarios:
apt-fileless-c2
apt-staged-drop
benign-ci-noise
```

默认 `MATRIX_MODE=cross`,直接跑完整三维组合:

```text
policy x workload x scenario
```

可选模式:

```text
MATRIX_MODE=workload  # only policy x workload
MATRIX_MODE=scenario  # only policy x scenario
MATRIX_MODE=all       # workload-only + scenario-only + cross
```

它会输出:

```text
test/.results/bench-matrix-vm/<run-id>/matrix.csv
test/.results/bench-matrix-vm/<run-id>/matrix.json
test/.results/effectiveness/<run-id>/summary.json
test/.results/effectiveness/<run-id>/matrix.csv
test/.results/effectiveness/<run-id>/matrix.json
test/.results/effectiveness/<run-id>/truth_steps.csv
test/.results/effectiveness/<run-id>/policy_comparison.csv
test/.results/effectiveness/<run-id>/policy_comparison.json
```

`bench-matrix-vm` 默认先执行 VM agent sync,避免 VM 内旧版 `sysarmor-agent` / `sysarmorctl` 或旧配置影响测试结果。可用 `SYSARMOR_BENCH_SYNC_VM_AGENT=0` 关闭。`bench-collection-vm` 默认在 collection policy 前加载 `test/policies/detection-cep-endpoint.json`,可用 `SYSARMOR_BENCH_DETECTION_POLICY=<path>` 替换,或用 `SYSARMOR_BENCH_APPLY_DETECTION=0` 只测采集。

这样可以把 policy 档位、业务工作负载和攻击场景放到同一张 matrix 中比较:采得准不准、全不全、快不快、贵不贵。

`policy_comparison.csv` 的综合分模型:

```text
overall_score = 0.65 * effectiveness_score
              + 0.25 * resource_score
              + 0.10 * stability_score

resource_score = 0.75 * inverse_cpu_score
               + 0.25 * inverse_rss_score
```

其中 `effectiveness_score` 来自 `labels.yaml` 的 event/signal precision-recall 结果,`resource_score` 来自 workload 阶段的 EDR CPU/RSS,`stability_score` 在 drop 和 parse error 都为 0 时为 1。

`effectiveness-report` 对 benchmark recorder 输出使用 `workload_start` 到 `workload_done` 窗口。`observed_events_total` / `observed_signals_total` 保留整段 recorder 累计帧数量,`observed_events` / `observed_signals` 是进入效果评估的 workload 窗口数量。这样 policy apply、Vagrant SSH、motd 等运行噪声不会被算作攻击场景命中。
