# Agent Standalone Local Store Design

## 目的与结论

SysArmor Agent 安装后默认以 standalone 模式持续运行，不连接任何云端服务。Agent 在本地完成采集、检测、持久化和查询；只有用户显式注册后才启动 Gateway 上传和 Manager 控制通道。

本地存储采用分层设计：原始 Event 和传输批次写入有界 Protobuf segment spool；Signal、设备身份、Policy、注册状态、segment 元数据和上传 checkpoint 写入 SQLite。Content 继续使用现有签名 JSON content store，不混入遥测数据库。

本地数据目录硬上限默认为 10GiB。保留时长由实际 EPS 和事件大小决定，不能突破磁盘预算。注册后默认只上传注册完成后的数据；用户显式选择上传历史时才补传注册前数据。

## 目标负载

- Event 持续速率：1,000 EPS。
- Event 短时峰值：5,000 EPS，持续 60 秒。
- Signal 持续速率：100 EPS。
- Signal 短时峰值：1,000 EPS，持续 60 秒。
- Agent 本地数据目录默认硬上限：10GiB。
- Signal 默认最多保留 100,000 条。

性能指标是最低验收目标，不是通过无界缓存换取的瞬时吞吐。磁盘不足、队列拥塞和数据清理必须可观测，禁止静默丢弃。

## 非目标

本期不实现：

- 本地 Web UI。
- Incident 本地生成或持久化。
- Event 字段级本地索引和复杂查询。
- 多云端、多租户或多个独立上传消费者。
- 用户登录、权限和本地多用户系统。
- TPM 密钥封装和 Content 加密。
- SQLite 中保存原始 Event payload。

## 运行模式

只定义两个持久稳定状态：

```text
standalone
managed
```

### Standalone

- 首次启动生成稳定 `device_id`。
- 不要求 tenant、agent token、Manager 地址、mTLS 证书。
- 不建立任何外部网络连接。
- 加载本地 Collection Policy、Detection Policy 和 Content。
- 采集 Event、生成 Signal并持久化。
- 通过本地 Unix Socket 提供 status、Event 和 Signal 查询。

### Managed

- 保留相同 `device_id`、本地存储和检测链路。
- 持有注册产生的 tenant、agent ID、Gateway 地址和 mTLS 凭据引用。
- 后台 Uploader 从本地 spool 消费 DataBatch。
- 启动 Manager 控制通道。
- 网络不可用时继续本地采集和检测，不退出 Agent。

注册和注销是状态转换操作，不是第三种长期运行模式。转换必须原子持久化；失败时保持原状态。

## 数据目录

```text
/var/lib/sysarmor/agent/
├── agent.db
├── content/
│   └── *.json
├── credentials/
│   ├── agent.pem
│   ├── agent-key.pem
│   └── ca.pem
└── spool/
    ├── 0000000000000001.seg
    ├── 0000000000000002.seg
    └── 0000000000000003.open
```

- `agent.db` 使用 `modernc.org/sqlite`，保持 `CGO_ENABLED=0`。
- Content 保持现有 `/var/lib/sysarmor/agent/content` 文件协议。
- mTLS 私钥不写入 SQLite；SQLite 只保存 root-only 文件引用。
- `.open` 是当前可追加 segment；封口后原子改名为 `.seg`。

## SQLite 模型

本期使用单一干净 baseline，不保留旧 schema 兼容路径。

### schema_meta

```text
version INTEGER PRIMARY KEY
```

第一版只接受空数据库或当前 baseline。未来 migration 必须有明确的顺序、事务和回滚测试。

### device_identity

```text
singleton       INTEGER PRIMARY KEY CHECK (singleton = 1)
device_id       TEXT NOT NULL UNIQUE
host_id         TEXT NOT NULL
created_at_ns   INTEGER NOT NULL
```

`device_id` 首次启动生成 UUIDv7 并永久稳定。hostname 变化不能改变它。`host_id` 是当前主机标识，可独立更新。

### enrollment

```text
singleton           INTEGER PRIMARY KEY CHECK (singleton = 1)
state               TEXT NOT NULL CHECK (state IN ('standalone', 'managed'))
tenant_id           TEXT
agent_id            TEXT
gateway_address     TEXT
tls_ca_path         TEXT
tls_cert_path       TEXT
tls_key_path        TEXT
tls_server_name     TEXT
upload_history      INTEGER NOT NULL DEFAULT 0
managed_from_seq    INTEGER
updated_at_ns       INTEGER NOT NULL
```

standalone 状态下云端字段必须为空。managed 状态下 tenant、agent ID、Gateway 和凭据路径必须完整。

### policy

```text
kind            TEXT PRIMARY KEY
version         INTEGER NOT NULL
document_json   BLOB NOT NULL
digest          TEXT NOT NULL
updated_at_ns   INTEGER NOT NULL
```

Policy 与 Content 分离。写入前必须完成解析、规范化和检测引擎重建，SQLite 提交成功后才切换运行态。

### signals

```text
sequence        INTEGER PRIMARY KEY
signal_id       TEXT NOT NULL UNIQUE
observed_at_ns  INTEGER NOT NULL
rule_id         TEXT NOT NULL
severity        TEXT NOT NULL
payload         BLOB NOT NULL
```

`payload` 保存 Signal protobuf，不保存 JSON。索引仅包括：

```text
observed_at_ns
(rule_id, observed_at_ns)
(severity, observed_at_ns)
```

不为 label 或任意 payload 字段建索引。清理时按 sequence/observed time 删除最旧记录，默认最多 100,000 条。

### segments

```text
segment_id          INTEGER PRIMARY KEY
path                TEXT NOT NULL UNIQUE
state               TEXT NOT NULL CHECK (state IN ('open', 'sealed'))
first_sequence      INTEGER NOT NULL
last_sequence       INTEGER NOT NULL
record_count        INTEGER NOT NULL
bytes               INTEGER NOT NULL
created_at_ns       INTEGER NOT NULL
sealed_at_ns        INTEGER
```

SQLite 只保存 segment 级元数据，不为每个 Event 建行。

### upload_checkpoint

```text
singleton           INTEGER PRIMARY KEY CHECK (singleton = 1)
segment_id          INTEGER
record_offset       INTEGER NOT NULL
last_batch_id       TEXT
updated_at_ns       INTEGER NOT NULL
```

上传成功首先推进内存 checkpoint，在固定时间或完成 segment 后事务提交。崩溃最多导致少量 DataBatch 重传，由现有确定性 batch ID 和 Gateway 幂等语义去重。

## Event Segment 格式

segment 是顺序追加文件：

```text
SegmentHeader
RecordHeader + compressed DataBatch + CRC32C
RecordHeader + compressed DataBatch + CRC32C
...
```

### SegmentHeader

固定字段：

```text
magic          "SYSASEG1"
format_version 1
segment_id
created_at_ns
compression    zstd
```

### RecordHeader

固定字段：

```text
compressed_length
uncompressed_length
batch_sequence
batch_id_length
```

CRC32C 覆盖 RecordHeader 和压缩 payload。payload 是当前 `DataBatch` protobuf bytes。schema compatibility 继续由 `DataBatch.schema_version` 控制，segment format version 只控制容器格式。

默认 segment 上限为 64MiB。达到上限后：

1. flush 当前缓冲；
2. `fdatasync`；
3. 更新 SQLite segment 元数据；
4. 原子改名 `.open` 为 `.seg`；
5. 创建下一个 `.open`。

正常批量写入不对每个 Event 执行 fsync。默认每秒或每 256 Event checkpoint 一次，先到者触发。

## 崩溃恢复

Agent 启动顺序：

1. 打开 SQLite 并验证 baseline；
2. 扫描 spool 文件；
3. 对 `.open` 顺序校验 record 长度和 CRC32C；
4. 截断最后一个不完整 record；
5. 以磁盘事实修复 segment 元数据；
6. 加载设备身份、注册状态、Policy 和 Content；
7. 启动 Sensor 与本地控制面；
8. managed 时再启动 Uploader 和控制通道。

中间 record 损坏不能静默跳过。该 segment 标记损坏、停止消费其后记录并在 health 中报告 degraded。只允许截断末尾因断电产生的不完整 record。

## 本地优先遥测管道

当前 `localBatchSender` 直接返回 accepted 并丢弃数据，必须删除。新链路为：

```text
Sensor Event
  → Normalize
  → Detect Signal
  → 构造 DataBatch
  → LocalStore.Append(DataBatch)
  → 本地提交成功
  → 发布给实时 Unix Socket watcher
  → managed Uploader 异步读取
```

Local Store 是唯一事实来源。Uploader 不接收检测引擎的内存 channel，也不能阻塞 Sensor 主链路。

本地提交失败时：

- Agent health 立即 degraded；
- 产生内存级 storage failure Signal；
- 应用有界退避；
- 不得将未持久化数据报告为成功；
- 达到硬磁盘保护条件时按容量策略清理，而不是无限阻塞宿主机。

## 容量治理

默认配置：

```yaml
storage:
  path: /var/lib/sysarmor/agent
  max_bytes: 10GiB
  min_free_bytes: 2GiB
  event_segment_size: 64MiB
  event_compression: zstd
  signal_max_count: 100000
```

硬限制判断包括整个 Agent 数据目录中的 SQLite、WAL、segment 和 Content 文件。凭据文件不因容量治理删除。

清理顺序：

1. 已上传的最旧 sealed segment；
2. 超过 Signal count 限制的最旧 Signal；
3. 未上传的最旧 sealed segment，仅在达到硬容量或最小剩余磁盘保护时；
4. 当前 `.open` 永不由清理器删除。

删除未上传 segment 时必须累计 dropped Event/Batch 数量，推进 checkpoint 越过被删除范围，并报告 storage pressure degraded 状态。不得静默删除。

## 本地查询

Unix Socket API 支持：

- `status`：模式、device ID、存储用量、最老/最新序列、Signal 数、上传 checkpoint 和 dropped 数量。
- `event query`：最近 N 条、时间范围和 behavior 过滤；通过有限 segment 倒序扫描实现。
- `signal query`：时间范围、rule ID、severity、limit 和 offset；走 SQLite 索引。
- 现有 watch：先读取持久化 recent，再订阅实时流，使用 sequence 去重。

Event 查询不支持任意字段表达式。复杂历史检索属于 Manager/OpenSearch 职责。

## 注册生命周期

### 首次安装

安装脚本生成最小 standalone 配置，只包含本地路径、Sensor 和资源限制。Agent 首次启动生成 device ID，不要求云端 token。

### 注册

```text
sysarmorctl enroll --manager URL [--upload-history]
```

流程：

1. 通过 Unix Socket 请求 Agent 生成私钥和 CSR；
2. ctl 调用 Manager enrollment API；
3. Agent 校验证书链和返回身份；
4. 凭据写入临时文件并原子 rename；
5. SQLite 事务写 managed enrollment；
6. Uploader 和控制通道由监督器启动；
7. 不重启采集和检测运行时。

默认 `managed_from_seq` 是注册事务提交后的下一个 DataBatch sequence。指定 `--upload-history` 时从最早仍保留 segment 开始。

### 注销

注销停止网络运行时，SQLite 原子切回 standalone，删除 mTLS 凭据引用并清理凭据文件。device ID、本地 Policy、Content、Signal 和 spool 保留。服务端撤销属于 Manager 独立操作；本地注销不能声称已完成服务端撤销。

## 配置演进

现有：

```yaml
agent:
  id:
  tenant_id:
  token:
manager:
  transport: grpc|local
```

替换为：

```yaml
agent:
  state_path: /var/lib/sysarmor/agent

storage:
  max_bytes: 10GiB
  min_free_bytes: 2GiB
  event_segment_size: 64MiB
  signal_max_count: 100000
```

云端身份和 Manager/Gateway 地址由 enrollment 状态拥有，不再作为 standalone 静态配置必填项。旧 `manager.transport: local` 不保留兼容路径；该特性与 clean baseline 一样只面向全新 Agent 安装。

## 安全边界

- 数据目录默认 root-only，目录 `0700`，文件 `0600`。
- SQLite 使用 WAL、`foreign_keys=ON` 和 `busy_timeout`，仅 Agent 进程直接打开。
- `sysarmorctl` 只能通过 root-owned Unix Socket 访问，不直接打开数据库或 segment。
- segment record 在解压和 protobuf 解析前验证长度上限和 CRC。
- enrollment 私钥不进入 SQLite、日志、CLI 参数或响应正文。
- standalone 模式禁止任何 Manager/Gateway连接尝试。
- Content 签名安全边界保持独立，本期不顺带实现 Content 加密。

## 可观测性

Agent health 新增：

```text
mode
device_id
storage_bytes
storage_max_bytes
storage_min_free_bytes
oldest_event_sequence
latest_event_sequence
signal_count
sealed_segment_count
open_segment_bytes
upload_segment_id
upload_record_offset
dropped_events_storage
dropped_batches_storage
storage_degraded
last_storage_error
```

不得将 standalone 的“未上传 backlog”报告为异常。只有 managed 模式下 checkpoint 落后才计算 upload backlog。

## 测试与验收

### 单元与组件测试

- SQLite baseline、设备身份幂等生成和 enrollment 事务约束。
- Policy 先验证后提交及重启恢复。
- Signal 批量写入、索引查询、100,000 条上限清理。
- segment 追加、轮转、CRC、Zstd、末尾截断和中间损坏拒绝。
- 容量清理顺序及未上传数据删除计数。
- upload checkpoint 崩溃后少量重传和幂等去重。
- standalone 不创建任何外部连接。
- managed 网络中断不影响采集、检测和本地提交。
- 注册默认不上传历史，`--upload-history` 从最早保留数据开始。

### 性能验收

在 Linux VM 使用真实 Tetragon 或可重复事件源：

- 1,000 Event EPS 持续 30 分钟，无静默丢弃。
- 5,000 Event EPS 持续 60 秒，恢复后 backlog 可排空。
- 100 Signal EPS 持续 30 分钟。
- Agent 重启后 Event、Signal、Policy、identity 和 checkpoint 恢复。
- 10GiB 配额测试使用缩小测试配额等比验证，不实际写满 10GiB。
- 记录 CPU、RSS、磁盘吞吐、压缩比和 fsync 延迟。

### 产品验收

```text
安装后无云端配置也能启动
standalone 不产生外连
本地能查询重启前 Signal 和近期 Event
注册后不重启 Sensor 即开始上传
默认不上传注册前历史
断网期间持续本地检测
恢复网络后从 checkpoint 继续
磁盘达到硬上限时不继续增长且明确报告数据丢弃
```

## 实施分解

该设计按四个可独立评审阶段实施：

1. Local Store baseline：SQLite、identity、Policy、Signal 和 segment codec。
2. Durable telemetry：Local Store 成为唯一写入路径，本地查询和容量治理。
3. Managed uploader：checkpoint、断线恢复、历史上传选择和动态网络监督器。
4. Enrollment UX：默认 standalone 安装、`sysarmorctl enroll/unenroll/status` 和端到端验收。

每阶段必须保持 Agent 可运行，并通过全量现有测试后再进入下一阶段。
