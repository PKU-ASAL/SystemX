# 发布前文档与 Release 说明设计

## 结论

精准修复发布相关的用户入口、维护者文档和 GitHub Release notes，不机械更新历史记录或
测试 fixture。新增一个共享 Bash 渲染器，RC 与 GA 工作流都通过 `--notes-file` 使用它；
同时补齐公开仓库必要的安全报告与贡献入口。

## 背景

旧 `dev-prerelease.yml` 在工作流内生成在线安装、容器安装和 provenance 验证说明。迁移到
`release-candidate.yml` 与 `release-stable.yml` 时改用了 `--generate-notes`，导致
`v0.1.0-rc.1` 初始发布时只有变更列表，没有安装入口；该页面已经手工修复，但工作流仍会复现
同一问题。

全仓审计还发现以下发布前缺口：

- 中英文 README 和部署指南仍将 GitHub 发行包描述为从 `dev` 构建的开发预发布；
- 部署指南使用了会快速过期的固定 dev tag；
- 开发指南没有完整说明 Release notes、默认分支工作流和正式发布前置配置；
- 公开仓库缺少 `SECURITY.md`、`CONTRIBUTING.md`，GitHub 仓库简介为空；
- `production-release` Environment 尚未配置，因此 GA 当前仍应被阻止。

## 方案

`deployments/packages/render-github-release-notes.sh` 接收三个位置参数：版本、仓库和发布类型
`rc|ga`，向标准输出生成 Markdown。内容固定包含：

- 精确版本和源码提交；
- 在线安装命令；
- `linux-container` Dockerfile 示例和运行约束；
- `gh attestation verify` 命令；
- 指向当前 tag 的 GitHub changelog 链接。

脚本严格校验版本、仓库和发布类型，不接受任意模板或 shell 片段。RC/GA 发布 job 将输出
写入 `$RUNNER_TEMP/release-notes.md`，并使用 `gh release create --notes-file`。不再使用
`--generate-notes`。

### 文档入口

- `README.md` 与 `README.zh-CN.md` 保持现有简洁产品语气，只将安装入口改成同时适用于 RC
  和正式版本的表述，不把完整发布流程搬到首页。
- `docs/operations/deployment.md` 使用 `<tag>` 作为稳定示例，说明公开发行包、主机 profile、
  容器 profile、来源验证和离线分发边界，不绑定某个短期版本。
- `docs/development/development.md` 按 release 分支、RC、公开验收、合入 `main`、GA 的顺序
  说明维护者操作，并明确受保护 Environment、审批人与签名 secret 是 GA 前置条件。
- 历史验收报告、已提交的设计记录和用于版本解析的测试 fixture 保持原样，以保留证据真实性和
  测试覆盖意图。

### 仓库治理

- `SECURITY.md` 说明受支持版本范围和私密漏洞报告入口，不要求用户在公开 Issue 披露漏洞。
- `CONTRIBUTING.md` 复用现有开发、测试文档，简要说明从 `dev` 拉分支、通过 PR 回到 `dev`
  以及提交和验证要求，避免复制长篇开发手册。
- GitHub 仓库简介使用一句准确英文描述，不设置尚不存在的产品主页。
- 不新增 `CHANGELOG.md`；GitHub Releases 继续作为发行变更的唯一事实源。

## 测试

- 独立脚本契约验证 RC、GA 输出及非法参数失败。
- workflow 契约要求两个入口调用共享渲染器、使用 `--notes-file` 且不使用
  `--generate-notes`。
- 检查 README 与主要文档链接，运行 `actionlint`、现有 release workflow contract、相关
  Product 测试和 `git diff --check`。

## 文风与边界

新增内容沿用相邻文档的语言和结构：先说明用户要完成的动作，再说明约束与原因；英文保持简洁，
中文保持技术说明口吻。避免模板化口号、重复定义和生硬直译。每项修改必须对应本次发布入口、
发布操作或公开仓库治理，不扩展到无关架构文档。

## 发布影响

这些改动会改变 Git tree，因此在 `release/v0.1.0` 提交后发布 `v0.1.0-rc.2`。它们不改变
Agent 二进制、签名格式或安装器，但 GA 仍以 rc.2 的相同 Git tree 为准。RC2 按既定门禁完成
fresh medium 与 Ubuntu 22.04、Ubuntu 24.04、Debian 12 三镜像公开验收后，才可作为 GA 基线。
