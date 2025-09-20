# SysArmor 数据库迁移说明

## 📋 迁移历史

### 统一迁移 (2025-09-12)

为了简化部署和维护，我们将所有历史迁移合并为一个统一的初始化脚本：

- **001_initial.sql**: 包含完整的表结构和索引定义

### 合并的迁移内容

1. **原 001_initial.sql**: 基础 collectors 表结构
2. **原 002_add_last_active.sql**: Nova 分支的 last_active 字段
3. **原 003_remove_kafka_topic.sql**: 移除 kafka_topic 字段的统一 topic 架构

## 🚀 部署说明

### 新环境部署

对于全新的环境，只需要运行：

```sql
\i shared/migrations/001_initial.sql
```

### 现有环境升级

如果你的环境已经运行了旧的迁移，可以：

1. **方案 A**: 重建数据库（推荐，适合开发环境）
   ```bash
   # 删除现有数据库
   docker-compose down -v
   
   # 重新启动并运行统一迁移
   docker-compose up -d postgres
   docker-compose exec postgres psql -U sysarmor -d sysarmor -f /migrations/001_initial.sql
   ```

2. **方案 B**: 手动调整现有表结构（生产环境）
   ```sql
   -- 如果 kafka_topic 字段仍存在，移除它
   ALTER TABLE collectors DROP COLUMN IF EXISTS kafka_topic;
   
   -- 确保 last_active 字段存在
   ALTER TABLE collectors ADD COLUMN IF NOT EXISTS last_active TIMESTAMP;
   
   -- 添加缺失的索引
   CREATE INDEX IF NOT EXISTS idx_collectors_last_active ON collectors(last_active);
   CREATE INDEX IF NOT EXISTS idx_collectors_status_last_active ON collectors(status, last_active);
   ```

## 📊 表结构说明

### collectors 表

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | UUID | 主键 |
| `collector_id` | VARCHAR(255) | Collector 唯一标识 |
| `hostname` | VARCHAR(255) | 主机名 |
| `ip_address` | VARCHAR(255) | IP 地址 |
| `os_type` | VARCHAR(50) | 操作系统类型 |
| `os_version` | VARCHAR(100) | 操作系统版本 |
| `status` | VARCHAR(20) | 状态 (active/inactive/error/unregistered/offline) |
| `worker_address` | VARCHAR(255) | Worker 地址 |
| `deployment_type` | VARCHAR(50) | 部署类型 (agentless/sysarmor-stack/wazuh-hybrid) |
| `last_heartbeat` | TIMESTAMP | 最后心跳时间 |
| `last_active` | TIMESTAMP | 最后活跃时间 |
| `heartbeat_interval` | INTEGER | 心跳间隔（秒） |
| `metadata` | JSONB | 元数据（标签、分组等） |
| `config_version` | VARCHAR(50) | 配置版本 |
| `created_at` | TIMESTAMP | 创建时间 |
| `updated_at` | TIMESTAMP | 更新时间 |

### 重要变更

1. **移除 kafka_topic 字段**: 现在使用统一的 topic 架构
2. **添加 last_active 字段**: 支持双向心跳机制
3. **增强索引**: 优化查询性能，特别是元数据查询
4. **添加约束**: 确保数据完整性

## 🔧 统一 Topic 架构

现在所有 collector 数据都通过以下方式路由：

- **Agentless**: `sysarmor.raw.audit` (原始 auditd 数据)
- **Sysdig**: `sysarmor.events.sysdig` (结构化事件)
- **转换后**: `sysarmor.events.audit` (处理后的 audit 事件)

分区键统一使用 `collector_id`，确保同一 collector 的数据有序且可精确查询。
