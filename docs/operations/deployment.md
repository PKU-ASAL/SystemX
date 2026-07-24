# 部署

本文说明当前仓库支持的两种部署方式：单机 Agent 和本地管理平台。测试专用的 VM、容器拓扑不属于生产部署，统一见测试指南。

## 部署选择

| 目标 | 入口 | 适用场景 |
|---|---|---|
| 单机 Agent | `make install-agent` | 无平台连接的主机采集、检测和本地调查 |
| GitHub 开发预发布 | Release 页面中的 `install.sh` | 从 `dev` 构建的可追溯 standalone 体验版本 |
| 本地管理平台 | `make deploy` | 端云链路、集中管理和开发验证 |

当前 Compose 配置面向单机开发和验证，默认凭据、无安全插件的 OpenSearch 以及宿主机暴露的基础设施端口不应直接用于生产环境。

## 前置条件

- Linux；安装 Agent 还要求 systemd 和 root 权限。
- Go、Docker Compose、`make`、OpenSSL 和 `curl`。
- 构建 release 或运行真实测试时，需要与主机架构匹配的 Tetragon 归档；根 Makefile 默认查找 `.cache/` 或 `.scratchpad/.cache/` 下的 `tetragon-v1.7.0-amd64.tar.gz`。

真实测试环境使用独立的完整预检：

```bash
make test-doctor
```

## 安装单机 Agent

```bash
make install-agent
sudo sysarmorctl agent health
```

安装器构建 Agent 和 `sysarmorctl`，安装 Tetragon bundle，写入默认配置和策略，并启用 `sysarmor-agent.service`。Sensor installer 未收到本地 archive 时会使用 `deployments/sensors/tetragon/bundle.env` 中的版本与下载地址。

| 路径 | 内容 |
|---|---|
| `/opt/sysarmor/agent/bin/sysarmor-agent` | Agent 可执行文件 |
| `/opt/sysarmor/agent/bundles/tetragon/` | Tetragon 原始 bundle |
| `/opt/sysarmor/agent/sensors/` | 已安装的传感器版本 |
| `/etc/sysarmor/agent/agent.yaml` | Agent 运行配置 |
| `/etc/sysarmor/agent/policy.json` | 端侧策略 |
| `/var/lib/sysarmor/agent/` | 身份、本地事件、Signal 与有界状态 |
| `/run/sysarmor/agent/control.sock` | 本地控制面 Unix socket |
| `/usr/local/bin/sysarmorctl` | 管理命令行 |

默认配置运行 managed Tetragon、host scope 和 observe-only 模式，不连接平台。注册信息由 enrollment 写入本地状态，不应手工添加到 YAML。自定义安装路径和配置项见[配置参考](../reference/configuration.md)。

### 安装 GitHub 开发预发布

在 GitHub Releases 页面选择标记为 Pre-release 的版本，使用该版本说明中的固定 URL：

```bash
curl -fsSL https://github.com/PKU-ASAL/sysarmor/releases/download/<version>/install.sh | sudo bash
```

开发预发布只支持 Linux x86_64 和 systemd。版本号同时包含构建时间与 Git commit，例如
`v0.1.0-dev.20260724+097acdae`。安装脚本下载同一 Release 的归档，校验固定 SHA-256，
安装后等待 Agent 健康检查通过。重复安装会更新程序和 systemd unit，但保留已有配置、策略和本地数据。

GitHub 公开归档只包含 SysArmor。安装时从 Tetragon 官方 Release 下载锁定版本并校验固定
SHA-256，从而避免在第三方许可证清单完成前重新分发其二进制。需要完全离线的一体包时，仍须先完成
第三方 LICENSE、NOTICE、SBOM 和源代码提供义务审查。

可使用 GitHub CLI 验证构建来源：

```bash
gh attestation verify sysarmor-agent-linux-amd64-<version>.tar.gz --repo PKU-ASAL/sysarmor
```

## 启动本地平台

```bash
make deploy
make status
make doctor
```

`make deploy` 依次完成：构建服务二进制、生成签名 Agent release、初始化 PKI 和登录凭据、构建镜像并启动 Compose。标准数据流为：

```text
Agent -> Gateway -> Kafka -> Worker -> PostgreSQL / OpenSearch
Browser -> Manager Console BFF -> Manager
```

默认宿主机入口：

| 服务 | 地址 | 用途 |
|---|---|---|
| Manager Console | `http://127.0.0.1:4173` | 登录、部署和查询界面 |
| Manager | `http://127.0.0.1:19443` | Operator HTTP API |
| Gateway | `127.0.0.1:19444` | Agent mTLS gRPC |
| Gateway health | `http://127.0.0.1:19445` | `/healthz` 和 `/metrics` |
| Package feed | `http://127.0.0.1:18080` | 已签名 Agent release |
| PostgreSQL | `127.0.0.1:15432` | 控制面状态 |
| Kafka | `127.0.0.1:19092` | 原始遥测交接 |
| Redis | `127.0.0.1:16379` | Gateway 热状态 |
| OpenSearch | `http://127.0.0.1:29200` | Event、Signal、Incident、Evidence 投影 |

端口均可通过 `SYSARMOR_*_PORT` 环境变量覆盖。例如：

```bash
SYSARMOR_OPENSEARCH_PORT=39200 make deploy
```

## 身份与信任

`make auth-init` 在 `deployments/pki/agent-plane-mtls/runtime/` 创建：

- Auth.js session 密钥和一次性 bootstrap 管理员凭据；
- BFF 签发 Manager JWT 所需的 RS256 密钥；
- Agent plane mTLS CA、Gateway 证书和 artifact 签名密钥。

已有完整凭据不会被覆盖。不要提交 `runtime/` 中的私钥或密码；非本地环境应使用外部密钥和机密管理系统。

浏览器只调用同源 BFF。BFF 校验 session 后签发短期 Manager JWT；浏览器不持有该 JWT，也不直接调用 Manager。Agent 证书使用以下 URI SAN 绑定 tenant 和 Agent：

```text
spiffe://sysarmor.local/tenant/<tenant_id>/agent/<agent_id>
```

Gateway 同时校验证书链、URI 身份和每个数据/控制帧中的 tenant、Agent ID。

## Release 与注册

```bash
make release RELEASE_VERSION=v1.0.0
```

产物写入 `dist/release/`。Package 服务提供不可变字节，Manager 管理 artifact 元数据、channel、一次性 enrollment 和安装脚本。

本地平台 Agent bundle 还包含 Tetragon、bpftool、gops 和 BPF 对象等第三方资产。仓库根目录的 MulanPSL-2.0 只覆盖 SysArmor，不改变第三方组件的许可证。在完成逐项许可证清单、LICENSE/NOTICE 携带和全部打包文件完整性校验前，该一体 bundle 只用于开发与评估，不能作为已经完成外部分发合规的制品发布。GitHub 开发预发布使用不携带 Tetragon 二进制的 thin 包，不属于该一体 bundle。

推荐从 Manager Console 的 Deploy 页面选择 artifact、channel 和安装 profile，然后在目标端执行生成的安装命令。完整流程为：

```text
构建并签名 artifact -> 绑定 channel -> 创建一次性 enrollment
-> 下载并校验安装器和包 -> 端点生成私钥与 CSR
-> Manager 签发 tenant/Agent 绑定证书 -> Agent 连接 Gateway
```

Manager 只保存 enrollment token 的哈希，端点私钥不离开端点。默认只上传 enrollment 边界之后的数据；只有明确需要历史数据时才启用 `--upload-history`。

安装 profile：

- `linux-systemd`：安装并启用 systemd 服务，适用于主机或 VM。
- `linux-container`：不使用 systemd，scope 为 `namespace/self`，容器入口为 `/opt/sysarmor/agent/bin/sysarmor-agent run --config /etc/sysarmor/agent/agent.yaml`。

## 生命周期命令

```bash
make status                 # 查看平台服务
make down                   # 停止并保留数据卷
make up                     # 使用现有镜像和 release 启动
make reset                  # 删除数据卷并重建；保留 PKI
make clean                  # 停止平台并删除数据卷
sudo sysarmorctl unenroll   # Agent 返回 standalone
make uninstall-agent        # 保留配置和本地数据
make uninstall-agent PURGE=1
```

`reset`、`clean` 和 `PURGE=1` 会删除状态或数据，执行前先按[维护指南](maintenance.md)完成备份。
