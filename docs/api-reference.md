# SysArmor Manager API 参考文档

## 📋 概述

SysArmor Manager API 提供了完整的EDR/HIDS系统管理接口，包括健康检查、服务管理、Collector管理、事件查询等功能。

**基础信息**:
- **Base URL**: `http://localhost:8080`
- **API版本**: v1
- **认证方式**: 暂无 (开发环境)
- **响应格式**: JSON

## 🏥 健康检查接口

### 基础健康检查
```http
GET /health
```

**响应示例**:
```json
{
  "database": "connected",
  "service": "sysarmor-manager", 
  "status": "healthy",
  "version": "1.0.0"
}
```

### 系统健康状态
```http
GET /api/v1/health
```

**响应示例**:
```json
{
  "data": {
    "checked_at": "2025-09-20T14:30:57.299962952Z",
    "healthy": true,
    "services": {
      "indexer": {
        "components": {
          "opensearch": {
            "healthy": true,
            "response_time": 7635981,
            "status": "connected"
          }
        },
        "healthy": true,
        "status": "running"
      },
      "manager": {
        "components": {
          "api": {"healthy": true, "status": "running"},
          "database": {"healthy": true, "status": "connected"}
        },
        "healthy": true,
        "status": "running"
      },
      "middleware": {
        "components": {
          "kafka": {"healthy": true, "status": "connected"},
          "prometheus": {"healthy": true, "status": "healthy"},
          "vector": {"healthy": true, "status": "running"}
        },
        "healthy": true,
        "status": "running"
      },
      "processor": {
        "components": {
          "flink": {"healthy": true, "status": "connected"}
        },
        "healthy": true,
        "status": "running"
      }
    },
    "status": "healthy",
    "summary": {
      "healthy_components": 5,
      "healthy_services": 4,
      "total_components": 5,
      "total_services": 4
    }
  }
}
```

### 其他健康检查接口
- `GET /api/v1/health/overview` - 健康状态概览
- `GET /api/v1/health/comprehensive` - 综合健康状态
- `GET /api/v1/health/workers` - Worker状态列表

## 📡 Kafka服务管理

### Kafka健康检查
```http
GET /api/v1/services/kafka/health
```

**响应示例**:
```json
{
  "broker_count": 1,
  "cluster_info": [{
    "cluster_id": "",
    "name": "kafka-cluster",
    "status": "online",
    "broker_count": 1,
    "topic_count": 7,
    "online_partition_count": 179,
    "version": "3.4-IV0",
    "health_status": "healthy"
  }],
  "connected": true,
  "success": true
}
```

### Kafka集群和Broker管理
- `GET /api/v1/services/kafka/clusters` - Kafka集群信息
- `GET /api/v1/services/kafka/brokers` - Kafka Brokers信息
- `GET /api/v1/services/kafka/brokers/overview` - Brokers概览

### Kafka Topics管理
- `GET /api/v1/services/kafka/topics` - Topics列表
- `GET /api/v1/services/kafka/topics/overview` - Topics概览
- `GET /api/v1/services/kafka/topics/{topic}` - 特定Topic详情
- `GET /api/v1/services/kafka/consumer-groups` - Consumer Groups列表

## 🔧 Flink服务管理

### Flink健康检查
```http
GET /api/v1/services/flink/health
```

**响应示例**:
```json
{
  "cluster_info": {
    "taskmanagers": 1,
    "slots-total": 8,
    "slots-available": 4,
    "jobs-running": 2,
    "jobs-finished": 0,
    "flink-version": "1.18.1"
  },
  "connected": true,
  "message": "Successfully connected to Flink",
  "success": true
}
```

### Flink集群和作业管理
- `GET /api/v1/services/flink/overview` - Flink集群概览
- `GET /api/v1/services/flink/config` - Flink配置信息
- `GET /api/v1/services/flink/jobs` - Flink作业列表
- `GET /api/v1/services/flink/jobs/overview` - Flink作业概览
- `GET /api/v1/services/flink/taskmanagers` - TaskManager信息
- `GET /api/v1/services/flink/taskmanagers/overview` - TaskManager概览

## 🔍 OpenSearch服务管理

### OpenSearch健康检查
```http
GET /api/v1/services/opensearch/health
```

**响应示例**:
```json
{
  "cluster_info": {
    "cluster_name": "sysarmor-indexer-cluster",
    "status": "green",
    "number_of_nodes": 1,
    "active_primary_shards": 7,
    "active_shards": 7
  },
  "connected": true,
  "success": true
}
```

### OpenSearch集群和索引管理
- `GET /api/v1/services/opensearch/cluster/health` - 集群健康状态
- `GET /api/v1/services/opensearch/cluster/stats` - 集群统计信息
- `GET /api/v1/services/opensearch/indices` - 索引列表

### OpenSearch事件查询
- `GET /api/v1/services/opensearch/events/recent` - 最近事件查询
- `GET /api/v1/services/opensearch/events/search` - 事件搜索
- `GET /api/v1/services/opensearch/events/aggregations` - 事件聚合统计

## 📱 Collector管理

### 注册Collector
```http
POST /api/v1/collectors/register
```

**请求体**:
```json
{
  "deployment_type": "agentless",
  "hostname": "test-collector",
  "ip_address": "192.168.1.100",
  "os_type": "linux",
  "os_version": "ubuntu-20.04",
  "metadata": {
    "environment": "test",
    "purpose": "api-testing"
  }
}
```

**响应示例**:
```json
{
  "success": true,
  "data": {
    "collector_id": "7fe1233b-2892-422b-9411-c6a28306cd24",
    "worker_url": "http://middleware-vector:6000",
    "script_download_url": "/api/v1/scripts/setup-terminal.sh?collector_id=7fe1233b-2892-422b-9411-c6a28306cd24"
  }
}
```

### Collector查询和管理
- `GET /api/v1/collectors` - Collector列表
- `GET /api/v1/collectors/{id}` - 获取Collector状态
- `POST /api/v1/collectors/{id}/heartbeat` - Collector心跳上报
- `POST /api/v1/collectors/{id}/probe` - 主动探测Collector
- `PUT /api/v1/collectors/{id}/metadata` - 更新Collector元数据

### Collector生命周期管理

#### 注销Collector (软删除)
```http
POST /api/v1/collectors/{id}/unregister
```

**响应示例**:
```json
{
  "data": {
    "collector_id": "7fe1233b-2892-422b-9411-c6a28306cd24",
    "status": "unregistered",
    "uninstall_script_url": "/api/v1/scripts/uninstall-terminal.sh?collector_id=..."
  },
  "message": "Collector unregistered successfully",
  "success": true
}
```

#### 删除Collector
```http
DELETE /api/v1/collectors/{id}
```

**软删除响应**:
```json
{
  "data": {
    "collector_id": "7fe1233b-2892-422b-9411-c6a28306cd24",
    "status": "inactive",
    "uninstall_script_url": "/api/v1/scripts/uninstall-terminal.sh?collector_id=..."
  },
  "message": "Collector deactivated successfully. Use force=true to permanently delete.",
  "success": true
}
```

**硬删除 (force=true)**:
```http
DELETE /api/v1/collectors/{id}?force=true
```

## 📊 事件查询接口

### 最新事件查询
```http
GET /api/v1/events/latest?topic=sysarmor.raw.audit&limit=10
```

### 事件查询
```http
GET /api/v1/events/query?topic=sysarmor.raw.audit&limit=5
```

### 事件Topics管理
- `GET /api/v1/events/topics` - 事件Topics列表

## 🔧 Topic配置管理

### Topic配置查询
```http
GET /api/v1/topics/configs
```

**响应示例**:
```json
{
  "data": {
    "categories": {
      "alerts": ["sysarmor.alerts", "sysarmor.alerts.high"],
      "events": ["sysarmor.events.audit", "sysarmor.events.sysdig"],
      "raw": ["sysarmor.raw.audit", "sysarmor.raw.other"]
    },
    "configs": {
      "sysarmor.alerts": {
        "name": "sysarmor.alerts",
        "partitions": 16,
        "retention": "30d",
        "purpose": "消费sysarmor.events.*后生成的一般预警事件"
      }
    }
  }
}
```

### 其他Topic配置接口
- `GET /api/v1/topics/categories` - Topic分类查询
- `GET /api/v1/topics/defaults` - 默认Topics查询

## 📁 资源管理接口

### 获取部署脚本
```http
GET /api/v1/resources/scripts/agentless/setup-terminal.sh?collector_id={id}
```

**响应**: 返回完整的bash安装脚本

### 获取配置文件
```http
GET /api/v1/resources/configs/agentless/audit-rules?collector_id={id}
```

**响应**: 返回auditd监控规则配置

## 🛡️ Wazuh集成接口

### Wazuh配置查询
```http
GET /api/v1/wazuh/config
```

**响应示例**:
```json
{
  "data": {
    "status": "inactive",
    "message": "Wazuh service is disabled"
  },
  "success": true
}
```

## 📊 API测试统计

根据最新的API测试结果：

- **总接口数**: 53个
- **测试通过**: 52个 (98%)
- **测试失败**: 1个 (2%)
- **测试时间**: 2025-09-20T14:31:27+00:00

### 接口分类统计
- **健康检查**: 5个接口 (100%通过)
- **Kafka服务**: 7个接口 (100%通过)
- **Flink服务**: 7个接口 (100%通过)
- **OpenSearch服务**: 7个接口 (100%通过)
- **事件查询**: 3个接口 (100%通过)
- **Topic配置**: 3个接口 (100%通过)
- **Collector管理**: 16个接口 (100%通过)
- **资源管理**: 3个接口 (100%通过)
- **Wazuh集成**: 1个接口 (100%通过)
- **数据流验证**: 1个接口 (0%通过，已知问题)

## 🔄 Collector生命周期

SysArmor支持完整的Collector生命周期管理：

1. **创建** → `POST /api/v1/collectors/register`
2. **查询** → `GET /api/v1/collectors/{id}`
3. **心跳** → `POST /api/v1/collectors/{id}/heartbeat`
4. **更新** → `PUT /api/v1/collectors/{id}/metadata`
5. **注销** → `POST /api/v1/collectors/{id}/unregister` (状态变为unregistered)
6. **软删除** → `DELETE /api/v1/collectors/{id}` (状态变为inactive)
7. **硬删除** → `DELETE /api/v1/collectors/{id}?force=true` (永久删除)

## 📊 数据流架构

### Topic结构
- **原始数据**: `sysarmor.raw.audit` - auditd原始事件
- **处理事件**: `sysarmor.events.audit` - 经过Flink处理的结构化事件
- **告警数据**: `sysarmor.alerts.audit` - 生成的安全告警

### 数据流向
```
auditd → rsyslog → Vector → Kafka(raw) → Flink → Kafka(events) → Flink → Kafka(alerts) → OpenSearch
```

## 🚨 错误处理

### 标准错误响应格式
```json
{
  "success": false,
  "error": "错误描述信息"
}
```

### 常见HTTP状态码
- **200 OK**: 请求成功
- **400 Bad Request**: 请求参数错误
- **404 Not Found**: 资源不存在
- **500 Internal Server Error**: 服务器内部错误

## 🔧 配置类型匹配

SysArmor支持不同的部署类型，每种类型使用不同的配置：

### Agentless类型
- **支持配置**: `audit-rules` (auditd监控规则)
- **不支持**: `cfg.yaml` (因为不需要安装collector程序)

### Collector类型  
- **支持配置**: `cfg.yaml` (OpenTelemetry配置)
- **支持配置**: 其他collector相关配置

### 类型不匹配示例
```http
GET /api/v1/resources/configs/collector/cfg.yaml?collector_id={agentless_id}
```

**错误响应**:
```json
{
  "error": "Failed to generate config: deployment type mismatch: collector is agentless but requested collector",
  "success": false
}
```

## 📈 性能指标

### 响应时间 (毫秒)
- **健康检查**: < 100ms
- **Kafka查询**: 100-500ms  
- **Flink查询**: 200-800ms
- **OpenSearch查询**: 500-2000ms
- **Collector操作**: 100-300ms

### 并发支持
- **最大并发**: 100个请求/秒
- **超时设置**: 30秒
- **连接池**: 10个连接

## 🛠️ 开发和测试

### API测试工具
```bash
# 完整API测试 (53个接口)
./tests/test-system-api.sh

# 快速健康检查 (8个核心组件)
./tests/test-system-health.sh
```

### 测试结果导出
测试结果自动导出到 `./data/api-exports/` 目录：
- **JSON格式**: 结构化测试数据
- **文本日志**: 人类可读的测试报告

## 📚 相关文档

- **[快速开始](guides/quick-start.md)** - 系统部署和基础使用
- **[系统概览](guides/overview.md)** - 架构设计和组件说明
- **[开发指南](development/)** - 开发环境搭建和API开发

---

**最后更新**: 2025-09-20  
**API版本**: v1.0.0  
**测试覆盖率**: 98% (53个接口)
