# Topology OpenSearch Query Fix Design

## 目的

修复 linux-container topology 中 `apt` 和 `staged` 场景的查询误判、跨 batch 关联失败与 endpoint signal 文档覆盖问题，使 `apt`、`staged`、`benign` 三个场景通过真实 Manager API 稳定验收。

## 根因

1. OpenSearch 搜索器把所有 `Exact` 字段转换为 `<field>.keyword`，但 `where`、`tenant_id`、`behavior` 等顶层字段在 mapping 中已经是 `keyword`，导致 layer、tenant 和 behavior 精确查询无法命中。
2. worker 历史读取复用错误的 `Exact` 查询，无法读取前序 batch 的 endpoint signals，导致 staged 跨 lineage 关联缺少历史输入。
3. endpoint signal 使用 agent 本地递增的 signal ID 作为 OpenSearch 文档 ID，不同 agent 或场景会相互覆盖。
4. topology 事件查询默认最多返回 1000 条，`process.exit` 噪声可能挤出关键事件。

## 方案

### Exact 查询语义

`SearchRequest.Exact` 表示调用方提供的字段已经是可执行 term 查询的精确字段。搜索器直接查询字段本身，不再自动追加 `.keyword`。

动态 label 继续使用 `labels.<key>.keyword`，因为 label 属性由动态 mapping 建成 text + keyword 多字段。布尔字段继续查询原字段。

测试必须同时约束查询 JSON 与部署 mapping，避免单元测试再次固化与真实 mapping 不一致的字段路径。

### Signal 文档身份

endpoint signal 的 OpenSearch 文档 ID 使用 tenant、agent、signal ID 组成的稳定哈希。cloud signal 保持现有 projection key，避免改变云侧派生结果的幂等语义。

同一 tenant、agent、signal ID 的重试必须覆盖同一文档；不同 tenant 或 agent 使用相同 signal ID 时必须生成不同文档。

### Topology 验收

场景继续只通过 Manager API 验收，不直接依赖 OpenSearch 内部接口。事件存在性查询使用足够窄的行为过滤或显式限制，避免 `process.exit` 噪声影响断言；signal layer 和 terminal 过滤必须保留，以覆盖修复后的产品查询能力。

## 测试策略

1. OpenSearch 查询单测先证明顶层 keyword 字段错误使用 `.keyword`，再验证 `where`、`tenant_id` 和 `behavior` 使用原字段，labels 仍使用 `.keyword`。
2. mapping 契约测试读取部署 mapping，验证 Exact 使用的顶层字段均为 keyword。
3. worker history 测试验证 tenant 和 endpoint layer 请求能够命中真实查询语义，并覆盖分批到达的 payload/connect 关联。
4. signal projection 测试验证跨 agent 隔离与同 agent 重试幂等。
5. 运行相关 Go 单元测试后，重建 container topology，依次执行 apt、staged、benign。

## 影响与风险

`Exact` 是共享搜索接口，修改会影响 event behavior、signal layer 和 worker history 等调用方。所有当前调用字段都来自显式 keyword mapping，因此直接 term 查询是正确语义；测试需要枚举这些调用点以防遗漏。

不修改索引 mapping，不需要迁移或重建生产索引。测试环境仍会由 topology 启动脚本重建。

## 验收标准

- `--layer endpoint|cloud` 查询返回对应 signals。
- worker 能从 OpenSearch 读取历史 endpoint signals，并为 staged 生成 `crossLineage=true` 的 cloud signal 和 incident。
- 不同 agent 的相同本地 signal ID 不再覆盖。
- apt 产生 endpoint、cloud、terminal signal 和 incident。
- staged 产生 endpoint、cross-lineage cloud signal 和 incident，且无 terminal signal。
- benign 有事件且无 signal、incident 或 terminal signal。
