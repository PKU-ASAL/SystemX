# Tetragon Container ID Matching Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 Tetragon backend 安全匹配 12、32、64 位 Container ID，而调用方始终可以传完整 ID。

**Architecture:** 保留 `scope_identity.go` 中唯一 matcher，正向前缀保持兼容，反向前缀增加十六进制与最短 12 位约束。container、legacy container prefix、namespace/self 三个过滤入口统一调用 matcher。

**Tech Stack:** Go、Tetragon backend 单元测试、Go race detector。

## Global Constraints

- 不修改 CVELab。
- 不改变 cgroup、pod 和普通 namespace scope。
- 不改变 namespace/self 身份解析与 `--cgroupns=host` 要求。
- 空 selector 不得匹配全部事件。

---

### Task 1: Container ID Matcher 与 Backend 接线

**Files:**
- Modify: `internal/sensors/linux/tetragon/scope_identity.go`
- Modify: `internal/sensors/linux/tetragon/backend.go`
- Modify: `internal/sensors/linux/tetragon/backend_test.go`

**Interfaces:**
- Produces: `containerIDsMatch(eventID, selector string) bool`
- Consumes: Tetragon event Container ID 与标准化 scope selector

- [ ] **Step 1: 写失败测试**

增加表驱动 matcher 测试，覆盖 64→32、12→32、大小写、空值、不同 ID、非法或不足 12 位反向匹配；增加 container、legacy、namespace/self 使用完整 selector 匹配截断事件 ID 的 Backend 测试。

- [ ] **Step 2: 验证 RED**

Run: `go test ./internal/sensors/linux/tetragon -run 'TestContainerIDsMatch|TestBackendContainerIDLengthCompatibility' -count=1`
Expected: 64→32 的 container/legacy 测试失败，非法反向匹配测试暴露现有 matcher 过宽。

- [ ] **Step 3: 最小实现**

matcher 先 trim/lower，拒绝空值；正向 `HasPrefix(eventID, selector)` 直接保持兼容；反向仅在双方匹配 `^[0-9a-f]+$` 且 event ID 至少 12 位时允许。Backend 的 container 和 legacy 分支改用 matcher。

- [ ] **Step 4: 验证 GREEN**

Run: `go test -race ./internal/sensors/linux/tetragon`
Expected: PASS。

- [ ] **Step 5: 全仓回归与提交**

Run: `go test ./...`
Run: `git diff --check`
Expected: 全部 PASS，无格式错误。

Commit: `fix(sensor): normalize tetragon container id matching`
