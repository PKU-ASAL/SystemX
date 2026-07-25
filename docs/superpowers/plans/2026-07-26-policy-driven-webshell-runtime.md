# Policy-Driven WebShell Runtime Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 使用现有通用 `sequence` 运行时表达 `web_runtime_spawns_shell`，严格拒绝无效规则，并删除 WebShell 专用 Go 状态和匹配逻辑，使规则语义可以通过 rulepack/contextset 更新。

**Architecture:** 保留 CanonicalEvent 和现有有界 CEP 状态，以 `same_as + step + step_field` 关联父进程 `stableId` 与 Shell `parentStableId`。内置默认内容只提供离线 standalone 的默认 rulepack/contextset；外部同类内容仍通过现有 Content Store 和 Detection Policy 加载。第一阶段不新增 DSL、图执行器、count/absence，也不迁移其他复杂 builtin。

**Tech Stack:** Go、JSON rulepack/contextset、现有 endpoint detection engine、Agent Content Store、Go testing。

## Global Constraints

- 新增和调整具体 WebShell 规则不得依赖重新编译专用匹配代码。
- 二进制中的内容集合和 Provider 使用威胁无关名称；禁止新增 WebShell 专用状态。
- 不支持任意 Tetragon YAML、任意代码、循环、递归或无界窗口。
- 规则语法错误、未知字段、未知 operator、非法 step 引用必须 rejected，不得静默编译为永不命中的规则。
- 同一 Rule ID 只有一个权威执行路径，不允许 builtin 与 sequence 双发 Signal。
- 所有改动遵循 TDD，并以独立 Conventional Commit 提交。

---

## File Structure

- Create `internal/endpoint/detection/validation.go`: RuleSpec、字段、operator、sequence step 和引用的静态校验。
- Create `internal/endpoint/detection/builtin_content.go`: standalone 默认 contextset 值和通用内容查询。
- Create `internal/endpoint/detection/builtin_rules.go`: 内置默认 RuleSpec；WebShell 使用通用 sequence。
- Modify `internal/endpoint/detection/engine.go`: 调用严格校验；删除 WebShell 专用 detector 和 lineage 状态。
- Modify `internal/endpoint/detection/compiled.go`: 暴露字段/operator 是否受支持的纯校验函数，内容查询回退到内置 contextset。
- Modify `internal/endpoint/detection/engine_test.go`: WebShell sequence、伪造 argv、父进程关系和动态内容覆盖测试。
- Modify `internal/agent/content/store_test.go`: WebShell sequence rulepack 解析契约。
- Modify `internal/agent/daemon/local_control_test.go`: 动态内容应用后重建 Detection Engine 的集成契约。
- Modify `internal/policy/model.go`: 默认 Detection Policy 引用通用 Web Runtime 和 Shell contextset。
- Modify `test/data/content/rulepack-cep-endpoint.json`: 增加声明式 WebShell 规则。
- Create `test/data/content/context-web-runtime-binaries.json`: 可更新 Web Runtime binary 集合。
- Create `test/data/content/context-shell-binaries.json`: 可更新 Shell binary 集合。
- Modify `test/data/policies/detection-cep-endpoint.json`: 引用新增 contextset。

### Task 1: Strict Rule Compilation Validation

**Files:**
- Create: `internal/endpoint/detection/validation.go`
- Modify: `internal/endpoint/detection/engine.go`
- Modify: `internal/endpoint/detection/compiled.go`
- Test: `internal/endpoint/detection/engine_test.go`

**Interfaces:**
- Produces: `validateRuleSpecs(rules []effectiveRule) []string`
- Consumes: `compileField(string) fieldID`、`compileOp(string) conditionOp`
- Contract: 未知字段/operator、非法 `by`、重复或向前引用 step、`same_as` 缺失 step 必须在 `NewWithRuntimeLimits` 中返回 `ApplyReport.Status == "rejected"`。

- [ ] **Step 1: Write failing validation tests**

在 `engine_test.go` 增加表驱动测试，至少覆盖：

```go
func TestRuleValidationRejectsUnknownFieldsAndOperators(t *testing.T) {
	tests := []struct {
		name string
		cond ConditionSpec
		want string
	}{
		{name: "field", cond: ConditionSpec{Field: "process.unknown", Op: "eq", Value: "x"}, want: "unsupported field"},
		{name: "operator", cond: ConditionSpec{Field: "process.binary", Op: "magic", Value: "x"}, want: "unsupported operator"},
	}
	// 为每个 case 构造 expr RuleSpec，断言 report rejected 且 Details 包含 want。
}

func TestRuleValidationRejectsInvalidSequenceReferences(t *testing.T) {
	// 构造第二步 same_as 一个不存在或位于后面的 step，断言 rejected。
}
```

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
GOCACHE=/tmp/sysarmor-go-build go test ./internal/endpoint/detection -run 'TestRuleValidationRejects' -count=1
```

Expected: FAIL，因为当前未知字段/operator 会被编译为 `fieldUnknown`/zero op，但 ApplyReport 仍可能 applied。

- [ ] **Step 3: Implement minimal static validation**

`validation.go` 按规则类型检查：

```go
func validateRuleSpecs(rules []effectiveRule) []string {
	var out []string
	for _, rule := range rules {
		out = append(out, validateRuleSpec(rule.spec)...)
	}
	return out
}
```

校验必须包含：字段、operator、`by` 字段、step ID 唯一且非空、`same_as` 只能引用已出现 step、`step_field` 有效。`engine.go` 将该结果合并到现有 `validateEffectiveRules`，任何错误使 report rejected。

- [ ] **Step 4: Run focused and package tests**

```bash
GOCACHE=/tmp/sysarmor-go-build go test ./internal/endpoint/detection -count=1
```

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/endpoint/detection/validation.go internal/endpoint/detection/compiled.go internal/endpoint/detection/engine.go internal/endpoint/detection/engine_test.go
git commit -m "fix(detection): reject invalid runtime rules"
```

### Task 2: Generic Default Context Sets

**Files:**
- Create: `internal/endpoint/detection/builtin_content.go`
- Modify: `internal/endpoint/detection/compiled.go`
- Modify: `internal/policy/model.go`
- Test: `internal/endpoint/detection/engine_test.go`

**Interfaces:**
- Produces: `builtinContentValues(ref string) []string`
- Context refs: `ctx:web-runtime-binaries`、`ctx:shell-binaries`
- Contract: ContentSnapshot 中的动态值优先；不存在动态值时回退 standalone 默认值。

- [ ] **Step 1: Write failing content override tests**

```go
func TestRuntimeContentOverridesBuiltinBinarySet(t *testing.T) {
	content := ContentSnapshot{ContextRefs: map[string]ContentRef{
		"ctx:web-runtime-binaries": {Ref: "ctx:web-runtime-binaries", Version: "v2", Values: []string{"custom-web"}},
	}}
	// 断言 custom-web 可匹配，默认 node 在该动态快照下不再匹配。
}
```

- [ ] **Step 2: Run test and verify RED**

```bash
GOCACHE=/tmp/sysarmor-go-build go test ./internal/endpoint/detection -run TestRuntimeContentOverridesBuiltinBinarySet -count=1
```

Expected: FAIL，因为默认 WebShell 仍由专用 Go matcher 执行。

- [ ] **Step 3: Add generic builtin content lookup**

`builtin_content.go` 只包含中性内容：

```go
var builtinContextValues = map[string][]string{
	"ctx:web-runtime-binaries": {"nginx", "apache2", "httpd", "php-fpm", "gunicorn", "uwsgi", "tomcat", "node", "nodejs"},
	"ctx:shell-binaries":       {"sh", "bash", "dash", "zsh", "ksh"},
}
```

`contentValuesFromSnapshot` 在动态 ref 存在时返回其值，否则返回 clone 后的 builtin 值。默认 Detection Policy 增加两个 ContextRef，版本为 `builtin`。

- [ ] **Step 4: Run package tests**

```bash
GOCACHE=/tmp/sysarmor-go-build go test ./internal/endpoint/detection ./internal/policy -count=1
```

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/endpoint/detection/builtin_content.go internal/endpoint/detection/compiled.go internal/endpoint/detection/engine_test.go internal/policy/model.go
git commit -m "feat(detection): add generic process binary contexts"
```

### Task 3: Migrate WebShell to the Sequence Runtime

**Files:**
- Create: `internal/endpoint/detection/builtin_rules.go`
- Modify: `internal/endpoint/detection/engine.go`
- Test: `internal/endpoint/detection/engine_test.go`
- Test: `internal/endpoint/detection/effectiveness_test.go`

**Interfaces:**
- Produces: embedded standalone default `RuleSpec{RuleID: "web_runtime_spawns_shell", RuntimeType: "sequence"}`
- Consumes: `same_as` cross-step comparison and generic context refs from Task 2。
- Contract: Signal 必须引用 Web Runtime 和 Shell 两个 Event；伪造 argv 和非 Web 父进程不触发。

- [ ] **Step 1: Strengthen WebShell tests before implementation**

修改现有测试，除了 Signal 数量还断言：

```go
if got, want := signal.GetEventRefs(), []string{"event-node", "event-shell"}; !slices.Equal(got, want) {
	t.Fatalf("event refs = %v, want %v", got, want)
}
```

增加不同 lineage 或错误 parentStableId 不触发的负例。

- [ ] **Step 2: Run tests and verify RED**

```bash
GOCACHE=/tmp/sysarmor-go-build go test ./internal/endpoint/detection -run 'TestWebRuntimeShell' -count=1
```

Expected: FAIL，因为专用 builtin Signal 当前只引用 Shell Event。

- [ ] **Step 3: Define the default sequence rule**

在 `builtin_rules.go` 中将 WebShell 默认规则定义为：

```go
SequenceSpec{
	Within: 30 * time.Second,
	By:     []string{"lineage_id"},
	Steps: []StepSpec{
		{ID: "runtime", Behavior: "process.exec", Conditions: []ConditionSpec{{Field: "process.binary_name", Op: "in", Ref: "ctx:web-runtime-binaries"}}},
		{ID: "shell", Behavior: "process.exec", Conditions: []ConditionSpec{
			{Field: "process.binary_name", Op: "in", Ref: "ctx:shell-binaries"},
			{Field: "parent.stable_id", Op: "same_as", Step: "runtime", StepField: "process.stable_id"},
		}},
	},
}
```

binary 集合匹配必须按 basename 语义完成，不能要求 contextset 写死发行版完整路径。新增中性规范字段 `process.binary_name`，由 Event View 从 `process.binary` 计算，而不是在规则中硬编码所有路径。

- [ ] **Step 4: Remove the specialized execution path**

从 `Engine.Process` 删除 `detectWebRuntimeShell` 调用；删除 `webShellExecRefs`、`processBinaryByStableID`、`detectWebRuntimeShell` 和 `looksLikeWebRuntime`。保留其他 builtin 仍使用的状态，不做无关迁移。

- [ ] **Step 5: Run detection regression and benchmarks**

```bash
GOCACHE=/tmp/sysarmor-go-build go test ./internal/endpoint/detection -count=1
GOCACHE=/tmp/sysarmor-go-build go test ./internal/endpoint/detection -run '^$' -bench 'BenchmarkEngineProcessBuiltinMixed|BenchmarkEngineProcessSequenceHeavy' -benchtime=100ms
```

Expected: tests PASS；benchmark 无 panic，输出 `cep_scans/op`、`conds/op`。

- [ ] **Step 6: Commit**

```bash
git add internal/endpoint/detection/builtin_rules.go internal/endpoint/detection/engine.go internal/endpoint/detection/engine_test.go internal/endpoint/detection/effectiveness_test.go
git commit -m "refactor(detection): express web shell as sequence rule"
```

### Task 4: Dynamic Rulepack and Agent Integration

**Files:**
- Modify: `internal/agent/content/store_test.go`
- Modify: `internal/agent/daemon/local_control_test.go`
- Modify: `test/data/content/rulepack-cep-endpoint.json`
- Create: `test/data/content/context-web-runtime-binaries.json`
- Create: `test/data/content/context-shell-binaries.json`
- Modify: `test/data/policies/detection-cep-endpoint.json`

**Interfaces:**
- Consumes: existing `runtime.type = "sequence"` JSON schema。
- Produces: 可独立更新 Web Runtime/Shell 集合和 WebShell sequence 的内容包。
- Contract: Agent 应用 rulepack/contextset 后重建引擎；不重启 Agent，不调用专用 builtin。

- [ ] **Step 1: Write failing Content Store and daemon tests**

Content Store 测试解析真实 sequence JSON，并断言：

```go
rule.RuntimeType == "sequence"
rule.Sequence.Steps[1].Conditions[1].Step == "runtime"
rule.Sequence.Steps[1].Conditions[1].StepField == "process.stable_id"
```

daemon 测试依次应用两个 contextset、rulepack 和 Detection Policy，再输入父子 Event，断言产生双 Event refs 的 WebShell Signal。

- [ ] **Step 2: Run tests and verify RED**

```bash
GOCACHE=/tmp/sysarmor-go-build go test ./internal/agent/content ./internal/agent/daemon -run 'WebRuntime|WebShell|RulePack' -count=1
```

Expected: FAIL，因为测试内容尚未包含声明式 WebShell 规则。

- [ ] **Step 3: Add test content and policy fixtures**

两个 contextset 使用 `value_type: process_binary_name`。Rulepack 中 WebShell rule 使用 Task 3 相同的 sequence；`requires.events` 明确声明 `process.binary_name`、`process.stable_id`、`parent.stable_id` 和 `lineage_id`。

- [ ] **Step 4: Run Agent integration tests**

```bash
GOCACHE=/tmp/sysarmor-go-build go test ./internal/agent/content ./internal/agent/daemon -count=1
```

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/agent/content/store_test.go internal/agent/daemon/local_control_test.go test/data/content test/data/policies/detection-cep-endpoint.json
git commit -m "test(detection): validate dynamic web shell rulepack"
```

### Task 5: End-to-End Regression and Documentation

**Files:**
- Modify only if tests expose a scoped defect: files from Tasks 1–4
- Verify: `test/release/test-assert.sh`
- Verify: `test/release/fixtures/test-fixtures.sh`
- Verify: `test/suites/product/endpoint/release-container-e2e-contract.sh`

**Interfaces:**
- Produces: 第一阶段验收证据和干净工作区。

- [ ] **Step 1: Run formatting and static checks**

```bash
gofmt -w internal/endpoint/detection/*.go internal/agent/content/*_test.go internal/agent/daemon/*_test.go
git diff --check
```

Expected: no output from `git diff --check`。

- [ ] **Step 2: Run affected Go packages**

```bash
GOCACHE=/tmp/sysarmor-go-build go test ./internal/endpoint/detection ./internal/policy ./internal/agent/content ./internal/agent/daemon ./internal/sensors/linux/tetragon -count=1
```

Expected: PASS。

- [ ] **Step 3: Run Release contracts**

```bash
bash test/release/test-assert.sh
bash test/release/fixtures/test-fixtures.sh
bash test/suites/product/endpoint/release-container-e2e-contract.sh
```

Expected: 三项均输出 `ok`。

- [ ] **Step 4: Review migration invariants**

确认：

```bash
rg -n 'detectWebRuntimeShell|looksLikeWebRuntime|processBinaryByStableID|webShellExecRefs' internal/endpoint/detection
```

Expected: no matches。确认 `web_runtime_spawns_shell` 只存在于 sequence RuleSpec、rulepack fixture、policy metadata 和测试期望中。

- [ ] **Step 5: Final atomic fix commit if required**

仅当回归暴露本阶段引入的问题时创建：

```bash
git add internal/endpoint/detection internal/agent/content/store_test.go internal/agent/daemon/local_control_test.go test/data/content test/data/policies/detection-cep-endpoint.json
git commit -m "fix(detection): complete web shell runtime migration"
```

若无额外修改，不创建空提交。
