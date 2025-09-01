# SysArmor Manager API 接口文档

## 📋 概述

SysArmor Manager 是系统的控制平面，提供完整的 REST API 接口用于管理 Collector、查询事件、监控系统健康状态等。

**Base URL**: `http://localhost:8080`  
**API Version**: `v1`  
**API Base Path**: `/api/v1`

## 🔐 认证

API 支持以下认证方式：
- **API Key**: 在请求头中添加 `X-API-Key`
- **Bearer Token**: 在请求头中添加 `Authorization: Bearer <token>`

## 📚 API 接口分类

### 1. 系统健康检查 (`/health`)

#### 基础健康检查
```http
GET /health
```
**响应示例**:
```json
{
  "status": "healthy",
  "service": "sysarmor-manager",
  "version": "1.0.0",
  "database": "connected"
}
```

#### 健康状态概览
```http
GET /api/v1/health
```
**功能**: 获取系统健康状态概览，包括所有组件的简要状态信息

#### 综合健康状态
```http
GET /api/v1/health/comprehensive
```
**功能**: 获取包括数据库、OpenSearch、Kafka、Prometheus、Vector 等所有组件的综合健康状态

#### 系统健康摘要
```http
GET /api/v1/health/system
```
**功能**: 获取系统整体健康状态摘要

#### Worker 管理
```http
GET /api/v1/health/workers              # 获取所有 Worker 状态
GET /api/v1/health/workers/healthy      # 获取健康的 Worker 列表
GET /api/v1/health/workers/select       # 选择一个健康的 Worker
GET /api/v1/health/workers/{name}       # 获取特定 Worker 详细信息
GET /api/v1/health/workers/{name}/metrics    # 获取特定 Worker 指标
GET /api/v1/health/workers/{name}/components # 获取特定 Worker 组件状态
```

---

### 2. Collector 管理 (`/collectors`)

#### Collector 注册与管理
```http
POST /api/v1/collectors/register        # 注册新的 Collector
GET  /api/v1/collectors/{id}            # 获取 Collector 状态
GET  /api/v1/collectors                 # 列出所有 Collectors（支持过滤）
POST /api/v1/collectors/{id}/heartbeat  # Collector 心跳
PUT  /api/v1/collectors/{id}/metadata   # 更新 Collector 元数据
DELETE /api/v1/collectors/{id}          # 删除 Collector
POST /api/v1/collectors/{id}/unregister # 注销 Collector（软删除）
```

**注册请求示例**:
```json
{
  "hostname": "web-server-01",
  "ip_address": "192.168.1.100",
  "os_type": "linux",
  "os_version": "Ubuntu 20.04",
  "deployment_type": "agentless",
  "metadata": {
    "group": "web-servers",
    "environment": "production",
    "owner": "ops-team",
    "tags": ["nginx", "api-server"],
    "region": "us-west-2",
    "purpose": "web-service"
  }
}
```

**查询参数支持**:
- `page`: 页码
- `limit`: 每页数量
- `status`: 按状态过滤
- `group`: 按分组过滤
- `environment`: 按环境过滤
- `owner`: 按负责人过滤
- `tags`: 按标签过滤（逗号分隔）
- `sort`: 排序字段
- `order`: 排序方向（asc/desc）

#### 脚本下载
```http
GET /api/v1/scripts/setup-terminal.sh?collector_id={id}      # 下载安装脚本
GET /api/v1/scripts/uninstall-terminal.sh?collector_id={id}  # 下载卸载脚本
```

---

### 3. 事件查询 (`/events`)

#### 通用事件查询
```http
GET  /api/v1/events/query               # 查询事件
GET  /api/v1/events/latest              # 获取最新事件
POST /api/v1/events/search              # 搜索事件
```

**查询参数**:
- `topic`: Kafka Topic 名称（必需）
- `collector_id`: Collector ID 过滤
- `event_type`: 事件类型过滤
- `limit`: 返回数量限制（默认100）
- `latest`: 是否只获取最新事件
- `from_time`: 开始时间（RFC3339格式）
- `to_time`: 结束时间（RFC3339格式）

#### Collector 相关事件
```http
GET /api/v1/events/collectors/{collector_id}  # 查询特定 Collector 的事件
GET /api/v1/events/collectors/topics          # 获取所有 Collector 相关的 Topic
```

#### Topic 管理
```http
GET /api/v1/events/topics                     # 列出所有 Topic
GET /api/v1/events/topics/{topic}/info        # 获取 Topic 信息
```

---

### 4. Kafka 管理 (`/kafka`)

#### 连接与集群管理
```http
GET /api/v1/kafka/test-connection       # 测试 Kafka 连接
GET /api/v1/kafka/clusters              # 获取集群信息
GET /api/v1/kafka/brokers               # 获取 Brokers 信息
GET /api/v1/kafka/brokers/overview      # 获取 Brokers 概览
```

#### Topic 管理
```http
GET    /api/v1/kafka/topics             # 获取 Topics 列表（增强版）
GET    /api/v1/kafka/topics/overview    # 获取 Topics 概览
POST   /api/v1/kafka/topics             # 创建新 Topic
GET    /api/v1/kafka/topics/{topic}     # 获取 Topic 详细信息
DELETE /api/v1/kafka/topics/{topic}     # 删除 Topic
GET    /api/v1/kafka/topics/{topic}/messages  # 获取 Topic 消息
```

**Topics 列表查询参数**:
- `page`: 页码（默认1）
- `limit`: 每页数量（默认20，最大100）
- `search`: 搜索关键词

**获取消息查询参数**:
- `limit`: 消息数量限制（默认10，最大100）
- `partition`: 指定分区
- `offset`: 起始偏移量（数字或'earliest'/'latest'）

#### Topic 配置管理
```http
GET /api/v1/kafka/topics/{topic}/config     # 获取 Topic 配置
PUT /api/v1/kafka/topics/{topic}/config     # 更新 Topic 配置
GET /api/v1/kafka/topics/{topic}/metrics    # 获取 Topic 指标
```

#### Consumer Group 管理
```http
GET /api/v1/kafka/consumer-groups           # 获取 Consumer Groups
GET /api/v1/kafka/consumer-groups/{group}   # 获取特定 Consumer Group 详情
```

---

### 5. OpenSearch 管理 (`/opensearch`)

#### 集群管理
```http
GET /api/v1/opensearch/cluster/health       # 获取集群健康状态
GET /api/v1/opensearch/cluster/stats        # 获取集群统计信息
GET /api/v1/opensearch/indices              # 获取索引列表
```

#### 事件搜索与查询
```http
GET /api/v1/opensearch/events/search        # 搜索安全事件
GET /api/v1/opensearch/events/time-range    # 根据时间范围获取事件
GET /api/v1/opensearch/events/high-risk     # 获取高风险事件
GET /api/v1/opensearch/events/by-source     # 根据数据源获取事件
GET /api/v1/opensearch/events/threats       # 获取威胁事件
GET /api/v1/opensearch/events/recent        # 获取最近事件
GET /api/v1/opensearch/events/aggregations  # 获取事件聚合统计
```

**搜索事件查询参数**:
- `index`: 索引模式（默认 `sysarmor-events-*`）
- `q`: 搜索查询字符串
- `size`: 返回结果数量（默认10，最大100）
- `from`: 结果偏移量（默认0）

**时间范围查询参数**:
- `from`: 开始时间（RFC3339格式）
- `to`: 结束时间（RFC3339格式）
- `size`: 返回结果数量
- `page`: 页码

**高风险事件查询参数**:
- `min_score`: 最小风险评分（必需）
- `size`: 返回结果数量

---

### 6. 文档与监控

#### API 文档
```http
GET /swagger/index.html                 # Swagger API 文档
GET /docs                              # 文档重定向
```

## 🔄 服务间通信

Manager 作为控制平面，与以下服务进行通信：

### 与 Middleware 通信
- **Kafka 管理**: `middleware-kafka:9092`

### 与 Processor 通信
- **Flink JobManager**: `http://processor-jobmanager:8081`
- **Flink Web UI**: 作业监控和管理
- **Flink REST API**: 作业状态、指标、配置查询

### 与 Indexer 通信
- **OpenSearch**: `http://indexer-opensearch:9200`

### 与监控系统通信
- **Prometheus**: 统一获取所有组件的健康状态和指标数据

### 内部服务
- **PostgreSQL**: `manager-postgres:5432`

## 📊 响应格式

### 成功响应
```json
{
  "success": true,
  "data": {
    // 响应数据
  }
}
```

### 错误响应
```json
{
  "success": false,
  "error": "错误描述",
  "message": "详细错误信息"
}
```

### 分页响应
```json
{
  "success": true,
  "data": {
    "items": [...],
    "total": 100,
    "page": 1,
    "limit": 20,
    "total_pages": 5
  }
}
```

## 🚀 使用示例

### 注册 Collector
```bash
curl -X POST http://localhost:8080/api/v1/collectors/register \
  -H "Content-Type: application/json" \
  -d '{
    "hostname": "web-server-01",
    "ip_address": "192.168.1.100",
    "os_type": "linux",
    "os_version": "Ubuntu 20.04",
    "deployment_type": "agentless",
    "metadata": {
      "group": "web-servers",
      "environment": "production"
    }
  }'
```

### 查询事件
```bash
curl "http://localhost:8080/api/v1/events/query?topic=collector-001&limit=50&latest=true"
```

### 获取系统健康状态
```bash
curl http://localhost:8080/api/v1/health/comprehensive
```

### 搜索安全事件
```bash
curl "http://localhost:8080/api/v1/opensearch/events/search?q=failed+login&size=20"
```

## 🔧 配置说明

Manager 服务通过以下环境变量进行配置：

- `MANAGER_PORT`: 服务端口（默认8080）
- `MANAGER_DB_URL`: PostgreSQL 连接字符串
- `KAFKA_BOOTSTRAP_SERVERS`: Kafka 服务器地址
- `OPENSEARCH_URL`: OpenSearch 服务地址
- `OPENSEARCH_USERNAME`: OpenSearch 用户名
- `OPENSEARCH_PASSWORD`: OpenSearch 密码

## 📈 监控与指标

Manager 提供以下监控能力：

1. **健康检查**: 多层次的健康状态检查
2. **服务发现**: 自动发现和选择健康的 Worker
3. **指标收集**: 支持 Prometheus 指标导出
4. **日志管理**: 结构化日志输出

---

**文档版本**: v1.0  
**最后更新**: 2025-08-31  
**维护团队**: SysArmor Team
