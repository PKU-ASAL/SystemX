# GitHub Release Make 命令设计

## 结论

保留现有 RC 到 Stable 的两阶段 GitHub 发布模型，在根 `Makefile` 增加两个面向发布人员的命令，隐藏 Workflow 文件名、触发 Ref 和 `accepted_rc_tag` 拼接细节。

```bash
make release-rc VERSION=1.0.0 RC=1
make release-stable VERSION=1.0.0 RC=1
```

现有 `make release` 保持不变，继续负责本地构建签名发行包。

## 命令行为

### `release-rc`

1. 校验 `VERSION` 符合 `MAJOR.MINOR.PATCH`。
2. 校验 `RC` 是正整数。
3. 校验本机存在 `gh` 且已通过 `gh auth status` 登录。
4. 触发 `.github/workflows/release-candidate.yml`。
5. 使用 `--ref release/v$(VERSION)` 和 `-f rc_number=$(RC)`。

### `release-stable`

1. 执行相同的参数与 GitHub CLI 校验。
2. 触发 `.github/workflows/release-stable.yml`。
3. 固定使用 `--ref main`。
4. 传递 `version=$(VERSION)`。
5. 自动传递 `accepted_rc_tag=v$(VERSION)-rc.$(RC)`。

## 错误处理

参数缺失、格式非法、缺少 `gh` 或 GitHub CLI 未登录时，以非零状态退出并打印具体修复方式。Make 命令只负责发起 Workflow，不等待完成、不自动合并分支、不创建 Release 分支，也不绕过 GitHub Environment 审批。

## 测试与文档

扩展现有 `test/suites/distribution/package/release-workflow-contract.sh`，验证 Make 目标、参数校验和准确的 `gh workflow run` 参数。更新根 Make 帮助及 `docs/development/development.md`，以 Make 命令作为推荐发布入口，同时保留 GitHub Actions 页面作为备用操作方式。
