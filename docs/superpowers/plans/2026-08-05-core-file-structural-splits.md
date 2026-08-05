# Core File Structural Splits Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 完成共享包准入治理、Agent 数据链路目录调整，以及 Manager Store、PostgreSQL、Tetragon 和 CLI 的低风险职责拆分。

**Architecture:** 目录迁移只改变 Agent 内部 import path；四个大文件拆分保持 package、标识符、锁、事务和业务语义不变。

**Tech Stack:** Go 1.24、Python unittest、PostgreSQL store tests、Tetragon fake/runtime tests。

## Global Constraints

- 不改变外部 API、protobuf、存储 schema、命令行为或第三方依赖。
- 完整函数、事务闭包和锁保护区整体移动。
- 每个任务独立完成 RED→GREEN 并形成原子 Commit。

---

### Task 1: packages 准入规则

**Files:**
- Modify: `test/contracts/test_monorepo_layout.py`
- Create: `packages/README.md`

**Interfaces:**
- Produces: 共享包准入说明；继续保证 `packages/*` 不依赖 `apps/*`。

- [ ] **Step 1: 增加失败合同**

```python
def test_packages_governance_is_documented(self):
    readme = self.repo / "packages/README.md"
    self.assertTrue(readme.is_file())
    text = readme.read_text()
    for phrase in ("跨产品", "apps/", "稳定契约", "生命周期"):
        self.assertIn(phrase, text)
```

Run: `python3 -m unittest test.contracts.test_monorepo_layout.MonorepoLayoutTest.test_packages_governance_is_documented -v`

Expected: FAIL，原因是 `packages/README.md` 不存在。

- [ ] **Step 2: 编写治理文档**

文档必须说明：跨产品稳定契约或两个真实消费者才能准入；单应用 helper、产品配置、生命周期编排、数据库实现和 UI 组件禁止进入；`packages/* -> apps/*` 禁止；新增包必须说明消费者、失败语义和兼容性。

- [ ] **Step 3: 验证并提交**

Run: `python3 -m unittest test.contracts.test_monorepo_layout -v`

Run: `git add packages/README.md test/contracts/test_monorepo_layout.py && git commit -m "test(architecture): define shared package boundaries"`

### Task 2: 迁移 Agent 数据链路目录

**Files:**
- Move: `endpoint/context` -> `event/context`
- Move: `endpoint/normalize` -> `event/normalize`
- Move: `endpoint/detection` -> `detection`
- Move: `endpoint/matcher` -> `detection/matcher`
- Move: `endpoint/dataappend` -> `telemetry/dataappend`
- Move: `endpoint/ringbuffer` -> `telemetry/ringbuffer`
- Modify: all affected Go imports and contract paths.

**Interfaces:**
- Produces: 相同 package names and APIs at new Agent-internal import paths。

- [ ] **Step 1: 增加失败布局合同**

```python
def test_agent_data_pipeline_layout(self):
    root = self.repo / "apps/agent/internal"
    for path in ("event/context", "event/normalize", "detection", "detection/matcher",
                 "telemetry/dataappend", "telemetry/ringbuffer"):
        self.assertTrue((root / path).is_dir(), f"missing {path}")
    self.assertFalse((root / "endpoint").exists())
```

Run: `python3 -m unittest test.contracts.test_monorepo_layout.MonorepoLayoutTest.test_agent_data_pipeline_layout -v`

Expected: FAIL，原因是新目录尚不存在。

- [ ] **Step 2: 建立绿色行为基线并移动目录**

Run: `go test ./apps/agent/internal/endpoint/... ./apps/agent/internal/telemetry/... ./apps/agent/internal/daemon/... -count=1`

使用 `git mv` 移动上述完整目录；将所有旧 import path 精确替换为新路径，不改变 package 声明或函数体；移除空 `endpoint`。

- [ ] **Step 3: 格式化、验证并提交**

Run: `go fmt ./apps/agent/...`

Run: `go test ./apps/agent/... -count=1`

Run: `python3 -m unittest test.contracts.test_monorepo_layout -v`

Run: `rg 'apps/agent/internal/endpoint' --glob '*.go'`

Expected: 测试 PASS，`rg` 无输出。

Run: `git add apps/agent test/contracts && git commit -m "refactor(agent): align endpoint data pipeline packages"`

### Task 3: 拆分 Manager Store

**Files:**
- Modify: `apps/manager/internal/store/store.go`, `agent.go`
- Create: `models.go`, `policy.go`, `control.go`, `enrollment.go`, `artifact.go`, `telemetry.go`, `persistence.go`
- Test: existing `apps/manager/internal/store/*_test.go`

**Interfaces:**
- Produces: 相同 `Store`、`State`、领域类型与方法集。

- [ ] **Step 1: 增加失败的 Store 文件职责合同**

```python
def test_manager_store_domain_files(self):
    root = self.repo / "apps/manager/internal/store"
    for name in ("models.go", "policy.go", "control.go", "enrollment.go",
                 "artifact.go", "telemetry.go", "persistence.go"):
        self.assertTrue((root / name).is_file(), f"missing store/{name}")
```

运行该用例并确认因文件缺失而 FAIL。

- [ ] **Step 2: 建立绿色行为基线**

Run: `go test -race ./apps/manager/internal/store -count=1`

- [ ] **Step 3: 按完整职责簇移动声明**

```text
models.go       State 和通用领域结构
policy.go       Policy、Assignment、Audit
control.go      Response、Control Command、Evidence pullback
agent.go        Agent、Health、Session
enrollment.go   Enrollment、Certificate、吊销
artifact.go     Artifact、Channel
telemetry.go    Event、Signal、Incident、Metrics
persistence.go  Import/Export/Save、atomic write、stable keys
store.go        Store、Open、backend binding/context、Info
```

保持 `policy_commit.go`、`enrollment_bootstrap.go`、`enrollment_issue.go`、`unenrollment.go` 独立。

- [ ] **Step 4: 验证并提交**

Run: `go fmt ./apps/manager/internal/store/...`

Run: `go test -race ./apps/manager/internal/store -count=1`

Run: `python3 -m unittest test.contracts.test_monorepo_layout -v`

Run: `git add apps/manager/internal/store test/contracts && git commit -m "refactor(manager): split store by domain"`

### Task 4: 拆分 PostgreSQL Store

**Files:**
- Modify: `apps/manager/internal/store/postgres/snapshot.go`
- Create: `policy.go`, `control.go`, `identity.go`, `artifact.go`, `telemetry.go`
- Test: PostgreSQL and backend tests.

**Interfaces:**
- Produces: 同一 `tableBackend` 方法集；SQL、参数、事务、超时不变。

- [ ] **Step 1: 增加失败的 PostgreSQL 文件职责合同**

```python
def test_postgres_store_domain_files(self):
    root = self.repo / "apps/manager/internal/store/postgres"
    for name in ("policy.go", "control.go", "identity.go", "artifact.go", "telemetry.go"):
        self.assertTrue((root / name).is_file(), f"missing postgres/{name}")
```

运行合同并确认因文件缺失而 FAIL。

- [ ] **Step 2: 建立绿色行为基线**

Run: `go test -race ./apps/manager/internal/store/postgres ./apps/manager/internal/store/backend -count=1`

- [ ] **Step 3: 移动 SQL 职责**

```text
policy.go     Policy、Assignment、Audit
control.go    Response、Command、Evidence
identity.go   Agent、Health、Session、Enrollment、Certificate
artifact.go   Artifact、Channel
telemetry.go  Metrics、Rarity、telemetry projections
snapshot.go   sqlExecutor、tableBackend、Open、timeout/transaction、SaveState
```

`CommitEnrollmentIssue`、`CommitPolicyPublication`、`CommitPolicyAssignment`、`RevokeAgentCertificate` 各自整体移动。

- [ ] **Step 4: 验证并提交**

Run: `go fmt ./apps/manager/internal/store/postgres`

Run: `go test -race ./apps/manager/internal/store/... -count=1`

Run: `python3 -m unittest test.contracts.test_monorepo_layout -v`

Run: `git add apps/manager/internal/store/postgres test/contracts && git commit -m "refactor(manager): split postgres store by domain"`

### Task 5: 拆分 Tetragon Backend

**Files:**
- Modify: `apps/agent/internal/sensors/linux/tetragon/backend.go`
- Create: `capability.go`, `tracing_policy.go`, `runtime.go`

**Interfaces:**
- Produces: 不变的 `Backend`、构造函数与 `contract.Sensor` 行为。

- [ ] **Step 1: 增加失败的 Tetragon 文件职责合同**

```python
def test_tetragon_backend_responsibility_files(self):
    root = self.repo / "apps/agent/internal/sensors/linux/tetragon"
    for name in ("capability.go", "tracing_policy.go", "runtime.go"):
        self.assertTrue((root / name).is_file(), f"missing tetragon/{name}")
```

运行合同并确认因文件缺失而 FAIL。

- [ ] **Step 2: 建立绿色行为基线**

Run: `go test -race ./apps/agent/internal/sensors/linux/tetragon -count=1`

- [ ] **Step 3: 移动职责簇**

```text
capability.go      capability、compile report、scope/selector/filter
tracing_policy.go  intent、render/write/apply/verify/delete policy、YAML helpers
runtime.go         bundle、managed process、event source、health、counters
backend.go         Backend、constructors、Apply、Subscribe、Enforce
```

- [ ] **Step 4: 验证并提交**

Run: `go fmt ./apps/agent/internal/sensors/linux/tetragon`

Run: `go test -race ./apps/agent/internal/sensors/... -count=1`

Run: `python3 -m unittest test.contracts.test_monorepo_layout -v`

Run: `git add apps/agent/internal/sensors/linux/tetragon test/contracts && git commit -m "refactor(agent): split tetragon backend responsibilities"`

### Task 6: 拆分 sysarmorctl 并完成阶段验证

**Files:**
- Modify: `apps/cli/cmd/sysarmorctl/main.go`
- Create: `local.go`, `payload.go`, `manager.go`, `manager_policy.go`, `manager_control.go`, `manager_artifact.go`, `http.go`
- Preserve: `enrollment.go`

**Interfaces:**
- Produces: 相同 CLI 命令、参数、默认值、环境变量、JSON 输出与退出码。

- [ ] **Step 1: 增加失败的 CLI 文件职责合同**

```python
def test_sysarmorctl_responsibility_files(self):
    root = self.repo / "apps/cli/cmd/sysarmorctl"
    for name in ("local.go", "payload.go", "manager.go", "manager_policy.go",
                 "manager_control.go", "manager_artifact.go", "http.go"):
        self.assertTrue((root / name).is_file(), f"missing sysarmorctl/{name}")
```

运行合同并确认因文件缺失而 FAIL。

- [ ] **Step 2: 建立绿色行为基线**

Run: `go test -race ./apps/cli/cmd/sysarmorctl -count=1`

- [ ] **Step 3: 移动命令职责**

```text
main.go              entry、usage、defaults、top-level dispatch
local.go             local query/stream、event/signal output
payload.go           policy/content payload、flags、request context、CSV
manager.go           common manager routing and URL normalization
manager_policy.go    policy operations
manager_control.go   response and command operations
manager_artifact.go  artifact、channel、multipart operations
http.go              HTTP methods and auth headers
```

- [ ] **Step 4: 验证阶段一并提交**

Run: `go fmt ./apps/cli/cmd/sysarmorctl`

Run: `go test -race ./apps/cli/cmd/sysarmorctl -count=1`

Run: `go build ./apps/cli/cmd/sysarmorctl`

Run: `go test -race ./apps/manager/internal/store/... ./apps/agent/internal/sensors/... ./apps/agent/internal/event/... ./apps/agent/internal/detection/... ./apps/agent/internal/telemetry/... -count=1`

Run: `python3 -m unittest test.contracts.test_monorepo_layout -v`

Run: `git add apps/cli/cmd/sysarmorctl test/contracts && git commit -m "refactor(cli): split sysarmorctl command domains"`
