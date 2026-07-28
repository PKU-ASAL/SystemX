# Remove Legacy Sensor Scope Design

## 结论

彻底删除 Agent Sensor 配置和 Tetragon Backend 中已经不可用的旧 scope 兼容入口，只保留 `sensor.scope.type/selector` 到规范化 `CollectionIntent.ScopeType/ScopeSelector` 的单一路径。

## 删除范围

- `SensorConfig.ScopeType`
- `SensorConfig.ScopeSelector`
- `SensorConfig.ContainerIDPrefix`
- `SensorConfig.EffectiveScope` 中旧字段合并、推断和冲突处理
- `Backend.ContainerIDPrefix`
- Backend 在空 scope 下通过 `ContainerIDPrefix` 过滤的旁路
- daemon 向 Backend 复制 `ContainerIDPrefix` 的接线
- 只验证旧旁路行为的测试

## 保留范围

- Agent YAML `sensor.scope.type/selector`
- `SensorConfig.Scope`
- `CollectionIntent.ScopeType/ScopeSelector`
- Collection Policy 的 `scope_type/scope_selector`
- Manager API、数据库及 Event/Signal 的 scope 字段
- `Backend.ScopeType/ScopeSelector`
- `containerIDsMatch` 统一 Container ID 匹配逻辑

## 失败语义

旧 YAML 字段继续由严格解析器返回 `unknown config key`，不迁移、不告警后继续运行，也不静默忽略。

## 验证

- 编译层面不存在旧结构体字段或 Backend 旁路。
- 旧 YAML `scope_type`、`scope_selector`、`container_id_prefix` 分别被拒绝。
- 嵌套 `sensor.scope` 的 host、container、namespace/self 和 pod 行为保持通过。
- Container ID 12/32/64 位兼容测试保持通过。
- 全仓搜索仅允许当前 Policy/API/存储/事件模型中的 `scope_type/scope_selector`。
