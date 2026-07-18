# SysArmor 技术图双版布局设计

## 目标

为同一组技术事实提供两种正式表达：A 版服务国自然式项目申报、技术尽调和书面评审；B 版服务投资 Pitch、路演和学术报告。两张主题图各制作 A/B 一版，共四张 Draw.io 与 SVG 成对资产。

## 共同真实性边界

- 实线仅表示当前代码、架构文档或测试已有依据的机制。
- 虚线及“规划/验证中”仅用于时间关系推理、候选路径排序、攻击阶段归纳、自然语言增强、完整事件回溯和完整 Incident UI。
- Endpoint Signal 的平台文档身份按 `tenant + agent + signal` 隔离；历史关联作用域按 `tenant + case_type/scenario/workload` 和 15 分钟窗口限定，不能混写为 Agent 隔离的关联保证。
- 当前关联实现表达为 Signal 视图、规则化 Signal 组合、稳定投影和结构化 Incident；不把规划中的路径推理写成当前算法。
- A/B 版本使用相同技术事实，不新增未经验证的指标或算法名称。

## 总体技术路线 A：严格国自然纵向树

### 阅读结构

```text
研究对象与边界
      ↓
三个关键问题
      ↓
统一研究目标
      ↓
三个研究任务
      ↓
关键机制与工程载体
      ↓
输出、系统性质、验证指标
```

### 版式

采用纵向中心树，不把 Agent、Gateway、Kafka、Worker 等组件横向当作主叙事。顶部定义 Linux 终端行为和私有化管理边界；第二层列出采集约束、跨批关联和结论解释三个问题；中部以同一目标节点分出三个任务：可信端点数据基础、跨批次关联与稳定投影、结构化 Incident 与调查基础；每个任务框内分别放输入、当前机制、工程载体和输出。

底部设置三列验收：产品链路测试、攻击/良性场景测试、独立 VM 资源测试。规划能力作为任务框底部的短虚线标签，不使用跨层反馈箭头。

## 总体技术路线 B：问题—任务—验证主轴

### 阅读结构

```text
可信行为 → 局部 Signal → 跨批关联 → 结构化 Incident → 可验证结论
```

### 版式

中心是一条从左上向右下的单一主轴。左侧只放三个问题，并用短连接指向主轴上的对应阶段；右侧只放阶段输出和验证。贯穿主轴的窄条标注本地有界、可靠交接、作用域隔离和重试幂等四项系统约束。

B 版不展开所有组件细节，保留“方法动作 + 可观察输出 + 验证方式”三元组；底部用一条短虚线标出规划/验证中的增强能力。该图作为投资 Pitch 默认引用。

## 关联方法 A：严格分步推导链

### 核心链路

```text
输入定义
  ↓
作用域与窗口
  ↓
Signal 视图 V
  ↓
规则判定 F(V, Policy)
  ↓
稳定投影 Π
  ↓
Incident + Evidence
```

### 版式

采用论文方法章节式的六步纵向推导。每一步左侧是输入/变换，右侧是一个边界或不变量：分析标签边界、15 分钟窗口、Endpoint Signal 稳定身份、规则与 terminal 条件、确定性文档 ID、贡献 Signal 与实体 Evidence。底部用 staged 示例逐步标出 Batch N 与 Batch N+1 的数据如何进入同一作用域并参与规则化组合。

规划能力集中在最底部一个“规划/验证中”区域，不回连当前步骤，避免视觉上暗示已实现反馈学习。

## 关联方法 B：ICM 报告风格

### 核心命题

顶部只保留一条主公式：

```text
Current ∪ History
  --Scope(tenant, labels, 15 min)-->
Signal View
  --Rules + Converge-->
Stable Incident
```

### 版式

中部使用一个大幅 staged 示例：Batch N 的 Signal A 与 Batch N+1 的 Signal B 进入同一 Signal View。右侧只放三条短注释：作用域防止错误混合，历史窗口提供跨 batch 连续性，稳定投影使重试收敛到同一逻辑结果。底部用一条虚线研究前沿列出时间推理、路径排序和解释增强。

该图减少卡片和装饰性连线，使用更大字号、更多留白和单一核心命题，作为学术报告和 Pitch 的补充图。

## 文件与引用

新增并保留以下四对资产：

- `technical-roadmap-a-nsfc.drawio` / `.svg`
- `technical-roadmap-b-narrative.drawio` / `.svg`
- `correlation-method-a-derivation.drawio` / `.svg`
- `correlation-method-b-lecture.drawio` / `.svg`

旧的 `technical-roadmap.*` 和 `correlation-method.*` 不再作为正式版本保留，避免同一主题出现第三套图。投资 Pitch 默认引用 `technical-roadmap-b-narrative.svg` 和 `correlation-method-b-lecture.svg`。图表 README 列出四个版本的用途和对应编辑源。

## 验收标准

1. 四个 Draw.io 与四个 SVG 均通过 XML 校验。
2. 每个 Draw.io/SVG 对的可见节点、文字、连接关系和实虚线语义一致。
3. A 版可以从上到下复述“问题—目标—任务—机制—验证”；B 版可以从主轴或主公式复述同一链路。
4. 四张图不使用穿过节点或文字的反馈箭头；规划能力不回连已实现节点。
5. Pitch 只引用两个 B 版 SVG；A 版只在 README 和技术附件语境中出现。
6. 旧无后缀图表不再被 Markdown 或 README 引用。
