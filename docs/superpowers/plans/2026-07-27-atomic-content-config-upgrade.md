# Atomic Content And Config Upgrade Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 保证运行时内容更新的一致性，并在 Release 升级时只更新发行方管理的内容配置字段。

**Architecture:** Agent 使用专用互斥锁串行提交 Store、detection engine 和 health；Store 持久化失败时恢复旧记录。安装器通过 Agent 内置的结构化配置合并命令生成 staging 配置，再与默认内容一起提交或回滚。

**Tech Stack:** Go、现有 Agent 配置解析器、Bash Release 合同测试。

## Global Constraints

- 不增加外部 YAML 工具依赖。
- 只覆盖 `content.default_path` 和 `content.trust_keys`，保留其他现有配置。
- 首次安装继续采用发行包完整配置。
- 实现前先得到失败测试，提交保持原子。

---

### Task 1: 运行时内容更新事务

**Files:**
- Modify: `internal/agent/daemon/daemon.go`
- Modify: `internal/agent/daemon/local_control.go`
- Modify: `internal/agent/daemon/local_control_test.go`
- Modify: `internal/agent/content/store.go`
- Modify: `internal/agent/content/store_test.go`

**Interfaces:**
- Produces: `AgentRuntime.detectionUpdateMu sync.Mutex`
- Produces: `Store.Commit(record Record) error` 的失败恢复保证

- [ ] **Step 1: 写失败测试**

增加并发 ApplyContent 测试，阻塞第一次提交并交错第二次请求，断言最终 Store、engine 行为和 health refs 版本一致；增加覆盖已有 ref 时持久化失败仍返回旧 record 的 Store 测试。

- [ ] **Step 2: 验证测试失败**

Run: `go test ./internal/agent/content ./internal/agent/daemon`
Expected: 并发一致性或旧 record 保留断言失败。

- [ ] **Step 3: 最小实现**

内容与 detection、collection、endpoint policy 更新进入同一 `detectionUpdateMu` 临界区；`Store.Commit` 在写盘失败时恢复提交前的 record，而不是直接删除 ref。

- [ ] **Step 4: 验证通过**

Run: `go test -race ./internal/agent/content ./internal/agent/daemon`
Expected: PASS。

- [ ] **Step 5: 提交**

Run: `git commit -m "fix(content): serialize runtime content transactions"`

### Task 2: Release 配置保留升级

**Files:**
- Modify: `cmd/sysarmor-agent/main.go`
- Modify: `cmd/sysarmor-agent/main_test.go`
- Modify: `internal/agent/config/config.go`
- Modify: `internal/agent/config/config_test.go`
- Modify: `deployments/agent/install-release.sh`
- Modify: `test/suites/product/endpoint/standalone-release-package.sh`

**Interfaces:**
- Produces: `sysarmor-agent merge-release-config --existing PATH --release PATH --output PATH`
- Consumes: 现有配置和包内配置；输出保留旧配置、替换发行管理 content 字段的 YAML。

- [ ] **Step 1: 写失败测试**

增加配置合并单测和 Release 合同：旧配置包含自定义 Agent labels、local、scope、telemetry、policy 及旧 content，升级后只替换 `content.default_path`、`content.trust_keys`；模拟提交失败时旧配置和旧内容均恢复。

- [ ] **Step 2: 验证测试失败**

Run: `go test ./internal/agent/config ./cmd/sysarmor-agent && bash test/suites/product/endpoint/standalone-release-package.sh`
Expected: 合并命令不存在或升级覆盖自定义字段。

- [ ] **Step 3: 最小实现**

复用现有严格配置解析器校验输入；保留现有 YAML 文本的非 `content` 顶层块，并用发行配置中经过解析的 content 值生成唯一 content 块。安装器仅在已有配置时调用该命令生成 staging 文件。

- [ ] **Step 4: 验证通过**

Run: `go test ./internal/agent/config ./cmd/sysarmor-agent`
Run: `bash test/suites/product/endpoint/standalone-release-package.sh`
Expected: PASS。

- [ ] **Step 5: 提交**

Run: `git commit -m "fix(release): preserve user config during content upgrade"`

### Task 3: 集成验证

**Files:**
- Modify: `docs/superpowers/plans/2026-07-27-atomic-content-config-upgrade.md`

- [ ] **Step 1: 运行聚焦回归**

Run: `go test -race ./internal/agent/content ./internal/agent/daemon`
Run: `go test ./internal/agent/config ./cmd/sysarmor-agent`
Run: `bash test/suites/product/endpoint/standalone-release-package.sh`
Expected: 全部 PASS。

- [ ] **Step 2: 检查差异和工作区**

Run: `git diff --check && git status --short --branch`
Expected: 无格式错误；原实验报告仍独立暂存。
