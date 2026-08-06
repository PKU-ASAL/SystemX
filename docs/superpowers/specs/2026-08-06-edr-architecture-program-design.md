# SysArmor EDR Architecture Program Design

## 目的与结论

SysArmor 已完成产品 monorepo 迁移、核心大文件拆分以及 Agent `control`、`localapi`、`remoteapi` 第一阶段边界建立。下一阶段不再以移动目录或减少行数为目标，而是让端点可信运行、控制状态机、安装事务和调查证据形成可验证的生产闭环。

整体工作拆成四个独立交付单元：当前治理分支收口、Agent 应用边界收口、安装与运行可靠性、生产 EDR 控制与调查能力。每个单元从最新 `dev` 建立独立分支，通过独立设计、TDD、代码审查和 E2E 后合回 `dev`。当前 `refactor/product-monorepo-layout` 只完成第一个单元，不继续承载新产品能力。

## 当前坐标

```text
产品目录迁移              已完成
核心大文件结构拆分        已完成
Local/Remote 传输分离     已完成
控制协议字段保真          已完成
Daemon composition root   部分完成
安装事务与升级可靠性      待收口
四层 Policy 生产闭环      部分完成
真实 Response             待实现
真实 Evidence pullback    待实现
动态采集与自动恢复        目标能力
```

这里的“部分完成”表示基础路径和安全不变量已经存在，但尚未满足对应阶段的全部退出标准，不使用主观完成百分比。

## 系统 Big Map

```text
                               SysArmor
                                  │
                 ┌────────────────┴────────────────┐
                 │                                 │
          Endpoint / Agent                  Manager Platform
                 │                                 │
      ┌──────────┼──────────┐          ┌───────────┼──────────┐
      │          │          │          │           │          │
 standalone   managed    Sensor      Gateway    Manager    Console
      │          │     Tetragon/未来     │           │
      └──────────┴──────────┬────────────┘           │
                            │                        │
                    Endpoint Policy          PostgreSQL 控制面
          collection / detection / telemetry / response
                            │
                    Event -> Signal
                            │
                   本地有界存储与上传
                            │
                          Kafka
                            │
                          Worker
                            │
             OpenSearch / Cloud Signal / Incident
```

### 端侧控制边界

```text
sysarmorctl ──Unix socket──> localapi ──┐
                                       ├──> control ──> runtime ports
Manager ─────mTLS stream──> remoteapi ──┘                  │
                                                          ├── policy/content
                                                          ├── localstore
                                                          ├── sensor runtime
                                                          ├── detection/telemetry
                                                          └── enrollment/response

daemon = 配置 + 依赖装配 + 启停顺序 + 健康聚合
```

`localapi` 与 `remoteapi` 是平级传输适配器，不能互相调用。`control` 表达传输无关的命令、状态机和结果。`daemon` 只应管理进程生命周期，不应成为 Policy、Content、Enrollment 或 Response 的业务实现容器。

## 架构不变量

### 运行模式

- standalone 与 managed 使用同一采集、检测、存储和控制核心。
- standalone 允许经 Local API 修改策略；managed 的本地写必须由 authority 检查拒绝。
- standalone 接入 Manager 后，Manager 默认策略先持久化为 pending；Sensor 和运行时成功后才激活。
- managed 退管必须先由 Manager 授权并完成证书吊销，Agent 才恢复 standalone 策略和凭据状态。

### 控制与数据

- Policy 必须先解析、展开引用、校验和编译，成功后才能替换最后有效版本。
- 未持久化的控制写入不能返回成功；未可靠确认的数据上传不能推进 checkpoint。
- Event、Signal、Evidence 和 Incident 的作用域必须绑定 tenant、Agent 和策略版本。
- 数据丢弃、降级、引用缺口、策略失败和恢复失败必须显式进入 Ack、Health、Metrics 或 Audit。

### Sensor 与安装

- Agent 继续拥有统一 Sensor 控制模型，Tetragon 只是第一个生产 backend，不拆成独立产品。
- 安装器负责包校验、staging、原子切换和回滚；Agent 负责运行时 Sensor 生命周期和能力协商。
- Tetragon bundle 版本、哈希和平台兼容性必须可验证；开发 bundle 模式不作为生产支持形态。

### 共享代码

- `packages/` 只接收跨产品、稳定、无 `apps/` 实现依赖的契约和能力。
- 不引入通用依赖注入框架、第二套控制模型或仅为目录对称而存在的抽象。
- 文件大小是职责混杂信号，不是机械门禁；生产文件和测试文件都按行为语义拆分。

## 分阶段交付

### P0：当前治理分支收口

**目标：** 将 monorepo、核心文件治理和 Agent 第一阶段边界作为一个可独立审查成果交付。

**范围：** 同步两个远端 `dev` 基线、检查文档和路径漂移、运行全量验证、独立审查、发布功能分支并向 `dev` 创建 PR。

**退出标准：**

- `origin/dev` 与 `github/dev` 基线一致，或者差异已明确处理；
- 工作树无生成物，分支只包含计划内的原子提交；
- 全仓 race、构建、Console、架构合同和适用 E2E 通过；
- 无未修复 Critical 或 Important 审查问题；
- 不直接提交或推送 `dev`、`main`。

### P1-A：Agent 应用边界收口

**目标：** 让 daemon 成为真正的 composition root，同时保持所有现有 authority、pending 和退管语义。

**顺序：**

1. 按行为拆分 `daemon_test.go` 与 `local_control_test.go`；
2. 用 Response 验证最小 control use-case 模式；
3. 迁移 Content、Enrollment，最后迁移复杂的 Policy；
4. 将远程 session、resume、重试和报告生命周期收敛到 `remoteapi`；
5. 删除 daemon 中迁移后失去职责的适配代码。

**退出标准：** daemon 只保留配置、装配、启动、停止和健康聚合；control 不依赖传输层；真实 Local/Remote E2E 语义不变。

### P1-B：安装与运行可靠性

**目标：** 让生产 Agent 安装、升级和回滚接近普通 Linux 系统软件的确定性体验。

**范围：** 包来源验证、Tetragon staging、原子切换、systemd 权限和 watchdog、失败回滚、升级与卸载状态策略。

**退出标准：** 首次安装、重复安装、升级、失败回滚和卸载均有真实 VM 测试；不支持的平台明确失败或降级；任何失败不留下半安装 Sensor。

### P1-C：生产控制闭环

**目标：** 使四层 Endpoint Policy 和至少一个真实响应动作具备版本、确认、审计和恢复语义。

**范围：** Policy 版本单调性、生效期限、回滚、Manager/Agent 有效状态一致性、Response action/capability 契约、真实可恢复响应动作。

**退出标准：** 旧版本不能覆盖新版本；失败保持最后有效策略；响应具备能力检查、幂等、超时、审计和恢复验证。

### P2：调查证据与效能闭环

**目标：** 让 Incident 能稳定下钻到真实依据，并在资源预算内按风险调整采集和传输。

**范围：** Cloud Signal lineage、真实 Evidence pullback、Telemetry 选择与优先级、资源预算、临时加深采集和自动恢复。

**退出标准：** 代表性攻击场景可从 Incident 回溯到 Signal、Event、有效策略和可用原始材料；所有数据缺口和资源降级可查询。

## 分支与提交策略

```text
dev
 ├── refactor/product-monorepo-layout       P0 当前分支
 ├── refactor/agent-application-boundaries  P1-A
 ├── fix/agent-install-reliability          P1-B
 ├── feat/production-control-loop           P1-C
 └── feat/evidence-efficiency-loop          P2 按更小能力继续拆分
```

- 每个分支从当时最新且两个远端一致的 `dev` 创建。
- 每个 Commit 只解决一个关注点，遵循 Conventional Commits。
- 功能分支通过 PR 合回 `dev`；版本发布前再由 `dev` 合入 `main`。
- 禁止直接提交到 `dev`、`main`，禁止向 `main` force push。

## 验证矩阵

| 层次 | 必需验证 |
|---|---|
| 静态边界 | `test/contracts/test_monorepo_layout.py`、`gofmt`、`git diff --check` |
| Go 单元与并发 | 受影响包测试、`go test -race ./... -count=1` |
| 构建 | `make build-binary`、`make build-agent-tools`、Console test/lint/build |
| 控制闭环 | Local socket、Remote Manager、Store status、managed authority E2E |
| 安装运行 | systemd、真实 Tetragon、升级、回滚、退管 VM E2E |
| 安全审查 | authority、证书、tenant/scope、fail-closed、持久化确认、生成物检查 |

## 风险控制

- P0 分支已经包含较多结构提交，不再追加产品功能，降低审查和回滚成本。
- Control 迁移按领域逐个完成，不先设计覆盖所有运行时的“大接口”。
- 安装器不接管 Sensor 运行时控制，避免形成第二套生命周期状态机。
- 真实 Response 在具备 capability、审批、审计和恢复前不默认启用 enforce。
- P2 不把控制通道占位结果描述为真实 Evidence，不把相关路径描述为因果攻击路径。
