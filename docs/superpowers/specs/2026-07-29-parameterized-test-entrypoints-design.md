# 参数化测试入口设计

## 结论

根目录 `Makefile` 只提供按测试分类命名的公共入口，使用与领域一致的显式参数选择子项。删除现有长入口，不提供兼容别名。`test/Makefile` 保留具体执行目标，作为公共入口、CI 和测试契约的实现层。

## 公共接口

| 分类 | 命令 | 参数 |
| --- | --- | --- |
| Unit | `make test-unit` | 无 |
| Functional | `make test-functional DOMAIN=...` | `endpoint\|platform\|topology\|all` |
| Detection | `make test-detection` | 无 |
| Performance | `make test-performance DOMAIN=...` | `endpoint\|platform\|modules\|all` |
| Distribution | `make test-distribution SOURCE=...` | `local\|published` |
| Release | `make test-release STAGE=...` | `pre-publish\|post-publish` |

已发布产物相关入口继续使用 `URL=...` 指定安装地址。Performance 保留现有 `PROFILE`、`WORKLOAD`、`SCENARIO` 和 `POLICIES` 调优参数。

## 参数语义

- `DOMAIN` 回答“测试哪个系统领域”，仅用于 Functional 和 Performance。
- `SOURCE` 回答“测试本地产物还是已发布产物”，仅用于 Distribution。
- `STAGE` 回答“执行发布前还是发布后门禁”，仅用于 Release。
- 不引入 `TARGET`、`VARIANT` 或通用 `SCOPE`，避免用一个模糊参数承载不同维度。

## 调度与校验

公共入口使用 Make 条件分支，将参数映射到 `test/Makefile` 的现有具体目标。参数缺失或取值不合法时必须以退出码 2 失败，并打印完整用法及合法值；禁止静默采用默认值，以免误跑耗时或有外部副作用的测试。

映射如下：

| 公共调用 | 内部目标 |
| --- | --- |
| `test-functional DOMAIN=endpoint` | `functional-endpoint` |
| `test-functional DOMAIN=platform` | `functional-platform` |
| `test-functional DOMAIN=topology` | `functional-topology` |
| `test-functional DOMAIN=all` | `functional-core` |
| `test-performance DOMAIN=endpoint` | `performance-endpoint` |
| `test-performance DOMAIN=platform` | `performance-platform` |
| `test-performance DOMAIN=modules` | `performance-modules` |
| `test-performance DOMAIN=all` | `performance-endpoint performance-platform performance-modules` |
| `test-distribution SOURCE=local` | `distribution-package` |
| `test-distribution SOURCE=published` | `distribution-published` |
| `test-release STAGE=pre-publish` | `release-candidate` |
| `test-release STAGE=post-publish` | `release-published` |

## 删除与清理

删除根目录以下公共长入口，不保留兼容别名：

- `test-functional-endpoint`
- `test-functional-platform`
- `test-functional-topology`
- `test-distribution-package`
- `test-distribution-published`
- `test-release-candidate`
- `test-release-published`

将仓库内活动文档、帮助输出和 CI 调用统一到新公共入口；仅在解释内部架构时展示 `make -C test ...`。清理 `test/` 中零引用的空目录、生成物和因入口迁移失效的重复说明，不修改历史结果 Markdown。

## 测试与验收

新增根 Makefile 调度契约，至少覆盖：

- 每个合法参数到内部目标的映射。
- 参数缺失和非法值均明确失败。
- 旧公共长入口不存在。
- 活动文档不再推荐旧入口。
- `make help` 和 `make test-help` 展示同一套公共接口。

完成后运行契约测试、`go test ./...`、Distribution 本地测试，以及与调度修改相关的 Functional 入口抽样验收。完整 VM 测试沿用已批准的沙箱外权限。
