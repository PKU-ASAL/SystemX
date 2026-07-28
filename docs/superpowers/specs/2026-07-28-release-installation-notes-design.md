# Release 安装说明设计

## 结论

新增一个共享 Bash 渲染器生成 GitHub Release notes，RC 与 GA 工作流都通过
`--notes-file` 使用它。GitHub 自动 changelog 由渲染器输出链接替代，避免
`--generate-notes` 覆盖面向用户的安装说明。

## 背景

旧 `dev-prerelease.yml` 在工作流内生成在线安装、容器安装和 provenance 验证说明。迁移到
`release-candidate.yml` 与 `release-stable.yml` 时改用了 `--generate-notes`，导致
`v0.1.0-rc.1` 只有变更列表，没有安装入口。

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

## 测试

- 独立脚本契约验证 RC、GA 输出及非法参数失败。
- workflow 契约要求两个入口调用共享渲染器、使用 `--notes-file` 且不使用
  `--generate-notes`。
- 继续运行 `actionlint` 与现有 release workflow contract。

## 发布影响

该改动会改变 Git tree，因此在 `release/v0.1.0` 提交后发布 `v0.1.0-rc.2`。它不改变
Agent 二进制、签名格式或安装器，但 GA 仍以 rc.2 的相同 Git tree 为准。
