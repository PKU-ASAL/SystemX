# SysArmor Kafka 工具 (增强版)

功能强大的 Kafka 事件导入导出工具，支持 Docker 网络连接、批量导出、Collector 过滤等高级功能。

## 🚀 快速使用

### 查看可用 topics
```bash
# 使用默认本地服务器
./kafka-tools.sh list

# 连接远程 Kafka
KAFKA_BROKERS=49.232.13.155:9094 ./kafka-tools.sh list

# 使用 Docker 网络连接
DOCKER_NETWORK=sysarmor-net ./kafka-tools.sh list
```

### 导出事件

#### 基础导出
```bash
# 导出 1000 条事件 (默认)
./kafka-tools.sh export sysarmor-agentless-b1de298c

# 导出指定数量的事件
./kafka-tools.sh export sysarmor-agentless-b1de298c 500

# 导出全部事件
./kafka-tools.sh export sysarmor-agentless-b1de298c all
```

#### 使用选项导出
```bash
# 指定输出目录
./kafka-tools.sh export sysarmor-agentless-b1de298c all --out ~/exports

# 指定批次大小 (仅在导出全部数据时生效)
./kafka-tools.sh export sysarmor-agentless-b1de298c all --batch-size 500000

# 过滤特定 collector 的数据
./kafka-tools.sh export sysarmor-agentless-b1de298c all --collector-id b1de298c

# 组合使用多个选项
./kafka-tools.sh export sysarmor-agentless-b1de298c all --out ~/exports --batch-size 500000 --collector-id b1de298c-38bd-479d-be94-459778086446
```

#### Docker 网络模式
```bash
# 使用 Docker 网络连接
DOCKER_NETWORK=sysarmor-net ./kafka-tools.sh export sysarmor-agentless-b1de298c all --out ~/exports

# 从远程 Docker Kafka 导出
DOCKER_NETWORK=sysarmor-net KAFKA_CONTAINER_NAME=my-kafka ./kafka-tools.sh export sysarmor-agentless-b1de298c all
```

### 后台运行和监控
```bash
# 后台运行大数据量导出
nohup ./kafka-tools.sh export sysarmor-agentless-b1de298c all --collector-id b1de298c > export.out 2>&1 &

# 监控导出进度
./kafka-tools.sh monitor

# 监控指定日志文件
./kafka-tools.sh monitor ~/kafka-export-20250907_221200.log

# 查看导出状态
./kafka-tools.sh status
```

### 导入事件
```bash
# 导入事件到指定 topic
./kafka-tools.sh import ./data/kafka-exports/sysarmor-agentless-b1de298c_20250905_222600.jsonl sysarmor-events-test

# 使用 Docker 网络导入
DOCKER_NETWORK=sysarmor-net ./kafka-tools.sh import ./data/events.jsonl sysarmor-test
```

## 📋 命令详解

### 命令列表

| 命令 | 说明 | 示例 |
|------|------|------|
| `list` | 列出所有可用的 topics | `./kafka-tools.sh list` |
| `export <topic> [count]` | 导出事件 (支持批量和过滤) | `./kafka-tools.sh export topic-name all --collector-id abc` |
| `import <file> <topic>` | 导入事件 | `./kafka-tools.sh import file.jsonl target-topic` |
| `monitor [log_file]` | 监控导出进度 | `./kafka-tools.sh monitor` |
| `status` | 显示导出状态 | `./kafka-tools.sh status` |

### 参数说明

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `topic` | Kafka topic 名称 | - |
| `count` | 导出事件数量 | 1000 |
| `file` | 输入文件路径 | - |

**count 参数特殊值**：
- `all` / `ALL` / `-1`：导出全部事件

### 选项说明

| 选项 | 说明 | 默认值 |
|------|------|--------|
| `--out <dir>` | 输出目录 | ./data/kafka-exports |
| `--batch-size <size>` | 批次大小 (仅在导出全部数据时生效) | 1000000 |
| `--collector-id <id>` | 过滤特定 collector 的数据 | - |

## ⚙️ 环境配置

### 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `KAFKA_BROKERS` | Kafka 服务器地址 | localhost:9094 |
| `DOCKER_NETWORK` | Docker 网络名称 | 未设置 (使用直连) |
| `KAFKA_CONTAINER_NAME` | Kafka 容器名称 | sysarmor-kafka-1 |

### 连接模式

#### 直连模式 (默认)
```bash
# 连接本地 Kafka
./kafka-tools.sh list

# 连接远程 Kafka
KAFKA_BROKERS=49.232.13.155:9094 ./kafka-tools.sh list
```

#### Docker 网络模式
```bash
# 连接到 Docker 网络中的 Kafka 容器
DOCKER_NETWORK=sysarmor-net ./kafka-tools.sh list

# 指定容器名称
DOCKER_NETWORK=sysarmor-net KAFKA_CONTAINER_NAME=my-kafka ./kafka-tools.sh list
```

## 🔄 典型使用场景

### 场景1：大数据量批量导出
```bash
# 导出特定 collector 的所有数据，使用批量模式
DOCKER_NETWORK=sysarmor-net ./kafka-tools.sh export sysarmor-agentless-b1de298c all \
  --out ~/kafka-exports \
  --batch-size 1000000 \
  --collector-id b1de298c-38bd-479d-be94-459778086446

# 后台运行
nohup DOCKER_NETWORK=sysarmor-net ./kafka-tools.sh export sysarmor-agentless-b1de298c all \
  --out ~/kafka-exports \
  --collector-id b1de298c > export.out 2>&1 &
```

### 场景2：远程到本地数据迁移
```bash
# 1. 从远程 Docker Kafka 导出数据
DOCKER_NETWORK=sysarmor-net ./kafka-tools.sh export sysarmor-agentless-b1de298c all --out ~/remote-data

# 2. 导入到本地 Kafka 进行测试
./kafka-tools.sh import ~/remote-data/sysarmor-agentless-b1de298c_*.jsonl sysarmor-events-test

# 3. 验证导入结果
./kafka-tools.sh list
```

### 场景3：开发调试
```bash
# 导出少量数据用于调试
./kafka-tools.sh export sysarmor-events-test 100 --collector-id test-collector

# 快速查看 topic 状态
./kafka-tools.sh list

# 监控正在进行的导出
./kafka-tools.sh status
```

### 场景4：数据分析
```bash
# 导出特定时间段的数据 (通过 collector 过滤)
./kafka-tools.sh export sysarmor-agentless-b1de298c all \
  --collector-id b1de298c \
  --out ~/analysis-data

# 导出多个 collector 的数据进行对比
./kafka-tools.sh export sysarmor-agentless-b1de298c all --collector-id collector1 --out ~/data1
./kafka-tools.sh export sysarmor-agentless-c289acf6 all --collector-id collector2 --out ~/data2
```

## 📊 批量导出特性

### 自动批量模式
- **触发条件**：导出全部数据且数据量 > batch_size
- **批次文件**：自动创建多个文件 `topic_batch_1.jsonl`, `topic_batch_2.jsonl`, ...
- **智能调整**：最后批次自动调整大小，避免等待超时
- **进度显示**：实时显示导出进度百分比

### 批量导出示例
```bash
# 大数据量自动启用批量模式
DOCKER_NETWORK=sysarmor-net ./kafka-tools.sh export sysarmor-agentless-b1de298c all --batch-size 500000

# 输出示例:
# [INFO] 数据量较大，启用批量导出模式
# [INFO] 计划分 4 个批次导出，每批 500000 条消息
# [INFO] 导出批次 1/4 (预期: 500000 条消息)...
# [SUCCESS] 批次 1 完成: 500000 条记录
# [INFO] 导出进度: 25% (1/4)
# [INFO] 最后批次，调整批次大小为: 27485
# [SUCCESS] 批次 4 完成: 27485 条记录
```

## 🔍 监控和调试

### 实时监控
```bash
# 监控最新的导出进度
./kafka-tools.sh monitor

# 监控指定日志文件
./kafka-tools.sh monitor ~/kafka-export-20250907_221200.log

# 查看导出状态
./kafka-tools.sh status
```

### 状态信息
`status` 命令显示：
- 正在运行的导出进程数量
- 活跃进程详情
- 最近的导出目录
- 最近的日志文件

### 日志文件
- **位置**：`~/kafka-export-YYYYMMDD_HHMMSS.log`
- **内容**：详细的导出进度、错误信息、统计数据
- **实时监控**：使用 `monitor` 命令实时查看

## 📁 输出文件结构

### 标准导出
```
./data/kafka-exports/
└── sysarmor-agentless-b1de298c_20250907_224832/
    ├── sysarmor-agentless-b1de298c_20250907_224832.jsonl
    └── export_summary.txt
```

### 批量导出
```
~/kafka-exports/
└── sysarmor-agentless-b1de298c_b1de298c-38bd-479d-be94-459778086446_20250907_224832/
    ├── sysarmor-agentless-b1de298c_batch_1_20250907_224832.jsonl  (524MB)
    ├── sysarmor-agentless-b1de298c_batch_2_20250907_224832.jsonl  (524MB)
    ├── sysarmor-agentless-b1de298c_batch_3_20250907_224832.jsonl  (15MB)
    └── export_summary.txt
```

### 汇总文件示例
```
SysArmor Kafka 数据导出汇总
===========================
Topic: sysarmor-agentless-b1de298c
Collector ID: b1de298c-38bd-479d-be94-459778086446
导出时间: Sun Sep  7 22:51:32 CST 2025
导出目录: /home/ubuntu/kafka-exports/...
批次大小: 1000000
总文件数: 3
总记录数: 2027485
总文件大小: 1.1G
```

## 🛠️ 高级用法

### 性能优化
```bash
# 大批次导出 (减少文件数量)
./kafka-tools.sh export sysarmor-agentless-b1de298c all --batch-size 2000000

# 小批次导出 (更频繁的进度更新)
./kafka-tools.sh export sysarmor-agentless-b1de298c all --batch-size 100000
```

### 数据过滤
```bash
# 完整 collector ID 过滤
./kafka-tools.sh export sysarmor-agentless-b1de298c all --collector-id b1de298c-38bd-479d-be94-459778086446

# 部分 collector ID 匹配
./kafka-tools.sh export sysarmor-agentless-b1de298c all --collector-id b1de298c

# 导出特定主机的数据
./kafka-tools.sh export sysarmor-agentless-b1de298c all --collector-id racknerd-915f21b
```

### 容器化环境
```bash
# 连接到 Docker Compose 部署的 Kafka
DOCKER_NETWORK=sysarmor-net KAFKA_CONTAINER_NAME=sysarmor-kafka-1 ./kafka-tools.sh list

# 连接到 Kubernetes 中的 Kafka
DOCKER_NETWORK=kafka-net KAFKA_CONTAINER_NAME=kafka-broker-0 ./kafka-tools.sh export my-topic all
```

## 🔧 故障排查

### 常见问题

#### 连接失败
```bash
# 检查 Docker 网络
docker network ls | grep sysarmor

# 检查 Kafka 容器状态
docker ps | grep kafka

# 测试连接
DOCKER_NETWORK=sysarmor-net ./kafka-tools.sh list
```

#### 导出缓慢
```bash
# 检查导出状态
./kafka-tools.sh status

# 监控实时进度
./kafka-tools.sh monitor

# 调整批次大小
./kafka-tools.sh export topic-name all --batch-size 500000
```

#### 磁盘空间不足
```bash
# 检查输出目录大小
du -sh ./data/kafka-exports/

# 使用自定义输出目录
./kafka-tools.sh export topic-name all --out /path/to/large/disk
```

### 调试技巧
```bash
# 先导出少量数据测试
./kafka-tools.sh export sysarmor-agentless-b1de298c 10

# 检查数据格式
head -1 ./data/kafka-exports/sysarmor-agentless-b1de298c_*.jsonl | jq .

# 验证 collector 过滤
./kafka-tools.sh export sysarmor-agentless-b1de298c 100 --collector-id test-id
```

## 📈 性能特性

### 批量导出优势
- **智能分批**：大数据量自动启用批量模式
- **进度可视**：实时显示导出进度百分比
- **资源优化**：使用 consumer group 确保数据连续性
- **超时避免**：最后批次智能调整大小，避免等待超时

### 性能数据
- **导出速度**：约 50万条/分钟
- **批次处理**：每批次 1-2 分钟
- **内存占用**：低内存占用，适合大数据量处理
- **网络优化**：支持 Docker 网络，减少网络开销

## 📝 注意事项

### 重要提醒
- ✅ 需要 Docker 环境
- ✅ 自动创建输出目录和目标 topic
- ✅ SysArmor 相关 topics 会用 ★ 标记显示消息数量
- ✅ 导入的事件会追加到目标 topic，不会覆盖现有数据
- ✅ 支持 jq 和 grep 两种过滤方式，自动选择最优方案

### 批量导出说明
- **自动触发**：当导出全部数据且数据量 > batch_size 时自动启用
- **文件命名**：`topic_batch_1.jsonl`, `topic_batch_2.jsonl`, ...
- **最后批次**：自动调整为实际剩余消息数量，避免等待超时
- **Consumer Group**：使用唯一的 consumer group 确保数据连续性
- **自动清理**：导出完成后自动删除 consumer group

### 数据过滤
- **JSON 过滤**：优先使用 jq 进行精确 JSON 过滤
- **文本过滤**：jq 不可用时自动降级为 grep 过滤
- **部分匹配**：collector-id 支持部分匹配，便于使用
- **实时过滤**：在导出过程中实时过滤，节省存储空间

---

**版本**: v2.0.0 (增强版)  
**更新**: 2025-09-07  
**新增功能**: Docker 网络支持、批量导出、Collector 过滤、进度监控
