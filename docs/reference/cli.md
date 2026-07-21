# `sysarmorctl` 参考

`sysarmorctl` 同时管理本地 Agent 和 Manager。命令以 `sysarmorctl help` 为最终事实来源；本页说明稳定边界和常见用法。

## 全局选项

```text
--socket PATH       Agent Unix socket；默认 $SYSARMOR_AGENT_SOCK 或 /run/sysarmor/agent/control.sock
--manager-url URL   Manager HTTP URL；默认 $SYSARMOR_MANAGER_URL 或 http://127.0.0.1:9443
--json              输出原始 JSON
```

本地 Compose 的 Manager 宿主机端口是 `19443`，因此从仓库主机调用时应设置：

```bash
export SYSARMOR_MANAGER_URL=http://127.0.0.1:19443
```

Operator API 认证使用 `SYSARMOR_MANAGER_JWT` Bearer token。`SYSARMOR_DEV_TOKEN` 只用于明确启用 development token 的受控环境，不能作为生产认证方案。

## 本地 Agent

```bash
sudo sysarmorctl agent health
sudo sysarmorctl agent capability
sudo sysarmorctl policy current
```

### Policy

```bash
sudo sysarmorctl policy explain --file policy.json
sudo sysarmorctl policy explain --file policy.json --report-only
sudo sysarmorctl policy apply --file policy.json --dry-run
sudo sysarmorctl policy apply --file policy.json
```

`explain` 使用 dry-run 编译和校验路径，不修改有效 Policy。执行 apply 前优先 explain；远程下发还应保留 actor、reason 和审计记录。

### Content

```bash
sudo sysarmorctl content apply --file content.json --dry-run
sudo sysarmorctl content apply --file content.json --allow-unsigned
sudo sysarmorctl content list
sudo sysarmorctl content list --kind iocpack
sudo sysarmorctl content get --ref ioc:example
sysarmorctl content diff old.json new.json
```

`--allow-unsigned` 只适用于受控开发或测试；生产内容应由 `content.trust_keys` 中的可信密钥验证。

### Event 与 Signal

```bash
sudo sysarmorctl event watch --include-recent --limit 10
sudo sysarmorctl event get --id EVENT_ID
sudo sysarmorctl signal watch --include-events --limit 10
```

不带 `--limit` 或 `--snapshot` 的 watch 为持续流；带 limit 时命令在收到指定数量后结束。可使用 `--behavior`、`--rule-id`、`--where` 和通用过滤参数缩小结果。

### 调试 profile

```bash
sudo sysarmorctl debug profile cpu --seconds 10 --output agent.cpu.pb.gz
```

profile 会带来额外开销，只在有限时间内启用，并妥善处理可能包含运行上下文的输出文件。

## 注册与退出注册

已安装 Agent 的显式注册命令：

```bash
sudo sysarmorctl \
  --manager-url https://manager.example \
  enroll \
  --token TOKEN \
  --tenant default \
  --agent-id agent-001 \
  --gateway gateway.example:9444 \
  --gateway-server-name gateway.example
```

只有确实需要上传注册边界前的本地历史时才添加 `--upload-history`。token 为一次性机密，不要写入 shell history、日志或仓库。退出平台并保留 standalone 能力：

```bash
sudo sysarmorctl unenroll
```

新端点优先使用 Console 生成的安装命令；它同时处理签名 artifact、profile、证书和注册。

## Manager 命令

所有 Manager HTTP 管理命令必须位于 `manager` namespace；旧式顶层 Manager 命令会被拒绝。

### 状态与 Agent

```bash
sysarmorctl manager status
sysarmorctl manager agents list --tenant-id default
sysarmorctl manager health --tenant-id default --agent-id agent-001
sysarmorctl manager sessions --tenant-id default --agent-id agent-001
```

### Policy 与控制命令

```bash
sysarmorctl manager policies list --tenant-id default
sysarmorctl manager policies effective --tenant-id default --agent-id agent-001
sysarmorctl manager policies assign \
  --tenant default --agent agent-001 \
  --policy-id baseline --version 3 --downlink \
  --actor operator --reason 'rollout baseline v3'

sysarmorctl manager control-commands list --tenant default --agent agent-001
sysarmorctl manager control-commands create content \
  --tenant default --agent agent-001 --file content.json \
  --actor operator --reason 'refresh IOC pack'
sysarmorctl manager control-commands cancel \
  --command-id COMMAND_ID --tenant default --agent agent-001 \
  --actor operator --reason 'cancel rollout'
```

支持的控制命令类型为 `content` 和 `policy`；生命周期操作为 `cancel`、`retry`、`expire`。

### Artifact、channel 与 enrollment

```bash
sysarmorctl manager artifacts upload \
  --file dist/sysarmor-agent-linux-amd64-v1.tar.gz \
  --name sysarmor-agent --kind agent --version v1 --os linux --arch amd64
sysarmorctl manager artifacts list --kind agent --status active
sysarmorctl manager channels upsert --channel stable --artifact-id ARTIFACT_ID
sysarmorctl manager channels list
sysarmorctl manager enrollments create \
  --agent-id agent-001 --gateway-addr gateway.example:9444 \
  --channel stable --ttl 24h
sysarmorctl manager enrollments list
```

### Evidence 回拉

```bash
sysarmorctl manager evidence pullbacks \
  --create --tenant-id default --agent-id agent-001 \
  --incident-id INCIDENT_ID --target process:PID \
  --reason 'collect process tree' --actor operator
```

Evidence 回拉和 Response 都是受审计控制动作；必须提供明确目标和原因，并按租户、Agent 和 Incident 约束范围。

## 输出与错误

- 成功的本地命令输出 protobuf JSON；`--json` 禁止额外格式化文本。
- HTTP 3xx 及以上、gRPC error、无效参数和文件读取失败均返回非零退出码。
- 空列表是成功；连接、权限或依赖故障不会静默转换为空列表。
- 自动化脚本应检查退出码，并使用 `--json` 配合结构化解析。

查看当前二进制支持的完整命令：

```bash
sysarmorctl help
sysarmorctl version
```
