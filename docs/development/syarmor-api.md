# SysArmor API 参考

## 📋 API 概览

SysArmor Manager 提供完整的 RESTful API，支持设备管理、数据查询、服务监控等功能。

**Base URL**: `http://localhost:8080/api/v1`  
**API 文档**: `http://localhost:8080/swagger/index.html`

## 🔧 核心 API

### 健康检查
```bash
GET /health                    # 基础健康检查
GET /api/v1/health            # 详细健康状态
GET /api/v1/health/comprehensive  # 完整系统健康报告
```

### 设备管理 (Collectors)
```bash
# 设备注册和管理
POST /api/v1/collectors/register     # 注册新设备
GET  /api/v1/collectors              # 获取设备列表
GET  /api/v1/collectors/:id          # 获取设备详情
PUT  /api/v1/collectors/:id/metadata # 更新设备元数据
DELETE /api/v1/collectors/:id        # 删除设备

# 心跳和状态
POST /api/v1/collectors/:id/heartbeat # 心跳上报
POST /api/v1/collectors/:id/probe     # 主动探测
```

### 事件查询
```bash
# 事件查询
GET  /api/v1/events/latest           # 最新事件
GET  /api/v1/events/query            # 条件查询
POST /api/v1/events/search           # 复杂搜索

# Topic 管理
GET /api/v1/events/topics            # 获取 Topics 列表
GET /api/v1/events/topics/:topic/info # Topic 详情
```

### 资源管理
```bash
# 脚本和配置资源
GET /api/v1/resources/scripts/:deployment_type/:script_name
GET /api/v1/resources/configs/:deployment_type/:config_name
GET /api/v1/resources/binaries/:filename
```

## 🔍 服务管理 API

### Kafka 服务
```bash
# 健康检查和集群信息
GET /api/v1/services/kafka/health
GET /api/v1/services/kafka/clusters
GET /api/v1/services/kafka/brokers

# Topic 管理
GET    /api/v1/services/kafka/topics
POST   /api/v1/services/kafka/topics
GET    /api/v1/services/kafka/topics/:topic
DELETE /api/v1/services/kafka/topics/:topic
GET    /api/v1/services/kafka/topics/:topic/messages

# Consumer Group 管理
GET /api/v1/services/kafka/consumer-groups
GET /api/v1/services/kafka/consumer-groups/:group
```

### Flink 服务
```bash
# 集群管理
GET /api/v1/services/flink/health
GET /api/v1/services/flink/overview
GET /api/v1/services/flink/cluster/health

# 作业管理
GET /api/v1/services/flink/jobs
GET /api/v1/services/flink/jobs/:job_id
GET /api/v1/services/flink/jobs/:job_id/metrics

# TaskManager 管理
GET /api/v1/services/flink/taskmanagers
GET /api/v1/services/flink/taskmanagers/overview
```

### OpenSearch 服务
```bash
# 集群管理
GET /api/v1/services/opensearch/health
GET /api/v1/services/opensearch/cluster/health
GET /api/v1/services/opensearch/cluster/stats
GET /api/v1/services/opensearch/indices

# 事件搜索
GET /api/v1/services/opensearch/events/search
GET /api/v1/services/opensearch/events/time-range
GET /api/v1/services/opensearch/events/high-risk
GET /api/v1/services/opensearch/events/threats
```

## 📊 响应格式

### 标准响应结构
```json
{
  "success": true,
  "data": { ... },
  "message": "操作成功",
  "timestamp": "2025-09-15T06:00:00Z"
}
```

### 错误响应结构
```json
{
  "success": false,
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "参数验证失败",
    "details": { ... }
  },
  "timestamp": "2025-09-15T06:00:00Z"
}
```

### 分页响应结构
```json
{
  "success": true,
  "data": {
    "items": [ ... ],
    "pagination": {
      "page": 1,
      "size": 20,
      "total": 100,
      "pages": 5
    }
  }
}
```

## 🔐 认证和授权

### API Key 认证
```bash
# 请求头
X-API-Key: your-api-key-here
```

### Bearer Token 认证
```bash
# 请求头
Authorization: Bearer your-token-here
```

## 📝 使用示例

### 注册设备
```bash
curl -X POST http://localhost:8080/api/v1/collectors/register \
  -H "Content-Type: application/json" \
  -d '{
    "hostname": "web-server-01",
    "ip_address": "192.168.1.100",
    "os_type": "linux",
    "deployment_type": "agentless"
  }'
```

### 查询最新事件
```bash
curl "http://localhost:8080/api/v1/events/latest?size=10&hours=1"
```

### 获取系统健康状态
```bash
curl http://localhost:8080/api/v1/health | jq '.data.services'
```

---

**SysArmor API 参考** - 完整的 RESTful API 接口文档
