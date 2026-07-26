# Standalone Signed Rule Pack Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 standalone 从发行版只读目录和用户持久化目录严格加载签名内容，并将所有具体 endpoint 规则从 Agent 二进制迁入发行规则包。

**Architecture:** 内容层新增 manifest 驱动的发行目录加载器，逐文件校验元数据、摘要和 Ed25519 签名，再与用户内容无冲突合并成单一 snapshot。规则引擎只消费显式 ruleset；发行脚本负责签名内容、生成 manifest，安装器负责原子替换默认内容目录。

**Tech Stack:** Go 1.x、Ed25519、JSON、Bash、OpenSSL、Docker/Tetragon Release 验收。

## Global Constraints

- standalone 缺少必需内容、验签失败、引用冲突或编译失败时必须非零退出，不允许 fallback。
- `/opt/sysarmor/agent/content/default/` 由发行版托管；`/var/lib/sysarmor/agent/content/` 保留用户内容。
- 发行版 ref 只读；两层 ref 冲突直接失败。
- 私钥仅通过 `SYSARMOR_CONTENT_SIGNING_KEY` 或 `--content-signing-key` 提供，禁止写入仓库和发行包。
- `process.pid`、`sequence`、`correlate`、`same_as` 保留在二进制；所有具体规则和内置值迁出。
- 现有工作区 PID 改动属于本功能，实施时保留并纳入对应原子提交。

---

### Task 1: 严格的内容解析与重复检测

**Files:**
- Create: `internal/agent/content/parse.go`
- Modify: `internal/agent/content/store.go`
- Test: `internal/agent/content/store_test.go`

**Interfaces:**
- Produces: `parseRecord(raw string, trustedKeys map[string]ed25519.PublicKey, requireSigned bool) (Record, error)`；`snapshotFromRecords(records map[string]Record) (Snapshot, error)`。

- [ ] **Step 1: 写失败测试**：覆盖 Load 对无签名、错签、重复 metadata.id、重复 rule ID、畸形 rulepack 的显式失败；断言错误包含 ref 和原因。
- [ ] **Step 2: 验证红灯**：运行 `go test ./internal/agent/content -run 'TestStoreLoad|TestSnapshotRejects' -count=1`，预期当前 Load 接受无签名或静默忽略解析错误。
- [ ] **Step 3: 最小实现**：把 envelope/rulepack/value-set 解析移入 `parse.go`；`Load` 对每个文件调用 `Validate(env, false)`，检测重复 ref；`snapshotFromRecords` 返回错误且不再使用 `if err == nil` 丢弃错误。
- [ ] **Step 4: 验证绿灯**：运行 `go test ./internal/agent/content -count=1`，预期 PASS。
- [ ] **Step 5: 提交**：`git commit -m "fix(content): reject invalid persisted content"`。

### Task 2: Manifest 驱动的双层内容加载

**Files:**
- Create: `internal/agent/content/manifest.go`
- Create: `internal/agent/content/layers.go`
- Test: `internal/agent/content/layers_test.go`
- Modify: `internal/agent/content/store.go`

**Interfaces:**
- Consumes: Task 1 的 `parseRecord` 和严格 snapshot 构建。
- Produces: `Manifest{Version string, Entries []ManifestEntry}`；`OpenLayered(Options{DefaultDir, Dir, TrustedKeys}) (*Store, error)`；`Store.IsDefaultRef(ref string) bool`。

- [ ] **Step 1: 写失败测试**：创建临时 default/user 目录，覆盖成功合并，以及 manifest 缺文件、额外 JSON、kind/version/digest 不符、层间重复 ref、用户内容错签分别失败。
- [ ] **Step 2: 验证红灯**：运行 `go test ./internal/agent/content -run 'TestOpenLayered' -count=1`，预期缺少 `OpenLayered`。
- [ ] **Step 3: 最小实现**：解析 `content-manifest.json`；按清单逐个加载并对照 `ref/kind/version/digest/file`；加载用户层；拒绝 ref 冲突；记录 default refs。manifest 本身只承担完整性清单，内容真实性由每个 envelope 的 Ed25519 签名保证。
- [ ] **Step 4: 阻止默认 ref 运行时修改**：在 `Prepare` 和删除入口前检查 `IsDefaultRef`，返回 `default content ref <ref> is read-only`。
- [ ] **Step 5: 验证绿灯**：运行 `go test ./internal/agent/content -count=1`，预期 PASS。
- [ ] **Step 6: 提交**：`git commit -m "feat(content): load signed layered rule packs"`。

### Task 3: Agent 启动时严格装配 detection

**Files:**
- Modify: `internal/agent/config/config.go`
- Modify: `configs/agent.example.yaml`
- Modify: `internal/agent/daemon/daemon.go`
- Create: `internal/agent/daemon/startup_content.go`
- Modify: `internal/agent/daemon/startup_policy.go`
- Test: `internal/agent/daemon/daemon_test.go`

**Interfaces:**
- Produces: 配置项 `content.default_path`；`loadStartupContent(cfg) (*agentcontent.Store, error)`；`validateStartupDetection() error`。

- [ ] **Step 1: 写失败测试**：standalone 配置缺 default manifest、签名失败、policy ruleset 未解析、内容编译失败时 `New` 或启动阶段返回带阶段/ref 的错误；合法双层内容可启动。
- [ ] **Step 2: 验证红灯**：运行 `go test ./internal/agent/daemon ./internal/agent/config -run 'Test.*StartupContent|Test.*DefaultPath' -count=1`，预期 FAIL。
- [ ] **Step 3: 最小实现**：解析 `content.default_path`；仅当该值非空时调用 `OpenLayered`，测试/managed 配置仍可通过显式注入内容；加载 policy 后、启动 sensor/control/health 前构建 detection，`report.Status == rejected` 直接返回错误。
- [ ] **Step 4: 健康信息**：沿用 `DetectionHealth.ContentRefs` 输出 ref/version/digest，并加入 manifest version 字段及映射；ready 只在启动内容编译成功后出现。
- [ ] **Step 5: 验证绿灯**：运行 `go test ./internal/agent/config ./internal/agent/daemon -count=1`，预期 PASS。
- [ ] **Step 6: 提交**：`git commit -m "feat(agent): fail startup on invalid default content"`。

### Task 4: 删除具体 builtin 并要求显式 ruleset

**Files:**
- Delete: `internal/endpoint/detection/builtin_rules.go`
- Delete: `internal/endpoint/detection/builtin_content.go`
- Modify: `internal/endpoint/detection/engine.go`
- Modify: `internal/endpoint/detection/engine_test.go`
- Modify: `internal/endpoint/detection/effectiveness_test.go`
- Modify: `internal/endpoint/detection/compiled.go`
- Modify: `internal/sensors/linux/tetragon/backend.go`

**Interfaces:**
- Consumes: `ContentSnapshot.Rules` 和 policy 显式 ruleset ref。
- Produces: 通用字段 `process.pid`；`resolveRules` 仅解析内容规则并对重复 rule ID、缺失/禁用 ruleset 返回拒绝详情。

- [ ] **Step 1: 写失败测试**：空 ruleset、未知 ruleset、重复 rule ID 均 rejected；显式测试 rulepack 可产生原有 signals；PID 为 0 不满足 `same_as`，同 PID re-exec 满足。
- [ ] **Step 2: 验证红灯**：运行 `go test ./internal/endpoint/detection -run 'Test.*ExplicitRuleSet|Test.*DuplicateRule|TestSuspiciousExecConnect' -count=1`，预期 builtin fallback 相关用例失败。
- [ ] **Step 3: 最小实现**：删除 `append(builtinRules(), ...)`、`builtinRuleSetRef` 和默认注入；将 `resolveRules` 改为返回 `([]effectiveRule, []string)` 并把结构错误合入 ApplyReport；保留当前工作区的 PID 字段和 Tetragon capability 改动。
- [ ] **Step 4: 迁移测试夹具**：从 `test/data/content/rulepack-cep-endpoint.json` 加载显式 RuleSpec snapshot，所有原 builtin 测试使用该 fixture，不在 `_test.go` 复制生产规则。
- [ ] **Step 5: 验证绿灯**：运行 `go test ./internal/endpoint/detection ./internal/sensors/linux/tetragon -count=1`，预期 PASS。
- [ ] **Step 6: 提交**：`git commit -m "refactor(detection): remove builtin endpoint rules"`。

### Task 5: 建立可签名的发行内容资产

**Files:**
- Create: `deployments/agent/content/rulepack-endpoint-linux.json`
- Create: `deployments/agent/content/context-*.json`（仅规则实际引用的 context）
- Create: `deployments/agent/content/ioc-*.json`（仅规则实际引用的 IOC）
- Create: `cmd/sysarmor-content-sign/main.go`
- Create: `cmd/sysarmor-content-sign/main_test.go`
- Modify: `deployments/agent/policy.json`

**Interfaces:**
- Produces: `sysarmor-content-sign --key FILE --key-id ID --input FILE --output FILE`；默认 ruleset ref `ruleset:endpoint-linux-default`。

- [ ] **Step 1: 写失败测试**：签名命令输出可被 `content.Store.Validate(..., false)` 验证，错误私钥格式和缺参数非零退出。
- [ ] **Step 2: 验证红灯**：运行 `go test ./cmd/sysarmor-content-sign -count=1`，预期包不存在。
- [ ] **Step 3: 最小实现**：读取 PKCS#8 Ed25519 私钥，按 content 包现有 canonical bytes 规则写入 digest/key_id/signature；将 canonical 签名函数暴露为 content 包公共函数，避免两套算法。
- [ ] **Step 4: 迁移规则**：把 7 条具体规则和引用值迁入无签名源资产；`suspicious_exec_connect` 包含 stable ID、父 stable ID、相同非零 PID 三个关联分支；policy 显式启用 `ruleset:endpoint-linux-default`。
- [ ] **Step 5: 资产契约验证**：运行 `go test ./cmd/sysarmor-content-sign ./internal/agent/content ./internal/endpoint/detection -count=1`，预期 PASS。
- [ ] **Step 6: 提交**：`git commit -m "feat(content): add standalone endpoint rule pack"`。

### Task 6: Release 打包与原子安装默认内容

**Files:**
- Modify: `deployments/packages/build-release.sh`
- Modify: `deployments/agent/package-agent.sh`
- Modify: `deployments/agent/install-release.sh`
- Modify: `deployments/agent/standalone.yaml`
- Modify: `deployments/agent/standalone-container.yaml`
- Modify: `test/suites/product/endpoint/standalone-release-package.sh`

**Interfaces:**
- Consumes: `--content-signing-key FILE`、`--content-key-id ID`。
- Produces: 包内 `content/default/content-manifest.json`、签名 JSON 和公钥配置；安装目标 `$AGENT_HOME/content/default`。

- [ ] **Step 1: 扩展失败的包测试**：断言缺内容私钥打包失败；包内每个内容 envelope 已签名；首次安装成功；升级整体替换 default；模拟 staging 校验失败时旧目录保留；用户目录与已有 policy 保留。
- [ ] **Step 2: 验证红灯**：运行 `bash test/suites/product/endpoint/standalone-release-package.sh`，预期缺少内容资产或参数。
- [ ] **Step 3: 最小打包实现**：构建签名工具；逐个签名源资产；生成含 `ref/kind/version/digest/file` 的 manifest；将公钥转换为 `key_id=base64_raw_ed25519_public_key` 写入 standalone 配置；Release 缺私钥立即失败。
- [ ] **Step 4: 最小安装实现**：增加 `SYSARMOR_DEFAULT_CONTENT_DIR`；校验包内 manifest 文件集和 SHA256；在同父目录 staging 后 rename，失败恢复旧目录；不修改 `$STATE_DIR/content`。
- [ ] **Step 5: 验证绿灯**：运行包测试和 `bash test/suites/product/endpoint/release-container-e2e-contract.sh`，预期 PASS。
- [ ] **Step 6: 提交**：`git commit -m "feat(release): ship signed default detection content"`。

### Task 7: 全仓迁移和负向启动验收

**Files:**
- Modify: `internal/agent/daemon/daemon_test.go`
- Modify: `internal/agent/daemon/local_control_test.go`
- Modify: `internal/endpoint/detection/engine_bench_test.go`
- Modify: `internal/endpoint/detection/effectiveness_test.go`
- Modify: `internal/endpoint/detection/engine_test.go`
- Modify: `test/fixtures/agent/policies/default.json`
- Modify: `test/release/assert.sh`
- Modify: `test/release/run.sh`
- Modify: `test/suites/product/endpoint/release-container-e2e-contract.sh`

**Interfaces:**
- Consumes: 签名默认内容发行包和严格启动语义。
- Produces: Release 结果包含 Agent/Policy/manifest/content ref-version-digest 及 Signal 统计。

- [ ] **Step 1: 迁移失败测试**：所有需要检测的测试显式加载测试 rulepack；不需要检测的测试显式使用关闭 detection 的 policy，禁止依赖 fallback。
- [ ] **Step 2: 增加篡改场景**：复制发行包后修改一个默认内容文件，启动容器并断言进程非零、日志含 `default content`、ref 和 `digest mismatch`/`signature verification failed`。
- [ ] **Step 3: 增加统计输出**：从 Signal JSONL 统计总数、按 rule 分布，并基于 scenario truth 输出 TP/FP/FN、precision、recall；保持原逐场景证据断言。
- [ ] **Step 4: 全仓验证**：运行 `go test ./...`、standalone package test、Release contract test，预期全部 PASS。
- [ ] **Step 5: 提交**：`git commit -m "test(release): verify signed rule pack startup"`。

### Task 8: 三镜像 privileged Release 验收

**Files:**
- Generated: `test/.results/release/<run-id>/`

**Interfaces:**
- Produces: Ubuntu 22.04、Ubuntu 24.04、Debian 12 的完整 Event/Signal、health、内容版本和 effectiveness 结果。

- [ ] **Step 1: 构建本地 Release**：使用临时 Ed25519 内容私钥和现有 artifact key，运行 `deployments/packages/build-release.sh`，预期产物包含签名 default 内容。
- [ ] **Step 2: 运行三镜像**：运行 `make -C test/release run`，允许 `--privileged --cgroupns=host` 并挂载 `/sys/fs/bpf`、`/sys/kernel/btf/vmlinux:ro`。
- [ ] **Step 3: 核验结果**：三镜像五个场景、sibling/host 隔离、health 内容 refs、Signal 总数/分布、precision/recall 全部满足断言。
- [ ] **Step 4: 清理**：停止验收容器和临时服务，删除临时私钥；保留 `test/.results` 证据。
- [ ] **Step 5: 最终回归**：运行 `go test ./...` 和 `git diff --check`，预期 PASS 且无格式错误。
- [ ] **Step 6: 提交验收契约的必要修正**：如有代码修正，使用 `fix(release): ...` 原子提交；纯生成结果不提交。
