# Core File Governance Design

## 目的与结论

本轮治理在 `refactor/product-monorepo-layout` 分支内完成，不创建 PR，不改变 SysArmor 的外部 API、协议、持久化格式、部署拓扑或运行行为。Agent 内部 package 路径按职责调整，但不改变可执行文件名称或用户接口。

六个超大生产文件按领域职责治理：Manager Store、PostgreSQL Store、Tetragon Backend 和 CLI 在现有 package 内拆分；Agent 控制路径建立明确的 package 边界。拆分优先保持语义内聚，500 行仅作为识别职责混杂的提示，不作为机械门禁。`packages/` 增加准入规则和自动化依赖边界检查；当前没有足够的维护团队，因此不增加 `CODEOWNERS`。

## 约束

- 保持外部 API 和跨产品共享类型、函数、方法签名不变；Agent 内部允许增加最小能力接口。
- 保持跨产品共享 package 和外部接口路径不变；允许调整 `apps/agent/internal` 内部 package 路径。
- 不引入通用接口层、依赖注入框架、Go module 或前端 workspace；只为 `control` 与两个 API 适配器建立最小能力接口。
- 不顺带修复或重写业务逻辑；发现独立缺陷时记录并另行处理。
- 只移动完整声明及其紧密辅助逻辑，不把同一事务或状态机机械切开。
- 每个拆分关注点形成可独立验证和回退的原子提交。

## 方案选择

Manager Store、PostgreSQL Store、Tetragon Backend 和 CLI 采用同 package 内按职责拆文件。Agent 控制路径采用核心控制逻辑与传输适配器分离的 package 边界。

Manager Store 不直接拆成多个子 package，因为它仍共享锁、状态和事务上下文。Agent 控制路径已经存在远程通道复用本地 Server 实现的反向边界，因此需要建立 `control`、`localapi`、`remoteapi` 三个平级 package，并通过窄能力依赖解除传输与业务逻辑耦合。

未选择按行数切分，因为文件边界必须表达变化原因和领域语义，而不是满足任意行数。

## 文件职责设计

### Manager Store

`apps/manager/internal/store/store.go` 最终只保留 Store 核心状态、打开与 backend 绑定等生命周期逻辑。现有声明按中等粒度领域移动：

- `models.go`：State 与通用领域结构。
- `policy.go`：Policy、Assignment 和 Audit。
- `control.go`：Response、Control Command 和 Evidence pullback。
- `agent.go`：Agent 身份、会话和健康状态。
- `enrollment.go`：Enrollment、Certificate 和吊销。
- `artifact.go`：Artifact 与 Channel。
- `telemetry.go`：Event、Signal、Incident、派生投影和 Metrics。
- `persistence.go`：文件 State 导入、导出、稳定键和原子持久化。

已有 `agent.go`、`policy_commit.go`、`enrollment_bootstrap.go`、`enrollment_issue.go` 和 `unenrollment.go` 的职责保持不变，避免重复抽象。

### PostgreSQL Store

`apps/manager/internal/store/postgres/snapshot.go` 保留 table backend 的构造、事务辅助和完整 State projection 入口，具体 SQL 按领域拆分：

- `policy.go`：Policy、Assignment 和 Audit。
- `control.go`：Response、Control Command 和 Evidence pullback。
- `identity.go`：Agent、Health、Session、Enrollment 和 Certificate。
- `artifact.go`：Artifact 与 Channel。
- `telemetry.go`：Metrics、Rarity baseline 及其 projection。

SQL 语句、事务范围、锁语义和错误返回保持原样。

### Agent Daemon

`apps/agent/internal/daemon` 是 composition root，只保留 `AgentRuntime`、构造、启动、停止、健康聚合和依赖装配。它创建 `control` 控制器、`localapi` Server 和 `remoteapi` Session，不承载 Policy、Content、Response 或 Enrollment 业务规则。

`Run` 的阶段顺序、清理顺序和失败报告路径保持不变。

### Agent Control

`apps/agent/internal/control` 承载本地和远程入口共享的控制用例：

- `types.go`：内部命令、结果和能力依赖。
- `policy.go`：Policy 准备、应用、持久化、激活和回滚。
- `content.go`：Detection Content 更新事务。
- `response.go`：Response 与 Evidence pullback 执行。
- `enrollment.go`：注册、退管和 Policy authority 切换。
- `status.go`：Policy、Health、Capability 和 management lifecycle 快照。

`control` 可以依赖 `policy`、`content`、`detection`、`sensors`、`telemetry` 和 `localstore`，禁止依赖 `localapi` 或 `remoteapi`。Policy 与 Content 控制流程不接收 gRPC stream 或 Server 类型。

### Agent Local API

`apps/agent/internal/localapi` 是本机 Unix socket gRPC 适配器：

- `server.go`：Server 生命周期与 RPC 注册。
- `handlers.go`：请求校验并调用 `control` 能力。
- `watch.go`：Event/Signal 本地查询与流式 Watch。
- `codec.go`：protobuf 与内部命令、结果的转换。

Local API 在 managed 模式下仍保持可用；写操作是否允许由 `control` 的 Policy authority 判断，适配器不复制该规则。

### Agent Remote API

`apps/agent/internal/remoteapi` 是 Agent 到 Manager 的 mTLS 长连接适配器：

- `client.go`：连接、身份和传输配置。
- `session.go`：长连接、重连、resume 与生命周期。
- `commands.go`：远程命令分发到 `control`。
- `reports.go`：Health、Capability、Ack 和完成状态上报。
- `codec.go`：ControlFrame 与内部命令、结果的转换。

`remoteapi` 禁止构造或调用 `localapi` Server。`localapi` 与 `remoteapi` 只能平行依赖 `control`，不能互相依赖。

### Agent Event、Detection 与 Telemetry

取消 `apps/agent/internal/endpoint` 这一冗余中间层：

- `event/context`：进程上下文、lineage 和实体关联状态。
- `event/normalize`：Sensor Event 到 Canonical Event 的规范化。
- `detection`：端点 Detection Engine、规则编译与关联。
- `detection/matcher`：Detection 条件匹配。
- `telemetry/dataappend`：DataBatch 构造、发送和确认。
- `telemetry/ringbuffer`：本地近期 Event/Signal 内存缓冲。

Agent 已经表达 endpoint 产品边界，因此不再用 `endpoint` 目录重复表达相同概念。数据方向固定为 `sensor -> event -> detection -> telemetry`。

### Tetragon Backend

`apps/agent/internal/sensors/linux/tetragon/backend.go` 保留 `Backend`、构造和核心 Sensor 接口。其他职责按中等粒度拆为：

- `capability.go`：Collection 能力、编译报告、scope 和 selector。
- `tracing_policy.go`：TracingPolicy 生成、写入、应用、校验和删除。
- `runtime.go`：bundle 准备、托管进程、事件源、Health 和计数器。

现有 `bundle.go`、`grpc_events.go` 和 `supervisor.go` 保持其既有边界。

### sysarmorctl

`apps/cli/cmd/sysarmorctl/main.go` 只保留程序入口、一级命令路由、usage 和默认地址。其他职责拆为：

- `local.go`：本地 Agent gRPC 查询、Watch 和事件引用展开。
- `payload.go`：Policy、Collection、Content payload 和参数解析。
- `manager.go`：Manager 通用查询与领域路由。
- `manager_policy.go`、`manager_control.go`、`manager_artifact.go`：领域请求构造。
- `http.go`：HTTP GET、JSON、multipart、raw body 和鉴权 Header。

CLI 命令、参数、输出 JSON、退出码、默认值和环境变量保持不变。

## 重构后的目标目录

下列目录树是本轮治理完成后的目标结构。`*_test.go` 与被测 package 放置，但不为了让测试文件与生产文件一一对应而机械拆分现有测试。

```text
apps/
├── agent/
│   ├── cmd/
│   │   ├── sysarmor-agent/
│   │   └── sysarmor-content-sign/
│   └── internal/
│       ├── daemon/
│       │   ├── daemon.go                         # Runtime、构造和依赖装配
│       │   ├── startup.go                        # 启动顺序
│       │   ├── shutdown.go                       # Drain、停止和清理
│       │   ├── health.go                         # 整体健康聚合
│       │   └── *_test.go
│       ├── control/
│       │   ├── types.go                          # 内部命令、结果和依赖
│       │   ├── policy.go                         # Policy 控制闭环
│       │   ├── content.go                        # Content 更新事务
│       │   ├── response.go                       # Response 与 Evidence
│       │   ├── enrollment.go                     # 注册、退管和 authority
│       │   ├── status.go                         # Policy、Health、Capability 状态
│       │   └── *_test.go
│       ├── localapi/
│       │   ├── server.go                         # Unix socket gRPC Server
│       │   ├── handlers.go                       # 本地查询与控制 Handler
│       │   ├── watch.go                          # Event/Signal Watch
│       │   ├── codec.go                          # protobuf 转换与校验
│       │   └── *_test.go
│       ├── remoteapi/
│       │   ├── client.go                         # Manager mTLS Client
│       │   ├── session.go                        # 长连接、重连和 resume
│       │   ├── commands.go                       # 远程命令分发
│       │   ├── reports.go                        # Health、Capability 和 Ack
│       │   ├── codec.go                          # ControlFrame 转换
│       │   └── *_test.go
│       ├── config/
│       ├── content/                              # 签名 Security Content
│       ├── event/
│       │   ├── context/                          # 进程上下文与 lineage
│       │   └── normalize/                        # Sensor Event 规范化
│       ├── detection/
│       │   ├── matcher/                          # 条件匹配
│       │   ├── engine.go                         # Detection Engine
│       │   ├── compiled.go
│       │   ├── condition_tree.go
│       │   ├── correlate.go
│       │   ├── validation.go
│       │   └── *_test.go
│       ├── localstore/                           # SQLite、Event segment 与 checkpoint
│       ├── policy/                               # Policy 模型解析与编译
│       ├── sensors/
│       │   ├── fake/
│       │   ├── runtime/
│       │   └── linux/tetragon/
│       │       ├── backend.go                    # Sensor 接口与 Backend 状态
│       │       ├── capability.go                 # Collection 能力与编译报告
│       │       ├── tracing_policy.go             # TracingPolicy 生命周期
│       │       ├── runtime.go                    # 进程、事件源和运行状态
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
│           ├── dataappend/                       # DataBatch、发送与确认
│           ├── ringbuffer/                       # 近期数据内存缓冲
│           ├── batch.go                          # 批次身份、序列与标签
│           ├── batcher.go
│           ├── bus.go
│           ├── exporter.go                       # 本地与远程 BatchSender
│           ├── pipeline.go                       # 端点数据管线编排
│           └── *_test.go
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
│           ├── policy.go                         # Policy、Assignment 和 Audit
│           ├── control.go                        # Response、Command 和 Evidence
│           ├── agent.go                          # Agent、Health 和 Session
│           ├── enrollment.go                     # Enrollment 和 Certificate
│           ├── artifact.go                       # Artifact 与 Channel
│           ├── telemetry.go                      # Event、Signal、Incident 和 Metrics
│           ├── persistence.go                    # 文件 State 导入、导出与原子持久化
│           ├── backend.go
│           ├── enrollment_bootstrap.go
│           ├── enrollment_issue.go
│           ├── policy_commit.go
│           ├── unenrollment.go
│           ├── backend/
│           ├── migrations/
│           ├── postgres/
│           │   ├── snapshot.go                   # Table Store 兼容入口
│           │   ├── policy.go                     # Policy、Assignment 和 Audit SQL
│           │   ├── control.go                    # Response、Command 和 Evidence SQL
│           │   ├── identity.go                   # Agent、Enrollment 和 Certificate SQL
│           │   ├── artifact.go                   # Artifact 与 Channel SQL
│           │   ├── telemetry.go                  # Telemetry、Metrics 和 Rarity SQL
│           │   ├── migrate.go
│           │   ├── unenrollment.go
│           │   └── *_test.go
│           └── *_test.go
├── cli/
│   └── cmd/sysarmorctl/
│       ├── main.go                               # 入口、一级路由、usage 与默认地址
│       ├── local.go                              # 本地 Agent 调用与 Watch
│       ├── payload.go                            # Policy、Content 与参数解析
│       ├── manager.go                            # Manager 通用查询
│       ├── manager_policy.go                     # Policy API
│       ├── manager_control.go                    # Response 与 Command API
│       ├── manager_artifact.go                   # Artifact 与 Channel API
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

该目录树表达职责归属。`control`、`localapi` 和 `remoteapi` 是独立 package；Manager Store、PostgreSQL Store、Tetragon 和 CLI 的职责拆分仍在各自现有 package 内完成。只有真实跨产品稳定契约才允许进入 `packages/`。

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
- `find apps packages -path '*/node_modules' -prune -o -type f -name '*.go' -exec gofmt -l {} +`
- `git diff --check`
- 六个目标文件及新文件的职责和规模复核

## 风险与控制

- **移动遗漏**：以完整 Go declaration 为单位移动，并在每个领域后编译测试。
- **初始化或方法解析变化**：保持初始化顺序和运行语义，不使用 `init` 重排；内部方法迁移由定向合同测试保护。
- **事务漂移**：PostgreSQL 事务闭包整体移动，不拆事务内部步骤。
- **锁语义漂移**：Store 和 AgentRuntime 的加锁代码与被保护操作整体移动。
- **CLI 输出漂移**：复用现有测试，并对命令路由与 JSON 输出运行定向测试。
- **过度碎片化**：紧密协作的短 helper 跟随主职责，不建立只有一两个微型函数的文件。

## 验收标准

- 六个目标文件显著缩小，剩余内容具有单一、可解释的职责。
- 新文件名称能够表达领域或生命周期边界，不按任意行号命名。
- 外部 API 和运行行为不变；内部包依赖方向符合设计合同。
- `packages/` 准入规则可见，`packages/* -> apps/*` 依赖由合同测试阻止。
- 全量验证通过，工作树中没有测试生成物。
