# Product Monorepo Branch Closure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 `refactor/product-monorepo-layout` 作为可验证、可审查、可合入 `dev` 的独立治理成果收口。

**Architecture:** 本计划不增加产品功能，只处理远端基线、文档漂移、全量回归、审查和分支交付。若两个远端 `dev` 不一致，或最新 `dev` 与当前分支产生语义冲突，则停止自动集成并先报告差异。

**Tech Stack:** Git、Go 1.24、Python unittest、pnpm/Next.js、Shell E2E、GitHub CLI。

## Global Constraints

- 不直接提交或推送 `dev`、`main`，不 force push。
- 不在当前分支增加 P1/P2 产品能力。
- 保持 managed/standalone authority、pending Policy、Manager 授权吊销退管语义不变。
- 只修复由本分支引入的回归或文档漂移。
- 所有生成物在交付前清理，工作树必须干净。
- 每个独立修复形成 Conventional Commit；纯验证不创建空提交。

---

### Task 1: 固化规划与建立交付基线

**Files:**
- Create: `docs/superpowers/specs/2026-08-06-edr-architecture-program-design.md`
- Create: `docs/superpowers/plans/2026-08-06-product-monorepo-branch-closure.md`

**Interfaces:**
- Consumes: `docs/architecture.md`、`docs/design-principles.zh-CN.md`、`docs/roadmap.md` 和已完成治理提交。
- Produces: 后续 P0-P2 的稳定边界、顺序和验收口径。

- [ ] **Step 1: 检查文档无占位内容**

Run: `rg -n 'T[B]D|T[O]DO|implement[ ]later|待[补]充' docs/superpowers/specs/2026-08-06-edr-architecture-program-design.md docs/superpowers/plans/2026-08-06-product-monorepo-branch-closure.md`

Expected: 无输出。

- [ ] **Step 2: 检查规格与当前架构事实一致**

Run: `python3 test/contracts/test_monorepo_layout.py`

Expected: 28 项合同全部 PASS。

- [ ] **Step 3: 提交规划文档**

```bash
git add docs/superpowers/specs/2026-08-06-edr-architecture-program-design.md
git add docs/superpowers/plans/2026-08-06-product-monorepo-branch-closure.md
git commit -m "docs(architecture): plan edr delivery program"
```

### Task 2: 刷新并比较两个 dev 基线

**Files:**
- Modify: none。

**Interfaces:**
- Produces: 已验证的 PR base；不会修改功能分支历史。

- [ ] **Step 1: 获取两个远端 dev**

Run: `git fetch origin dev`

Run: `git fetch github dev`

Expected: 两个命令成功，不修改工作树。

- [ ] **Step 2: 比较远端提交图**

Run: `git rev-parse origin/dev`

Run: `git rev-parse github/dev`

Run: `git log --left-right --cherry-pick --oneline origin/dev...github/dev`

Expected: SHA 相同且差异日志无输出。若不一致，停止后续集成，先确认哪一侧是有效 `dev`。

- [ ] **Step 3: 计算功能分支漂移**

Run: `git log --left-right --cherry-pick --oneline origin/dev...HEAD`

Run: `git diff --stat origin/dev..HEAD`

Expected: 明确列出功能分支提交和文件范围，不出现来源不明的提交。

- [ ] **Step 4: 仅在 dev 前进时集成**

若 `git rev-list --count HEAD..origin/dev` 输出非零，运行：

Run: `git merge --no-ff --no-edit origin/dev`

Expected: 合并成功；发生冲突时不猜测业务语义，停止并逐项报告。首次发布前不使用 rebase 重写 35 个已审查提交。

### Task 3: 审计目录、文档和构建入口漂移

**Files:**
- Modify only: 发现且确认由 monorepo 迁移导致的文档、Makefile、workflow 或部署路径。
- Test: `test/contracts/test_monorepo_layout.py`

**Interfaces:**
- Produces: 所有用户和 CI 入口只引用当前 `apps/`、`packages/`、`deployments/` 布局。

- [ ] **Step 1: 搜索废弃源码入口**

Run: `rg -n '(^|[ /])(cmd|internal/agent|internal/manager|web/manager)/' README.md CATALOG.md Makefile docs deployments test .github --glob '!docs/superpowers/**'`

Expected: 不存在仍作为有效入口的旧路径；历史说明或明确兼容测试可以保留。

- [ ] **Step 2: 验证 Go package 图**

Run: `go list ./...`

Expected: 所有 package 可解析，无旧 import path。

- [ ] **Step 3: 验证结构合同**

Run: `python3 test/contracts/test_monorepo_layout.py`

Expected: PASS。

- [ ] **Step 4: 对实际漂移 fail-closed**

若前三步发现实际漂移，暂停本 Task；先把准确文件、失败断言、修复方式和验证命令补入本计划，再按 TDD 修复并以 `fix(repo): align monorepo integration paths` 提交。没有实际漂移时不创建空提交。

### Task 4: 执行完整回归矩阵

**Files:**
- Modify only: 本分支引入回归的最小修复及对应测试。

**Interfaces:**
- Produces: 可重复的绿色验收记录。

- [ ] **Step 1: 静态质量检查**

Run: `gofmt -l apps packages`

Expected: 无输出。

Run: `git diff --check`

Expected: 无输出。

- [ ] **Step 2: 全仓 Go race 测试**

Run: `GOCACHE=/tmp/sysarmor-core-go-cache go test -race ./... -count=1`

Expected: PASS；Unix socket 测试在允许创建 socket 的执行环境运行。

- [ ] **Step 3: 二进制构建**

Run: `make build-binary`

Run: `make build-agent-tools`

Expected: 所有二进制构建成功。

- [ ] **Step 4: Console 验证**

Run: `pnpm --dir apps/console test`

Run: `pnpm --dir apps/console lint`

Run: `pnpm --dir apps/console build`

Expected: PASS；生产构建使用权限 `0600` 的临时 secret 和 RSA key，不提交 secret。

- [ ] **Step 5: 控制面 E2E**

Run: `bash test/suites/functional/platform/e2e-control-contract.sh`

Run: `bash test/suites/functional/platform/e2e-control-roundtrip-local.sh`

Run: `bash test/suites/functional/platform/e2e-agent-gateway-manager-local.sh`

Run: `bash test/suites/functional/platform/e2e-store-status.sh`

Expected: 全部 PASS。

- [ ] **Step 6: 清理生成物**

Run: `make clean-bin`

Run: `git status --short`

Expected: 除计划内提交外无修改或未跟踪生成物。

### Task 5: 独立审查与分支交付

**Files:**
- Modify only: 审查确认的 Critical 或 Important 修复及对应测试。

**Interfaces:**
- Produces: 两个远端上的同名功能分支和面向 `dev` 的 GitHub PR。

- [ ] **Step 1: 独立代码审查**

Review range: `origin/dev..HEAD`。

重点检查：外部合同、依赖方向、authority、pending 激活、退管吊销、持久化 fail-closed、安装路径、测试有效性。

Expected: 无未修复 Critical 或 Important。

- [ ] **Step 2: 推送功能分支**

Run: `git push -u origin refactor/product-monorepo-layout`

Run: `git push github refactor/product-monorepo-layout`

Expected: 两个远端功能分支指向同一 HEAD；不更新远端 `dev` 或 `main`。

- [ ] **Step 3: 创建面向 dev 的 PR**

创建前运行：

Run: `gh pr list --base dev --head refactor/product-monorepo-layout --state open`

Expected: 没有同 head/base 的开放 PR。随后运行：

```bash
gh pr create --base dev --head refactor/product-monorepo-layout \
  --title "refactor: adopt product monorepo architecture" \
  --body '## 目的/结论

将仓库迁移到 apps/packages/deployments 产品 monorepo 布局，并完成核心大文件与 Agent 控制边界治理。

## 改动

- 按 Agent、Manager、Console 和 CLI 建立产品边界
- 建立 packages 准入与自动依赖合同
- 按领域拆分 Manager Store、Tetragon backend 和 CLI
- 建立 control、localapi、remoteapi 平级边界并收敛 daemon

## 影响与风险

外部 protobuf、持久化 schema、CLI 行为和部署拓扑保持不变。主要风险是目录迁移造成构建路径漂移，以及控制 DTO 转换丢失字段；均由结构合同、race 测试和真实控制链路 E2E 覆盖。

## 验收

- 全仓 Go race 测试通过
- 二进制与 Console 构建通过
- 28 项架构合同通过
- 控制面关键 E2E 通过
- 独立审查无未修复 Critical 或 Important'
```

- [ ] **Step 4: 最终状态检查**

Run: `git status --short --branch`

Run: `git rev-parse origin/refactor/product-monorepo-layout`

Run: `git rev-parse github/refactor/product-monorepo-layout`

Expected: 工作树干净，两个远端功能分支 SHA 与本地 HEAD 一致。
