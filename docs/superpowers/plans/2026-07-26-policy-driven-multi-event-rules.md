# Policy-Driven Multi-Event Rules Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 新增通用有界条件树和无序 `correlate` runtime，将剩余三条 endpoint builtin 检测规则迁移到动态 rulepack，并删除规则专用 lineage 状态。

**Architecture:** Content Store 解析稳定 JSON schema，daemon 转换为 Detection RuleSpec，validation 联合拒绝非法输入，compiler 将条件树和 correlate 编译为按 behavior 索引的执行计划。通用 runtime 不包含规则名称或威胁语义；三条规则只是首批 rulepack 消费者。

**Tech Stack:** Go、JSON rulepack/contextset/iocpack、endpoint detection engine、Bash Release harness、Tetragon。

## Global Constraints

- 单 Agent、有限事实、有限窗口、明确关联键。
- `correlate.within` 必须位于 `(0, 24h]`。
- 每规则活动分组与 Event refs 继续受 `MaxCEPGroups` 和 `MaxCEPRefs` 限制。
- 同一 Rule ID 只能有一个 Signal 生成路径。
- 通用 runtime 测试使用中性规则名称，不引用三条迁移规则的专用语义。
- 不实现跨主机、任意图遍历、循环、递归或任意代码执行。
- 每个任务遵循 RED、GREEN、REFACTOR，并创建原子 Conventional Commit。

---

### Task 1: Add Generic Boolean Condition Trees

**Files:**
- Modify: `internal/agent/content/store.go`
- Test: `internal/agent/content/store_test.go`
- Modify: `internal/agent/daemon/daemon.go`
- Test: `internal/agent/daemon/daemon_test.go`
- Modify: `internal/endpoint/detection/engine.go`
- Modify: `internal/endpoint/detection/compiled.go`
- Modify: `internal/endpoint/detection/validation.go`
- Test: `internal/endpoint/detection/engine_test.go`

**Interfaces:**
- Produces: `ConditionNodeSpec{All, Any []ConditionNodeSpec; Not *ConditionNodeSpec; Condition *ConditionSpec}`，每个节点加载时必须且只能设置一种节点类型。
- Consumes: existing `ConditionSpec`, `compileCondition`, `eventView` and condition metrics.
- JSON: `condition_group: {"any":[{"condition":{...}},{"all":[...]}]}`; old `conditions` remains implicit `all`.

- [ ] Add parser tests for nested `all/any/not/condition`, preserving all leaf fields and old flat conditions.
- [ ] Run `GOCACHE=/tmp/sysarmor-go-build go test ./internal/agent/content -run TestStoreParsesConditionTree -count=1`; expect RED because schema is absent.
- [ ] Add only the Content schema and parsing required for the test; run it to GREEN.
- [ ] Add Detection validation tests rejecting empty groups, multiple node kinds, invalid `not`, excessive depth above 8, unknown fields and unknown operators.
- [ ] Run `GOCACHE=/tmp/sysarmor-go-build go test ./internal/endpoint/detection -run TestRuleValidationRejectsInvalidConditionTree -count=1`; expect RED.
- [ ] Implement recursive validation and compilation with maximum depth 8 and maximum 256 leaf conditions per rule.
- [ ] Add neutral runtime tests proving nested boolean semantics and dynamic Context replacement:

```go
// (process.binary_name in ctx:test-tools OR process.argv contains ctx:test-markers)
// AND socket.port in ioc:test-ports
```

- [ ] Run detection/content/daemon focused tests; expect PASS.
- [ ] Commit `feat(detection): add generic boolean condition trees`.

### Task 2: Add Generic Bounded Correlate Runtime

**Files:**
- Modify: `internal/agent/content/store.go`
- Test: `internal/agent/content/store_test.go`
- Modify: `internal/agent/daemon/daemon.go`
- Test: `internal/agent/daemon/daemon_test.go`
- Modify: `internal/endpoint/detection/engine.go`
- Modify: `internal/endpoint/detection/compiled.go`
- Modify: `internal/endpoint/detection/validation.go`
- Test: `internal/endpoint/detection/engine_test.go`

**Interfaces:**
- Produces: `CorrelateSpec{Within time.Duration, By []string, Facts []FactSpec}`.
- Produces: `FactSpec{ID string, Behaviors []string, Conditions []ConditionSpec, ConditionGroup ConditionNodeSpec}`.
- JSON accepts exactly one of `event` or non-empty `events`; daemon normalizes both into `Behaviors`.
- Runtime indexes each fact by all declared behaviors and stores one bounded state per rule/group.

- [ ] Add Content parser tests for `correlate`, singular `event`, plural `events`, duration, by-fields and fact condition trees.
- [ ] Run focused Content tests; expect RED on absent schema.
- [ ] Implement Content and daemon conversion; invalid duration remains distinguishable and causes Detection rejection rather than silent zeroing.
- [ ] Add validation tests for zero/over-24h window, empty by, unknown by-field, duplicate/empty fact ID, event+events conflict, empty behaviors and fewer than two facts.
- [ ] Run focused validation tests; expect RED.
- [ ] Compile facts into behavior-indexed candidates and implement state with:

```text
rule_id -> group_key -> {started_at, expires_at, matched_fact_ids, refs, captured_values}
```

- [ ] Add neutral runtime tests for all fact permutations, duplicate facts, timeout, by isolation, single emission, group eviction and refs truncation.
- [ ] Run detection/content/daemon focused tests; expect PASS.
- [ ] Commit `feat(detection): add bounded correlate runtime`.

### Task 3: Migrate Reverse Shell Detection to Expr

**Files:**
- Modify: `internal/endpoint/detection/builtin_rules.go`
- Modify: `internal/endpoint/detection/engine.go`
- Test: `internal/endpoint/detection/engine_test.go`
- Modify: `test/data/content/rulepack-cep-endpoint.json`

**Interfaces:**
- Expr: `process.binary_name in ctx:shell-binaries` AND `socket.port in ioc:c2-control-port-feed`.
- Output: `terminal: true`, existing `collect_evidence` response intent.

- [ ] Write failing tests proving dynamic shell/IOC replacement, exactly one Event ref, process/socket entities, terminal output and response intent.
- [ ] Run `GOCACHE=/tmp/sysarmor-go-build go test ./internal/endpoint/detection -run TestReverseShellExpr -count=1`; expect RED because builtin ignores dynamic shell Context and appends download refs.
- [ ] Convert builtin and fixture rule to expr and remove `detectReverseShell` as a Signal path.
- [ ] Run detection/effectiveness tests; update only the intentional evidence-ref expectation.
- [ ] Commit `refactor(detection): express reverse shell as expr rule`.

### Task 4: Migrate Suspicious Exec-Connect to Sequence

**Files:**
- Modify: `internal/endpoint/detection/builtin_rules.go`
- Modify: `internal/endpoint/detection/engine.go`
- Test: `internal/endpoint/detection/engine_test.go`
- Modify: `test/data/content/rulepack-cep-endpoint.json`

**Interfaces:**
- Step `exec`: payload binary prefix OR argv contains a payload prefix.
- Step `connect`: control port AND (`process.stable_id same_as exec.process.stable_id` OR `parent.stable_id same_as exec.process.stable_id`).
- Group by `lineage_id`, bounded window 2 minutes.

- [ ] Write failing tests for direct process association, parent association, unrelated connection rejection, dynamic payload/IOC replacement and complete two Event refs.
- [ ] Run focused tests; expect RED because builtin state owns the behavior.
- [ ] Express the rule as sequence using the generic condition tree; remove `suspicious_exec_connect` Signal generation from `detectPayloadConnect`.
- [ ] Run detection/effectiveness tests; expect PASS without duplicate Signals.
- [ ] Commit `refactor(detection): express exec connect as sequence rule`.

### Task 5: Migrate Payload Lifecycle to Correlate

**Files:**
- Modify: `internal/endpoint/detection/builtin_rules.go`
- Modify: `internal/endpoint/detection/engine.go`
- Test: `internal/endpoint/detection/engine_test.go`
- Test: `internal/endpoint/detection/effectiveness_test.go`
- Modify: `test/data/content/rulepack-cep-endpoint.json`

**Interfaces:**
- Correlate within 2 minutes by `lineage_id`.
- Facts: drop (`file.write|file.chmod` + payload path), exec (payload binary OR argv payload reference), connect (control port).

- [ ] Write failing migration tests for all six fact permutations, dynamic Context/IOC replacement, exactly three necessary Event refs, entities and one emission per completed group.
- [ ] Keep the existing reordered effectiveness scenario as a mandatory RED/GREEN contract.
- [ ] Convert builtin and fixture rule to correlate; remove `detectPayloadLifecycle` Signal generation.
- [ ] Run detection/effectiveness tests; expect ordered and reordered cases to PASS.
- [ ] Commit `refactor(detection): express payload lifecycle as correlate rule`.

### Task 6: Remove Rule-Specific Lineage State

**Files:**
- Modify: `internal/endpoint/detection/engine.go`
- Modify: `internal/endpoint/detection/compiled.go`
- Test: `internal/endpoint/detection/engine_test.go`
- Test: `internal/endpoint/detection/effectiveness_test.go`

**Interfaces:**
- Removes: `observeDownloadEvidence`, `observePayloadEvidence`, `detectPayloadExec`, `detectPayloadConnect` and all dedicated lineage maps/flags if no consumer remains.
- Keeps: only generic CEP/correlate state and generic process fact indexes with a live consumer.

- [ ] Add a migration audit test or source assertion that no rule-specific detector names remain.
- [ ] Run it to RED while temporary observers remain.
- [ ] Remove orphan state and simplify `Engine.Process` to generic runtime dispatch.
- [ ] Run full detection/effectiveness tests and benchmarks; expect PASS.
- [ ] Commit `refactor(detection): remove rule-specific lineage state`.

### Task 7: Expand Real Release Scenarios

**Files:**
- Modify: `test/release/scenarios.sh`
- Modify: `test/release/README.md`
- Modify: `test/release/testdata/fake-docker.sh`
- Modify: `test/release/test-assert.sh`
- Modify: `test/suites/product/endpoint/release-container-e2e-contract.sh`
- Modify: `test/release/fixtures/web-app/server.js` or existing fixture scripts only where a real process/network action is required.
- Create: scenario attack scripts under `test/release/attacks/` following existing naming.

**Interfaces:**
- Adds independent scenarios for `reverse_shell_pattern` and `suspicious_exec_connect`; retains the real `payload_lifecycle` scenario.
- Each scenario maps exact Rule ID, severity, terminal expectation, behaviors and ports.

- [ ] Extend contract tests first and run them to RED on missing scenario metadata/scripts.
- [ ] Implement real shell/control-channel and payload-exec/control-channel attacks without marker-only Signal simulation.
- [ ] Update fake Docker only as a harness contract fixture, not as evidence of real EDR effectiveness.
- [ ] Run Release assert, fixture and container contract tests; expect all `ok`.
- [ ] Commit `test(release): cover migrated multi-event rules`.

### Task 8: Final Regression and Inline Review

**Files:**
- Verify all files above.

**Interfaces:**
- No new feature; verifies final architecture and compatibility.

- [ ] Run gofmt, JSON parsing and `git diff --check`.
- [ ] Run:

```bash
GOCACHE=/tmp/sysarmor-go-build go test \
  ./internal/endpoint/detection \
  ./internal/policy \
  ./internal/agent/content \
  ./internal/agent/daemon \
  ./internal/sensors/linux/tetragon -count=1
```

- [ ] Run:

```bash
bash test/release/test-assert.sh
bash test/release/fixtures/test-fixtures.sh
bash test/suites/product/endpoint/release-container-e2e-contract.sh
```

- [ ] Confirm no remaining concrete builtin rule entrypoints or rule-specific lineage state.
- [ ] Review duplicate Signal risk, dynamic Content override, source-field Coverage, condition-tree bounds, correlate bounds, refs and entity precision.
- [ ] Create a scoped `fix(detection): ...` commit only for issues found by final review.
