# Policy-Driven Multi-Event Detection Design

## 结论

将 `reverse_shell_pattern`、`suspicious_exec_connect` 和 `payload_lifecycle` 从专用 Go builtin 迁移到动态 rulepack。Agent 新增的能力仅限通用、有界的条件分支和无序事实关联；具体威胁语义、Context、IOC、窗口、严重度和输出继续由 Detection Policy 控制。

迁移完成后，endpoint detection 二进制不再包含上述三条规则的专用 detector 或专用 lineage 状态。完整 NodLink 风格图恢复仍属于云侧，端侧不支持任意图遍历。

## 目标与边界

### 目标

- 新增和调整三条多事件规则时不更新 Agent 二进制。
- 新增其他满足“单 Agent、有限事实、有限窗口、明确关联键”边界的规则时，同样只更新 Collection/Detection Policy 和 Content；通用 runtime 测试不得使用三条首批迁移规则的专用名称或语义。
- 保留现有 `payload_lifecycle` 对事件乱序的容忍能力。
- 同一 Rule ID 只有一个权威 Signal 生成路径。
- Signal 的严重度、terminal、响应意图、实体和必要 Event refs 与规则语义一致。
- 所有运行时状态具有窗口、分组数、Event refs 和淘汰指标边界。

### 非目标

- 不实现跨 Agent、跨主机或长时间窗口关联。
- 不实现任意代码、循环、递归或通用图查询。
- 不在端侧实现 NodLink 的 Hopset、Steiner Tree 或全局路径排序。
- 不增加与本次三条规则无关的 DSL 语法。

## 运行时设计

### 条件分支

现有 `expr` 和 step conditions 是扁平 AND。新增最小条件树，只支持：

- `all`: 子条件全部满足。
- `any`: 至少一个子条件满足。
- `not`: 单个子条件不满足。
- `condition`: 复用现有字段、operator、literal 和 content ref 条件。

条件树编译为版本化、受类型检查的 IR。叶子仍使用现有 `compiledCondition`，不会引入脚本执行或规则专用 operator。旧的扁平 `conditions` 等价于 `all`，保持兼容。

### 无序关联 correlate

新增 `runtime.type = "correlate"`：

```json
{
  "type": "correlate",
  "correlate": {
    "within": "2m",
    "by": ["lineage_id"],
    "facts": [
      {"id": "drop", "events": ["file.write", "file.chmod"], "conditions": []},
      {"id": "exec", "event": "process.exec", "conditions": []},
      {"id": "connect", "event": "network.connect", "conditions": []}
    ]
  }
}
```

语义：

- facts 可以任意顺序到达。
- fact 使用 `event` 声明单个 behavior，或使用 `events` 声明一个或多个 behavior；两者同时出现或均为空时拒绝。
- 每个分组保存已命中的 fact、捕获字段和 Event refs。
- 重复 fact 更新该 fact 的最近有效证据，但不会重复增加同一 Event ref。
- 全部 fact 满足后立即发出一次 Signal，并删除分组状态。
- `within` 从该分组首个 fact 开始计算。
- `by` 必须是已支持字段，默认行为不隐式回退；空 `by` 在加载时拒绝。
- fact ID 必须唯一，未知字段、operator、空窗口和超限窗口在加载时拒绝。

资源边界复用 CEP 限制：

- `MaxCEPGroups`：每条规则最大活动分组数。
- `MaxCEPRefs`：每组最大 Event refs。
- `within` 必须位于 `(0, 24h]`，不允许无界窗口。
- 过期、容量淘汰和丢弃 refs 进入现有 CEP metrics。

`correlate` 不是图查询。它只判断同一有界分组内声明的有限事实是否全部出现。

## 规则迁移

### reverse_shell_pattern

使用单事件 `expr`：

- behavior 为 `network.connect`。
- `process.binary_name in ctx:shell-binaries`。
- `socket.port in ioc:c2-control-port-feed`。
- `terminal: true`。
- 保留 `collect_evidence` 响应意图。

Signal 只引用实际命中的连接 Event，不再附加不是命中必要条件的历史下载 Event。实体为 process 和 socket。

### suspicious_exec_connect

使用有序 `sequence`，窗口内按 `lineage_id` 关联：

1. `process.exec` 命中 payload 身份：binary 位于 `ctx:payload-path-prefixes`，或 argv 引用了 payload 路径。
2. `network.connect` 命中控制端口，并满足连接进程 stable ID 或 parent stable ID 与 exec step 的 process stable ID 相同。

条件中的 OR 使用通用 `any`，跨步骤身份比较继续使用 `same_as`。Signal 引用 exec 和 connect Event，实体为 process、payload file 和 socket，保持非 terminal。

### payload_lifecycle

使用 `correlate`，窗口内按 `lineage_id` 收集：

- `drop`: `events` 为 `file.write` 和 `file.chmod`，路径位于 `ctx:payload-path-prefixes`。
- `exec`: `process.exec`，binary 位于 payload 路径，或 argv 引用了 payload 路径。
- `connect`: `network.connect`，端口位于 `ioc:c2-control-port-feed`。

三个事实允许任意顺序。Signal 引用三个事实的 Event，实体为 process、payload file 和 socket，保持非 terminal。下载事件不是完成条件，也不加入 Signal refs。

## 状态清理

规则迁移后删除：

- `detectReverseShell`
- `detectPayloadConnect`
- `detectPayloadLifecycle`
- `observeDownloadEvidence`
- `observePayloadEvidence`
- download、payload、reverse-connect 和 lifecycle emitted 等专用 lineage 字段

只有仍被其他通用能力使用的进程事实索引可以保留；若无消费者则删除整个 `lineageState` 和 `Engine.state`。

## Content 与兼容

- Content Store 增加条件树和 correlate schema。
- daemon 将 schema 转换为 Detection RuleSpec，不静默丢弃字段或 duration 解析错误。
- 老 rulepack 的扁平 conditions、expr 和 sequence 保持可用。
- 未知 runtime、未知条件节点或非法 correlate 配置必须使策略 `rejected`。
- standalone builtin rulepack 使用同一 `expr/sequence/correlate` RuleSpec 作为无外部内容时的默认值。
- 动态 rulepack 同名规则仍由选中的 ruleset 决定，禁止生产双发。

## Release 验收

新增或拆分真实场景，独立验证：

- reverse shell：真实 shell 进程连接本地控制服务，命中 terminal Signal。
- suspicious exec connect：真实 payload 执行后连接控制服务，命中 exec/connect 双 Event Signal。
- payload lifecycle：真实写入、执行、连接链路命中三 Event Signal。
- payload lifecycle 乱序：通过可控事件顺序测试 correlate，不用字符串伪造事实。
- benign：正常业务读写和网络访问不命中上述规则。

每个场景校验 Rule ID、严重度、terminal、所需 behaviors、Event refs 和动态 Content refs。Release 三镜像验收要求安装包含 correlate 和条件树通用能力的新 standalone 二进制。

## 测试策略

- Content parser：条件树、correlate、非法 duration 和未知节点。
- Validation：空 by、重复 fact、未知字段/operator、空/超限窗口。
- Runtime：任意顺序、超时、重复 fact、按 by 隔离、容量淘汰、refs 上限和单次发射。
- Migration：三条规则动态 Context/IOC 替换、无重复 Signal、实体与 refs。
- Regression：Detection、Policy、Content、daemon、Tetragon 包和三条 Release shell contract。

## 风险控制

- 条件树只提供布尔组合，不提供任意表达式求值。
- correlate 仅允许固定 facts，不允许动态增加节点。
- 所有规则先联合校验再编译；失败沿用现有 `rejected/degraded` 语义。
- 迁移按规则独立提交，每条规则完成红灯、绿灯和回归后再进入下一条。
