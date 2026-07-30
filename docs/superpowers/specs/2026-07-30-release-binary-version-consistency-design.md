# Release 二进制版本一致性设计

## 结论

Release Workflow 必须把同一个 Release 版本注入 `sysarmor-agent` 和
`sysarmorctl`，并在打包前执行两个真实二进制，确认它们的输出与待发布版本完全一致。
发布包最终必须满足：

```text
Git Tag = manifest.json version = sysarmor-agent version = sysarmorctl version
```

本次修复发布新的 `v0.1.0-rc.5`，不覆盖已有的 `v0.1.0-rc.4`。

## 构建行为

`.github/workflows/release-build.yml` 中两个 Go 构建命令统一使用：

```text
-ldflags "-X main.version=$VERSION"
```

`VERSION` 继续由 RC 或 Stable Workflow 的受校验输入提供。两个命令不得分别拼装版本，
避免 Agent 与 CLI 漂移。

## 发布门禁

构建完成后、打包前，Workflow 分别执行：

```text
dist/bin/sysarmor-agent version
dist/bin/sysarmorctl version
```

任一输出与 `VERSION` 不完全一致时立即失败，不生成、不签名、不上传发行资产。现有
Distribution Workflow 契约测试扩展为检查版本注入和两个真实二进制校验均存在。

## 错误处理

版本比较使用精确字符串匹配，不接受 `dev`、空值、前后缀或宽松语义版本兼容。
RC 仍使用 `vMAJOR.MINOR.PATCH-rc.N`，Stable 仍使用 `vMAJOR.MINOR.PATCH`。

## 验收

1. 先运行扩展后的 Workflow 契约测试，确认它能在缺少版本注入时失败。
2. 修复后运行 Distribution 本地测试及相关 Go 测试。
3. 按 `fix -> dev -> release/v0.1.0` 的 PR 流程合并。
4. 发布 `v0.1.0-rc.5`，下载并解包公开资产。
5. 确认 manifest、Agent 和 CLI 均报告 `v0.1.0-rc.5`。
6. 运行三系统 post-publish Distribution 验收。
