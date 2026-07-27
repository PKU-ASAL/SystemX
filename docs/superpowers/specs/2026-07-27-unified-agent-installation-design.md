# Unified Agent Installation Design

## 目的与结论

SysArmor Agent 的正式发行、源码开发和测试安装必须共享同一套安装事务语义。采用“一个内部安装引擎、两个薄入口”：新增 `deployments/agent/install-core.sh` 作为唯一安装实现，`install-release.sh` 负责发行包验证和输入适配，`install-agent.sh` 负责源码及测试输入适配。

两个入口不得直接提交安装文件、管理 systemd 服务或实现回滚。安装顺序、内容校验、原子提交、服务启动、健康检查和失败恢复只能存在于核心安装引擎中。

## 问题背景

正式 Release 安装会在 Agent 启动前安装默认 content；开发和性能测试通过 `test/shared/vm/sync-agent.sh` 调用旧的 `install-agent.sh`，只安装默认 policy，没有安装它引用的 `ruleset:cep-endpoint`。性能脚本原计划在 Agent 启动后通过 CLI 上传 content，但严格的 ruleset 校验使 Agent 无法启动，形成启动环。

这不是检测引擎的兼容性问题，而是多个安装入口的事务语义发生漂移。修复不得放宽缺失 ruleset 时拒绝启动的规则。

## 架构边界

### 标准安装源

核心安装引擎只接收显式路径，不识别“开发”或“Release”模式。调用者必须提供以下标准输入：

- Agent 和 CLI 二进制
- systemd service 文件
- Agent 配置源
- 默认 policy
- 默认 content 目录及 `content-manifest.json`
- Tetragon bundle 或安装器输入
- 安装目标目录、socket 路径和是否启用服务

缺少必需输入时必须在修改目标系统前失败。路径必须解析为普通文件或目录，不能依赖调用者当前工作目录。

### `install-core.sh`

核心安装引擎负责：

1. 校验所有输入文件、content manifest 和安装平台。
2. 停止现有 Agent，但保留可恢复的旧版本状态。
3. 将配置、policy、默认 content、二进制和 sensor bundle 写入同文件系统的暂存位置。
4. 验证默认 policy 引用的每个启用 ruleset 都存在于暂存 content 中。
5. 提交安装事务；提交失败或收到 `INT`、`TERM` 时恢复旧状态。
6. 启用并启动 Agent，等待控制 socket 和健康检查成功。
7. 首次安装启动失败时显式失败；升级启动失败时恢复旧版本并重新启动旧服务。

核心脚本不负责验证 Release 文件清单或签名，也不负责构建二进制。

### `install-release.sh`

正式入口负责：

- 解析 `linux-systemd`、`linux-container` profile。
- 验证 Release 包结构、SHA256 清单和发行内容完整性。
- 合并现有用户配置与新版 Release 配置。
- 将包内文件映射为标准安装源，然后调用核心安装引擎。

发行入口不得绕过核心引擎直接安装或启动服务。

### `install-agent.sh`

开发和测试入口负责：

- 接收现有 `SYSARMOR_*` 路径覆盖参数。
- 将源码树中的 `deployments/agent/content` 作为默认 content 输入。
- 将开发配置、policy、构建产物和可选 Tetragon archive 映射为标准安装源。
- 调用核心安装引擎。

为避免静默产生不完整安装，默认 content 不再是可省略输入。测试若需要无检测基线，必须显式传入一个自洽的空 detection policy 和对应 manifest，而不能省略 content 安装阶段。

## 安装事务

安装的可观察顺序固定为：

```text
验证安装源
  -> 暂存二进制、配置、policy、content、sensor
  -> 验证 policy/content 引用闭合
  -> 停止旧服务并原子提交
  -> 启动 Agent
  -> 控制 socket 与 health 验证
  -> 成功后删除备份
```

配置、policy 和默认 content 必须属于同一事务。不能先提交引用新 ruleset 的 policy，再在服务启动后补装 rulepack。

动态 `sysarmorctl content apply` 仍然保留。性能测试在 Agent 正常启动后再次应用 content，是为了验证运行时内容更新路径，而不是补齐启动必需状态。

## 错误处理与回滚

- 输入或引用校验失败：不停止现有服务，不修改安装目标。
- 暂存失败：删除暂存文件，保留旧安装。
- 提交中断：通过统一 trap 恢复二进制、配置、policy、content 和 sensor 状态。
- 新服务启动或健康检查失败：恢复旧安装；升级场景重新启动旧服务。
- 回滚失败：保留诊断文件并返回非零状态，不得静默继续。

所有错误必须指出失败阶段和相关路径，但不得输出密钥或配置中的敏感值。

## 测试与验收

### 合同测试

- 两个入口都必须调用核心安装引擎，且自身不包含 `systemctl enable --now` 或安装提交逻辑。
- 默认 policy 引用缺失 ruleset 时，安装在启动服务前失败。
- 默认 policy 与 content 自洽时，首次安装成功启动。
- content、配置或 policy 提交失败时，完整恢复旧版本。
- `INT`、`TERM` 中断时恢复旧版本。
- 升级后健康检查失败时恢复旧安装并重新启动旧 Agent。
- 用户配置合并、文件权限和容器 profile 行为保持现有合同。

### 集成回归

- `test/suites/product/endpoint/standalone-release-package.sh` 通过。
- 开发态 `install-agent.sh` 安装测试通过。
- `make test-performance PROFILE=quick WORKLOAD=business-normal` 完整通过。
- `medium` 使用 `business-normal`、`apt-fileless-c2-local` 和 `collection-balanced` 完整进入采样并通过。
- 相关 Go、Release、回滚和内容事务测试通过。

## 非目标

- 不放宽检测 policy 的 ruleset 严格校验。
- 不改变 rulepack、context 或 IOC 数据格式。
- 不要求日常开发测试先生成完整签名 Release 包。
- 不重构与安装事务无关的性能采样逻辑。
- 不新增可选兼容入口或保留第三套安装实现。
