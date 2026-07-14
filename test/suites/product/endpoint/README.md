# Product Endpoint

结论：product endpoint 验证单台被保护主机上的 agent/sensor/local control 产品能力。它不验证 manager 侧 incident/query，也不作为正式性能 benchmark。

## System Under Test

| 对象 | 说明 |
|---|---|
| `sysarmor-agent` | endpoint agent 主进程 |
| owned sensor | agent 托管的 sensor/Tetragon runtime |
| local control | `sysarmorctl --socket` 本地 Unix socket API |
| local telemetry | agent 本地 event/signal watch |
| health/recover | agent/sensor restart、recover、drop/parse health |

## 推荐入口

```bash
make -C test product-endpoint
```

`product-endpoint` 不是 smoke：它在单 VM 上使用 agent 托管的真实 Tetragon，验证端侧产品路径。

容器内 namespace-scoped 采集验证使用：

```bash
make -C test product-endpoint-namespace-container
```

该路径使用 `make deploy` 预置的 agent release。deploy 会构建 signed agent package、启动 `sysarmor-packages` 静态文件服务，并让 manager 从 `SYSARMOR_AGENT_PACKAGE_INDEX_URL` 导入 artifact metadata 和默认 channel。测试随后通过 manager API 创建 `profile=linux-container` 的 enrollment，在 privileged Linux 容器内执行 manager 返回的 `agent-install.sh`。安装 profile 会生成 `scope.type=namespace`、`scope.selector=self`，再由容器 entrypoint 方式启动 agent，验证自身 pid/mnt namespace 下推到 Tetragon `matchNamespaces` 后仍能采集容器内事件。
脚本会优先检测本机 `make deploy` 启动的 SysArmor 平台；如果 manager/gateway/worker/infra/packages 容器不存在，会先执行 `make deploy`。如果运行中的 manager 还没有当前 `linux-container` installer 能力，或缺少 seeded package channel，脚本会自动执行 `make deploy` 刷新平台。

### `product-endpoint-namespace-container` 验证点

结论：这个测试验证的是“manager 分发的 `linux-container` agent 只采集自身容器 namespace 内的事件”。负向断言不是简单确认某个 marker 没出现，而是用同形命令证明 `process.exec` policy 本来覆盖这类事件，host 侧事件因为 namespace/self scope 才不会被该 agent 上报。

测试流程按产品路径执行：

1. 确认 manager 已从 deploy package index 导入 `linux-container-dev` channel。
2. 通过 manager API 创建 enrollment，profile 为 `linux-container`，channel 为 `linux-container-dev`。
3. 在 privileged Linux 容器内执行 manager 返回的 `install_url`，即 `curl install_url | bash`。
4. installer 从 `sysarmor-packages` 下载 agent package，校验 sha256 和 manifest signature。
5. 使用 installer 输出的 entrypoint 启动 `/opt/sysarmor/agent/bin/sysarmor-agent run --config /etc/sysarmor/agent.yaml`。
6. 通过 manager API 验证 agent health、Tetragon policy、容器内正向事件、host 负向事件。

事件断言分三组：

| 断言 | 输入 | 期望 |
|---|---|---|
| 容器内 `id` 正向 | 容器内执行 `/bin/sh -c 'id >/dev/null # <marker>'` | manager events 中存在同时匹配 `agentId`、`scenario`、`behavior=process.exec`、argv marker、argv `id` 的事件。 |
| 容器内 `echo` 正向 | 容器内执行 `/bin/sh -c '/bin/echo <marker> >/dev/null'` | manager events 中存在同时匹配 `agentId`、`scenario`、`behavior=process.exec`、argv marker、argv `/bin/echo` 的事件，证明采集策略覆盖该类 exec。 |
| host `echo` 负向 | host 执行同形 `/bin/sh -c '/bin/echo <marker> >/dev/null'` | manager events 中不存在同时匹配 `agentId`、`scenario`、`behavior=process.exec`、argv marker 的事件，证明 namespace/self scope 隔离生效。 |

调试产物写入 `test/.results/`，常用文件如下：

| 文件 | 说明 |
|---|---|
| `e2e-agent-namespace-self-container.channels.json` | manager seeded channel 查询结果。 |
| `e2e-agent-namespace-self-container.enrollment.json` | enrollment token 和 install URL。 |
| `e2e-agent-namespace-self-container.install.log` | 容器内 installer 输出，应包含跳过 systemd 和 entrypoint 启动命令。 |
| `e2e-agent-namespace-self-container.health.json` | manager 视角 agent health，应包含 `scope.type=namespace`、`scope.selector=self`、`sensor_health.policy_loaded=true`。 |
| `e2e-agent-namespace-self-container.tracingpolicy-loaded.txt` | 容器内 `tetra tracingpolicy list` 输出，应显示 `sysarmor-runtime-collection` 为 `enabled`。 |
| `e2e-agent-namespace-self-container.positive-input.txt` / `positive-match.json` | 容器内 `id` 正向输入与命中事件。 |
| `e2e-agent-namespace-self-container.echo-positive-input.txt` / `echo-positive-match.json` | 容器内 `/bin/echo` 正向输入与命中事件。 |
| `e2e-agent-namespace-self-container.negative-input.txt` / `events-negative.json` | host `/bin/echo` 负向输入与 manager 查询响应。 |

## Smoke 子脚本

endpoint 目录里还保留一组 runtime smoke，用于快速验证 agent 生命周期、健康状态和托管 sensor 行为。它们不等价于真实检测效果测试。

| 子脚本类型 | Smoke | Sensor | 说明 |
|---|---|---|---|
| `go test ./internal/agent/daemon ./internal/sensors/linux/tetragon`、`capability.sh` | 是 | 构造数据 / fake 输入 | 本地 Agent runtime、health、异常语义。 |
| `e2e-managed-*` | 是 | fake Tetragon bundle | 验证 agent 托管 sensor 的启动、重启、恢复。 |
| `e2e-real-tetragon-owned-*` | 否 | owned real Tetragon | 验证真实 Tetragon owned sensor 路径。 |
| `e2e-namespace-self-container.sh` | 否 | manager-installed real Tetragon | 验证 manager 分发安装容器内 agent，以及 namespace/self 内核态过滤路径。 |

性能 benchmark 使用：

```bash
make -C test performance-endpoint \
  SYSARMOR_BENCH_PROFILE=medium \
  SYSARMOR_BENCH_WORKLOAD=business-normal \
  SYSARMOR_BENCH_SCENARIO=apt-fileless-c2-local \
  SYSARMOR_BENCH_POLICIES='test/data/policies/collection-balanced.json'
```

## 适用环境

| 环境 | 用途 |
|---|---|
| `vm-endpoint` | endpoint 产品功能和性能 benchmark 的主环境 |
| `container` | 轻量兼容/冒烟路径 |

正式端侧 CPU/RSS 结论应使用 `performance-endpoint`，不是直接使用 product e2e 脚本。

## 不覆盖

- manager cloud signal；
- incident/query；
- manager/gateway/worker 资源占用；
- 多节点 attacker C2 拓扑；
- 平台控制面下发语义。
