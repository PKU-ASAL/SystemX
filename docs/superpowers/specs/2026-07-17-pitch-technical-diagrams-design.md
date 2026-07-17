# SysArmor Pitch 技术图设计

## 目的

在现有三张商业叙事图之外，新增两张具有技术路线和方法原理深度的图。两张图必须同时满足三个要求：非安全专业投资人可以沿主线理解，技术尽调人员可以核对关键机制，规划能力不会被误解为当前已实现能力。

## 共同表达规则

- 已实现并有代码或测试依据的机制使用实线边框和实线箭头。
- 下一阶段研究或产品化能力使用虚线边框和虚线箭头，并直接标注“规划/验证中”。
- 不使用未经实现或无法解释的算法名称，不填写没有测试口径支撑的性能或检测数字。
- 每个阶段都说明输入、处理机制和可检查输出，避免只列组件名称。
- 图中技术词第一次出现时附带通俗解释；产品组件名只作为机制的工程载体。
- 主色用于现状，灰色和虚线用于规划，红色仅标识约束或风险边界。

## 图一：端到端技术路线图

### 核心问题

图的顶部并列三个问题：

1. 主机行为数量大，采集完整性、端侧开销与存储容量难以同时满足。
2. 单条事件和单条告警缺少上下文，跨时间、跨批次的攻击步骤难以关联。
3. 底层系统行为难以直接转化为可复核、可行动的安全结论。

### 技术主线

图的中部按数据流从左到右分为五个阶段。

#### 1. 端点行为获取

输入为 Linux 进程、文件、网络、身份和容器行为。Agent 管理传感器，将不同来源转换为统一事件结构，并保留原始引用以便后续取证。

已实现约束：采集由统一策略控制，Agent 在 standalone 和 managed 两种模式下共享同一条本地运行路径。

#### 2. 本地实时判断与有界保存

统一事件同时进入端点检测和本地存储。低延迟规则生成 Endpoint Signal；SQLite 保存身份、策略、Signal 和上传进度，高吞吐事件进入有界追加段。

已实现约束：只有本地持久化成功才确认写入；容量压力和数据丢弃显式计数；断网或未注册不停止本地检测。

#### 3. 可信传输与可靠交接

完成注册的 Agent 使用终端私钥和独立证书上传选定事件与 Signal。Gateway 校验租户与 Agent 身份，Kafka 承担平台内部的持久交接。

已实现约束：上传响应区分 accepted、duplicate、retryable 和 terminal invalid；Agent 只在 accepted 或 duplicate 后推进 checkpoint。

#### 4. 跨批次关联与证据投影

Worker 以 tenant 和受影响的分析标签为边界，读取当前 batch 与 OpenSearch 中 15 分钟历史事件及 Endpoint Signal，重新计算 Cloud Signal、Incident 和 Evidence。

已实现约束：Endpoint Signal 文档标识按 `tenant + agent + signal` 隔离；派生文档使用稳定标识；必需写入成功后才提交 Kafka offset。

#### 5. 查询与调查基础

Manager API 和 Web 提供终端、事件与 Signal 查询，Manager API 提供 Incident 与 Evidence 查询。Incident 是可重复计算的分析报告，Evidence 保留报告与贡献 Signal 和实体之间的关系。

当前边界：概览、部署、终端和事件检索已接入；完整 Incident 页面作为虚线规划能力标识。

### 规划研究支线

从 Evidence 和人工复核结果引出一条虚线反馈支线：证据充分性评估、候选路径排序优化和自然语义解释质量评估。反馈只改进派生分析，不修改原始事件和既有证据。

### 输出与验证

图的底部将输出分为三类：

- 产品输出：事件、Endpoint Signal、Cloud Signal、Incident、Evidence 和终端状态。
- 系统性质：离线可工作、数据边界明确、重试幂等、租户与终端隔离、结论可回溯。
- 验证方法：产品链路测试、攻击/良性场景检测测试、独立虚拟机 CPU/内存/吞吐/丢弃性能测试。

## 图二：攻击关联与根因解释方法原理图

### 输入层

输入分为两组：当前 batch 中的事件与 Endpoint Signal，以及同一作用域 15 分钟历史窗口中的事件与 Endpoint Signal。事件包含进程、文件、网络、身份、时间、lineage 和原始引用等关系信息。

### 方法步骤

#### 1. 作用域隔离与时间对齐

先按 tenant 和分析标签（case type、scenario 或 workload）限定分析边界，再合并当前数据与历史窗口。该步骤防止不同组织或不同分析作用域的数据被错误拼接；Endpoint Signal 的平台文档身份另按 tenant、Agent 和 Signal ID 隔离。

#### 2. Signal 视图与实体聚合

按名称组织 Endpoint Signal，汇总 lineage、terminal 和实体引用，形成后续规则可以检查的 Signal 视图与局部实体证据。

#### 3. 规则化 Signal 组合

Cloud Rule 和 Converge Policy 检查 Signal 组合、terminal 和 cross-lineage 条件，输出 Cloud Signal 或 Incident 判定。staged 场景允许所需 Signal 分布在不同 batch，只要它们处于同一分析作用域和历史窗口内。

#### 4. 关联收敛与稳定投影

对受影响作用域重新计算派生结果，以 correlation key 和 analysis version 收敛到稳定 Incident；Signal、Incident 和 Evidence 使用确定性文档标识，使重试或重复处理更新同一逻辑结果。

#### 5. 结构化 Incident 表达

当前实现输出摘要、lineage、terminal、贡献 Signal 和 Evidence 实体图。时间关系推理、候选攻击路径排序、攻击阶段归纳、自然语言增强和分析员反馈学习统一标为“规划/验证中”；这些增强不能修改原始事件或既有 Evidence。

### 图中示例

使用一个抽象 staged 场景展示跨 batch 关联，不绑定具体客户数据：

```text
Batch N：下载或落地可疑载荷 -> Endpoint Signal A
Batch N+1：载荷执行并建立外联 -> Endpoint Signal B
历史窗口合并：A + B + 满足 Signal 组合/terminal 策略 -> Incident + Evidence
```

示例旁标注：关联成立依赖分析作用域、时间窗口和策略条件；实体与 lineage 作为 Incident 上下文保留，而非通用关联前提。

### 可验证性质

- 隔离性：Endpoint Signal 文档标识在不同 tenant 或 Agent 间不冲突；关联数据不跨 tenant 或分析作用域串联。
- 连续性：同一作用域内分批到达的阶段可以通过历史窗口关联。
- 幂等性：重复 batch 和 Worker 重试不会产生新的逻辑 Incident。
- 可解释性：Incident 保留贡献 Signal 与 Evidence；从 Evidence 进一步回到原始事件的完整产品体验仍需继续验证和产品化。
- 边界性：超出窗口、作用域或策略条件的数据不参与该次关联。

## 文档集成

- 新增 `docs/business/diagrams/technical-roadmap.drawio` 与对应 SVG。
- 新增 `docs/business/diagrams/correlation-method.drawio` 与对应 SVG。
- 在 Pitch 的“技术与工程壁垒”章节后引用总体技术路线图。
- 在“从研究能力到工程原型”章节中引用攻击关联与根因解释方法图。
- 保留现有产品价值流、技术壁垒和商业闭环图，形成“商业概览 -> 技术路线 -> 方法原理 -> 商业路径”的阅读层次。

## 验收标准

1. `.drawio` 和 `.svg` 均为有效 XML，且两种表示的可见文字和结构一致。
2. Markdown 相对链接有效，中文正文继续保持一段一行。
3. 图中所有“已实现”机制能够在正式架构文档、代码或测试中找到依据。
4. 所有规划内容同时具有虚线样式和“规划/验证中”文字，不只依赖颜色区分。
5. SVG 在窄屏允许横向缩放，但文字在常见桌面宽度下无需放大即可阅读。
6. 图注明确说明现状与规划的视觉编码，避免脱离正文后被误读。
