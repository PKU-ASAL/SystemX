# Core File Governance Design

## 目的与结论

本轮治理在 `refactor/product-monorepo-layout` 分支内完成，不创建 PR，不改变 SysArmor 的公开 API、包路径、协议、持久化格式、部署拓扑或运行行为。

六个超大生产文件采用“同 package、按领域职责拆文件”的方式治理。拆分优先保持语义内聚，500 行仅作为识别职责混杂的提示，不作为机械门禁。`packages/` 增加准入规则和自动化依赖边界检查；当前没有足够的维护团队，因此不增加 `CODEOWNERS`。

## 约束

- 保持现有导出类型、函数和方法签名不变。
- 保持现有 package 名称和 import path 不变。
- 不引入新的接口层、依赖注入框架、Go module 或前端 workspace。
- 不顺带修复或重写业务逻辑；发现独立缺陷时记录并另行处理。
- 只移动完整声明及其紧密辅助逻辑，不把同一事务或状态机机械切开。
- 每个拆分关注点形成可独立验证和回退的原子提交。

## 方案选择

采用同 package 内按职责拆文件。

未选择子 package 拆分，因为 Manager Store 与 AgentRuntime 共享锁、状态和事务上下文。当前直接建立子 package 会迫使内部状态公开化或引入大量窄接口，增加架构复杂度和行为漂移风险。

未选择按行数切分，因为文件边界必须表达变化原因和领域语义，而不是满足任意行数。

## 文件职责设计

### Manager Store

`apps/manager/internal/store/store.go` 最终只保留 Store 核心状态、打开与 backend 绑定等生命周期逻辑。现有声明按以下领域移动到同 package 文件：

- `models.go`：通用状态与领域数据结构。
- `policy.go`：默认策略、策略 CRUD、发布、分配和审计。
- `response.go`：Response 创建、审批、查询和确认。
- `control_command.go`：控制命令创建、状态转换、查询和确认。
- `evidence.go`：Evidence pullback 生命周期。
- `agent_state.go`：Agent 身份、会话和健康状态。
- `enrollment.go`：注册记录及规范化。
- `artifact.go`：Artifact 与 Channel。
- `certificate.go`：Agent 证书和吊销。
- `telemetry_state.go`：Event、Signal、Incident、派生投影和指标。
- `persistence.go`：文件持久化、State 导入导出和原子写入。
- `keys.go`：稳定键、标签匹配和确定性排序辅助逻辑。

已有 `agent.go`、`policy_commit.go`、`enrollment_bootstrap.go`、`enrollment_issue.go` 和 `unenrollment.go` 的职责保持不变，避免重复抽象。

### PostgreSQL Store

`apps/manager/internal/store/postgres/snapshot.go` 保留 table backend 的构造和兼容入口，具体 SQL 按领域拆分：

- `backend.go`：backend 类型、超时和事务辅助。
- `agent_state.go`：Agent、Health 和 Session 查询与 projection。
- `policy.go`：Policy、Assignment、Audit 查询与写入。
- `control.go`：Response、Control Command 和 Evidence pullback。
- `enrollment.go`：注册消费、签发提交和查询。
- `artifact.go`：Artifact、Channel 和 Certificate。
- `metrics.go`：Metrics 与 Rarity baseline。
- `projection.go`：完整 State projection 的编排入口。

SQL 语句、事务范围、锁语义和错误返回保持原样。

### Agent Daemon

`apps/agent/internal/daemon/daemon.go` 保留 `Options`、`AgentRuntime`、构造函数和主运行生命周期。其他职责拆为：

- `health.go`：启动、运行和关闭健康状态采集。
- `runtime_state.go`：策略、Detection、Collection 和 Supervisor 的并发状态访问。
- `detection_runtime.go`：Detection engine 构建、内容转换和应用状态。
- `data_batch.go`：批次、序列、标签和发送端创建。
- `managed_network.go`：Manager TLS 和 managed network 启动。
- `response_runtime.go`：Response 执行与 Evidence pullback。
- `sensor_factory.go`：Sensor 和重启策略构造。

`Run` 的阶段顺序、清理顺序和失败报告路径保持不变。

### Agent Local Control

`apps/agent/internal/daemon/local_control.go` 保留 Unix gRPC server 的启动、依赖装配和服务类型。RPC 与转换按职责拆为：

- `local_control_health.go`：Health、Capability 和 DebugProfile。
- `local_control_policy.go`：CurrentPolicy、ApplyPolicy 和各策略层应用。
- `local_control_content.go`：Content 应用、事务和查询。
- `local_control_watch.go`：Event/Signal 查询、watch 和过滤。
- `local_control_status.go`：Store 与 managed lifecycle 状态。
- `local_control_messages.go`：Ack、Health、Capability 和报告的协议转换。
- `local_control_validation.go`：请求上下文及 telemetry policy 校验。

所有 RPC 仍由同一个 `localControlServer` 实现，不增加服务间转发。

### Tetragon Backend

`apps/agent/internal/sensors/linux/tetragon/backend.go` 保留 `Backend`、构造、核心 Sensor 接口方法和共享状态。其他职责拆为：

- `capability.go`：Collection 能力、编译报告和 selector 分类。
- `collection_intent.go`：scope 解析、intent 匹配和过滤辅助。
- `tracing_policy.go`：TracingPolicy 生成、写入、应用、校验和删除。
- `event_source.go`：CLI/gRPC 事件源打开和订阅读取。
- `managed_process.go`：bundle 准备、托管进程启动与就绪等待。
- `runtime_status.go`：Health、计数器和错误状态。

现有 `bundle.go`、`grpc_events.go` 和 `supervisor.go` 保持其既有边界。

### sysarmorctl

`apps/cli/cmd/sysarmorctl/main.go` 只保留程序入口、一级命令路由、usage 和默认地址。其他职责拆为：

- `local_agent.go`：本地 Agent gRPC 查询与流式调用。
- `watch.go`：Event/Signal watch 输出和事件引用展开。
- `payload.go`：Policy、Collection 和 Content payload 构造。
- `flags.go`：参数读取、超时和请求上下文。
- `manager.go`：Manager 一级领域路由。
- `manager_agents.go`、`manager_policies.go`、`manager_control.go`、`manager_artifacts.go`：各领域请求构造。
- `http.go`：HTTP GET、JSON、multipart、raw body 和鉴权 Header。

CLI 命令、参数、输出 JSON、退出码、默认值和环境变量保持不变。

## 重构后的目标目录

下列目录树是本轮治理完成后的完整目标结构。`*_test.go` 继续与被测职责同 package 放置；本轮以移动生产声明为主，不为了让测试文件与生产文件一一对应而机械拆分现有测试。

```text
apps/
├── agent/
│   ├── cmd/
│   │   ├── sysarmor-agent/
│   │   └── sysarmor-content-sign/
│   └── internal/
│       ├── config/
│       ├── content/
│       ├── daemon/
│       │   ├── daemon.go                         # Runtime 类型、构造和主生命周期
│       │   ├── health.go                         # 启动、运行和关闭健康状态
│       │   ├── runtime_state.go                  # 并发运行状态访问
│       │   ├── detection_runtime.go              # Detection 构建与应用
│       │   ├── data_batch.go                     # DataBatch、标签、序列和 sender
│       │   ├── managed_network.go                # Manager TLS 与 managed network
│       │   ├── response_runtime.go               # Response 与 Evidence pullback 执行
│       │   ├── sensor_factory.go                 # Sensor 与重启策略构造
│       │   ├── local_control.go                  # Unix gRPC server 启动与装配
│       │   ├── local_control_health.go           # Health、Capability、DebugProfile
│       │   ├── local_control_policy.go           # Policy 查询与应用
│       │   ├── local_control_content.go          # Content 事务与查询
│       │   ├── local_control_watch.go            # Event/Signal 查询、watch 与过滤
│       │   ├── local_control_status.go           # Store 与 management lifecycle 状态
│       │   ├── local_control_messages.go         # gRPC message 与 ack 转换
│       │   ├── local_control_validation.go       # 请求与 telemetry policy 校验
│       │   ├── control_channel.go
│       │   ├── control_frame_convert.go
│       │   ├── endpoint_policy_control.go
│       │   ├── endpoint_runtime.go
│       │   ├── enrollment_client.go
│       │   ├── enrollment_control.go
│       │   ├── enrollment_coordinator.go
│       │   ├── enrollment_revocation_client.go
│       │   ├── export_pipeline.go
│       │   ├── exporter.go
│       │   ├── local_runtime.go
│       │   ├── network_supervisor.go
│       │   ├── policy_observation.go
│       │   ├── policy_reconciler.go
│       │   ├── runtime_identity.go
│       │   ├── startup_content.go
│       │   ├── startup_policy.go
│       │   ├── transport_runtime.go
│       │   ├── transport_runtime_component.go
│       │   ├── unenrollment_completion_client.go
│       │   ├── unenrollment_completion_reporter.go
│       │   └── *_test.go
│       ├── endpoint/
│       ├── localstore/
│       ├── policy/
│       ├── sensors/
│       │   ├── fake/
│       │   ├── runtime/
│       │   └── linux/tetragon/
│       │       ├── backend.go                    # Backend 与核心 Sensor 接口
│       │       ├── capability.go                 # 能力与 selector 编译报告
│       │       ├── collection_intent.go          # Scope、intent 与过滤
│       │       ├── tracing_policy.go             # TracingPolicy 生命周期
│       │       ├── event_source.go               # CLI/gRPC 事件源
│       │       ├── managed_process.go            # Bundle 与托管进程生命周期
│       │       ├── runtime_status.go              # Health、计数器与错误状态
│       │       ├── adapter.go
│       │       ├── bundle.go
│       │       ├── grpc_events.go
│       │       ├── process_group_linux.go
│       │       ├── process_group_other.go
│       │       ├── scope_identity.go
│       │       ├── supervisor.go
│       │       └── *_test.go
│       ├── tamper/
│       └── telemetry/
├── manager/
│   ├── cmd/
│   │   ├── sysarmor-gateway/
│   │   ├── sysarmor-manager/
│   │   └── sysarmor-worker/
│   ├── integration/
│   └── internal/
│       ├── analytics/
│       ├── api/
│       ├── auth/
│       ├── distribution/
│       ├── gateway/
│       ├── ingest/
│       ├── platform/
│       └── store/
│           ├── store.go                          # Store 核心状态与生命周期
│           ├── models.go                         # State 与领域数据结构
│           ├── policy.go                         # Policy、Assignment 与 Audit
│           ├── response.go                       # Response 生命周期
│           ├── control_command.go                # Control Command 生命周期
│           ├── evidence.go                       # Evidence pullback 生命周期
│           ├── agent_state.go                    # Agent、Health 与 Session
│           ├── enrollment.go                     # Enrollment 状态
│           ├── artifact.go                       # Artifact 与 Channel
│           ├── certificate.go                    # Agent Certificate 与吊销
│           ├── telemetry_state.go                # Event、Signal、Incident 与 Metrics
│           ├── persistence.go                    # 文件 State 导入、导出与原子持久化
│           ├── keys.go                           # 稳定键、标签和排序 helper
│           ├── agent.go
│           ├── backend.go
│           ├── enrollment_bootstrap.go
│           ├── enrollment_issue.go
│           ├── policy_commit.go
│           ├── unenrollment.go
│           ├── backend/
│           ├── migrations/
│           ├── postgres/
│           │   ├── snapshot.go                   # Table Store 兼容入口
│           │   ├── backend.go                    # Backend、超时和事务
│           │   ├── projection.go                 # 完整 State projection 编排
│           │   ├── agent_state.go                # Agent、Health 与 Session SQL
│           │   ├── policy.go                     # Policy、Assignment 与 Audit SQL
│           │   ├── control.go                    # Response、Command 与 Evidence SQL
│           │   ├── enrollment.go                 # Enrollment SQL 与签发事务
│           │   ├── artifact.go                   # Artifact、Channel 与 Certificate SQL
│           │   ├── metrics.go                    # Metrics 与 Rarity SQL
│           │   ├── migrate.go
│           │   ├── unenrollment.go
│           │   └── *_test.go
│           └── *_test.go
├── cli/
│   └── cmd/sysarmorctl/
│       ├── main.go                               # 入口、一级路由、usage 与默认地址
│       ├── local_agent.go                        # 本地 Agent gRPC 客户端
│       ├── watch.go                              # Event/Signal 流输出
│       ├── payload.go                            # Policy 与 Content payload
│       ├── flags.go                              # 参数、超时和请求上下文
│       ├── manager.go                            # Manager 领域路由
│       ├── manager_agents.go                     # Agent、Health、Enrollment API
│       ├── manager_policies.go                   # Policy API
│       ├── manager_control.go                    # Response 与 Control Command API
│       ├── manager_artifacts.go                  # Artifact 与 Channel API
│       ├── http.go                               # HTTP transport 与鉴权 Header
│       ├── enrollment.go
│       └── *_test.go
└── console/                                      # 本轮不调整内部目录

packages/
├── README.md                                     # 共享包准入与依赖规则
├── contracts/
│   ├── controlmodel/
│   ├── health/
│   ├── proto/
│   └── schema/
├── eventmodel/
├── policy/
├── response/
├── sensor-sdk/
│   └── contract/
└── tlsconfig/

test/
└── contracts/
    └── test_monorepo_layout.py                   # 目录与 packages 依赖合同
```

该目录树表达职责归属，不表示所有同级目录都必须拆成独立 package。新文件仍使用其所在目录的原 package，共享同一内部状态；只有真实跨产品稳定契约才允许进入 `packages/`。

## packages 准入规则

新增 `packages/README.md`，定义以下准入条件：

1. 至少被两个产品边界消费，或明确承载跨产品稳定协议。
2. 表达稳定领域契约、纯模型或无产品归属的基础能力。
3. 不依赖 `apps/*`，不读取产品配置，不拥有产品生命周期。
4. API 足够小，兼容性和失败语义可独立测试。

禁止把单一应用的复用候选、临时 helper、数据库实现、进程编排或 UI 组件提前放入 `packages/`。单一消费者代码默认留在所属 `apps/*/internal`，出现真实第二消费者后再提取。

现有 `test/contracts/test_monorepo_layout.py` 增加治理文档存在性和依赖方向合同，但不通过自动化猜测“是否足够稳定”，该判断保留给代码评审。

## 验证策略

每次移动前先运行对应 package 测试建立绿色基线。移动完整声明后立即执行 `gofmt` 和同一测试，确认行为未漂移。纯文件移动不新增业务断言；结构治理规则按 TDD 先增加失败的合同测试，再补文档或规则使其通过。

最终验证包括：

- `go test -race ./... -count=1`
- Agent、Manager、CLI 二进制构建
- Console 测试、lint 和生产构建
- monorepo、topology、控制面和发行 workflow 合同
- 本地 distribution 测试与适用的 E2E 测试
- 修改过的 Shell 脚本语法检查
- `gofmt -l apps packages`
- `git diff --check`
- 六个目标文件及新文件的职责和规模复核

## 风险与控制

- **移动遗漏**：以完整 Go declaration 为单位移动，并在每个领域后编译测试。
- **初始化或方法解析变化**：保持 package、标识符和声明内容不变，不使用 `init` 重排。
- **事务漂移**：PostgreSQL 事务闭包整体移动，不拆事务内部步骤。
- **锁语义漂移**：Store 和 AgentRuntime 的加锁代码与被保护操作整体移动。
- **CLI 输出漂移**：复用现有测试，并对命令路由与 JSON 输出运行定向测试。
- **过度碎片化**：紧密协作的短 helper 跟随主职责，不建立只有一两个微型函数的文件。

## 验收标准

- 六个目标文件显著缩小，剩余内容具有单一、可解释的职责。
- 新文件名称能够表达领域或生命周期边界，不按任意行号命名。
- 导出 API、包依赖方向和运行行为不变。
- `packages/` 准入规则可见，`packages/* -> apps/*` 依赖由合同测试阻止。
- 全量验证通过，工作树中没有测试生成物。
