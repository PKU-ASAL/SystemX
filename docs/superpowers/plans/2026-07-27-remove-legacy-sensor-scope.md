# Remove Legacy Sensor Scope Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 删除 Agent Sensor 配置和 Tetragon Backend 中已经不可用的旧 scope 入口，保留单一规范化 scope 路径。

**Architecture:** Agent 只从 `SensorConfig.Scope` 生成 `RuntimeScope`，daemon 只向 Backend 写入 `ScopeType/ScopeSelector`。Backend 不再支持独立 `ContainerIDPrefix` 旁路，所有容器过滤均由 container scope 调用统一 matcher。

**Tech Stack:** Go、反射结构合同、严格配置解析测试、全仓搜索。

## Global Constraints

- 保留 `CollectionIntent.ScopeType/ScopeSelector`。
- 保留 Policy、API、数据库、Event/Signal 的 scope 字段。
- 旧 YAML 必须显式返回 unknown key。
- 不提供迁移、告警兼容或静默忽略。

---

### Task 1: 删除旧 Sensor 与 Backend 入口

**Files:**
- Modify: `internal/agent/config/config.go`
- Modify: `internal/agent/config/config_test.go`
- Modify: `internal/agent/daemon/daemon.go`
- Modify: `internal/sensors/linux/tetragon/backend.go`
- Modify: `internal/sensors/linux/tetragon/backend_test.go`
- Modify: `internal/sensors/linux/tetragon/scope_identity_test.go`

**Interfaces:**
- Preserves: `SensorConfig.Scope RuntimeScope`
- Preserves: `Backend.ScopeType`、`Backend.ScopeSelector`
- Removes: `SensorConfig.ScopeType`、`ScopeSelector`、`ContainerIDPrefix` 和 `Backend.ContainerIDPrefix`

- [ ] **Step 1: 写失败合同**

增加反射测试，断言 SensorConfig 和 Backend 不存在旧字段；增加表驱动配置测试，断言三个旧 YAML key 分别返回 unknown key。

- [ ] **Step 2: 验证 RED**

Run: `go test ./internal/agent/config ./internal/sensors/linux/tetragon -run 'Test.*NoLegacyScope|TestLoadFileRejectsLegacyScopeKeys' -count=1`
Expected: 反射测试发现旧字段仍存在。

- [ ] **Step 3: 最小删除**

删除旧字段、EffectiveScope 合并逻辑、daemon 接线、Backend 空 scope 旁路和仅覆盖旧旁路的测试。`EffectiveScope` 直接规范化 `Scope.Type/Selector`。

- [ ] **Step 4: 验证 GREEN**

Run: `go test ./internal/agent/config ./internal/agent/daemon ./internal/sensors/linux/tetragon`
Expected: PASS。

- [ ] **Step 5: 搜索与全仓回归**

Run: `rg 'ContainerIDPrefix|container_id_prefix|SensorConfig.*ScopeType|SensorConfig.*ScopeSelector' internal configs deployments`
Expected: 无旧入口结果。

Run: `go test ./...`
Expected: PASS。

Commit: `refactor(sensor): remove legacy scope compatibility`
