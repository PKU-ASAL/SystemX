# SysArmor Kafka 工具

简单实用的 Kafka 数据导入导出工具，自动读取 `.env` 配置。

## 🚀 主要功能

- **list** - 查看 Kafka topics 和消息数量
- **import** - 导入 JSONL 文件到 Kafka topic
- **export** - 导出 Kafka topic 数据到文件
- **monitor** - 监控导出进度
- **status** - 查看导出状态

## � 基本用法

### 查看 topics
```bash
./kafka-tools.sh list
```

### 导入数据
```bash
# 导入文件到指定 topic
./kafka-tools.sh import docs/draft/sysarmor-agentless-b1de298c_20250905_225242.jsonl sysarmor-events-test
```

### 导出数据
```bash
# 导出指定数量
./kafka-tools.sh export sysarmor-events-test 100

# 导出全部数据
./kafka-tools.sh export sysarmor-events-test all

# 指定输出目录
./kafka-tools.sh export sysarmor-events-test all --out ~/exports
```

## ⚙️ 配置

### 自动配置
脚本自动读取 `.env` 文件中的配置：
- `SYSARMOR_NETWORK` - Docker 网络名称
- `KAFKA_HOST` - Kafka 主机名
- `KAFKA_PORT` - Kafka 端口

### 环境变量覆盖
```bash
# 连接远程 Kafka
KAFKA_HOST=remote-server KAFKA_PORT=9094 ./kafka-tools.sh list

# 使用不同端口
KAFKA_PORT=9095 ./kafka-tools.sh export topic-name 10
```

## 🔧 高级选项

### 批量导出
```bash
# 大数据量自动启用批量模式
./kafka-tools.sh export large-topic all --batch-size 500000
```

### 数据过滤
```bash
# 从混合 topic 中过滤特定 collector
./kafka-tools.sh export sysarmor-events-test all --collector-id b1de298c
```

### 后台运行
```bash
# 后台导出大量数据
nohup ./kafka-tools.sh export large-topic all > export.log 2>&1 &

# 监控进度
./kafka-tools.sh monitor
./kafka-tools.sh status
```

## 📁 输出文件

导出的文件保存在 `./data/kafka-exports/` 目录下：
```
./data/kafka-exports/
└── topic-name_20250910_123456/
    ├── topic-name_20250910_123456.jsonl
    └── export_summary.txt
```

## � 使用提示

- **SysArmor topics** 会用 ★ 标记并显示消息数量
- **自动创建** topic 和输出目录
- **批量模式** 在数据量大时自动启用
- **进度监控** 支持实时查看导出进度
- **数据过滤** 主要用于混合 topic，专用 topic 通常不需要

---

**版本**: v3.1.0  
**更新**: 2025-09-10
