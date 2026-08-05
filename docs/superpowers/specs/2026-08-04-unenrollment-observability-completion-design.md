# 退管可观测性与最终完成确认设计

## 目的与结论

本设计补齐 managed Agent 退管过程的可观测性和 Manager 最终完成确认。完成后，系统不仅能证明
Manager 已吊销 enrollment 证书，还能持久记录 Agent 已恢复 standalone policy、删除 managed
凭据并切换本地 authority。

采用以下边界：

- Agent local store 持有本地 enrollment 状态和待上报 completion outbox。
- `EnrollmentCoordinator` 仍是本地状态转换的唯一编排者。
- Manager 使用独立 `UnenrollmentRecord` 持有吊销与最终完成事实。
- Health 只投影状态，不驱动状态转换或重试。
- completion 使用 Agent 预生成的一次性 token；Manager 只持久化 token 哈希。

不使用已吊销的 mTLS 证书完成最终回报，因为 Agent 必须在删除 managed 凭据后仍能重试。也不把
completion 状态塞入证书模型，避免证书事实和设备生命周期事实继续耦合。

## 第一性原则

生产退管闭环必须分别回答三个问题：

1. Manager 是否已经取消该 Agent 的 managed 访问权限。
2. Agent 是否已经恢复 standalone authority 并清理 managed 凭据。
3. Manager 是否持久获知第二项已经完成。

证书吊销只能证明第一项。网络断开或旧证书被拒绝不能证明第二、三项。因此最终状态必须来自
一个经过鉴权、可幂等重放、可跨重启恢复的显式 completion 协议。

## 状态所有权

### Agent Enrollment

现有状态机保持不变：

```text
standalone -> enrolling -> managed -> unenrolling -> standalone
```

`enrollment` 继续保存本地 authority 和正在执行的转换，包括：

- `state`
- `transition_phase`
- `revocation_confirmed`
- `revoked_at`
- `revocation_receipt`
- `last_transition_error`
- `updated_at`

### Agent Completion Outbox

新增单一职责的 durable outbox。记录在请求吊销前创建，并在 Manager 确认 completion 后删除：

```text
tenant_id
agent_id
enrollment_id
certificate_serial
manager_url
revocation_receipt
completion_token
completion_token_hash
status
attempt_count
last_error
created_at
updated_at
```

`status` 仅包含 `prepared` 和 `ready`：

- `prepared`：token 已持久化，可以安全请求 Manager 吊销。
- `ready`：本地退管已全部完成，可以上报 completion。

outbox 不成为第二套 enrollment authority。即使 Manager 暂时不可达，Agent 在本地完成后仍是
standalone；outbox 只表示 Manager 最终确认尚未闭合。

### Manager Unenrollment Record

Manager 新增独立的 `UnenrollmentRecord`，以 tenant 和 enrollment ID 唯一定位：

```text
tenant_id
agent_id
enrollment_id
certificate_serial
revocation_receipt
completion_token_hash
status
revoked_at
endpoint_completed_at
created_at
updated_at
```

Manager 状态单向推进：

```text
revocation_pending -> revoked_endpoint_pending -> endpoint_completed
```

首期正常写路径会在同一事务中直接创建 `revoked_endpoint_pending`。`revocation_pending` 用于查询
投影和未来 Manager 主动退管兼容，不引入新的异步工作流。

## Health 投影

在 `HealthResponse` 新增独立的 `ManagementLifecycleStatus`：

```proto
message ManagementLifecycleStatus {
  string mode = 1;
  string transition_phase = 2;
  bool revocation_confirmed = 3;
  string manager_completion_status = 4;
  string last_transition_error = 5;
  string updated_at = 6;
}
```

`mode` 来自 enrollment state。`manager_completion_status` 根据 outbox 派生：

- 无 outbox：空值；
- `prepared`：`revocation_pending`；
- `ready`：`endpoint_completion_pending`。

Health 查询 local store 失败时显式返回错误，不返回伪造的 standalone 或 healthy 状态。退管转换错误
或 completion pending 会使总体 Health 为 `degraded`。该投影同时用于本地 CLI 和 managed Health
frame，但不修改状态、不触发回报。

## 最终完成协议

### 1. 准备吊销

Agent 使用 CSPRNG 生成至少 256 bit completion token，计算 SHA-256 哈希，并把 token、哈希、
Manager URL 和 enrollment identity 原子持久化为 `prepared`。Manager URL 在 enrollment 时规范化并
持久化，completion endpoint 只能从该 origin 派生，不能接受吊销响应提供的任意回调地址。生成或
落盘失败时不得请求证书吊销。

`RevokeEnrollmentRequest` 新增 `completion_token_hash`。Manager 验证哈希编码和 identity 后，在同一
事务中：

1. 吊销证书；
2. 创建或读取对应 `UnenrollmentRecord`；
3. 绑定 completion token hash；
4. 返回稳定的 revocation receipt。

完全相同的请求幂等返回原结果。相同 enrollment 携带不同 token hash、agent 或 serial 时返回冲突。
`RevokeEnrollment` 是唯一允许已吊销证书读取自身原吊销结果的 RPC，不能借此访问其他控制或数据
接口。

### 2. 完成本地退管

Agent 持久化吊销 receipt 后，现有 Coordinator 按以下顺序完成本地操作：

1. 停止 managed network flow；
2. 应用并订阅 standalone policy；
3. 删除 enrollment CA、证书和私钥；
4. 在同一 SQLite 事务中激活 standalone slot、清除 managed desired policy、切换 enrollment state
   并将 completion outbox 更新为 `ready`；
5. 应用 standalone runtime identity 和网络模式。

步骤 2 至 5 任一步失败都不得发送 completion。已确认吊销后不允许恢复 managed flow，只能从持久
状态继续本地收尾。

### 3. 回报最终完成

Agent 通过 Manager HTTP API 提交：

```json
{
  "schema_version": "sysarmor.unenrollment-completion/v1",
  "tenant_id": "...",
  "agent_id": "...",
  "enrollment_id": "...",
  "certificate_serial": "...",
  "revocation_receipt": "...",
  "completion_token": "..."
}
```

Manager 对 token 做 SHA-256 后常量时间比较，并校验所有绑定字段。验证成功后原子写入
`endpoint_completed` 和服务端 `endpoint_completed_at`。相同请求重复提交返回相同成功结果；token
错误或绑定字段冲突统一拒绝，响应不得泄露哪一字段不匹配。

Manager 成功响应后，Agent 才删除 completion outbox。若响应丢失，Agent 重试并获得幂等成功。

## 传输与安全边界

- completion endpoint 只允许 POST，并设置严格的请求体大小上限。
- completion token 只出现在请求体，不写日志、URL、Health、审计详情或 Manager API 响应。
- Manager 数据库只保存 token hash，Agent outbox 文件权限沿用 local store 的受限权限。
- 生产 profile 的非 loopback Manager URL 必须使用 HTTPS；显式开发和测试 profile 可以沿用受控
  环境中的 HTTP。
- Manager 使用服务端时间作为权威完成时间，不信任 Agent 时间。
- completion endpoint 不授予 enrollment、policy、data append 或其他权限。
- 成功后 token 逻辑失效；重复请求只返回原完成结果，不再次产生状态转换。

## 自动恢复

Coordinator 负责生成 outbox、推进本地状态和把记录标记为 `ready`。独立的轻量
`CompletionReporter` 只负责读取 ready outbox、调用 Manager 和清理成功记录，避免网络重试进入
EnrollmentCoordinator 临界区。

Reporter 使用有上限的指数退避并服从 Agent lifecycle context。启动时发现 ready outbox 会自动
恢复上报。`prepared` outbox 仍由 EnrollmentCoordinator 恢复吊销和本地收尾，Reporter 不越权推进
enrollment 状态。

## 失败矩阵

| 失败点 | Agent 持久状态 | Manager 持久状态 | 恢复方式 |
| --- | --- | --- | --- |
| token 生成或 outbox 落盘失败 | `managed` | 不变 | 返回失败后重试 |
| Manager 吊销事务失败 | `unenrolling` + `prepared` | 不变 | 使用同一 token hash 重试 |
| 吊销成功但响应丢失 | `unenrolling` + `prepared` | `revoked_endpoint_pending` | 已吊销证书仅重放自身吊销 RPC |
| Agent 无法持久化 receipt | `unenrolling` + `prepared` | `revoked_endpoint_pending` | 重放吊销 RPC后重新落盘 |
| standalone policy 恢复失败 | `unenrolling`，吊销已确认 | `revoked_endpoint_pending` | 禁止 managed 重连，重启后继续收尾 |
| 凭据删除或本地事务失败 | `unenrolling`，吊销已确认 | `revoked_endpoint_pending` | 幂等清理并继续本地事务 |
| completion endpoint 不可达 | `standalone` + `ready` | `revoked_endpoint_pending` | Reporter 退避重试 |
| completion 已落库但响应丢失 | `standalone` + `ready` | `endpoint_completed` | 幂等重报后清理 outbox |
| token 或 identity 冲突 | 状态不变并 degraded | 状态不变 | fail closed，等待运维处理 |

## 查询与运维可见性

Manager enrollment 查询增加以下只读字段：

- `unenrollment_status`
- `revoked_at`
- `endpoint_completed_at`

不返回 token 或 token hash。首期不修改 Web Console；稳定 API 可供后续 UI 和告警系统消费。

本地 `sysarmorctl health` 展示 management lifecycle。`sysarmorctl unenroll` 在本地退管完成但 Manager
completion 尚未确认时返回 `pending`，只有 Manager completion 成功后才返回 `applied`。请求超时
不会停止后台 Reporter。

## 测试设计

所有行为修改遵循现有 `test/` 分类和测试先行要求。

### Agent 单元测试

- Health 正确投影 managed、revocation pending、revocation confirmed 和 completion pending。
- local store 读取失败时 Health 显式失败。
- outbox 先于吊销 RPC 持久化，token 长度和文件权限满足要求。
- 本地任一步骤失败时 completion 不发送。
- completion 响应丢失、Agent 重启和重复成功均能清理 outbox。
- Reporter 不持有 enrollment 或 policy authority 锁执行网络请求。

### Manager、Gateway 与 Store 测试

- 吊销和 `UnenrollmentRecord` 在同一事务提交或回滚。
- 已吊销证书只能重放完全相同的 `RevokeEnrollment` 请求。
- token hash、identity、receipt 冲突全部 fail closed。
- completion 首次提交、重复提交和响应丢失重试均幂等。
- Postgres 读写失败不回退内存状态。
- Manager enrollment 查询只暴露状态和时间，不泄露 token hash。

### Functional 与端到端测试

- 现有 topology 在线退管扩展为等待 Manager `endpoint_completed`。
- completion endpoint 暂时不可达时 Agent 已保持 standalone，恢复后 Manager 最终收敛。
- 在吊销响应、本地完成事务和 completion 响应三个边界注入重启，最终状态一致。
- 旧证书在整个恢复过程中始终不能访问普通控制面或数据面。

### 回归门槛

- `go test ./... -count=1`
- `go vet ./...`
- Agent、Gateway、Manager API、Store 和 Postgres 包执行 `go test -race`
- `make test-functional DOMAIN=platform`
- `make test-functional DOMAIN=topology`
- `git diff --check`

## 兼容与非目标

数据库迁移为新增表和新增可选 protobuf 字段，不改变旧字段编号。旧 Agent 不发送 token hash 时，
Manager 可以继续执行 legacy 吊销，但明确记录 completion 为 `unknown_legacy`，不能伪造
`endpoint_completed`。新 Agent 必须要求 Manager 返回支持 completion 的协议结果，否则保持
`unenrolling`，避免静默降级。

本阶段不实现 Manager 主动推送退管、不实现 break-glass、不扩展 Web Console、不引入通用任务队列，
也不改变 standalone/managed policy authority 和 Sensor 模块边界。
