# Pre-release Documentation And Notes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 RC 与 GA 自动提供完整安装说明，并使发布相关文档和公开仓库入口与当前流程一致。

**Architecture:** 一个纯 Bash 渲染器集中生成 Release notes，RC 与 GA 工作流只负责精确检出源码、调用渲染器和创建 Release。README、部署指南、开发指南各自只描述其受众需要的信息；治理文件链接到现有事实源，不复制长篇规则。

**Tech Stack:** Bash、GitHub Actions、GitHub CLI、Markdown。

## Global Constraints

- 不改变 release 制品、签名、安装器或 provenance 构建逻辑。
- RC 与 GA 使用同一个 notes 渲染器，且不再使用 `--generate-notes`。
- 版本、仓库和发布类型必须严格校验，错误退出码为 2。
- 英文保持简洁产品语气，中文保持仓库现有技术说明风格。
- 不修改历史验收报告、设计历史和版本解析 fixture，不新增 `CHANGELOG.md`。
- `v0.1.0-rc.2` 必须完成 fresh medium 与三镜像公开验收后才可作为 GA 基线。

---

### Task 1: Release Notes Renderer And Workflow Integration

**Files:**
- Create: `deployments/packages/render-github-release-notes.sh`
- Create: `test/suites/product/endpoint/release-notes.sh`
- Modify: `test/Makefile`
- Modify: `test/suites/product/endpoint/release-workflow-contract.sh`
- Modify: `.github/workflows/release-candidate.yml`
- Modify: `.github/workflows/release-stable.yml`

**Interfaces:**
- Consumes: `render-github-release-notes.sh VERSION REPOSITORY rc|ga`。
- Produces: 标准输出 Markdown；非法输入或参数数量错误时退出 2。

- [ ] **Step 1: 写 renderer 失败契约**

  创建 Shell 测试，分别执行：

  ```bash
  deployments/packages/render-github-release-notes.sh v0.1.0-rc.2 PKU-ASAL/sysarmor rc
  deployments/packages/render-github-release-notes.sh v0.1.0 PKU-ASAL/sysarmor ga
  ```

  断言输出包含精确版本、`install.sh`、`--profile linux-container`、容器运行约束、
  `gh attestation verify` 和 `/commits/<tag>` 变更列表 URL；`v0.1`、`owner only`、`beta` 以及
  RC/GA 类型不匹配均退出 2。

- [ ] **Step 2: 运行测试并确认 RED**

  Run: `bash test/suites/product/endpoint/release-notes.sh`

  Expected: 非零退出，提示缺少 `render-github-release-notes.sh`。

- [ ] **Step 3: 实现最小 renderer**

  Bash 脚本使用 `set -euo pipefail`，校验参数数量、`owner/name` 和类型对应的 SemVer；从
  `GITHUB_SHA` 读取源码提交，未设置时使用 `unknown`。用单个 heredoc 输出在线安装、容器
  Dockerfile、运行要求、provenance 命令和当前 tag 的提交列表链接，不执行输入内容。

- [ ] **Step 4: 扩展 workflow 失败契约**

  在 `release-workflow-contract.sh` 要求两个 workflow 都检出 `SOURCE_SHA`、调用 renderer、
  使用 `--notes-file`，且都不含 `--generate-notes`。

  Run: `bash test/suites/product/endpoint/release-workflow-contract.sh`

  Expected: 非零退出，旧 workflow 仍使用 `--generate-notes`。

- [ ] **Step 5: 接入 RC 与 GA workflow**

  release job 在下载资产前使用固定 revision 的 `actions/checkout`，设置：

  ```yaml
  with:
    ref: ${{ needs.build.outputs.source_sha }}
  ```

  调用 renderer 输出 `$RUNNER_TEMP/release-notes.md`，并将 `gh release create` 参数替换为：

  ```bash
  --notes-file "$RUNNER_TEMP/release-notes.md"
  ```

- [ ] **Step 6: 验证并提交**

  ```bash
  bash test/suites/product/endpoint/release-notes.sh
  bash test/suites/product/endpoint/release-workflow-contract.sh
  go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.7 .github/workflows/release-candidate.yml .github/workflows/release-stable.yml
  git diff --check
  git add deployments/packages/render-github-release-notes.sh test/suites/product/endpoint/release-notes.sh test/suites/product/endpoint/release-workflow-contract.sh test/Makefile .github/workflows/release-candidate.yml .github/workflows/release-stable.yml
  git commit -m "fix(release): restore installation notes"
  ```

  Expected: 两个契约输出 `ok`，actionlint 和 diff 检查退出 0。

### Task 2: User And Maintainer Documentation

**Files:**
- Modify: `README.md`
- Modify: `README.zh-CN.md`
- Modify: `docs/operations/deployment.md`
- Modify: `docs/development/development.md`

**Interfaces:**
- Consumes: 当前 RC/GA workflow、`linux-systemd` 与 `linux-container` 安装契约。
- Produces: 面向用户的安装入口和面向维护者的单一发布顺序。

- [ ] **Step 1: 写文档契约检查并确认 RED**

  使用 `rg` 确认活跃文档不再包含 `v0.1.0-dev.20260724+097acdae`、`从 dev 构建` 或把
  GitHub 安装限定为“开发预发布”，并要求开发指南包含 `--notes-file`、`main`、
  `production-release` 和三镜像验收语义。

- [ ] **Step 2: 精准更新四个文档入口**

  README 只说明从 GitHub Releases 选择目标版本并执行页面中的固定命令；部署指南将表格和章节
  统一为“GitHub 发行包”，使用 `<tag>`，说明 RC/GA 与 profile；开发指南按
  `dev -> release/vX.Y.Z -> RC -> 验收 -> main -> GA` 的顺序补齐操作和前置条件。

- [ ] **Step 3: 验证并提交**

  ```bash
  ! rg -n 'v0\.1\.0-dev\.20260724\+097acdae|从 `dev` 构建的可追溯' README.md README.zh-CN.md docs/operations/deployment.md docs/development/development.md
  git diff --check
  git add README.md README.zh-CN.md docs/operations/deployment.md docs/development/development.md
  git commit -m "docs(release): align installation and publishing guides"
  ```

  Expected: 过期表述扫描无输出，diff 检查退出 0。

### Task 3: Public Repository Governance

**Files:**
- Create: `SECURITY.md`
- Create: `CONTRIBUTING.md`

**Interfaces:**
- Consumes: GitHub private vulnerability reporting、`docs/development/development.md`、
  `docs/development/testing.md`。
- Produces: GitHub 自动识别的安全报告与贡献入口。

- [ ] **Step 1: 新增最小治理文档**

  `SECURITY.md` 明确 `0.1.x` 为当前受支持系列，要求通过 GitHub Security Advisories 的
  “Report a vulnerability” 私密报告，并列出复现、影响、版本和缓解信息。`CONTRIBUTING.md`
  说明 Issue、从 `dev` 建分支、PR 回 `dev`、Conventional Commits 和按范围验证，并链接已有
  开发与测试指南。

- [ ] **Step 2: 验证链接、内容和提交**

  ```bash
  rg -n 'Report a vulnerability|0\.1\.x' SECURITY.md
  rg -n 'dev|Conventional Commits|development\.md|testing\.md' CONTRIBUTING.md
  git diff --check
  git add SECURITY.md CONTRIBUTING.md
  git commit -m "docs: add security and contribution guidance"
  ```

  Expected: 所有扫描命中，diff 检查退出 0。

### Task 4: Integrated Verification And RC2

**Files:**
- Modify: GitHub repository description through `gh repo edit`。
- Create after acceptance: RC2 acceptance report/evidence assets outside the committed source tree。

**Interfaces:**
- Consumes: Tasks 1-3 的提交和 GitHub release workflow。
- Produces: 已发布并公开验收的 `v0.1.0-rc.2`。

- [ ] **Step 1: 运行发布相关回归**

  ```bash
  bash test/suites/product/endpoint/release-notes.sh
  bash test/suites/product/endpoint/release-workflow-contract.sh
  bash test/suites/product/endpoint/standalone-github-assets.sh
  bash test/suites/product/endpoint/standalone-release-package.sh
  git diff --check
  ```

  Expected: 全部 exit 0，工作区仅包含已知的计划文档改动或保持干净。

- [ ] **Step 2: 更新仓库简介并推送 release 分支**

  ```bash
  gh repo edit PKU-ASAL/sysarmor --description "Linux endpoint security and correlation system with standalone detection and centralized investigation."
  git push github release/v0.1.0
  ```

  Expected: GitHub 仓库简介非空，远端 release 分支指向本地 HEAD。

- [ ] **Step 3: 发布并核验 RC2**

  ```bash
  gh workflow run release-candidate.yml --repo PKU-ASAL/sysarmor --ref release/v0.1.0 -f rc_number=2
  ```

  等待 workflow 成功；确认 `v0.1.0-rc.2` 是 Pre-release，target 为 release 分支 HEAD，资产完整，
  Release body 自动包含安装、容器、provenance 和 changelog。

- [ ] **Step 4: 完成公开验收**

  对 RC2 的公开 `install.sh` 运行 fresh medium 和 Ubuntu 22.04、Ubuntu 24.04、Debian 12
  三镜像矩阵；要求 dropped/parse/watcher errors 为 0，三镜像各 5/5 场景、16 Signal、
  Precision/Recall 1，并将报告与证据上传到 RC2 Assets。

- [ ] **Step 5: 最终发布就绪检查**

  确认 `production-release` Environment、审批人和三个签名配置已建立；在此之前不触发 GA。
  汇报提交、RC2、验收结果以及唯一剩余的正式发布门禁。
