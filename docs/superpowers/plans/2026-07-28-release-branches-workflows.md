# Release Branches And Workflows Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 用短生命周期 `release/vX.Y.Z` 分支发布 RC，并只从与已验收 RC 同 tree 的 `main` 发布 GA。

**Architecture:** 两个薄入口工作流分别负责 RC 和 GA 的权限、来源校验及 GitHub Release 创建，共同调用一个只构建、测试、签名和上传制品的可复用工作流。静态 shell 契约测试验证 GitHub Actions 无法在本地直接执行的权限、分支、版本、签名和 tree 一致性约束。

**Tech Stack:** GitHub Actions YAML、Bash、Go、OpenSSL、GitHub CLI。

## Global Constraints

- `release/vX.Y.Z` 只发布 `vX.Y.Z-rc.N`；GA 只从 `main` 发布 `vX.Y.Z`。
- GA job 必须使用 `production-release` Environment，并验证当前 Git tree 与输入 RC tag 的 Git tree 一致。
- RC 使用临时 RSA 与 Ed25519 私钥；GA 使用 Environment secrets `SYSARMOR_ARTIFACT_SIGNING_KEY_PEM`、`SYSARMOR_CONTENT_SIGNING_KEY_PEM` 和 variable `SYSARMOR_CONTENT_KEY_ID`。
- 私钥不得进入仓库、artifact、cache、日志或容器层。
- Actions 必须固定到完整 commit SHA；构建 job 使用最小 `contents: read`、`id-token: write`、`attestations: write` 权限，发布 job 单独使用 `contents: write`。
- 不保留旧 `dev-prerelease.yml` 的重复发布入口。

---

### Task 1: Release Workflow Contract

**Files:**
- Create: `test/suites/product/endpoint/release-workflow-contract.sh`
- Modify: `test/Makefile:131-140`
- Delete: `test/suites/product/endpoint/dev-prerelease-workflow.sh`

**Interfaces:**
- Consumes: `.github/workflows/release-build.yml`、`release-candidate.yml`、`release-stable.yml` 的静态 YAML。
- Produces: `bash test/suites/product/endpoint/release-workflow-contract.sh`，成功输出 `[release-workflow-contract] ok`。

- [ ] **Step 1: 写失败的契约测试**

  创建测试，要求三个工作流存在；公共构建必须包含 `workflow_call`、结构化 `version/release_type/content_key_id` 输入、完整 action SHA、全部发布契约测试、两种签名 key 和 SHA256 自检；RC 必须检查 `release/vX.Y.Z`、正整数 `rc_number`、临时 RSA/Ed25519 key、`--prerelease` 和精确 SHA；GA 必须检查 `main`、`production-release`、正式 secrets、RC tag 格式、`git rev-parse "$RC_TAG^{tree}"` tree 比较及非 prerelease 发布。测试还必须拒绝旧 `dev-prerelease.yml` 存在。

- [ ] **Step 2: 运行测试并确认 RED**

  Run: `bash test/suites/product/endpoint/release-workflow-contract.sh`

  Expected: FAIL，原因是 `.github/workflows/release-build.yml` 尚不存在。

- [ ] **Step 3: 更新 standalone 测试入口**

  将 `test/Makefile` 中的 `dev-prerelease-workflow.sh` 替换为 `release-workflow-contract.sh`，删除旧契约脚本。

- [ ] **Step 4: 提交 RED 契约**

  ```bash
  git add test/Makefile test/suites/product/endpoint/release-workflow-contract.sh
  git rm test/suites/product/endpoint/dev-prerelease-workflow.sh
  git commit -m "test(release): define rc and ga workflow contracts"
  ```

### Task 2: Reusable Release Build

**Files:**
- Create: `.github/workflows/release-build.yml`

**Interfaces:**
- Consumes: `workflow_call` inputs `version: string`、`release_type: rc|ga`、`content_key_id: string`，以及 GA secrets `artifact_signing_key_pem`、`content_signing_key_pem`。
- Produces: outputs `version`、`source_sha`、`artifact_name`；artifact 内含版本 tarball、`install.sh`、`SHA256SUMS`。

- [ ] **Step 1: 实现公共构建工作流**

  校验版本与类型组合；RC 在 `$RUNNER_TEMP/sysarmor-release` 生成 3072-bit RSA 和 Ed25519 key；GA 以 `umask 077` 将调用方 secrets 写入临时文件并校验非空；构建两个 Go 二进制；调用：

  ```bash
  deployments/agent/package-agent.sh \
    --version "$VERSION" \
    --output "$package" \
    --agent-bin dist/bin/sysarmor-agent \
    --ctl-bin dist/bin/sysarmorctl \
    --tetragon-mode download \
    --signing-key "$manifest_key" \
    --content-signing-key "$content_key" \
    --content-key-id "$CONTENT_KEY_ID"
  ```

  随后生成 GitHub assets、执行 `(cd "$asset_dir" && sha256sum -c SHA256SUMS)`、attest 三类文件并上传以版本命名的 artifact。测试列表必须改用 `release-workflow-contract.sh`，避免引用已删除脚本。

- [ ] **Step 2: 运行契约测试观察进展**

  Run: `bash test/suites/product/endpoint/release-workflow-contract.sh`

  Expected: 仍 FAIL，但公共构建相关断言通过，首个失败转移至缺少 RC 入口。

- [ ] **Step 3: 静态检查 YAML 和 shell 块**

  Run: `ruby -e 'require "yaml"; %w[.github/workflows/release-build.yml].each { |f| YAML.load_file(f) }'`

  Expected: exit 0。

### Task 3: RC And GA Entrypoints

**Files:**
- Create: `.github/workflows/release-candidate.yml`
- Create: `.github/workflows/release-stable.yml`
- Delete: `.github/workflows/dev-prerelease.yml`

**Interfaces:**
- RC consumes: `workflow_dispatch.inputs.rc_number`，触发 ref `release/vX.Y.Z`。
- GA consumes: `workflow_dispatch.inputs.version` 和 `accepted_rc_tag`，触发 ref `main`。
- Both consume: `./.github/workflows/release-build.yml` outputs and immutable artifact.

- [ ] **Step 1: 实现 RC 入口**

  首个 job 用 Bash 正则从 `$GITHUB_REF_NAME` 提取 base version，要求 `rc_number` 为非零正整数，使用 `git ls-remote --exit-code --tags origin "refs/tags/$version"` 和 `gh release view` 显式拒绝重复 tag/release，并输出 `vX.Y.Z-rc.N`。调用公共构建时传 `release_type: rc` 和 `content_key_id: sysarmor-rc`。发布 job 下载精确 artifact，并用 `gh release create "$VERSION" ... --target "$SOURCE_SHA" --prerelease` 创建 RC。

- [ ] **Step 2: 实现 GA 入口**

  校验触发 ref 为 `main`、版本为 `X.Y.Z`、RC tag 为同 base version 的 `vX.Y.Z-rc.N`；fetch RC tag 后比较 `git rev-parse "HEAD^{tree}"` 与 `git rev-parse "$RC_TAG^{tree}"`；拒绝重复 GA tag/release。公共构建调用透传 Environment secrets，发布 job 声明 `environment: production-release` 并创建不带 `--prerelease` 的 Release。

- [ ] **Step 3: 删除旧入口并运行 GREEN 契约**

  Run: `bash test/suites/product/endpoint/release-workflow-contract.sh`

  Expected: `[release-workflow-contract] ok`。

- [ ] **Step 4: 验证所有 workflow YAML**

  Run: `ruby -e 'require "yaml"; Dir[".github/workflows/*.yml"].each { |f| YAML.load_file(f) }'`

  Expected: exit 0。

- [ ] **Step 5: 提交工作流**

  ```bash
  git add .github/workflows/release-build.yml .github/workflows/release-candidate.yml .github/workflows/release-stable.yml
  git rm .github/workflows/dev-prerelease.yml
  git commit -m "feat(release): add rc and ga workflows"
  ```

### Task 4: Documentation And Regression Verification

**Files:**
- Modify: `docs/development/development.md:65-82`

**Interfaces:**
- Consumes: 已实现的分支和工作流行为。
- Produces: 面向维护者的 RC/GA 操作说明，不包含私钥值。

- [ ] **Step 1: 更新发布开发文档**

  将“GitHub 开发预发布”替换为短生命周期 release 分支说明，列出 `release-candidate.yml` 和 `release-stable.yml` 输入、`production-release` 的两项 secret 和一项 variable，并强调 RC 公开 URL 验收及 GA tree 一致性门禁。

- [ ] **Step 2: 运行发布相关回归**

  ```bash
  bash test/suites/product/endpoint/release-workflow-contract.sh
  bash test/suites/product/endpoint/standalone-release-package.sh
  bash test/suites/product/endpoint/container-entrypoint.sh
  bash test/suites/product/endpoint/standalone-github-assets.sh
  bash test/suites/product/endpoint/release-container-e2e-contract.sh
  go test ./...
  ```

  Expected: 全部 PASS，无 warning 或静默跳过。

- [ ] **Step 3: 检查最小改动和敏感信息**

  ```bash
  git diff --check
  git diff --stat
  rg -n 'BEGIN (RSA |ED25519 |)PRIVATE KEY|SYSARMOR_.*KEY_PEM: [^-{$]' .github docs test
  ```

  Expected: diff 检查通过；没有私钥或硬编码 secret。

- [ ] **Step 4: 提交文档**

  ```bash
  git add docs/development/development.md
  git commit -m "docs(release): document rc and ga operations"
  ```

### Task 5: Integrate And Publish First RC

**Files:**
- No local file changes required.

**Interfaces:**
- Consumes: 用户确认的版本 `vX.Y.Z-rc.1`、GitHub 分支保护和仓库权限。
- Produces: 合入 `dev` 的实现、远端 `release/vX.Y.Z` 分支和公开 RC Release。

- [ ] **Step 1: 推送当前功能分支并创建 PR 到 dev**

  ```bash
  git push origin fix/release-short-lived-shell
  gh pr create --base dev --head fix/release-short-lived-shell \
    --title "feat(release): add rc and ga workflow" \
    --body "目的：用短生命周期 release 分支发布 RC，并从 main 发布同 tree 的 GA。\n\n改动：新增复用构建、RC、GA 工作流，替换 dev prerelease；加入签名、权限、分支和 tree 契约。\n\n验证：release workflow contract、standalone release contracts、go test ./...。"
  gh pr checks --watch "$(gh pr view --json number --jq .number)"
  ```

  Expected: required checks 全绿；PR 通过正常 merge 合入 `dev`，不直接提交 `dev`。

- [ ] **Step 2: 创建短生命周期 release 分支**

  从最新 `origin/dev` 创建并推送 `release/vX.Y.Z`，确认远端分支 SHA 与 `origin/dev` 相同。

- [ ] **Step 3: 触发首个 RC**

  Run: `gh workflow run release-candidate.yml --ref release/vX.Y.Z -f rc_number=1`

  Expected: workflow 成功，`gh release view vX.Y.Z-rc.1` 显示 prerelease、正确 target 和三个公开制品。

- [ ] **Step 4: 运行公开 RC 验收**

  使用 GitHub Release 的公开 `install.sh` URL 运行 Ubuntu 22.04、Ubuntu 24.04、Debian 12 三镜像矩阵和 fresh medium；要求三镜像各 5/5、Precision/Recall 1、无 dropped/parse/watcher error，并把原始证据作为 Actions artifact 上传。

- [ ] **Step 5: 停在 GA 人工门禁前**

  确认 `production-release` Environment 已配置审批人、`SYSARMOR_ARTIFACT_SIGNING_KEY_PEM`、`SYSARMOR_CONTENT_SIGNING_KEY_PEM` 和 `SYSARMOR_CONTENT_KEY_ID` 后，才允许 release PR 合入 `main` 并触发 `release-stable.yml`。缺少任一项时明确报告阻塞，不创建 GA。
