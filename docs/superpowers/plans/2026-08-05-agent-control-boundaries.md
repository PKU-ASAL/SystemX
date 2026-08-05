# Agent Control Boundaries Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 建立 Agent `control`、`localapi`、`remoteapi` 平级边界，消除 Remote API 对 Local API Server 的反向复用，并将 daemon 收口为 composition root。

**Architecture:** `control` 承载 Policy、Content、Response、Enrollment 和 Status 用例；`localapi` 是 Unix gRPC Server 适配器；`remoteapi` 是 Manager mTLS Client/Session 适配器。两种 API 只依赖 control 能力，不互相依赖。

**Tech Stack:** Go 1.24、gRPC/protobuf、Agent local store、Tetragon sensor runtime、Python architecture contracts、Shell E2E。

## Global Constraints

- 不改变 AgentControlPlaneService、ControlFrame、CLI、本地 socket、Manager mTLS 或持久化行为。
- Policy authority、pending activation、certificate revocation、rollback 和 fail-closed 语义保持不变。
- Control 不接受 gRPC Server/stream 或依赖 `localapi`、`remoteapi`。
- Local API 在 managed 模式继续可读，但本地写入由 control 拒绝。
- 每个任务完成 RED→GREEN 后独立提交。

---

### Task 1: 建立 control Policy 与 Content 核心

**Files:**
- Create: `apps/agent/internal/control/types.go`, `policy.go`, `content.go`
- Modify: daemon policy/content runtime files and focused tests.

**Interfaces:**
- Produces:

```go
type PolicyCommand struct {
    RequestID  string
    TenantID   string
    AgentID    string
    PolicyType string
    Document   []byte
    DryRun     bool
    Source     localstore.PolicySource
}

type SectionResult struct {
    Name, Status, Message string
}

type Result struct {
    RequestID, TenantID, AgentID string
    Status, Message, PolicyID    string
    Version                      uint64
    RequiresRestart              bool
    Sections                     []SectionResult
}

type PolicyController interface {
    ApplyPolicy(context.Context, PolicyCommand) Result
    CurrentPolicy(context.Context) (PolicySnapshot, error)
}

type ContentController interface {
    ApplyContent(context.Context, ContentCommand) Result
    ListContent(context.Context, string) ([]content.Record, error)
    GetContent(context.Context, string) (content.Record, bool, error)
}
```

`PolicySnapshot` 包含当前与 pending Policy；`ContentCommand` 包含请求身份、JSON、dry-run 和 allow-unsigned。内部 runtime ports 只能表达原子领域操作，不暴露 Server 或 stream。

- [ ] **Step 1: 写失败的统一控制测试**

在 `control/policy_test.go` 覆盖：同一文档的 standalone/managed source 共用 controller；managed sensor apply 失败保持 pending；managed authority 下 local source 写入被拒绝；持久化失败不切换 active Policy。

Run: `go test ./apps/agent/internal/control -run 'TestPolicyController' -count=1`

Expected: FAIL，因为 control 尚未实现。

- [ ] **Step 2: 迁移 Policy 用例**

整体移动 `preparedEndpointPolicy`、prepare、collection/detection/telemetry 编译、persist、activate、rollback 和 pending completion。保持锁顺序 `policyAuthorityMu -> detectionUpdateMu`，保持 sensor apply 与持久化的先后顺序。

- [ ] **Step 3: 迁移 Content 用例**

整体移动 Content prepare、Detection rebuild、commit 和失败回滚。现有 daemon/local handler 暂时成为薄 wrapper，只做 protobuf 与 control 类型转换。

- [ ] **Step 4: 验证并提交**

Run: `go fmt ./apps/agent/internal/control ./apps/agent/internal/daemon`

Run: `go test -race ./apps/agent/internal/control ./apps/agent/internal/daemon -count=1`

Run: `git add apps/agent/internal/control apps/agent/internal/daemon && git commit -m "refactor(agent): centralize policy control logic"`

### Task 2: 收口 Response、Enrollment 与 Status

**Files:**
- Create: `control/response.go`, `enrollment.go`, `status.go`
- Modify: daemon enrollment coordinator/revocation/completion、health、policy observation、response/evidence files.

**Interfaces:**
- Produces:

```go
type ResponseController interface {
    ExecuteResponse(context.Context, responsemodel.Command) responsemodel.Ack
    CollectEvidence(context.Context, controlmodel.EvidencePullbackRequest) controlmodel.EvidencePullbackResult
}

type EnrollmentController interface {
    Enroll(context.Context, EnrollmentCommand) Result
    Unenroll(context.Context, UnenrollmentCommand) Result
}

type StatusReader interface {
    Health(context.Context) (agenthealth.AgentHealth, error)
    Capability(context.Context) (agenthealth.SensorCapability, error)
    ManagementStatus(context.Context) (ManagementStatus, error)
}
```

Enrollment 命令包含本地请求身份和 Manager 授权 receipt；生命周期副作用通过 `NetworkPort`、`CredentialPort` 等窄接口注入。

- [ ] **Step 1: 写失败的控制闭环测试**

覆盖：Manager 授权且证书吊销完成后才切 standalone；吊销失败保持 managed；完成 receipt 重试；Response/Evidence 身份与结果不漂移；pending Policy 和 management lifecycle 可观测。

- [ ] **Step 2: 迁移业务逻辑**

移动 coordinator、revocation、completion、response、evidence 和 status projection。Control 不 import daemon；网络启停、凭据删除和 sensor reconcile 通过明确 port 调用。

- [ ] **Step 3: 验证并提交**

Run: `go fmt ./apps/agent/internal/control ./apps/agent/internal/daemon`

Run: `go test -race ./apps/agent/internal/control ./apps/agent/internal/daemon -count=1`

Run: `git add apps/agent/internal/control apps/agent/internal/daemon && git commit -m "refactor(agent): isolate endpoint control use cases"`

### Task 3: 提取 localapi

**Files:**
- Create: `apps/agent/internal/localapi/server.go`, `handlers.go`, `watch.go`, `codec.go`
- Move/Modify: daemon `local_control.go`, `enrollment_control.go` and tests.

**Interfaces:**
- Consumes: control controllers and a consumer-defined telemetry reader。
- Produces:

```go
type TelemetryReader interface {
    EventByID(context.Context, string) (*controlplanev1.EventFrame, bool, error)
    RecentEvents(context.Context, *controlplanev1.WatchEventsRequest) ([]*dataplanev1.EventFrame, error)
    RecentSignals(context.Context, *controlplanev1.WatchSignalsRequest) ([]*dataplanev1.SignalFrame, error)
}

type Dependencies struct {
    Policy     control.PolicyController
    Content    control.ContentController
    Response   control.ResponseController
    Enrollment control.EnrollmentController
    Status     control.StatusReader
    Telemetry  TelemetryReader
}

func New(socketPath string, deps Dependencies, out io.Writer) *Server
func (s *Server) Start(context.Context) (func(), error)
```

- [ ] **Step 1: 写失败的 Local API adapter 测试**

使用 recording controllers 验证 ApplyPolicy、ApplyContent、Enroll、Unenroll 的 protobuf 转换；复用 Unix socket 测试验证 Health、CurrentPolicy、Watch 和错误码。

- [ ] **Step 2: 实现薄 Server**

`handlers.go` 只校验请求并调用 control；`watch.go` 保留过滤、recent replay 和 backpressure；`codec.go` 编码 Ack/Health/Capability。禁止存在 Policy 持久化、证书吊销或 Detection rebuild。

- [ ] **Step 3: daemon 装配 localapi**

用 `localapi.New(...).Start(ctx)` 替换 `startLocalControlServer`，删除 `localControlServer`。将 transport-independent tests 留在 control，将 Unix/protobuf tests 移到 localapi。

- [ ] **Step 4: 验证并提交**

Run: `go fmt ./apps/agent/internal/localapi ./apps/agent/internal/daemon`

Run: `go test -race ./apps/agent/internal/localapi ./apps/agent/internal/control ./apps/agent/internal/daemon -count=1`

Run: `git add apps/agent/internal/localapi apps/agent/internal/daemon && git commit -m "refactor(agent): extract local control api"`

### Task 4: 提取 remoteapi

**Files:**
- Create: `apps/agent/internal/remoteapi/client.go`, `session.go`, `commands.go`, `reports.go`, `codec.go`
- Move/Modify: daemon control channel、transport runtime、frame converter、network supervisor and tests.

**Interfaces:**
- Consumes: 与 localapi 相同的 control controllers。
- Produces:

```go
type Config struct {
    Manager, Token            string
    TLS                       tlsconfig.ClientConfig
    RequestTimeout            time.Duration
    RetryInitial, RetryMax    time.Duration
    HealthInterval            time.Duration
}

type Dependencies struct {
    Policy     control.PolicyController
    Content    control.ContentController
    Response   control.ResponseController
    Enrollment control.EnrollmentController
    Status     control.StatusReader
}

func New(cfg Config, deps Dependencies, out io.Writer) *Client
func (c *Client) Run(context.Context) error
```

- [ ] **Step 1: 写失败的 Remote API 分发测试**

使用协议级 fake Manager 验证 PolicyUpdate 生成 managed `PolicyCommand`；Content、Response、Evidence、Health、Capability、Ack 的帧顺序和 session identity 与现有合同一致。

- [ ] **Step 2: 移动连接和 Session**

保持 mTLS、Hello/Resume、sequence、指数退避、定时健康上报和 reconnect 语义。`commands.go` 直接调用 control，不构造 localapi Server。

- [ ] **Step 3: 增加失败后转绿的依赖合同**

```python
def test_agent_control_dependency_direction(self):
    forbidden = {
        "control": ("localapi", "remoteapi"),
        "localapi": ("remoteapi",),
        "remoteapi": ("localapi",),
    }
    root = self.repo / "apps/agent/internal"
    for owner, targets in forbidden.items():
        self.assertTrue((root / owner).is_dir(), f"missing {owner}")
        for source in (root / owner).rglob("*.go"):
            text = source.read_text()
            for target in targets:
                path = f"github.com/sysarmor/sysarmor-next-project/apps/agent/internal/{target}"
                self.assertNotIn(path, text, f"{source} imports forbidden {target}")
```

先在旧 remote transport 尚未迁移完成时运行并确认正确失败；完成迁移后必须 PASS。

- [ ] **Step 4: daemon 装配 Remote API 并验证**

Run: `go fmt ./apps/agent/internal/remoteapi ./apps/agent/internal/daemon`

Run: `go test -race ./apps/agent/internal/remoteapi ./apps/agent/internal/control ./apps/agent/internal/daemon -count=1`

Run: `python3 -m unittest test.contracts.test_monorepo_layout -v`

- [ ] **Step 5: 提交**

Run: `git add apps/agent/internal/remoteapi apps/agent/internal/daemon test/contracts && git commit -m "refactor(agent): extract remote control api"`

### Task 5: 收紧 daemon composition root

**Files:**
- Modify: `apps/agent/internal/daemon/daemon.go`
- Create/Consolidate: `startup.go`, `shutdown.go`, `health.go`
- Move/Create: `apps/agent/internal/telemetry/batch.go`, `exporter.go`, `pipeline.go`
- Remove: migrated empty control/local/remote files.

**Interfaces:**
- Produces: 保持 `New`、`NewAgentRuntime`、`(*AgentRuntime).Run` 行为；daemon 只装配和管理生命周期。

- [ ] **Step 1: 写生命周期顺序测试**

使用 recording callbacks 断言 Sensor、Control、Local API、Remote API、Telemetry 的启动顺序、逆序清理，以及中途失败只清理已启动组件。

- [ ] **Step 2: 收口文件职责**

```text
daemon.go    AgentRuntime、Options、New、Run、dependency wiring
startup.go   content/policy bootstrap、sensor and component start
shutdown.go  drain timeout、sender drain、component stop、final health
health.go    collect/report health and capability aggregation
telemetry/batch.go     DataBatch identity、sequence、labels
telemetry/exporter.go  local/cloud BatchSender
telemetry/pipeline.go  normalize、detect、buffer、batch、send pipeline
```

将 daemon 的 `export_pipeline.go`、`exporter.go` 和批次构造移动到 telemetry；`local_runtime.go`、`endpoint_runtime.go` 的生命周期装配合并进 startup。Daemon 不保留 protobuf Handler、Manager frame 分发、Policy apply、Content transaction、certificate revocation 或数据批次实现。

- [ ] **Step 3: 验证并提交**

Run: `go fmt ./apps/agent/internal/daemon`

Run: `go test -race ./apps/agent/... -count=1`

Run: `git add apps/agent/internal && git commit -m "refactor(agent): reduce daemon to composition root"`

### Task 6: 全量回归与架构审查

**Files:**
- Modify only regressions introduced by this plan.

**Interfaces:**
- Produces: 绿色全量验证、职责报告、干净工作树。

- [ ] **Step 1: 静态和结构检查**

Run: `find apps packages -path '*/node_modules' -prune -o -type f -name '*.go' -exec gofmt -l {} +`

Run: `git diff --check`

Run: `python3 -m unittest test.contracts.test_monorepo_layout -v`

Expected: 前两项无输出，合同 PASS。

- [ ] **Step 2: Go 与构建验证**

Run: `go test -race ./... -count=1`

Run: `make build-binary && make build-agent-tools`

- [ ] **Step 3: 产品合同与 E2E**

Run: `make test-distribution SOURCE=local`

Run: `bash test/suites/functional/platform/e2e-control-contract.sh`

Run: `bash test/suites/functional/platform/e2e-control-roundtrip-local.sh`

Run: `bash test/suites/functional/platform/e2e-agent-gateway-manager-local.sh`

Run: `bash test/suites/functional/platform/e2e-store-status.sh`

如宿主 GOCACHE 只读，使用 `/tmp/sysarmor-core-go-cache`，不修改仓库配置。

- [ ] **Step 4: Console 验证**

Run: `pnpm --dir apps/console test && pnpm --dir apps/console lint && pnpm --dir apps/console build`

构建凭据只使用权限 `0600` 的 `/tmp` 临时占位文件并在完成后删除。

- [ ] **Step 5: 规模和职责复核**

Run: `find apps packages -path '*/node_modules' -prune -o -type f -name '*.go' -print0 | xargs -0 wc -l | sort -nr | head -30`

六个目标文件应显著缩小；略超 500 行的文件必须能用单一领域或生命周期解释。

- [ ] **Step 6: 请求独立代码审查**

使用 `requesting-code-review` 审查 `c4b5c1ad..HEAD`，重点检查外部行为漂移、authority 绕过、锁/事务变化、依赖逆转、控制逻辑重复和测试缺口。修复全部 Critical/Important 问题后重跑受影响测试。

- [ ] **Step 7: 最终工作树检查**

回归修复使用对应 `fix(...)`/`test(...)` 原子 Commit；没有修复则不创建空 Commit。运行 `git status --short`，确认没有 `bin/` 等测试生成物。
