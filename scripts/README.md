# SysArmor Kafka 工具

简单易用的 Kafka 事件导入导出工具，用于 SysArmor 系统的数据测试和迁移。

## 🚀 快速使用

### 查看可用 topics
```bash
# 使用默认本地服务器
./kafka-tools.sh list

# 连接远程 Kafka
KAFKA_BROKERS=49.232.13.155:9094 ./kafka-tools.sh list
```

### 导出事件
```bash
# 导出 1000 条事件 (默认本地服务器)
./kafka-tools.sh export sysarmor-agentless-b1de298c

# 导出指定数量的事件
./kafka-tools.sh export sysarmor-agentless-b1de298c 500

# 导出全部事件
./kafka-tools.sh export sysarmor-agentless-b1de298c all
./kafka-tools.sh export sysarmor-agentless-b1de298c -1

# 从远程 Kafka 导出全部事件
KAFKA_BROKERS=49.232.13.155:9094 ./kafka-tools.sh export sysarmor-agentless-b1de298c all

# 导出到指定目录
./kafka-tools.sh export sysarmor-agentless-b1de298c 1000 /tmp/kafka-data
```

### 导入事件
```bash
# 导入事件到默认本地服务器的指定 topic
./kafka-tools.sh import ./data/kafka-exports/sysarmor-agentless-b1de298c_20250905_222600.jsonl sysarmor-events-test

# 导入到远程 Kafka
KAFKA_BROKERS=49.232.13.155:9094 ./kafka-tools.sh import ./data/events.jsonl sysarmor-test
```

## 📋 命令说明

| 命令 | 说明 | 示例 |
|------|------|------|
| `list` | 列出所有可用的 topics | `./kafka-tools.sh list` |
| `export <topic> [count] [dir]` | 导出事件 | `./kafka-tools.sh export topic-name 1000` |
| `export <topic> all [dir]` | 导出全部事件 | `./kafka-tools.sh export topic-name all` |
| `import <file> <topic>` | 导入事件 | `./kafka-tools.sh import file.jsonl target-topic` |

## ⚙️ 配置

### Kafka 服务器配置
通过环境变量 `KAFKA_BROKERS` 配置 Kafka 服务器地址：

```bash
# 使用默认本地服务器 (localhost:9094)
./kafka-tools.sh list

# 连接远程 Kafka
KAFKA_BROKERS=49.232.13.155:9094 ./kafka-tools.sh list

# 连接其他服务器
KAFKA_BROKERS=192.168.1.100:9092 ./kafka-tools.sh list

# 连接多个 brokers
KAFKA_BROKERS=broker1:9092,broker2:9092 ./kafka-tools.sh list
```

### 其他配置
- **默认服务器**: localhost:9094
- **默认导出目录**: ./data/kafka-exports
- **文件格式**: JSONL (每行一个 JSON 事件)

## 🔄 典型工作流程

### 远程到本地数据迁移
```bash
# 1. 从远程服务器导出全部数据
KAFKA_BROKERS=49.232.13.155:9094 ./kafka-tools.sh export sysarmor-agentless-b1de298c all

# 2. 导入到本地 Kafka 进行测试
./kafka-tools.sh import ./data/kafka-exports/sysarmor-agentless-b1de298c_20250905_222600.jsonl sysarmor-events-test

# 3. 验证本地导入结果
./kafka-tools.sh list
```

### 本地开发测试
```bash
# 1. 查看本地 topics
./kafka-tools.sh list

# 2. 导出本地测试数据
./kafka-tools.sh export test-topic 100

# 3. 导入到另一个测试 topic
./kafka-tools.sh import ./data/kafka-exports/test-topic_20250905_222600.jsonl new-test-topic
```

## 📝 注意事项

- 需要 Docker 环境
- 自动创建输出目录和目标 topic
- SysArmor 相关 topics 会用 ★ 标记
- 导入的事件会追加到目标 topic，不会覆盖现有数据

### 导出全部数据说明
- 使用 `all` 或 `-1` 参数可以导出 topic 的全部数据
- 导出全部数据可能需要较长时间，请耐心等待
- 超时时间设置为 60 秒，适合大多数场景
- 建议先用小数量测试连接，确认无误后再导出全部数据

---

**版本**: v1.0.0  
**更新**: 2025-09-05
