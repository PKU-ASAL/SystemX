# Release 分支与发布工作流设计

## 目标

建立职责单一、可审计的 SysArmor 发布流程：短生命周期 `release/vX.Y.Z` 分支只发布
RC，`main` 只发布 GA；两个入口复用同一构建工作流，保证测试、签名、校验和 provenance
一致。

## 分支模型

```text
dev
  └─ release/vX.Y.Z
       ├─ vX.Y.Z-rc.1
       ├─ vX.Y.Z-rc.2
       └─ PR -> main -> vX.Y.Z
```

- 功能和修复分支仍从 `dev` 创建并通过 PR 合回 `dev`。
- 准备发布时从已冻结的 `dev` 创建 `release/vX.Y.Z`。
- release 分支只接受发布阻塞修复、版本信息和发布说明，不接受新功能。
- RC 只允许从匹配 `release/vX.Y.Z` 的分支发布。
- GA 只允许从 `main` 发布，且 `main` 的 Git tree 必须与已经通过验收的 RC tag 完全一致。
- GA 发布后，将 release 分支上的发布修复同步回 `dev`，然后删除 release 分支。
- 禁止直接提交到 `dev` 和 `main`，禁止向两者 force push。

## 工作流边界

### 可复用构建工作流

`.github/workflows/release-build.yml` 使用 `workflow_call`，只负责产生经过验证的 Release
制品，不创建 GitHub Release。它执行：

1. 校验版本格式、源提交和调用方声明的发布类型。
2. 运行 `go test ./...` 及 standalone/package/container/release contract 测试。
3. 构建 `sysarmor-agent` 与 `sysarmorctl`。
4. 生成或读取 artifact manifest RSA key 与 Ed25519 content key。
5. 调用 `package-agent.sh`，显式传入 `--signing-key`、`--content-signing-key` 和
   `--content-key-id`。
6. 生成 `install.sh`、`SHA256SUMS`，并立即执行 SHA-256 自校验。
7. 为包、安装器和校验文件生成 GitHub build provenance attestation。
8. 上传不可变 workflow artifact，并输出版本、提交 SHA 和 artifact 名称。

构建工作流不接受任意 shell 参数。版本、发布类型和 key ID 都使用结构化输入并进行格式
校验，避免参数注入和误发布。

### RC 入口

`.github/workflows/release-candidate.yml` 使用 `workflow_dispatch`：

- 只允许从 `release/vX.Y.Z` 分支运行。
- 输入 `rc_number` 必须是正整数。
- 分支版本与输入组合为 `vX.Y.Z-rc.N`。
- 调用公共构建工作流。
- 创建带 `--prerelease` 标记的 GitHub Release。
- tag 和 Release 均指向触发工作流的准确提交 SHA。
- 同一版本或 tag 已存在时显式失败，不覆盖现有制品。

### GA 入口

`.github/workflows/release-stable.yml` 使用 `workflow_dispatch`：

- 只允许从 `main` 运行。
- 输入版本必须严格匹配 `X.Y.Z`，生成 tag `vX.Y.Z`。
- job 使用受保护的 GitHub Environment `production-release`，要求人工审批。
- 要求输入已通过验收的 RC tag，并验证当前 Git tree 与 RC tag 的 Git tree 完全一致。
- 调用公共构建工作流，并创建非 prerelease 的 GitHub Release。
- tag 和 Release 均指向触发工作流的准确提交 SHA。
- tag、Release 或版本制品已存在时显式失败。

RC 和 GA 重新构建各自制品，但两者使用同一构建逻辑。GA 不直接复制 RC 包，因为版本号
进入包内容和文件名；二者通过相同 Git tree、相同测试门禁和 provenance 建立可追溯关系。

## 签名与密钥

- artifact manifest 使用 RSA key，内容包使用独立 Ed25519 key。
- RC 默认使用 runner 临时生成的两把私钥，job 结束后由 runner 临时目录清理。
- GA 使用 `production-release` Environment secrets 提供正式私钥：
  `SYSARMOR_ARTIFACT_SIGNING_KEY_PEM` 和 `SYSARMOR_CONTENT_SIGNING_KEY_PEM`。
- workflow 将 secret 写入权限 `0600` 的 runner 临时文件，不输出私钥内容，不上传私钥。
- GA content key ID 使用可审计的稳定标识，例如 `sysarmor-release-2026-01`，由
  Environment variable `SYSARMOR_CONTENT_KEY_ID` 提供。
- 公钥、内容签名、包 manifest、SHA256 和 provenance 属于公开制品；私钥不进入仓库、
  artifact、cache、日志或容器层。
- 密钥轮换和吊销不在首个工作流实现中自动化，但必须在正式 GA 前形成操作文档。

## 发布门禁

公共构建工作流必须通过：

```text
go test ./...
standalone-release-package.sh
container-entrypoint.sh
standalone-github-assets.sh
dev-prerelease-workflow.sh
release-container-e2e-contract.sh
```

RC 创建后，使用公开 GitHub URL 运行三镜像矩阵：

```text
Ubuntu 22.04
Ubuntu 24.04
Debian 12
```

三镜像必须满足：安装成功、health 为 ok、detection 为 applied、五场景全部检出、Signal
EventRef 完整、namespace/self 隔离通过。验收报告和原始证据作为不可变 GitHub Actions
artifact 上传，并在 RC Release 或对应发布 Issue 中链接。RC 创建后冻结 release 分支，
不得为了提交报告而改变待发布 Git tree。

Fresh medium 作为 RC 性能门禁运行。首版保留现有“无丢弃、无解析错误、watcher 无错误”
硬门禁，并记录 steady/workload CPU 与 RSS；在积累稳定基线后再加入资源预算，避免用单次
样本设定不可靠阈值。GA 不重复运行 30 分钟 medium，但必须绑定已通过 medium 的 RC tag。

## 发布数据流

```text
release/vX.Y.Z @ SHA
  -> RC build/test/sign/attest
  -> GitHub pre-release vX.Y.Z-rc.N
  -> public URL three-image acceptance + fresh medium
  -> acceptance evidence uploaded and release branch frozen
  -> PR release/vX.Y.Z -> main
  -> production-release approval
  -> GA build/test/sign/attest @ main SHA
  -> GitHub Release vX.Y.Z
  -> release fixes synced to dev
```

## 失败处理

- 任一测试、签名、SHA 校验、attestation 或上传失败，工作流立即失败且不创建 Release。
- 如果 GitHub Release 创建失败，已上传的 workflow artifact 保留用于诊断，但不得手工改名
  冒充正式制品。
- 已存在 tag 或 Release 时不覆盖；修复后递增 RC 编号，GA 则先人工确认是否需要补丁版本。
- 公开三镜像验收失败时，修复必须提交到 release 分支并发布新的 RC，禁止复用失败 RC。
- GA 发布后发现阻塞缺陷时，停止推荐该版本并从 `dev` 创建补丁发布分支
  `release/vX.Y.(Z+1)`；不修改既有 Release 制品。

## 权限与保护

- `release/v*`：禁止 force push，至少一项必需状态检查通过后才能发布 RC。
- `main`：只允许 PR 合并，保护 tag `v*`，GA job 通过 `production-release` Environment
  审批后获得 `contents: write`。
- 公共构建 job 默认 `contents: read`、`id-token: write`、`attestations: write`。
- 只有创建 GitHub Release 的入口 job 获得 `contents: write`。
- 工作流 action 使用完整 commit SHA 固定版本。

## 测试策略

- 增加静态 workflow 契约测试，验证 RC/GA 分支限制、版本格式、RC/GA Git tree 一致性、
  Environment 审批、最小 permissions，以及 content signing key 参数不可缺失。
- 扩展 `dev-prerelease-workflow.sh` 或拆出通用 release workflow contract，避免测试名称
  与新架构不符。
- 使用临时目录和假 `gh` 命令验证重复 tag、错误分支、非法版本和缺少密钥时显式失败。
- 真实 GitHub 发布只能在受保护分支和 Environment 审批后执行；本地测试不调用外部发布。

## 首次发布顺序

1. 在当前功能分支实现并验证三个工作流及契约测试。
2. 通过 PR 合入 `dev`。
3. 从 `dev` 创建首个 `release/vX.Y.Z` 分支。
4. 发布 `vX.Y.Z-rc.1`。
5. 对公开 RC URL 完成 fresh medium 和三镜像验收，上传报告与原始证据并冻结 release 分支。
6. 将 release 分支通过 PR 合入 `main`。
7. 配置 `production-release` Environment、审批人和正式签名 secrets。
8. 从 `main` 发布 `vX.Y.Z`，验证安装器与 provenance。
9. 将发布修复同步回 `dev` 并删除 release 分支。

## 非目标

- 不建立长期常驻 `release` 分支。
- 不在同一入口工作流中通过自由文本参数切换 RC/GA。
- 不自动决定语义版本号。
- 不在首版实现跨仓库制品镜像、自动回滚或密钥自动轮换。
- 不绕过 PR 将 release 分支直接推入 `main` 或 `dev`。
