# Policy-Driven Single-Event Rules Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 `download_by_lolbin`、`payload_dropped` 和 `credential_file_read` 从专用 Go detector 迁移到动态 `expr` rulepack，并用通用 suppression 原语保持凭据读取去重语义。

**Architecture:** 复用现有按 behavior 索引的 expr runtime、contextset/iocpack 和 Event View。新增的 suppression 是规则无关的有界输出控制：规则匹配后按声明字段组成 key，在有限窗口内抑制重复 Signal；它不理解凭据、下载或 payload 语义。

**Tech Stack:** Go、JSON rulepack/contextset、现有 endpoint detection engine、Agent Content Store、Go testing。

## Global Constraints

- 同一 Rule ID 只能有一个权威执行路径。
- 迁移后 Signal 的严重度、terminal、Event refs、entities、context/IOC refs 与原实现兼容。
- suppression 必须有最大窗口、最大 key 数和淘汰指标，不允许无界状态。
- 动态内容优先于 standalone 内置默认集合。
- 不迁移 `reverse_shell_pattern`、`suspicious_exec_connect` 或 `payload_lifecycle`；它们属于后续多事件阶段。

---

### Task 1: Migrate Download Detection to Expr

**Files:**
- Modify: `internal/endpoint/detection/builtin_content.go`
- Modify: `internal/endpoint/detection/builtin_rules.go`
- Modify: `internal/endpoint/detection/engine.go`
- Test: `internal/endpoint/detection/engine_test.go`
- Modify: `test/data/content/rulepack-cep-endpoint.json`
- Create: `test/data/content/context-download-client-binaries.json`

**Interfaces:**
- Context ref: `ctx:download-client-binaries`
- Expr: `process.binary_name in context` AND `socket.port in ioc:c2-download-port-feed`

- [ ] Write tests asserting curl/wget trigger, benign port and dynamically removed clients do not trigger, and Signal keeps process/socket entities plus IOC ref.
- [ ] Run `GOCACHE=/tmp/sysarmor-go-build go test ./internal/endpoint/detection -run TestDownloadByLOLBin -count=1`; expect RED on dynamic replacement before migration.
- [ ] Define the expr RuleSpec and neutral client context; remove `detectDownloadByLOLBin` and `downloadRefs` only after confirming no remaining consumer needs them. If reverse-shell evidence still consumes `downloadRefs`, retain that generic evidence state until its later migration, but never emit the download Signal through builtin.
- [ ] Update dynamic rulepack fixture and run the full detection package; expect PASS.
- [ ] Commit `refactor(detection): express download detection as expr rule`.

### Task 2: Migrate Payload Drop Detection to Expr

**Files:**
- Modify: `internal/endpoint/detection/builtin_rules.go`
- Modify: `internal/endpoint/detection/engine.go`
- Test: `internal/endpoint/detection/engine_test.go`
- Modify: `test/data/content/rulepack-cep-endpoint.json`

**Interfaces:**
- Expr: behavior is `file.write` or `file.chmod`, and `file.path prefix ctx:payload-path-prefixes`
- Existing payload lifecycle state may observe matching Event facts but must not emit `payload_dropped` itself.

- [ ] Strengthen tests for write/chmod, non-payload paths, dynamic path replacement, Event ref, process/file entities and context ref.
- [ ] Run focused tests; expect RED because the old detector reads resolved engine context rather than the expr rule.
- [ ] Convert the default and fixture rule to expr. Split state observation from Signal emission so later lifecycle builtins can temporarily retain payload facts without owning `payload_dropped`.
- [ ] Run detection/effectiveness tests; expect identical lifecycle behavior and one payload-drop Signal per matching Event.
- [ ] Commit `refactor(detection): express payload drop as expr rule`.

### Task 3: Add Generic Suppression and Migrate Credential Read

**Files:**
- Modify: `internal/agent/content/store.go`
- Test: `internal/agent/content/store_test.go`
- Modify: `internal/agent/daemon/daemon.go`
- Modify: `internal/endpoint/detection/engine.go`
- Modify: `internal/endpoint/detection/compiled.go`
- Modify: `internal/endpoint/detection/validation.go`
- Modify: `internal/endpoint/detection/builtin_rules.go`
- Test: `internal/endpoint/detection/engine_test.go`
- Modify: `test/data/content/rulepack-cep-endpoint.json`

**Interfaces:**
- Add `SuppressionSpec{Within time.Duration, By []string}` to `RuleSpec`.
- JSON output/runtime contract: `suppress: {"within":"5m","by":["process.stable_id","file.path"]}`.
- Default maximum suppression keys remains `8192`; expired entries are reclaimed before capacity eviction.

- [ ] Write failing parser, validation and runtime tests for valid suppression, invalid/zero window, unknown by-field, same key suppression, different key emission and post-window emission.
- [ ] Run focused tests and verify failures are due to missing suppression schema/runtime.
- [ ] Parse and compile suppression into the rule execution plan; after expr conditions match, build a typed group key and call generic suppression before Signal creation.
- [ ] Express credential read as `file.path prefix ctx:credential-path-prefixes` AND `process.binary not_in ctx:trusted-admin-binaries`, with 5-minute suppression by process stable ID and file path.
- [ ] Remove `detectCredentialRead`, `credentialReadKey` and credential-specific suppression constant.
- [ ] Run content, daemon and detection packages; expect PASS.
- [ ] Commit `feat(detection): add generic rule suppression`.

### Task 4: Regression and Migration Audit

**Files:**
- Verify all files above
- Verify: `test/release/test-assert.sh`
- Verify: `test/release/fixtures/test-fixtures.sh`
- Verify: `test/suites/product/endpoint/release-container-e2e-contract.sh`

- [ ] Run gofmt, JSON parsing and `git diff --check`.
- [ ] Run `GOCACHE=/tmp/sysarmor-go-build go test ./internal/endpoint/detection ./internal/policy ./internal/agent/content ./internal/agent/daemon ./internal/sensors/linux/tetragon -count=1`.
- [ ] Run the three Release shell tests and expect each to output `ok`.
- [ ] Confirm `detectDownloadByLOLBin|detectPayloadDrop|detectCredentialRead|credentialReadKey` no longer exist, while temporary generic lifecycle observation is explicitly named and tested.
- [ ] Perform inline review for duplicate Signals, content override semantics, Coverage source fields and bounded state.
- [ ] Create a scoped final fix commit only if regression review finds an issue.
