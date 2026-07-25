# 策略驱动的端侧检测运行时设计

## 目的与结论

SysArmor 应允许通过 Collection Policy 和 Detection Policy 新增、调整和停用具体端侧检测规则，而不需要重新编译 Agent。Agent 二进制只提供稳定的遥测适配、规范事件模型、通用 EDR 原语、受限规则执行环境和资源治理。

本设计采用渐进路线：扩展现有 `expr` 和 `sequence` 运行时，引入版本化 Detection IR，并将 WebShell、payload lifecycle、reverse shell 等具体威胁语义从 Go builtin 迁移到动态 rulepack。未来更强的 DSL、可视化编辑器或图查询前端必须编译到同一受限 IR，不能绕过 Agent 的类型、能力和资源校验。

完整 NodLink 风格的异常 terminal 筛选、Hopset 和在线 Steiner Tree 不属于本轮。端侧保留轻量 provenance 关系和确定性规则；长窗口 provenance 子图恢复主要属于云侧分析。

## 架构边界

### 二进制更新边界

以下变化不应要求更新 Agent：

- 新增或修改使用已有 Event 字段和通用原语的检测规则；
- 调整规则步骤、窗口、阈值、严重度、模式和响应意图；
- 更新 binary、路径、端口、CIDR、IOC 和可信主体集合；
- 启用、停用或替换签名 rulepack。

以下变化可以要求更新 Agent：

- 新增 CanonicalEvent 尚未表达的底层遥测行为或字段；
- 新增经过代表性规则证明的通用 Detection IR 算子；
- 新增稳定、威胁无关的能力型 Provider；
- 修复 Collection Compiler、事件适配器或 Detection Runtime 缺陷。

### Tetragon 边界

Collection Policy 只能选择 Agent Capability Catalog 中声明支持的行为、字段、过滤器和作用域。Agent 负责将其编译为受控的 Tetragon 配置，并将原始事件转换为稳定 CanonicalEvent。

正式策略面不透传任意 Tetragon YAML。Tetragon 新能力只有在 SysArmor 完成行为建模、规范化、能力报告、资源评估和兼容性测试后，才进入 Capability Catalog。

### 端云边界

- 端侧支持单 Agent、有限时间和有限状态下的确定性关联；
- 端侧关联键必须显式声明，例如 process stable ID、parent stable ID、lineage ID、容器、用户和文件身份；
- 跨主机、长时间窗口、全局实体图和候选攻击路径排序由云侧负责；
- Endpoint Signal 可作为云侧 provenance 图分析的高价值 terminal 候选；
- standalone 保持离线检测能力，但不默认运行完整 STP 或机器学习异常检测。

## 目标组件

### Capability Catalog

Capability Catalog 是 Agent 对策略公开的稳定能力契约，至少声明：

- 支持的 Event behavior；
- 每个 behavior 可保证的规范字段及类型；
- 支持的 selector 和下推位置；
- Detection IR、operator 和 Provider 版本；
- 平台、内核、Tetragon 版本和运行模式约束；
- 已知采集成本与资源限制。

Collection Coverage 和 Detection 编译都使用同一份能力目录，避免声明、编译和健康报告各自维护能力列表。

### CanonicalEvent 与关系语义

CanonicalEvent 继续作为 Sensor 与 Detection Runtime 的唯一稳定边界。实现必须稳定表达：

- 进程、文件、网络、用户、容器和 namespace 实体；
- process-parent、process-exec、process-read-file、process-write-file 和 process-connect-socket 等有类型关系；
- Event 时间、Agent、tenant、scope、lineage 和原始材料引用；
- 可跨 Event 比较的稳定实体身份。

Sensor 私有字段不得直接成为普通 Detection Rule 的长期契约。

### Versioned Detection IR

Rulepack 的 `expr` 和 `sequence` 首先编译为版本化 Detection IR，再生成有界执行计划。IR 节点必须具有明确类型和版本，未知字段、未知节点、类型不匹配或不支持的 capability 必须在加载时显式拒绝。

IR 与外部 YAML/JSON 语法分离。未来 DSL、图查询语言或可视化规则编辑器可以成为新的编译前端，但仍输出受 Agent 校验的 Detection IR。

### 通用 EDR 原语

第一阶段原语分为三组。

事件与类型原语：

- 字段存在性与类型检查；
- 等值、不等值、数值比较；
- 集合包含、前缀、正则和 CIDR；
- contextset 和 iocpack 引用；
- path、IP、port、process identity 等规范类型。

捕获与关系原语：

- 捕获某一步的字段值；
- 当前 Event 字段与历史步骤字段比较；
- same、different 和集合关系；
- parent、ancestor、same-container、same-user 和 same-file-identity；
- Signal 输出中保留所有贡献 Event refs 和实体关系。

时间与状态原语：

- 有序 sequence、`within` 和显式 `by`；
- count 和 distinct-count；
- bounded absence；
- dedupe 和 suppress；
- 每条规则、每组和 Agent 全局状态预算。

不支持任意代码、循环、递归、无界窗口、无界图遍历或未声明关联键。

### 能力型 Provider

Provider 在二进制中维护通用事实，不直接生成具体威胁 Signal：

- Process Graph Provider：解析 parent 和 ancestor，带 TTL 和节点上限；
- Path Identity Provider：路径规范化、脚本参数中的 payload 路径和可选文件身份；
- Network Classifier：IP、端口、CIDR、内外网与 IOC 集合；
- Subject Context Provider：容器、namespace、用户、capability 和 workload 身份。

Provider API 禁止使用 `webshell`、`reverse-shell` 等具体威胁命名。新增 Provider 或二进制原语必须至少服务两个不同攻击类别，并通过代表性规则语料证明通用性。

## 规则表达示例

`web_runtime_spawns_shell` 应由 rulepack 表达：

1. 捕获 `process.exec`，要求 binary 属于 `ctx:web-runtime-binaries`；
2. 在有界窗口内观察新的 `process.exec`；
3. 要求新进程 binary 属于 `ctx:shell-binaries`；
4. 要求第二步 `parentStableId` 等于第一步 `stableId`，或由 Process Graph Provider 证明 parent 关系；
5. 产生 Signal，并引用父进程和 Shell 的 Event。

该规则不得依赖 argv 中出现 `node` 等字符串，也不得在 Agent 中维护 `webShellExecRefs` 等规则专用状态。

`payload_lifecycle` 应由同一路径或文件身份上的写入、执行和控制连接步骤组成；`reverse_shell_pattern` 应由 Shell 身份、process ancestry 和控制网络分类组合。具体 binary、路径和端口集合来自 contextset 或 iocpack。

## 资源与失败语义

### 有界状态

所有规则状态统一进入通用 Runtime State Store，不再由具体 builtin 创建永久 map。至少限制：

- 最大活动 group 数；
- 每组最大 Event refs；
- 单条规则最大窗口；
- Process Graph 节点数和 TTL；
- Agent 全局状态内存预算；
- 单 Event 最大候选规则数和条件计算量。

淘汰、过期、引用丢弃和计算错误必须进入 Metrics 和 Agent health。

### Policy 应用

本轮沿用现有 Endpoint Policy 应用机制，不新增复杂的 Bundle generation 或双运行时事务：

- Detection Engine 构建或编译失败时保留上一有效引擎；
- Collection 应用失败时保留上一有效采集配置；
- 必需 behavior 或 field 缺失时使用现有 Coverage 和 `degraded` 语义；
- health 必须指出受影响规则、缺失 behavior/field 和实际有效版本；
- `degraded` 不得被展示为规则完全正常。

规则语法错误、未知 capability、类型错误和静态预算超限属于 rejected，而不是 degraded。

## 避免片面设计

建立 20 至 30 条代表性 EDR 规则语料，覆盖：

- Initial Access 与 Execution；
- Persistence；
- Privilege Escalation；
- Credential Access；
- Defense Evasion；
- Discovery；
- Command and Control；
- Exfiltration；
- Container Escape 与 Runtime Abuse。

每次新增 IR operator 或 Provider 时必须回答：

1. 是否无法使用现有 IR 表达；
2. 是否至少被两个不同攻击类别复用；
3. 是否保持威胁无关命名和稳定语义；
4. 是否具有明确状态、时间和计算上限；
5. 是否通过完整规则语料回归，而非只验证提出该能力的单条规则。

## 迁移策略

迁移按风险和依赖分阶段进行：

1. 建立字段目录、IR 版本、跨步骤字段比较和统一 Runtime State Store；
2. 先迁移 `web_runtime_spawns_shell`，验证 parent 关系与完整双 Event 引用；
3. 迁移 `download_by_lolbin`、`payload_dropped` 和 `credential_file_read` 等单事件或低状态规则；
4. 补齐 count、absence、suppress 等原语；
5. 迁移 `suspicious_exec_connect`、`reverse_shell_pattern` 和 `payload_lifecycle`；
6. 删除已迁移的具体 builtin 路径和专用 lineage 状态；
7. 保留必要的兼容映射，使旧策略引用得到明确升级或拒绝结果，不静默改变规则语义。

迁移期间同一 Rule ID 只能有一个权威执行实现。可以在测试或 shadow 模式中比较新旧输出，但不得在生产模式长期双发 Signal。

## 验证标准

### 功能

- 现有 endpoint builtin 效果场景全部可以由动态 rulepack 表达；
- WebShell 使用真实父进程关系，伪造 argv 不触发；
- 所有多事件 Signal 保留完整、存在且顺序可解释的 Event refs；
- 更新 rulepack、contextset 和 iocpack 不需要重启或重新编译 Agent；
- 不支持的规则在应用前被明确拒绝或以现有 Coverage 语义标记 degraded。

### 资源

- 状态组、Process Graph、Event refs 和窗口均可证明有界；
- 达到上限时行为确定，并产生可查询指标；
- 代表性良性负载下的 CPU、RSS、Event 延迟和 Signal 速率有回归基线；
- 规则数量增加时，候选索引避免每个 Event 扫描全部规则。

### 兼容与安全

- Rulepack 声明最低 Event schema、IR、operator 和 Provider 版本；
- Agent 对未知或超预算规则 fail closed；
- 内容摘要、规则版本和运行时版本进入 Signal 与 health；
- malformed、恶意高基数和状态耗尽规则具有负向测试；
- standalone 与 managed 模式使用同一端侧运行时语义。

## 非目标

本轮不实现：

- 任意 Tetragon YAML 正式透传；
- 完整通用图查询语言；
- 用户规则中的任意代码、WASM 或插件执行；
- NodLink 的 VAE terminal scorer、Hopset 或在线 Steiner Tree；
- 跨主机端侧关联；
- 新的 Endpoint Policy Bundle 原子事务协议；
- 自动 Agentic 规则生成和无审批下发。
