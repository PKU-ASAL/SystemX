# SysArmor

[English](README.md) | [简体中文](README.zh-CN.md)

SysArmor 是一个面向 Linux 的端点安全与检测平台。它由默认独立运行的 Agent
和可选的管理平面组成，可提供集中注册、遥测处理、调查与响应能力。

项目目前处于活跃开发阶段，适合开发、评估和测试；在稳定版本发布前，接口与
部署流程仍可能调整。

## 核心能力

- **独立端点运行：** Agent 无需依赖 Manager 即可在本地启动，并负责传感器
  生命周期、数据采集、检测和有界本地存储。
- **显式注册：** 端点仅在用户执行注册后连接管理平面，并使用与 tenant 和
  Agent 绑定的 mTLS 身份。
- **端点检测：** 事件在本地完成标准化和检测，signal 可通过 Agent 控制
  socket 查询。
- **集中分析：** Gateway、Worker 和 Manager 支持可靠接入、关联分析、
  incident、evidence、policy 和 response 工作流。
- **可复现验证：** 容器和虚拟机测试覆盖产品行为、检测效果，以及端侧和
  平台性能。

## 架构

```text
Linux sensor -> Agent -> 本地 signal 与有界存储
                       -> Gateway -> Kafka -> Worker -> PostgreSQL / OpenSearch
                                                   -> Manager API 与 Web UI
```

Agent 在 standalone 模式下即可独立使用。注册只增加集中上传和控制能力，
不会在端点创建第二条数据通路。

## 环境要求

当前开发流程面向使用 systemd 的 x86_64 Linux 主机。

- Go 1.26 或更高版本
- `make`、`curl`，以及安装 Agent 所需的 root 权限
- 本地平台需要 Docker、Docker Compose 和 `openssl`
- 仅基于虚拟机的测试需要 KVM/libvirt 和 Vagrant

使用 `make api` 重新生成 API binding 还需要将 `protoc`、
`protoc-gen-go` 和 `protoc-gen-go-grpc` 放在 `PATH` 或
`$(go env GOPATH)/bin` 下。

部分构建和安装命令会下载 Go module、操作系统软件包或 Tetragon 传感器包。

## 快速开始

### Standalone Agent

构建并安装 Agent、CLI、默认 policy 和由 Agent 管理的 Tetragon bundle：

```bash
make install-agent
sudo sysarmorctl agent health
```

Agent 将本地状态保存在 `/var/lib/sysarmor/agent`，并通过
`/run/sysarmor/agent/control.sock` 提供控制 API。

卸载程序但保留配置和本地数据：

```bash
make uninstall-agent
```

仅在确定需要同时删除配置和本地数据时，使用
`make uninstall-agent PURGE=1`。

### 本地平台

构建二进制和 release package、初始化本地凭据并启动平台：

```bash
make deploy
make status
make doctor
```

使用 `make down` 停止平台。服务布局、注册、mTLS、配置和运维命令见
[部署文档](deployments/README.md)。

## 开发

常用仓库命令：

```bash
make build-binary  # 构建 Agent、Gateway、Manager、Worker 和 sysarmorctl
make test          # 运行 Go 测试
make api           # 重新生成 protobuf binding
make release       # 构建签名的 Agent release package 和索引
```

生成的二进制位于 `dist/bin/`，release artifact 位于 `dist/release/`。这两个
目录均可重新生成，并已被 Git 忽略。

运行产品、效果和性能测试：

```bash
make -C test help
make -C test product-endpoint-standalone
make -C test product-topology
make -C test performance-endpoint SYSARMOR_BENCH_PROFILE=quick
```

虚拟机测试会创建本地特权基础设施，并可能下载较大的 artifact。运行前请先
阅读测试文档。

## 文档

- [仓库结构](docs/architecture/repo-layout.md)
- [部署与注册](deployments/README.md)
- [测试指南](test/README.md)
- [测试环境与报告细节](test/DETAILS.md)
- [遥测语义](docs/architecture/telemetry-semantics.md)
- [Schema 演进](docs/architecture/schema-evolution.md)
- [Manager UI API 契约](docs/architecture/manager-ui-api-contract.md)

`api/proto/` 下的 protobuf 定义是 wire contract 的事实来源。

## 参与贡献

项目目前尚未发布正式的贡献指南。开始较大改动前，请先与维护者协调范围，
并保持改动聚焦、经过测试且有相应文档。

## 许可证

仓库目前尚未包含许可证文件。在正式发布许可证前，不应假定已获得任何开源
许可证授权。
