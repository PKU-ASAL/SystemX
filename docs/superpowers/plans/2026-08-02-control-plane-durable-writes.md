# Control Plane Durable Writes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让六条 EDR 控制面高敏写路径显式报告持久化失败，并保证失败不会形成可见的未持久化状态或策略复合操作的部分提交。

**Architecture:** Store 使用明确的创建/更新错误签名，Backend 增加单条控制命令 upsert。单实体操作先完成持久化再发布内存状态；策略发布和策略分配由专用复合提交方法通过现有 `SaveState` 事务一次保存，并在失败时恢复本次操作前状态。

**Tech Stack:** Go、`net/http`、gRPC、PostgreSQL、Go testing

## Global Constraints

- 只修改响应、控制命令、策略发布和策略分配相关写路径。
- 不保留吞掉持久化错误的兼容入口。
- 不引入通用事务框架或新依赖。
- 所有生产代码修改必须先有能正确失败的测试。

---

### Task 1: 控制命令持久化接口

**Files:**
- Modify: `internal/store/backend.go`
- Modify: `internal/store/postgres/snapshot.go`
- Test: `internal/store/backend/backend_test.go`

**Interfaces:**
- Produces: `WriteControlCommand(context.Context, controlmodel.ControlCommand) error`

- [ ] **Step 1: 写入失败/成功的单条控制命令后端测试**

测试通过 `WriteControlCommand` 写入一条命令，再用 `ListControlCommands` 验证字段；错误数据库执行器返回错误时断言 error 非空。

- [ ] **Step 2: 运行测试确认 RED**

Run: `go test ./internal/store/backend -run 'Test.*WriteControlCommand'`
Expected: FAIL，原因是 `WriteControlCommand` 尚不存在。

- [ ] **Step 3: 实现最小 upsert**

在 `Backend` 增加方法，并在 PostgreSQL table backend 中复用控制命令投影的单条 upsert SQL，不执行全量 `SaveState`。

- [ ] **Step 4: 运行测试确认 GREEN**

Run: `go test ./internal/store/backend -run 'Test.*WriteControlCommand'`
Expected: PASS。

### Task 2: Store 六条显式错误路径

**Files:**
- Modify: `internal/store/store.go`
- Test: `internal/store/store_test.go`

**Interfaces:**
- Produces: `CreateResponse(responsemodel.Command) (responsemodel.Command, error)`
- Produces: `AckResponse(responsemodel.Ack) (responsemodel.Command, bool, error)`
- Produces: `CreateControlCommand(controlmodel.ControlCommand) (controlmodel.ControlCommand, error)`
- Produces: `AckControlCommand(controlmodel.ControlCommandAck) (controlmodel.ControlCommand, bool, error)`
- Produces: `PublishPolicy(string, string, uint64, bool) (policymodel.Policy, bool, error)`
- Produces: `AssignPolicy(policymodel.Assignment) (policymodel.Assignment, bool, error)`

- [ ] **Step 1: 为六条路径编写失败后端测试**

每个测试使用只覆盖目标 `Write*` 的失败 Backend，断言 error 包含操作上下文、`bool` 不把基础设施失败表达为 not-found，并比较调用前后 Store 切片完全相同。

- [ ] **Step 2: 运行测试确认 RED**

Run: `go test ./internal/store -run 'Test(Create|Ack|Publish|Assign).*ReturnsBackendFailure'`
Expected: FAIL，原因是当前签名没有 error 或错误被忽略。

- [ ] **Step 3: 实现持久化后发布状态**

在锁保护下计算候选值，调用对应 Backend writer；成功后替换/追加内存值，失败直接返回并保留旧值。控制面吞错的旧签名不再保留。

- [ ] **Step 4: 迁移 Store 和 backend 包内测试调用**

所有测试显式断言或忽略仅限内存 Store 的 error；所有 `bool` 调用改为三返回值。

- [ ] **Step 5: 运行 Store 测试确认 GREEN**

Run: `go test ./internal/store ./internal/store/backend`
Expected: PASS。

### Task 3: Manager 与 Gateway 错误传播

**Files:**
- Modify: `internal/manager/api/http.go`
- Modify: `internal/manager/api/http_policy.go`
- Modify: `internal/manager/api/http_responses.go`
- Modify: `internal/manager/api/http_control.go`
- Modify: `internal/gateway/backend.go`
- Modify: `internal/gateway/control_grpc.go`
- Test: `internal/manager/api/http_policy_test.go`
- Test: `internal/manager/api/http_responses_test.go`
- Test: `internal/manager/api/http_control_test.go`
- Test: `internal/gateway/grpc_test.go`

**Interfaces:**
- Consumes: Task 2 的六个显式错误签名。

- [ ] **Step 1: 编写 HTTP 和 gRPC 失败传播测试**

Manager 测试断言后端失败返回 `500`；Gateway 测试断言 response/control ACK 写失败返回 `codes.Internal` 且没有 accepted frame。

- [ ] **Step 2: 运行测试确认 RED**

Run: `go test ./internal/manager/api ./internal/gateway -run 'Test.*PersistenceFailure'`
Expected: FAIL，当前接口不能传播目标错误。

- [ ] **Step 3: 更新接口与调用方**

Manager 对 `error` 先返回 `500`，再判断 `ok`；Gateway 对 `error` 返回 `codes.Internal`，对 `ok=false` 返回 `codes.NotFound`。成功路径保留现有响应格式。

- [ ] **Step 4: 迁移仓库其余调用点并运行定向测试**

Run: `go test ./internal/manager/api ./internal/gateway ./internal/agent/daemon`
Expected: PASS。

### Task 4: 策略复合事务

**Files:**
- Modify: `internal/store/store.go`
- Modify: `internal/manager/api/http.go`
- Modify: `internal/manager/api/http_policy.go`
- Test: `internal/store/store_test.go`
- Test: `internal/manager/api/http_policy_test.go`

**Interfaces:**
- Produces: 专用策略发布提交方法，原子保存 policy 与 audit。
- Produces: 专用策略分配提交方法，原子保存 assignment、audit 与可选 control command。

- [ ] **Step 1: 写复合提交失败回滚测试**

使用 `SaveState` 失败 Backend，分别断言发布失败后 policy/audit 未变化，分配失败后 assignment/audit/control command 均未变化。

- [ ] **Step 2: 运行测试确认 RED**

Run: `go test ./internal/store ./internal/manager/api -run 'TestPolicy.*Atomic|TestPolicy.*Rollback'`
Expected: FAIL，当前流程在保存失败后保留内存修改。

- [ ] **Step 3: 实现两个专用复合提交方法**

保存目标切片的操作前副本，在同一锁域内应用业务变化并导出候选 State，通过 `Backend.SaveState` 或原子文件写入提交；失败恢复副本。方法保持在 50 行以内，共享的保存逻辑提取为小型内部 helper。

- [ ] **Step 4: Manager 改用复合提交方法**

`policyPublish` 不再分开调用 publish、audit、Save；`policyAssignments` 不再分开调用 assign、audit、create command、Save。

- [ ] **Step 5: 运行定向测试确认 GREEN**

Run: `go test ./internal/store ./internal/manager/api`
Expected: PASS。

### Task 5: 全量验证与复核

**Files:**
- Modify: only files required by previous tasks

- [ ] **Step 1: 格式化修改文件**

Run: `gofmt -w <本计划修改的 Go 文件>`
Expected: 无输出。

- [ ] **Step 2: 运行全量测试和静态检查**

Run: `go test ./...`
Expected: PASS。

Run: `go vet ./...`
Expected: PASS。

- [ ] **Step 3: 运行高风险包竞态测试**

Run: `go test -race ./internal/store ./internal/manager/api ./internal/gateway`
Expected: PASS。

- [ ] **Step 4: 审查 diff**

确认没有吞错兼容方法、没有超出范围的重构、没有未处理的六方法调用点，并按关注点形成 Conventional Commits。
