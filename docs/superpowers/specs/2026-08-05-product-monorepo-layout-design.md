# Product Monorepo Layout Design

## 目的与结论

将仓库从以 Go 技术目录为主的布局调整为以 SysArmor 产品边界为主的 monorepo。迁移后，独立交付单元位于 `apps/`，跨产品稳定能力位于 `packages/`，部署、测试、工具和文档继续保持顶层独立。

本次迁移保持单一 `go.mod`，不改变运行行为、协议兼容性、发布物名称或部署拓扑。

## 目标结构

```text
apps/
  agent/
    cmd/
      sysarmor-agent/
      sysarmor-content-sign/
    internal/
      config/
      content/
      daemon/
      endpoint/
      health/
      localstore/
      policy/
      sensors/
      tamper/
      telemetry/

  manager/
    cmd/
      sysarmor-manager/
      sysarmor-gateway/
      sysarmor-worker/
    internal/
      analytics/
      api/
      auth/
      distribution/
      gateway/
      ingest/
      platform/
      store/

  cli/
    cmd/sysarmorctl/

  console/

packages/
  contracts/
    controlmodel/
    health/
    proto/
    schema/
  eventmodel/
  policy/
  response/
  sensor-sdk/
    contract/
  tlsconfig/

deployments/
test/
tools/
docs/
```

`packages/tlsconfig` 是对初始目录草案的必要补充：Agent 与 Gateway 都依赖同一套 mTLS 语义，它既不属于单一产品，也不是 wire contract。

## 产品边界

### Agent

`apps/agent` 是端侧 EDR 产品，包含 standalone 与 managed 两种模式共享的运行时、本地 Store、Endpoint Detection、Sensor 管理、内容、策略和自保护。`sysarmor-content-sign` 服务于 Agent 内容包，因此归 Agent 所有。

### Manager

`apps/manager` 是中心控制面产品。Manager API、Gateway、Worker 是独立进程，但共享控制面 Store、分析、分发和基础设施适配器，因此保留独立 `cmd`，共享同一产品内部实现。

### CLI

`apps/cli` 是独立交付单元。`sysarmorctl` 同时包含本地 Agent 控制和远程 Manager 管理，不能归入任一产品内部。此次仅迁移目录；其 2286 行入口文件在后续独立重构中按 `agent`、`manager`、`output` 拆分。

### Console

`apps/console` 是 Manager Web UI。当前只有一个前端应用，继续使用自身的 pnpm lockfile，不引入无收益的多包 workspace。

## 共享包准入

共享代码只有同时满足以下条件才能进入 `packages/`：

1. 至少两个交付单元真实依赖，或它是明确的跨进程契约。
2. 不依赖 `apps/*`。
3. 接口稳定、职责单一且可独立测试。
4. 不包含某个产品的生命周期编排或持久化实现。

`sensor-sdk` 当前只公开 contract 子包，不把 Tetragon、fake sensor 或 runtime 实现提升为共享 API。

## 依赖方向

```text
apps/* -> packages/*
apps/agent -X-> apps/manager
apps/manager -X-> apps/agent
apps/cli -X-> apps/agent/internal
apps/cli -X-> apps/manager/internal
packages/* -X-> apps/*
```

Manager 集成测试若需要访问 Manager internal 包，必须位于 `apps/manager` 子树内，以遵守 Go `internal` 可见性规则。跨产品测试继续通过公开协议或进程边界验证。

## 迁移策略

迁移分为五个可独立验证的批次：

1. 建立 monorepo 架构合同和入口目录。
2. 迁移共享 contracts 与领域包。
3. 迁移 Agent 内部实现。
4. 迁移 Manager 内部实现及其白盒集成测试。
5. 迁移 Console，更新部署、测试、CI 与文档路径，并删除旧顶层目录。

每批只做目录移动、导入路径和构建路径修复，不改变业务逻辑。每批必须通过目标包测试；最终必须通过 `go test ./...`、前端构建、分发合同和 topology 合同。

## 风险控制

- 使用 `git mv` 保留历史可追踪性。
- 不拆 Go Module，不引入 `go.work`。
- 不在目录迁移中顺手重构超大文件。
- Proto 源路径、Go package 路径和生成代码必须原子迁移。
- Dockerfile、Makefile、发布工作流、安装脚本和测试中的路径必须由合同测试覆盖。
- 架构合同阻止 `packages -> apps` 和 `app -> other app` 的反向依赖。

## 验收标准

- `cmd/`、`api/`、`internal/`、`web/` 旧顶层产品代码目录不再存在。
- 四个应用和共享包均位于目标目录。
- 所有 Go import、Proto import、构建、Docker、发布和文档路径更新完成。
- 生产二进制名称和发布包内容不变。
- 单元、race、分发合同、前端构建和关键 E2E 合同通过。
