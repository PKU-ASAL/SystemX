# EDR Control State Hardening Design

## 目的与结论

本设计修复当前分支中 enrollment、策略运行时、Manager Store 和证书生命周期的状态所有权问题，同时保持现有 Agent 核心、可替换 Sensor、standalone/managed 双运行形态不变。

采用两个有限职责协调器：

- `EnrollmentCoordinator` 独占 enrollment 状态、网络模式、会话身份和证书生命周期切换。
- `PolicyReconciler` 独占 Sensor 策略应用、订阅建立、重试和 active policy 激活。

不引入通用工作流引擎或事件溯源框架。Store 只负责持久事实和原子领域事务，不主动驱动运行时。

## 设计原则

- Managed 退管必须在线获得 Manager 授权并完成证书吊销，不支持离线降级退管。
- desired policy 与 active policy 分离；只有 Sensor Apply 和 Subscribe 成功后才能激活。
- 连接身份、运行时事件身份和证书身份是三个独立概念，不允许隐式覆盖。
- 生产控制面存储失败必须显式返回，禁止回退进程缓存或默认策略。
- 所有远端和本地写操作都必须幂等，重试不能把已成功提交误报为失败。
- 不持有策略或 enrollment 锁等待网络 goroutine 退出。

## Enrollment 状态模型

Enrollment 使用以下持久状态：

```text
standalone -> enrolling -> managed -> unenrolling -> standalone
```

`unenrolling` 保存以下恢复信息：

- enrollment ID
- 证书序列号
- 吊销是否已由 Manager 确认
- Manager 返回的吊销时间和回执
- 当前本地收尾阶段
- 最近一次转换错误

状态的有效行为定义如下：

| 状态 | 策略 authority | 网络行为 | 本地策略修改 |
| --- | --- | --- | --- |
| `standalone` | local | standalone flow | 允许 |
| `enrolling` | local，等待首个 managed policy | managed enrollment flow | 禁止 |
| `managed` | Manager | managed flow | 禁止 |
| `unenrolling`，未确认吊销 | Manager | 保持 managed flow | 禁止 |
| `unenrolling`，已确认吊销 | local finalize only | 禁止 managed 重连 | 禁止普通修改 |

`EnrollmentCoordinator` 是唯一允许执行状态转换和切换网络 flow 的组件。网络停止和启动发生在协调器锁外，避免与控制帧策略处理形成等待环。

## 身份模型

### Runtime Identity

`runtimeIdentity` 决定本地产生事件、信号和持久遥测的归属。enrolling 阶段在 managed policy 激活前仍保持原 standalone 身份，避免历史数据提前改变租户归属。

### Session Identity

`sessionIdentity` 在建立 managed mTLS 会话时从持久 enrollment 固定获取。该会话的 Hello、Control ACK、Health、Capability 及其他出站控制帧必须统一使用 session identity，不读取可变化的 runtime identity。

### Certificate Identity

Gateway 从 TLS peer 提取 tenant ID、agent ID 和证书序列号。控制面每个 frame、数据面每个 append 请求以及 enrollment 吊销 RPC 都通过 `CertificateStore` 验证证书登记和吊销状态。Store 不可用时 fail closed。

## 在线退管协议

Agent 本地 `Unenroll` 请求执行以下流程：

1. Coordinator 将 enrollment 持久化为 `unenrolling`，保留 managed authority、网络和凭据。
2. Agent 使用当前 mTLS 身份调用 Gateway `RevokeEnrollment` RPC。
3. Gateway 校验证书身份、enrollment ID 和序列号，并原子写入 `RevokedAt` 和审计记录。
4. 同一证书重复请求返回原吊销结果；身份或 enrollment 内容冲突返回明确拒绝。
5. Agent 持久化吊销回执。若此写入失败，重启或重试时再次调用幂等 RPC 获取同一结果。
6. Coordinator 停止 managed 网络，禁止后续 managed 重连。
7. PolicyReconciler 应用保存的 standalone policy，并确认 Sensor Apply 和 Subscribe 成功。
8. Agent 使用 enrollment 中仍保存的路径删除本地凭据；失败时保留 `unenrolling` 以便重试。
9. 本地完成事务原子切换 active slot 并清除 enrollment authority。
10. Agent 切换 runtime identity，启动 standalone 网络 flow。

Manager 吊销前的任何失败都保持 managed。Manager 吊销成功后不可回滚；本地失败保留 `unenrolling` 阶段并在重启后继续收尾。

RPC 超时或连接中断属于不确定结果：Agent 保持 `unenrolling` 且有效行为仍为 managed，并自动重试获取幂等回执，不能假定 Manager 未提交。只有 Manager 返回明确的身份、权限或 enrollment 冲突拒绝时，Coordinator 才能恢复 `managed`。`RevokeEnrollment` 是唯一允许已吊销证书读取自身原吊销回执的接口，其他控制面和数据面请求全部拒绝。

## Policy Reconciler

API handler、startup 和 enrollment 不再直接调用 Sensor。它们只向 `PolicyReconciler` 提交包含 source、version、digest 和 endpoint document 的 desired policy。

Reconciler 执行：

```text
validate -> persist desired -> apply sensor -> subscribe -> activate -> publish observation
```

Sensor Apply 使用显式结果：

```go
type ApplyState string

const (
	ApplyStateApplied  ApplyState = "applied"
	ApplyStateDeferred ApplyState = "deferred"
)

type ApplyResult struct {
	State ApplyState
}
```

`Deferred` 是正常生命周期状态，不通过 sentinel error 表达。真正的 Apply 错误使 desired policy 保持 pending，active policy 不变。Reconciler 负责退避重试，并在 Health 和 CurrentPolicy 中发布 pending/degraded 信息。

同一 desired version 只有 Reconciler 能驱动 Apply。控制路径不再执行一次 Apply 后又通过 Supervisor 触发第二次 Apply。

Sensor 已应用但本地激活事务失败时，Reconciler 保持 degraded 并重试持久化，调用方不能收到 applied。所有补偿失败进入可观测状态，禁止忽略 rollback error。

## 升级兼容

v1 managed Agent 的 legacy 单一 policy 只迁移到 managed slot，不能同时伪造 standalone slot。

Agent startup 在发现 standalone slot 缺失时，从配置的 bootstrap policy 文件解析并初始化 standalone slot：

- bootstrap policy 有效时保存为 standalone fallback，但不改变当前 managed activation。
- bootstrap policy 缺失或无效时继续运行已激活的 managed policy，并报告 standalone fallback unavailable。
- standalone fallback 不可用时拒绝 Unenroll，不能用 managed policy 代替。

Installer 继续保留用户已有 bootstrap policy 文件，因此升级不会覆盖本地恢复来源。

## Manager Store 边界

Manager 和 Gateway 使用按职责拆分的窄接口：

- `PolicyQueryStore`
- `PolicyCommandStore`
- `EnrollmentStore`
- `CertificateStore`
- `AgentObservationStore`

所有生产读取返回显式 error。现有无 error 便利方法可以暂时保留给内存测试，但不能出现在生产 Server/Gateway 接口或 handler 中。

Gateway Hello 读取 effective policy、pending response、evidence 和 control command 时任一权威查询失败，整个请求返回错误，不下发缓存或默认内容。

`EnsureDefaultPolicyWithError` 成为正式 production capability，不再通过匿名 type assertion 获取。

## 写入幂等性

复合 assignment、audit 和 downlink command 事务按 command ID 幂等：

- 不存在时原子创建 assignment、audit 和 command。
- 已存在且 assignment、policy payload、target 和 command 内容一致时返回原提交结果。
- command ID 相同但内容不一致时返回 typed conflict，由 HTTP 映射为 409。
- 后端失败返回 500，不能发布进程内状态。

证书吊销同样按 tenant、agent、enrollment ID 和 serial 幂等。

## 错误恢复

| 失败点 | 持久状态 | 运行行为 | 恢复方式 |
| --- | --- | --- | --- |
| 写入 `unenrolling` 失败 | `managed` | 继续 managed | 返回失败后重试 |
| Manager 不可达或 RPC 结果不确定 | `unenrolling`，未确认 | 继续 managed | 自动重试获取幂等回执 |
| Manager 明确拒绝吊销 | 恢复 `managed` | 继续 managed | 修正权限或 enrollment 冲突后重新请求 |
| 吊销成功但回执落盘失败 | 可能仍显示未确认 | 当前连接结束后不主动切换 | 幂等重调 Manager 并落盘 |
| standalone Sensor Apply/Subscribe 失败 | `unenrolling`，已确认 | 保留采集重试，禁止 managed 重连 | Reconciler 后台重试 |
| active slot 事务失败 | `unenrolling`，已确认 | Sensor 可能已准备 standalone，状态 degraded | 重试本地完成事务 |
| 凭据删除失败 | `unenrolling`，已确认 | Sensor 已准备 standalone，禁止 managed 重连 | 按持久路径重试清理 |
| Postgres 查询失败 | 不变 | 控制请求失败 | 后端恢复后重试 |

## 测试设计

所有行为修改遵循测试先行，并接入现有 `go test`、distribution、functional 和 VM topology 体系。

### Agent 单元与并发测试

- enrolling、Sensor deferred 时，managed session 的 pending ACK、Health 和 Capability 使用 enrollment identity。
- Enroll/Unenroll 与 policy frame 并发时无死锁，并通过 `go test -race`。
- 每个 desired version 仅由 Reconciler 执行一次生命周期切换。
- Apply、Subscribe、activation transaction 分别失败时，active/pending/health 状态正确。
- Manager 不可达时 Unenroll 保持 pending，不切换策略、网络或凭据，恢复连接后继续确认。
- 吊销成功后的每个本地阶段崩溃，重启均能继续完成。

### Migration 测试

- v1 standalone policy 迁移为 standalone active slot。
- v1 managed policy 只迁移为 managed active slot。
- bootstrap policy 初始化缺失的 standalone slot，且不改变 managed activation。
- bootstrap 不可用时 Unenroll 被明确拒绝。

### Manager 与 Gateway 测试

- 证书吊销首次成功和重复成功。
- 已吊销证书不能访问控制面、数据面或再次建立有效业务会话。
- Certificate Store 不可用时认证 fail closed。
- Postgres policy/command 查询失败时 Gateway Hello 和 Manager HTTP 返回错误。
- assignment 响应丢失后的完全相同重试成功，内容冲突返回 409。

### 端到端测试

- 暂停 Tetragon 后 enrollment 进入 pending，Manager 可观察 pending；恢复后自动 managed。
- Manager 在线授权退管后恢复 bootstrap standalone policy，旧证书被拒绝。
- Manager 不可达时本地退管保持 pending 和 managed 有效行为，恢复连接后完成吊销。
- systemd 重启覆盖 enrolling、managed 和 revocation-confirmed unenrolling 状态。

### 回归门槛

- `go test ./... -count=1`
- `go vet ./...`
- 高风险 Agent、Sensor、Gateway、Store 包执行 `go test -race`
- `make -C test distribution-package`
- `make -C test functional-platform`
- `make -C test functional-topology`
- Endpoint 和 Topology Python contract tests
- `git diff --check`

## 实施边界

实施拆为四个原子阶段：

1. session identity 与 EnrollmentCoordinator。
2. PolicyReconciler 与显式 Sensor ApplyResult。
3. v1 migration、严格 Store 接口和写入幂等。
4. 在线证书吊销、恢复测试和端到端验证。

本轮不实现离线 break-glass、不引入通用任务队列、不改变 Sensor 独立模块边界，也不拆分安装包中的 Sensor 管理职责。
