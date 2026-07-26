# Standalone Signed Rule Pack Design

## 结论

SysArmor Agent 二进制只提供规则语言、字段模型、编译校验和执行引擎。所有具体 endpoint 检测规则、context set 和 IOC pack 都由独立签名内容包分发。standalone 缺少必需内容、验签失败、引用冲突或编译失败时必须启动失败，不允许使用二进制 fallback 后继续运行。

本次迁移一次性交付：完成 standalone 内容引导、签名信任、安装升级/回滚和所有生产入口迁移后，删除 Go 二进制中的具体 builtin 规则及隐式 builtin ruleset 注入。

## 目标

- 修改具体检测逻辑时只需发布内容包，不需重新发布 Agent 二进制。
- standalone 启动前确定默认检测内容完整、可信、可编译。
- 默认内容随发行包升级和回滚，用户内容不被覆盖。
- Agent 健康状态不能掩盖默认规则缺失或加载失败。
- Endpoint Policy、rule pack、context set 和 IOC pack 版本均可审计。

## 非目标

- 本次不设计独立于 Agent Release 的在线内容仓库或自动更新协议。
- 本次不改变 Manager/Gateway 已有 content 控制命令协议。
- 本次不修改 Tetragon stable ID、事件语义或规则语言之外的数据模型。
- 本次不增加多级内容覆盖或复杂优先级；ref 冲突直接失败。

## 内容分层

发行版内容与用户内容物理隔离：

```text
/opt/sysarmor/agent/content/default/     发行版托管，只读
/var/lib/sysarmor/agent/content/         用户/平台下发，持久化
```

发行版目录至少包含：

- 签名 endpoint rule pack；
- 规则引用的签名 context set；
- 规则引用的签名 IOC pack；
- `content-manifest.json`，列出必需 ref、kind、version、digest 和文件名。

用户内容继续通过 `content apply` 写入现有持久化目录。发行版 ref 禁止通过运行时控制接口修改或删除。两层出现相同 ref 时直接失败，不定义覆盖顺序。

## 信任模型

默认内容使用独立 Ed25519 内容签名密钥：

- 私钥只存在于 Release CI secret；
- 公钥通过 standalone 配置的 `content.trust_keys` 分发；
- 包内默认内容不享受免签特权，使用现有 content 验签逻辑；
- Release 构建缺少内容签名私钥时失败；
- Agent 启动时任何必需内容缺签、错签、未知 key ID 或 digest 不一致都失败。

Agent 发行包自身的 artifact 签名与内容签名是两条独立校验链：前者保护安装包，后者支持内容独立分发与验证。

## 安装、升级与回滚

安装器把发行版内容部署到目标目录同一文件系统中的 staging 目录，完成文件清单和权限检查后通过目录重命名原子替换默认内容目录。

- 首次安装：写入默认内容和 trust key 配置。
- 升级：整体替换发行版默认内容，不逐文件覆盖。
- 失败：保留旧默认目录，安装返回非零。
- 回滚：旧 Agent 包携带的默认内容整体恢复。
- 用户目录：安装、升级和回滚均不修改。

配置与默认内容必须属于同一发行事务，避免新 trust key 配旧内容或旧 trust key 配新内容。

## Agent 启动顺序

Agent 启动调整为：

1. 读取 Agent 配置、Endpoint Policy 和内容信任公钥。
2. 读取发行版 manifest，确认所有必需文件存在且元数据匹配。
3. 使用现有 content store 验签并解析全部发行版内容。
4. 加载并验签持久化用户内容。
5. 检查两层 ref 冲突。
6. 合并为单一不可变 content snapshot。
7. 校验 Endpoint Policy 的 ruleset/context/IOC 引用。
8. 校验 collection capability 和规则字段依赖。
9. 编译 detection engine。
10. 全部成功后启动 Tetragon、控制面和健康循环。

启动失败必须在 stderr 和进程退出错误中包含失败阶段、内容 ref、版本和具体原因，但不得输出密钥材料或完整签名。

## 规则解析语义

- 删除 `builtinRules()` 中的所有具体检测规则。
- 删除 policy 未声明 ruleset 时自动启用 builtin ruleset 的行为。
- 默认 standalone Endpoint Policy 必须显式引用发行版 rule pack 提供的 ruleset。
- 重复 rule ID、重复 ref 或未解析 ruleset 视为错误，不能通过 map 后写覆盖。
- `rule_overrides` 继续只调整启用、严重级别、模式和响应，不修改规则结构。
- 规则结构变更通过发布新版本 rule pack 完成。

`process.pid`、`sequence`、`correlate`、`same_as`、布尔条件树等属于二进制规则语言能力，继续保留。`suspicious_exec_connect`、`payload_lifecycle` 等具体规则迁移到签名 rule pack。

## 运行时更新

运行时 `content apply` 保持 prepare、验签、构建新 snapshot、编译、commit 的原子流程。更新失败时继续使用上一个已生效 snapshot，并将失败写入控制命令结果和 detection health。

发行版 ref 是只读保留命名空间，运行时更新请求命中这些 ref 时拒绝。用户 rule pack 可以使用其他 ruleset/ref，但 Endpoint Policy 必须显式引用后才生效。

## 健康与可观测性

Agent health 增加或保证能够表达：

- 当前 Agent 版本；
- Endpoint Policy ID/version；
- 生效 rule pack、context set、IOC pack 的 ref/version/digest；
- detection apply 状态；
- 默认内容 manifest 版本。

standalone ready 断言必须检查必需默认内容已经生效，不能只检查 Tetragon running 和 policy loaded。

## 测试与验收

### 单元测试

- 发行版内容 manifest 完整加载。
- 缺文件、错 kind/version/digest、重复 ref 分别失败。
- 无签名、未知 key、错签名分别失败。
- rule pack 引用缺失 context/IOC 或 collection field 时失败。
- policy 未引用任何有效 ruleset 时失败。
- `rule_overrides` 不改变规则结构。
- builtin 具体规则和隐式 fallback 已不存在。

### 安装测试

- 首次安装得到正确默认内容和 trust key。
- 升级原子替换默认内容并保留用户内容。
- staging 校验失败保留旧默认内容。
- 回滚恢复旧默认内容版本。
- systemd 与 container profile 行为一致。

### 运行测试

- fake backend 使用显式测试 rule pack，不依赖 builtin。
- standalone Agent 缺失或损坏默认内容时进程非零退出并包含明确错误。
- 动态用户内容更新失败时保留旧 engine。
- 全仓测试不再隐式获得 builtin 规则。

### Release 验收

- Ubuntu 22.04、Ubuntu 24.04、Debian 12 三镜像从发行包加载签名默认内容。
- health 输出默认内容 ref/version/digest。
- 五个真实攻击场景产生完整 Event/Signal 证据。
- sibling 与 host 隔离检查通过。
- 负向篡改一个默认内容文件后容器必须启动失败。
- 验收结果记录 Agent、Policy、rule pack、context set 和 IOC pack 版本。

## 迁移完成标准

- 生产和 standalone 路径不存在具体 builtin 检测规则。
- 没有有效签名默认内容时检测型 Agent 无法启动。
- 修改 `suspicious_exec_connect` 等规则只改内容包即可通过测试和 Release 验收。
- Agent 二进制只在规则语言或采集能力变化时才需要升级。
