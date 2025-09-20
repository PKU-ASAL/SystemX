# Kafka 配置和初始化

## 📋 概述

这个目录包含 SysArmor Kafka 的配置文件和初始化脚本。

## 🚀 自动初始化

### Topics 自动创建

当启动 middleware 服务时，`kafka-init` 服务会自动创建所需的 topics：

```bash
docker-compose -f docker-compose.middleware.yml up -d
```

### 创建的 Topics

| Topic 名称 | 分区数 | 用途 | 保留期 |
|-----------|--------|------|--------|
| `sysarmor.raw.audit.1` | 32 | 主要的 audit 事件数据 | 3天 |
| `sysarmor.raw.other.1` | 8 | 其他类型的事件数据 | 3天 |

### 配置参数

通过环境变量可以自定义 topic 配置：

```bash
# 在 .env 文件中设置
SYSARMOR_AUDIT_TOPIC=sysarmor.raw.audit.1
SYSARMOR_OTHER_TOPIC=sysarmor.raw.other.1
KAFKA_AUDIT_PARTITIONS=32
KAFKA_AUDIT_RETENTION_MS=259200000  # 3天
```

## 🔧 初始化脚本

### `init-topics.sh`

这个脚本在 Kafka 启动后自动运行，负责：

1. 等待 Kafka 服务就绪
2. 检查 topics 是否已存在
3. 创建不存在的 topics
4. 配置 topic 参数（分区数、保留期、压缩等）
5. 验证创建结果

### 脚本特性

- **幂等性**: 多次运行不会重复创建 topics
- **健康检查**: 等待 Kafka 完全启动后再执行
- **错误处理**: 创建失败时提供详细错误信息
- **验证**: 创建后验证 topics 配置
