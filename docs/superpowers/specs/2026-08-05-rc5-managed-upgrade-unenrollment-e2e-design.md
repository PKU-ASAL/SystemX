# rc.5 Managed 状态升级退管 E2E 设计

## 目的与结论

本设计补齐 `v0.1.0-rc.5` managed Agent 持久状态升级到当前版本后，通过真实 Gateway mTLS
完成 legacy 退管的端到端验证。测试必须证明 schema v1 状态迁移、legacy 身份补全、Manager
证书吊销、`unknown_legacy` 投影、managed 凭据清理和 standalone 重启恢复能够组成一个完整闭环。

采用以下边界：

- 兼容基线固定为 tag `v0.1.0-rc.5`，commit
  `454b69d6c01f778add5836e0af1c9ba3299fd5b1`。
- 仓库保存从该版本 schema v1 状态导出的可审查 SQL fixture 和来源 manifest，不保存私钥。
- 测试运行时使用现有 Manager enrollment 生成临时证书，再把 fixture 副本绑定到临时身份。
- Manager 证书投影在测试环境中显式转换为旧版本没有协议字段的 legacy 记录。
- 当前 Agent 启动后执行真实 v1 到 v5 migration，并通过真实 Gateway mTLS 调用完成退管。
- 所有测试辅助写入仅存在于 `test/` 和隔离 VM，不增加生产 API 或修改生产状态机。

该测试验证的是公开旧版本的持久状态升级，而不是每次 CI 都重新构建并启动整套旧平台。测试名称和
结果说明必须使用“managed state upgrade”，避免把覆盖范围误写成 Manager、Gateway 和 Agent 二进制的
全平台滚动升级。

## 第一性原则

旧版本升级兼容必须回答四个问题：

1. 新 Agent 是否能无损理解旧 Agent 留下的权威本地状态。
2. 旧状态缺少新协议身份字段时，系统是否只从可信 mTLS peer 和 Manager 证书记录补全。
3. Manager 是否把 legacy 退管记录为无法提供 endpoint completion 的 `unknown_legacy`，而不是伪造
   `endpoint_completed`。
4. Agent 是否只有在 Manager 完成证书吊销后才删除 managed 凭据并切换 standalone。

单元测试分别证明迁移函数和协议分支，但不能证明它们通过真实 SQLite、证书、Gateway、Manager Store
和 systemd 组合后仍满足以上约束。因此需要一条部署形态的 E2E。

## 方案选择

不在常规测试中构建和启动完整 `rc.5` 平台。该方案真实性更高，但会引入旧依赖构建、双平台编排和
更大的 CI 故障面，超出本次仅补齐 legacy 状态升级闭环的目标。

也不在 E2E 脚本中临时手写 schema v1。临时 SQL 无来源约束，容易在后续维护中偏离实际发布版本。

采用受版本控制的 golden fixture：保存 `rc.5` schema v1 managed SQLite 的规范 SQL dump，并通过
manifest 固定来源 tag、commit、schema version 和 fixture SHA-256。SQL dump 不包含 WAL、机器相关
时间戳或任何证书私钥，便于代码审查和稳定比较。

## Fixture 契约

fixture 目录为：

```text
test/fixtures/agent/upgrades/v0.1.0-rc.5/
  agent-managed-v1.sql
  manifest.json
  README.md
```

`agent-managed-v1.sql` 必须包含 `rc.5` 的完整 baseline tables，并满足：

- `schema_meta.version = 1`；
- enrollment state 为 `managed`；
- enrollment 仅包含 v1 字段，不预先加入 `enrollment_id`、`certificate_serial`、`manager_url` 或
  `unenrollment_protocol`；
- active policy 来自旧版单一 `policy` 表；
- 身份、路径和时间使用非敏感稳定占位值；
- 不包含证书、私钥、token 或生产地址。

`manifest.json` 固定以下字段：

- `format = sysarmor.agent-upgrade-fixture/v1`；
- `source_tag = v0.1.0-rc.5`；
- `source_commit = 454b69d6c01f778add5836e0af1c9ba3299fd5b1`；
- `schema_version = 1`；
- `fixture_file = agent-managed-v1.sql`；
- `sha256` 为 SQL 文件的小写十六进制 SHA-256。

`README.md` 说明来源、更新约束和测试覆盖边界。仓库级 contract test 读取 SQL 和 manifest，验证摘要、
schema 版本、managed 行以及所有新协议字段确实不存在；fixture 漂移或来源不明时测试必须失败。

E2E 不直接修改仓库 fixture。每次运行把 SQL 导入临时 Agent state 的新数据库，再只修改该数据库中的
v1 可用字段：
tenant、agent、Gateway 地址、TLS 路径和更新时间。动态证书身份不写入 v1 SQLite，因为这正是 migration
后需要由 legacy mTLS 路径安全补全的缺失数据。

## 组件与职责

### Fixture contract test

扩展现有 `test/suites/functional/topology/test_e2e_contract.py`。它只负责验证静态测试资产，不启动 VM，
也不复制生产 migration 逻辑。

### Legacy upgrade 场景脚本

新增独立脚本 `test/suites/functional/topology/legacy-managed-upgrade-unenrollment.sh`。脚本只负责第二个
legacy Agent 场景，接收现有 topology 主脚本已经准备好的 Manager JWT、artifact/channel 和结果目录，
避免继续扩大主脚本职责。

该脚本可以调用现有 Manager CLI、安装 URL、Vagrant 和 VM 内 Python `sqlite3` 标准库。不得增加仅供
测试使用的 Manager HTTP endpoint，也不得绕过 Gateway 直接调用生产 Store 退管函数。

### Topology 主脚本

现有 `e2e-systemd-vm.sh` 继续负责平台启动、当前 artifact 发布及正常 completion_v1 场景。完成现有场景
后调用 legacy upgrade 脚本，并把 legacy 结果纳入 summary。两个场景使用不同 Agent ID 和 enrollment，
防止状态和断言互相污染。

## 数据流

测试按以下顺序运行：

1. 当前 Manager 为 legacy 场景创建 enrollment，并通过现有安装链路生成短期测试证书和受限凭据。
2. 停止 Agent，保留安装器写入的配置、systemd unit、bootstrap policy 和动态凭据。
3. 从 manifest 验证通过的 SQL fixture 创建新的 `agent.db`，把 v1 managed 行绑定到本次 tenant、Agent、
   Gateway 和凭据路径。
4. 在隔离测试 Postgres 中删除该证书 JSON 投影的 `unenrollment_protocol` 字段，使 Manager 记录与
   `rc.5` 签发时代的证书事实一致。结构化列中的 tenant、agent、enrollment 和 serial 保持不变。
5. 启动当前 Agent。Agent 必须把 SQLite 从 v1 原子迁移到 v5，并把非 standalone enrollment 标记为
   `legacy_mtls`。
6. 通过 VM 内只读 SQLite 查询断言 schema version、protocol 和旧 policy 的 managed slot 迁移结果。
7. 调用本地 `sysarmorctl --json unenroll --timeout 60s`。Coordinator 通过真实 Gateway mTLS 发起不带
   completion hash 的 legacy revoke。
8. Gateway 从 peer certificate 和 Manager certificate record 补全旧 SQLite 缺失的 enrollment ID 和
   serial，验证协议为 legacy 后执行 durable revoke。
9. 断言 Manager enrollment 查询投影 `unenrollment_status = unknown_legacy`，并确认其不为
   `endpoint_completed`。
10. 断言 Agent 返回 standalone policy、managed CA/证书/私钥已删除，本地 SQLite 已切换 standalone。
11. 重启 systemd Agent，再次断言 standalone policy 和 standalone lifecycle 持久恢复。

## 安全与错误处理

- fixture 和结果文件不得包含私钥、bootstrap token 或 completion token。
- 所有证书由测试环境临时生成，测试结束后沿用现有 VM 生命周期和结果目录清理策略。
- Postgres legacy 转换必须按 tenant 和 certificate serial 精确更新一条记录；更新数量不是 1 时立即失败。
- fixture 摘要、schema、旧字段边界或来源 commit 不匹配时，在启动 VM 前失败。
- migration 未达到 v5、协议不是 `legacy_mtls`、Manager 不是 `unknown_legacy`、凭据仍存在或重启后不为
  standalone，任一条件均使测试失败。
- 不允许通过 `|| true`、空默认值或只检查命令退出码掩盖上述断言失败。
- 测试不得把 Manager 证书协议设置为客户端提供的值；测试准备阶段只模拟旧 Manager 已持久化的无协议
  记录，生产 Gateway 仍依据 Manager durable record 做协议选择。

## 测试分层与验收

### 静态 contract

- manifest 指向 `v0.1.0-rc.5` 和固定 commit；
- SQL SHA-256 与 manifest 一致；
- SQL 生成 schema v1 managed 状态；
- enrollment 不含任何 v3 到 v5 新字段；
- topology 主脚本调用独立 legacy 场景；
- legacy 场景包含 migration、`legacy_mtls`、`unknown_legacy`、凭据清理和重启断言。

### 定向 Go 回归

现有 localstore、Coordinator、Gateway 和 Store 测试继续作为 E2E 的分层诊断基础：

```text
go test ./internal/agent/localstore ./internal/agent/daemon ./internal/gateway ./internal/store ./internal/store/postgres -count=1
```

### 真实 topology

```text
make test-functional DOMAIN=topology
```

结果 summary 至少新增：

- `legacy_fixture_source`；
- `legacy_schema_migrated`；
- `legacy_mtls_unenrollment_applied`；
- `manager_legacy_status_unknown`；
- `legacy_standalone_after_restart`。

最终回归执行 `go test ./... -count=1`、`go vet ./...` 和 `git diff --check`。若本次仅修改测试资产，
无需重复运行与该场景无关的性能或 detection 矩阵。

## 非目标

- 不验证 `rc.5` Manager、Gateway 和 Agent 二进制逐个滚动升级。
- 不承诺支持所有历史开发 tag；首个兼容基线仅为 `v0.1.0-rc.5`。
- 不为 legacy Agent 增加 endpoint completion，也不把 `unknown_legacy` 提升为 completed。
- 不修改 EnrollmentCoordinator、Store、Gateway 协议或生产 API。
- 不引入静态测试私钥、外部下载、运行时 Git checkout 或旧版本依赖构建。
- 不新增 break-glass、离线退管或通用升级框架。
