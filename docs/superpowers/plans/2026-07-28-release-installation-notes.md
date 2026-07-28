# Release Installation Notes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 RC 与 GA 自动生成包含安装、容器和 provenance 说明的 GitHub Release notes。

**Architecture:** 一个纯 Bash 渲染器负责内容与输入校验，两个发布入口只负责调用并传给 `gh release create --notes-file`。Shell 契约直接验证渲染输出，workflow 契约验证接线。

**Tech Stack:** Bash、GitHub Actions、GitHub CLI。

## Global Constraints

- 不改变 release 制品、签名、安装或 provenance 构建逻辑。
- RC 与 GA 使用同一个 notes 渲染器。
- 不再使用 `--generate-notes`。
- 版本、仓库和发布类型必须严格校验。

---

### Task 1: Release Notes Renderer

**Files:**
- Create: `deployments/packages/render-github-release-notes.sh`
- Create: `test/suites/product/endpoint/release-notes.sh`
- Modify: `test/Makefile:131-140`

**Interfaces:**
- Consumes: `render-github-release-notes.sh VERSION REPOSITORY rc|ga`。
- Produces: 标准输出 Markdown；非法输入退出 2。

- [ ] **Step 1: 写失败测试**

  测试 `v0.1.0-rc.2 PKU-ASAL/sysarmor rc` 和 `v0.1.0 PKU-ASAL/sysarmor ga`，要求输出包含
  `install.sh`、`--profile linux-container`、`gh attestation verify` 和 changelog URL；错误版本、
  仓库或类型必须失败。

- [ ] **Step 2: 确认 RED**

  Run: `bash test/suites/product/endpoint/release-notes.sh`

  Expected: FAIL，缺少 `deployments/packages/render-github-release-notes.sh`。

- [ ] **Step 3: 实现最小渲染器**

  使用 Bash 正则分别校验 RC/GA 版本，仓库匹配 `owner/name` 安全字符；通过单个 heredoc 输出
  完整 Markdown，不执行用户输入。

- [ ] **Step 4: 确认 GREEN**

  Run: `bash test/suites/product/endpoint/release-notes.sh`

  Expected: `[release-notes] ok`。

### Task 2: Workflow Integration

**Files:**
- Modify: `.github/workflows/release-candidate.yml:81-94`
- Modify: `.github/workflows/release-stable.yml:100-112`
- Modify: `test/suites/product/endpoint/release-workflow-contract.sh`

**Interfaces:**
- Consumes: Task 1 渲染器标准输出。
- Produces: `$RUNNER_TEMP/release-notes.md` 和 `gh release create --notes-file`。

- [ ] **Step 1: 扩展 workflow 契约并确认 RED**

  要求 RC/GA 都包含 `render-github-release-notes.sh`、`--notes-file`，并拒绝任一文件出现
  `--generate-notes`。运行契约，预期因旧接线失败。

- [ ] **Step 2: 接入两个工作流**

  在 release job checkout 精确 `SOURCE_SHA`，调用：

  ```bash
  deployments/packages/render-github-release-notes.sh \
    "$VERSION" "$GITHUB_REPOSITORY" rc >"$RUNNER_TEMP/release-notes.md"
  ```

  GA 使用 `ga`，`gh release create` 使用
  `--notes-file "$RUNNER_TEMP/release-notes.md"`。

- [ ] **Step 3: 验证 GREEN 和语法**

  ```bash
  bash test/suites/product/endpoint/release-notes.sh
  bash test/suites/product/endpoint/release-workflow-contract.sh
  go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.7 .github/workflows/release-candidate.yml .github/workflows/release-stable.yml
  git diff --check
  ```

  Expected: 全部 exit 0。

- [ ] **Step 4: 提交并发布 rc.2**

  ```bash
  git add deployments/packages/render-github-release-notes.sh test/suites/product/endpoint/release-notes.sh test/suites/product/endpoint/release-workflow-contract.sh test/Makefile .github/workflows/release-candidate.yml .github/workflows/release-stable.yml
  git commit -m "fix(release): restore installation notes"
  git push github release/v0.1.0
  gh workflow run release-candidate.yml --ref release/v0.1.0 -f rc_number=2
  ```

  Expected: `v0.1.0-rc.2` 为 Pre-release，描述包含三类用户说明并指向新提交。
