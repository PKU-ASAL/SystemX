# Agent 管理

本文说明 Agent 的安装、运行模式、注册、策略管理、健康检查和取消注册。精确路径与字段见[配置参考](../reference/configuration.md)。

## 运行模式

### Standalone

Agent 安装后首先以 standalone 模式运行。它创建本地设备身份，管理 sensor，保存有界 Event 与 Signal，并通过 Unix socket 提供本地控制接口。它不会自动连接 Manager 或 Gateway。

### Managed

注册成功后，Agent 保留同一条本地采集和检测路径，同时增加：

- tenant/Agent 绑定的 mTLS 身份；
- Gateway 数据上传；
- Manager 控制通道；
- 集中策略、健康和响应工作流。

注册不是第二条采集路径。平台不可用时，本地采集、检测和查询仍应继续。

## 安装

开发环境可以从仓库安装：

```bash
make install-agent
sudo sysarmorctl agent health
```

生产形态使用 Manager 发布的签名 artifact 和一次性 enrollment 安装命令。安装配置、签名和 PKI 见[部署指南](../operations/deployment.md)。

## 注册流程

标准流程是：

```text
发布签名 artifact
-> 绑定 release channel
-> 创建一次性 enrollment
-> Console 生成只能兑换一次的安装 URL
-> 端点下载并验证包
-> 端点生成私钥和 CSR
-> Manager 颁发 tenant/Agent 绑定证书
-> Agent 启用 Gateway 上传和控制通道
```

已安装 standalone Agent 也可直接注册：

```bash
sudo sysarmorctl enroll --manager-url https://manager.example --token TOKEN
```

Manager enrollment 是 tenant、Agent ID、Gateway 和 TLS server name 的唯一事实来源。私钥始终在端点生成；同一 token 重试复用待签发私钥，Manager 对同一公钥返回同一证书，对不同公钥拒绝签发。凭据先写入独立版本目录，随后才切换本地 managed 状态，失败不会覆盖当前有效身份。

Console 安装 URL 携带的是一次性 bootstrap ticket，不是 enrollment token。ticket 首次获取脚本时即失效，同时生成仅写入临时 `0600` 文件的 enrollment token；artifact 下载通过 Authorization header 携带该 token，URL 和访问日志不包含凭据。Gateway 校验证书 URI 与每个 frame 中的 tenant 和 Agent ID。

默认只上传注册边界之后生成的数据。只有明确要求本地历史时才启用 history upload。

## 健康与能力

```bash
sudo sysarmorctl agent health
sudo sysarmorctl agent capability
```

管理 Agent 时至少检查：

- Agent、host、tenant 和运行作用域；
- sensor backend、进程和 policy 状态；
- telemetry bus、batcher 和 sender；
- detection runtime；
- 本地存储、drop 和 parse error；
- 当前有效策略和控制通道状态。

能力发现必须发生在下发策略之前。控制平面不能向不支持对应 behavior、scope 或 response action 的 Agent 静默下发策略。

## 策略生命周期

本地或平台策略均应遵循：

```text
获取能力
-> explain/校验
-> 编译 collection 与 detection
-> 检查资源和 response 边界
-> 原子替换有效版本
-> 持久化
-> 报告结果与健康状态
```

常用本地操作：

```bash
sudo sysarmorctl policy current
sudo sysarmorctl policy explain --file /etc/sysarmor/agent/policy.json
sudo sysarmorctl policy apply --file /path/to/policy.json
```

精确参数以 `sysarmorctl policy --help` 为准。四层策略语义见[策略指南](policy.md)。

## 取消注册

```bash
sudo sysarmorctl unenroll
```

取消注册会移除平台凭据并回到 standalone 模式，不应停止本地采集、检测或删除本地历史。需要清除安装和数据时使用独立卸载流程。

## 故障边界

- 注册失败不能破坏已有 standalone 身份和本地数据。
- 注册重试不能生成新的端点私钥或重复签发不同证书。
- 策略编译失败不能替换当前有效策略。
- Gateway 未确认 batch 时不能推进上传 checkpoint。
- 重复确认和重试必须收敛到同一批次状态。
- 取消注册必须清理云凭据，但不能静默清理本地 Event 或 Signal。

诊断、恢复和卸载步骤见[维护指南](../operations/maintenance.md)。
