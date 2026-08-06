# Core File Governance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不改变外部合同和运行行为的前提下，完成核心大文件治理、Agent 模块边界重构和共享包准入治理。

**Architecture:** 工作拆为两个可独立验证的阶段。阶段一完成低风险的目录迁移以及 Manager Store、PostgreSQL、Tetragon、CLI 职责拆分；阶段二建立 Agent `control`、`localapi`、`remoteapi` 边界并完成全量回归。

**Tech Stack:** Go 1.24、gRPC/protobuf、SQLite、PostgreSQL、Python unittest、pnpm/Next.js、Shell 合同与 E2E 测试。

## Global Constraints

- 不改变外部 API、protobuf、持久化 schema、二进制名称、CLI 参数、JSON 输出或部署拓扑。
- 不增加通用依赖注入框架、新 Go module、新前端 workspace 或第三方依赖。
- `control` 禁止依赖 `localapi`、`remoteapi`；两个 API 适配器禁止互相依赖。
- managed 与 standalone 复用同一控制逻辑；managed 模式本地写策略仍应被拒绝。
- PostgreSQL 事务、Store 锁、Daemon 启停和 Tetragon 进程生命周期整体迁移。
- 文件拆分尊重语义内聚，500 行不是硬门禁。
- 每项任务形成一个 Conventional Commit，且提交前对应测试保持绿色。

## Execution Order

1. 执行 [Structural Splits Plan](2026-08-05-core-file-structural-splits.md)：`packages/` 治理、`endpoint` 目录迁移、Manager Store、PostgreSQL、Tetragon 和 CLI 拆分。
2. 确认阶段一全量 Go 测试通过。
3. 执行 [Agent Control Boundaries Plan](2026-08-05-agent-control-boundaries.md)：Control 核心、Local API、Remote API、Daemon 收口和全量产品回归。
4. 使用 `requesting-code-review` 对从 `c4b5c1ad` 到最终 HEAD 的全部改动做独立审查。

## Final Acceptance

- 六个原始超大文件显著缩小并保持语义内聚。
- `apps/agent/internal/endpoint` 不再存在，数据路径为 `sensor -> event -> detection -> telemetry`。
- `localapi -> control`、`remoteapi -> control`，不存在反向或交叉依赖。
- Remote API 不再构造或调用 Local API Server。
- `packages/README.md` 和结构合同共同约束共享包准入与依赖方向。
- Go、Console、构建、distribution、控制面合同和适用 E2E 全部通过。
- 工作树无测试生成物，不创建 PR，不主动推送。
