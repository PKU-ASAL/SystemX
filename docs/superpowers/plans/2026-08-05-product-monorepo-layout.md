# Product Monorepo Layout Implementation Plan

> **For agentic workers:** Execute this plan inline, task by task. Each task follows test-first or contract-first verification and ends in an atomic commit.

**Goal:** 将 SysArmor 重组为 `apps/{agent,manager,cli,console}` 与稳定 `packages/*`，保持单一 Go Module 和现有运行行为。

**Architecture:** 应用只能依赖共享包，不能跨应用导入内部实现；共享包不能反向依赖应用。Go `internal` 目录用于强制 Agent 与 Manager 的产品边界。

**Tech Stack:** Go 1.26、Protocol Buffers、Next.js/pnpm、Make、Docker、Python unittest、Shell contract tests。

## Global Constraints

- 保持单一 `go.mod`，不引入 `go.work` 或子 Module。
- 所有移动使用 `git mv`。
- 不改变二进制名称、CLI 行为、协议字段或部署拓扑。
- 不在目录迁移中拆分既有超大逻辑文件；迁移完成后单独重构。
- 每个提交只迁移一个可独立验证的边界。

---

### Task 1: 建立入口层和架构合同

**Files:**
- Create: `test/contracts/test_monorepo_layout.py`
- Move: `cmd/sysarmor-agent` -> `apps/agent/cmd/sysarmor-agent`
- Move: `cmd/sysarmor-content-sign` -> `apps/agent/cmd/sysarmor-content-sign`
- Move: `cmd/sysarmor-manager` -> `apps/manager/cmd/sysarmor-manager`
- Move: `cmd/sysarmor-gateway` -> `apps/manager/cmd/sysarmor-gateway`
- Move: `cmd/sysarmor-worker` -> `apps/manager/cmd/sysarmor-worker`
- Move: `cmd/sysarmorctl` -> `apps/cli/cmd/sysarmorctl`
- Modify: `Makefile`
- Modify: `.github/workflows/release-build.yml`
- Modify: `deployments/agent/package-agent.sh`
- Modify: distribution contract scripts referencing `cmd/*`

**Interfaces:**
- Produces unchanged binaries: `sysarmor-agent`, `sysarmor-content-sign`, `sysarmor-manager`, `sysarmor-gateway`, `sysarmor-worker`, `sysarmorctl`.

- [ ] Add a failing layout contract asserting all six command directories exist under their owning app and old `cmd/sysarmor-*` directories do not exist.
- [ ] Run `python3 test/contracts/test_monorepo_layout.py`; expect failure before moves.
- [ ] Move command directories with `git mv` and update every build/release path.
- [ ] Run the layout contract, command package tests, release workflow contract, and `make build-binary`.
- [ ] Commit as `refactor(repo): group executable entrypoints by product`.

### Task 2: 迁移共享 contracts 与领域包

**Files:**
- Move: `api/proto` -> `packages/contracts/proto`
- Move: `internal/contracts/schema` -> `packages/contracts/schema`
- Move: `internal/controlmodel` -> `packages/contracts/controlmodel`
- Move: `internal/agent/health` -> `packages/contracts/health`
- Move: `internal/eventmodel` -> `packages/eventmodel`
- Move: `internal/policy` -> `packages/policy`
- Move: `internal/response` -> `packages/response`
- Move: `internal/sensors/contract` -> `packages/sensor-sdk/contract`
- Move: `internal/tlsconfig` -> `packages/tlsconfig`
- Modify: all Go and Proto imports, `Makefile`, generation and contract paths.

**Interfaces:**
- Produces stable import roots under `github.com/sysarmor/sysarmor-next-project/packages/...`.
- `packages/sensor-sdk/contract` preserves Go package name `contract`.

- [ ] Extend the layout contract with required shared directories, forbidden old directories, and a scan rejecting imports from `packages/*` to `apps/*`.
- [ ] Run the contract; expect failure before moves.
- [ ] Move the packages, update `.proto` imports and `go_package` options, then regenerate with `make api`.
- [ ] Mechanically update Go imports and documentation paths.
- [ ] Run `go test ./packages/...`, `make api`, and `git diff --check`.
- [ ] Commit as `refactor(repo): establish shared product packages`.

### Task 3: 迁移 Agent 产品实现

**Files:**
- Move: `internal/agent/*` -> `apps/agent/internal/*`
- Move: `internal/endpoint` -> `apps/agent/internal/endpoint`
- Move: `internal/sensors/{fake,runtime,linux}` -> `apps/agent/internal/sensors/{fake,runtime,linux}`
- Modify: Agent Go imports, tests, Make/test targets, docs and deployment references.

**Interfaces:**
- Agent main imports only `apps/agent/internal/*` and `packages/*`.
- CLI communicates with Agent through `packages/contracts/proto`, not Agent internal packages.

- [ ] Extend the layout contract to require Agent-owned directories and reject `apps/agent` imports of `apps/manager`.
- [ ] Run the contract; expect failure before moves.
- [ ] Move Agent packages and mechanically update imports and path-based tests.
- [ ] Run `go test -race ./apps/agent/... ./packages/...` and Agent functional contracts.
- [ ] Commit as `refactor(agent): move endpoint runtime under product boundary`.

### Task 4: 迁移 Manager 产品实现

**Files:**
- Move: `internal/manager/{api,auth}` -> `apps/manager/internal/{api,auth}`
- Move: `internal/gateway` -> `apps/manager/internal/gateway`
- Move: `internal/store` -> `apps/manager/internal/store`
- Move: `internal/analytics` -> `apps/manager/internal/analytics`
- Move: `internal/workers/ingest` -> `apps/manager/internal/ingest`
- Move: `internal/platform` -> `apps/manager/internal/platform`
- Move: `internal/distribution` -> `apps/manager/internal/distribution`
- Move: `test/suites/functional/platform/agent_gateway_manager_test.go` -> `apps/manager/integration/agent_gateway_manager_test.go`
- Modify: Manager Go imports, tests, Docker/build paths, docs and shell test targets.

**Interfaces:**
- Manager commands import only `apps/manager/internal/*` and `packages/*`.
- White-box Manager integration tests remain below `apps/manager` to satisfy Go internal visibility.

- [ ] Extend the layout contract to require Manager-owned directories and reject `apps/manager` imports of `apps/agent`.
- [ ] Run the contract; expect failure before moves.
- [ ] Move Manager packages and integration test, then update imports and path-based tests.
- [ ] Run `go test -race ./apps/manager/... ./packages/...` and platform functional contracts.
- [ ] Commit as `refactor(manager): move control plane under product boundary`.

### Task 5: 迁移 Console 并完成仓库收口

**Files:**
- Move: `web/manager` -> `apps/console`
- Modify: `Makefile`
- Modify: `tools/web-console.sh`
- Modify: `deployments/manager-ui/Dockerfile`
- Modify: docs, test contracts and all remaining old paths.

**Interfaces:**
- Console continues producing the same Next.js artifact and Manager UI image.

- [ ] Extend the layout contract to require `apps/console` and reject old top-level `cmd`, `api`, `internal`, and `web` product directories.
- [ ] Run the contract; expect failure before the move.
- [ ] Move Console and update pnpm, Docker, Make and documentation paths.
- [ ] Run `pnpm --dir apps/console build`, `go test ./...`, distribution contracts, topology contracts and shell syntax checks.
- [ ] Verify `rg 'cmd/sysarmor-|api/proto|internal/(agent|manager|gateway|store|endpoint|sensors)|web/manager'` only finds historical documents explicitly exempted by policy.
- [ ] Commit as `refactor(console): complete product monorepo layout`.

### Task 6: 最终架构与回归审查

**Files:**
- Modify only defects found by review.

- [ ] Review dependency direction, package ownership, generated code and deployment inputs.
- [ ] Run `go test -race ./...` where environment permits local sockets.
- [ ] Run `make test-distribution SOURCE=local` and `make test-functional DOMAIN=topology` when the VM environment is available.
- [ ] Run Console lint/build and all Python/Shell contracts.
- [ ] Confirm the worktree has no generated artifacts staged.
- [ ] Record remaining oversized-file decomposition as separate follow-up work, not mixed into layout commits.
