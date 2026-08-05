# EDR Control State Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 enrollment、策略运行时、Manager Store 和证书吊销的状态所有权问题，同时保持 standalone/managed 与可替换 Sensor 边界稳定。

**Architecture:** `EnrollmentCoordinator` 独占 enrollment 与网络转换，`PolicyReconciler` 独占 Sensor Apply/Subscribe 与 policy activation。Managed session 使用固定 session identity；Gateway 通过严格 Store capability 验证证书和权威控制状态。

**Tech Stack:** Go、gRPC/protobuf、SQLite、PostgreSQL、Bash functional tests、Python contract tests。

## Global Constraints

- 生产代码修改必须先有失败测试。
- 不引入通用工作流引擎、任务队列或新的外部依赖。
- 不持锁等待网络 goroutine；错误必须显式返回或进入 Health。
- Manager 不可达时不完成退管；RPC 不确定结果必须幂等确认。
- 只修改本设计涉及的边界，不清理无关历史代码。

---

### Task 1: Managed Session Identity

**Files:**
- Modify: `internal/agent/daemon/transport_runtime.go`
- Modify: `internal/agent/daemon/control_channel.go`
- Test: `internal/agent/daemon/daemon_test.go`
- Test: `internal/gateway/grpc_test.go`

**Interfaces:**
- Consumes: enrollment identity passed to `runControlChannel`.
- Produces: session-bound ACK、Health 和 Capability frames。

- [ ] **Step 1: Write the failing identity test**

构造 runtime identity 为 `local/device-a`、session identity 为 `tenant-a/agent-a` 的 managed session，强制 policy deferred，断言 ACK、Health、Capability 都携带 session identity。

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/agent/daemon ./internal/gateway -run 'TestManagedSessionUsesEnrollmentIdentityWhilePolicyPending' -count=1`

Expected: FAIL，pending frame 仍携带 standalone identity。

- [ ] **Step 3: Implement immutable session binding**

将 frame handler 改为显式接收 identity：

```go
func (r *TransportRuntime) handleControlFrame(ctx context.Context, session *ControlChannel, identity runtimeIdentity, frame *controlplanev1.ControlFrame) error
```

发送 Health/Capability 前复制 health 并替换 tenant/agent；ACK 使用显式 session identity，不再调用 `currentIdentity()`。

- [ ] **Step 4: Verify GREEN**

Run: `go test ./internal/agent/daemon ./internal/gateway -count=1`

- [ ] **Step 5: Commit**

Commit: `fix(agent): bind managed frames to session identity`

### Task 2: Explicit Sensor Apply Result And Single Reconciler

**Files:**
- Modify: `internal/sensors/contract/contract.go`
- Modify: `internal/sensors/runtime/runtime.go`
- Modify: `internal/sensors/runtime/supervisor.go`
- Modify: `internal/sensors/linux/tetragon/backend.go`
- Modify: `internal/sensors/fake/fake.go`
- Create: `internal/agent/daemon/policy_reconciler.go`
- Modify: `internal/agent/daemon/endpoint_policy_control.go`
- Modify: `internal/agent/daemon/daemon.go`
- Test: `internal/sensors/runtime/runtime_test.go`
- Test: `internal/sensors/runtime/supervisor_test.go`
- Test: `internal/agent/daemon/endpoint_policy_control_test.go`

**Interfaces:**
- Produces: `contract.ApplyResult{State contract.ApplyState}`。
- Produces: `endpointPolicyReconciler.Submit(ctx, prepared, source) policyApplyOutcome`。
- Guarantees: API 和 startup 不直接调用 Sensor Apply。

- [ ] **Step 1: Write failing lifecycle tests**

断言 deferred Apply 是 typed result；同一 desired revision 即使更新 subscription intent 也只执行一次 Apply。

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/sensors/runtime ./internal/agent/daemon -run 'TestApplyResult|TestEndpointPolicyReconcilerAppliesDesiredVersionOnce' -count=1`

- [ ] **Step 3: Introduce explicit result**

```go
type ApplyState string

const (
	ApplyStateApplied  ApplyState = "applied"
	ApplyStateDeferred ApplyState = "deferred"
)

type ApplyResult struct { State ApplyState }
```

将 Sensor 和 Runtime Apply 改为 `(contract.ApplyResult, error)`；Tetragon 未就绪时返回 deferred，不再使用 sentinel error。

- [ ] **Step 4: Make Reconciler sole lifecycle owner**

把 desired persistence、Supervisor intent、activation callback 和 pending observation 收进 `endpointPolicyReconciler`。从 endpoint、collection、startup 和 standalone restoration 删除直接 Apply。

- [ ] **Step 5: Verify GREEN and race safety**

Run: `go test ./internal/sensors/... ./internal/agent/daemon -count=1`

Run: `go test -race ./internal/sensors/runtime ./internal/agent/daemon -count=1`

- [ ] **Step 6: Commit**

Commit: `refactor(agent): centralize sensor policy reconciliation`

### Task 3: Enrollment Coordinator And Durable Recovery

**Files:**
- Create: `internal/agent/daemon/enrollment_coordinator.go`
- Modify: `internal/agent/daemon/enrollment_control.go`
- Modify: `internal/agent/daemon/network_supervisor.go`
- Modify: `internal/agent/localstore/enrollment.go`
- Modify: `internal/agent/localstore/policy.go`
- Modify: `internal/agent/localstore/schema.go`
- Test: `internal/agent/daemon/enrollment_control_test.go`
- Test: `internal/agent/daemon/network_supervisor_test.go`
- Test: `internal/agent/localstore/enrollment_test.go`

**Interfaces:**
- Produces: `EnrollmentCoordinator.Enroll`、`Unenroll`、`Resume`。
- Produces: `localstore.CompleteUnenrollment(ctx, kind string) error`。

- [ ] **Step 1: Write deadlock and recovery tests**

用 channel 固定 policy frame 等待 authority lock 的时序，同时触发 Enroll/Unenroll，断言操作在 deadline 前结束。增加 unconfirmed/confirmed unenrollment restart fixtures。

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/agent/daemon -run 'TestEnrollmentTransitionDoesNotWaitForPolicyWhileLocked|TestConfirmedUnenrollmentResumes' -count=1`

- [ ] **Step 3: Add durable state**

Enrollment 增加 enrollment ID、certificate serial、revocation confirmation、receipt、phase 和 last error；schema 支持 `unenrolling`。本地完成事务只在凭据清理后原子激活 standalone slot 并清除 enrollment。

- [ ] **Step 4: Move orchestration behind Coordinator**

网络 stop/start 必须在 authority lock 外。所有非 standalone 状态禁止本地策略写入；unconfirmed unenrolling 保持 managed 有效行为。

- [ ] **Step 5: Verify GREEN and race safety**

Run: `go test ./internal/agent/daemon ./internal/agent/localstore -count=1`

Run: `go test -race ./internal/agent/daemon ./internal/agent/localstore -count=1`

- [ ] **Step 6: Commit**

Commit: `refactor(agent): coordinate durable enrollment transitions`

### Task 4: Correct V1 Policy Migration

**Files:**
- Modify: `internal/agent/localstore/schema.go`
- Modify: `internal/agent/daemon/startup_policy.go`
- Modify: `internal/agent/policy/endpoint.go`
- Test: `internal/agent/localstore/policy_test.go`
- Test: `internal/agent/daemon/daemon_test.go`

**Interfaces:**
- Produces: `EnsureStandaloneEndpointPolicy(ctx, store, bootstrapPath) (bool, error)`。
- Guarantees: v1 managed policy 不会被复制为 standalone。

- [ ] **Step 1: Write content-aware migration tests**

v1 DB 保存 `managed-legacy`，bootstrap 文件保存 `bootstrap-standalone`；断言 managed 保持 active，standalone slot 只含 bootstrap document。

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/agent/localstore ./internal/agent/daemon -run 'TestOpenMigratesManagedLegacyPolicyWithoutFabricatingStandalone|TestStartupInitializesStandaloneFallback' -count=1`

- [ ] **Step 3: Implement migration and startup fallback**

managed v1 只插入 managed slot。Startup 解析 bootstrap policy 初始化缺失 standalone slot但不激活；bootstrap 不可用只报告 fallback unavailable，不中断有效 managed Agent。

- [ ] **Step 4: Verify GREEN**

Run: `go test ./internal/agent/localstore ./internal/agent/daemon ./internal/agent/policy -count=1`

- [ ] **Step 5: Commit**

Commit: `fix(agent): preserve standalone policy across upgrades`

### Task 5: Strict Manager Store Boundaries And Idempotency

**Files:**
- Create: `internal/manager/api/store_capabilities.go`
- Modify: `internal/manager/api/http.go`
- Modify: `internal/manager/api/http_identity.go`
- Modify: `internal/manager/api/http_policy.go`
- Modify: `internal/manager/api/http_ui_overview.go`
- Modify: `internal/gateway/backend.go`
- Modify: `internal/gateway/control_grpc.go`
- Modify: `internal/store/store.go`
- Modify: `internal/store/policy_commit.go`
- Modify: `internal/store/postgres/snapshot.go`
- Test: `internal/manager/api/http_persistence_test.go`
- Test: `internal/gateway/grpc_test.go`
- Test: `internal/store/backend/backend_test.go`

**Interfaces:**
- Produces: narrow error-aware production capabilities。
- Produces: typed `store.ErrConflict`。

- [ ] **Step 1: Write fail-closed and replay tests**

权威读取失败时 Gateway Hello 和 Manager API 必须返回 internal error。相同 assignment+audit+command 重试返回原成功结果；同 ID 不同 payload 返回 conflict。

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/store/... ./internal/manager/api ./internal/gateway -run 'Test.*FailClosed|TestPolicyAssignmentCommandReplay' -count=1`

- [ ] **Step 3: Split production capabilities**

Manager/Gateway 只依赖窄接口，生产 handlers 全部改用显式 error reads。Gateway Hello 使用 strict pending queries；default policy initialization 进入正式 capability。

- [ ] **Step 4: Implement transactional replay**

Postgres assignment transaction 内读取 tenant+command ID。normalized command 与 assignment 一致时成功重放；不一致返回 `ErrConflict`，HTTP 映射 409。

- [ ] **Step 5: Verify GREEN**

Run: `go test ./internal/store/... ./internal/manager/api ./internal/gateway -count=1`

- [ ] **Step 6: Commit**

Commit: `refactor(manager): enforce strict control store boundaries`

### Task 6: Online Certificate Revocation

**Files:**
- Modify: `api/proto/controlplane/v1/agentcontrol.proto`
- Regenerate: `api/proto/controlplane/v1/agentcontrol.pb.go`
- Regenerate: `api/proto/controlplane/v1/agentcontrol_grpc.pb.go`
- Create: `internal/store/certificate.go`
- Modify: `internal/store/backend.go`
- Modify: `internal/store/postgres/snapshot.go`
- Modify: `internal/gateway/identity.go`
- Modify: `internal/gateway/control_grpc.go`
- Create: `internal/agent/daemon/enrollment_revocation_client.go`
- Modify: `internal/agent/daemon/enrollment_coordinator.go`
- Test: `internal/store/store_test.go`
- Test: `internal/gateway/grpc_test.go`
- Test: `internal/agent/daemon/enrollment_control_test.go`

**Interfaces:**
- Produces: gRPC `RevokeEnrollment` 和幂等 receipt。
- Produces: strict `CertificateStore` validation/revocation。

- [ ] **Step 1: Write revocation tests**

首次和重复 revoke 返回相同 receipt；身份冲突拒绝；已吊销证书不能发送 control/data frame；仅 revoke RPC 可读取自身原 receipt。

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/store ./internal/gateway ./internal/agent/daemon -run 'Test.*Revok' -count=1`

- [ ] **Step 3: Add protobuf and durable operation**

请求包含 enrollment ID 和 serial；响应包含 status、revoked timestamp、receipt ID。使用仓库生成命令更新 protobuf。memory/file/Postgres 原子持久化 revocation 和 audit。

- [ ] **Step 4: Enforce Gateway certificate status**

从 TLS peer 提取 serial。所有业务 control/data 请求查询证书状态并 fail closed；匹配的已吊销证书只能调用 revoke 获取旧 receipt。

- [ ] **Step 5: Connect Coordinator**

RPC 前持久化 unenrolling；timeout 保持 pending 并重试；确认后停止 managed flow、reconcile standalone、删除凭据、提交本地完成事务并启动 standalone flow。

- [ ] **Step 6: Verify GREEN and race safety**

Run: `go test ./internal/store/... ./internal/gateway ./internal/agent/daemon -count=1`

Run: `go test -race ./internal/store ./internal/gateway ./internal/agent/daemon -count=1`

- [ ] **Step 7: Commit**

Commit: `feat(security): require manager-authorized certificate revocation`

### Task 7: Functional And Topology Regression

**Files:**
- Modify: `test/suites/functional/platform/e2e-manager-api-policy.sh`
- Modify: `test/suites/functional/topology/e2e-systemd-vm.sh`
- Modify: `test/suites/functional/topology/test_e2e_contract.py`
- Modify: `test/shared/agent/enroll.sh`

**Interfaces:**
- Consumes: public CLI、Manager API、Gateway mTLS、local control socket。
- Produces: pending enrollment、online revocation、restart recovery 和旧证书拒绝的回归覆盖。

- [ ] **Step 1: Extend functional contracts first**

增加 Tetragon 暂停期间 enrollment pending、恢复后 managed、在线 Unenroll、旧证书重试和 confirmed finalize 中途重启场景。

- [ ] **Step 2: Verify RED**

Run: `make -C test functional-platform`

Run: `make -C test functional-topology`

Expected: FAIL 在 pending enrollment、online revoke 或 restart finalize 合约断言，不是环境准备步骤。

- [ ] **Step 3: Add only required integration wiring**

复用现有 shared enrollment helpers，不增加第二套安装或 enrollment 流程。

- [ ] **Step 4: Run full verification**

Run: `go test ./... -count=1`

Run: `go vet ./...`

Run: `make -C test distribution-package`

Run: `make -C test functional-platform`

Run: `make -C test functional-topology`

Run: `git diff --check`

- [ ] **Step 5: Request final code review**

按设计文档复核完整 diff，修复所有 Critical/Important finding，并重跑受影响测试。

- [ ] **Step 6: Commit**

Commit: `test(edr): cover managed lifecycle recovery`
