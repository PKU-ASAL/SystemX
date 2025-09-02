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
**功能**: 获取系统健康状态概览，包括所有组件的简要状态信息，替代原有的 `/health` 接口

#### 综合健康状态
```http
GET /api/v1/health/comprehensive
```
**功能**: 获取包括数据库、OpenSearch、Kafka、Prometheus、Vector 等所有组件的综合健康状态

#### 系统健康摘要
```http
GET /api/v1/health/system
```
**功能**: 获取系统整体健康状态摘要，包含汇总指标

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
GET  /api/v1/collectors                 # 列出所有 Collectors（支持过滤和分页）
POST /api/v1/collectors/{id}/heartbeat  # Collector 心跳
PUT  /api/v1/collectors/{id}/metadata   # 更新 Collector 元数据
DELETE /api/v1/collectors/{id}          # 删除 Collector（支持force参数）
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

**注册响应示例**:
```json
{
  "success": true,
  "data": {
    "collector_id": "uuid-string",
    "worker_url": "http://middleware-vector:6000",
    "script_download_url": "/api/v1/scripts/setup-terminal.sh?collector_id=uuid-string"
  }
}
```

**支持的部署类型**:
- `agentless`: 无代理部署（当前支持）
- `sysarmor`: SysArmor Stack 部署（未实现）
- `wazuh`: Wazuh 混合部署（未实现）

**查询参数支持**:
- `page`: 页码（默认1）
- `limit`: 每页数量（默认20，最大100）
- `status`: 按状态过滤
- `group`: 按分组过滤
- `environment`: 按环境过滤
- `owner`: 按负责人过滤
- `tags`: 按标签过滤（逗号分隔）
- `region`: 按区域过滤
- `purpose`: 按用途过滤
- `sort`: 排序字段
- `order`: 排序方向（asc/desc，默认desc）

#### 删除操作说明
- **普通删除**: 将状态设置为 `inactive`，提供卸载脚本链接
- **强制删除** (`force=true`): 永久删除记录并清理相关资源（Kafka Topic等）

#### 脚本下载
```http
GET /api/v1/scripts/setup-terminal.sh?collector_id={id}      # 下载安装脚本
GET /api/v1/scripts/uninstall-terminal.sh?collector_id={id}  # 下载卸载脚本
```

**脚本特性**:
- 基于模板系统生成
- 支持不同部署类型的脚本模板
- 自动包含 collector_id 和 worker 配置
- 文件名格式: `setup-terminal-{collector_id前8位}.sh`

---

### 3. 事件查询 (`/events`) - 当前实现

> **⚠️ 注意**: 当前events接口实现基于Kafka底层消息查询，与services/kafka功能存在重复。建议未来重构为业务层面的安全事件查询接口。

#### 当前已实现的接口
```http
GET  /api/v1/events/query                      # 查询事件（需要topic参数）
GET  /api/v1/events/latest                     # 获取最新事件
POST /api/v1/events/search                     # 搜索事件（支持关键词过滤）
GET  /api/v1/events/collectors/{collector_id}  # 查询特定Collector的事件
GET  /api/v1/events/collectors/topics          # 获取所有Collector相关的Topic
GET  /api/v1/events/topics                     # 列出所有Topic（分类显示）
GET  /api/v1/events/topics/{topic}/info        # 获取Topic信息
```

#### 查询参数（当前实现）
- `topic`: Kafka Topic 名称（query和latest接口必需）
- `collector_id`: Collector ID 过滤
- `event_type`: 事件类型过滤
- `limit`: 返回数量限制（默认100）
- `latest`: 是否只获取最新事件
- `from_time`: 开始时间（RFC3339格式）
- `to_time`: 结束时间（RFC3339格式）

#### 搜索请求体示例（当前实现）
```json
{
  "topic": "sysarmor-agentless-558c01dd",
  "collector_id": "optional-collector-id",
  "event_type": "syslog",
  "keyword": "sudo",
  "limit": 50,
  "from_time": "2025-01-01T00:00:00Z",
  "to_time": "2025-01-01T23:59:59Z",
  "latest": true
}
```

#### Collector Topics 响应示例
```json
{
  "success": true,
  "data": {
    "collector_topics": [
      {
        "topic": "sysarmor-agentless-558c01dd",
        "collector_id": "558c01dd-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
      }
    ],
    "total_collectors": 1,
    "queried_at": "2025-01-01T12:00:00Z"
  }
}
```

#### Topics 列表响应示例
```json
{
  "success": true,
  "data": {
    "collector_topics": ["sysarmor-agentless-558c01dd"],
    "other_topics": ["__consumer_offsets"],
    "total_topics": 2,
    "queried_at": "2025-01-01T12:00:00Z"
  }
}
```

#### 🔮 建议的未来改进

为了更好地服务于EDR/HIDS业务需求，建议将events接口重构为：

**业务导向的安全事件查询**:
```http
GET /api/v1/events/threats                    # 获取威胁事件
GET /api/v1/events/threats/recent             # 获取最近威胁事件
GET /api/v1/events/collectors/{id}/security   # 获取Collector安全事件
POST /api/v1/events/search/advanced           # 高级安全事件搜索
```

**建议的查询参数**:
- `severity`: low, medium, high, critical
- `risk_score_min`: 最小风险评分（0-100）
- `threat_type`: privilege_escalation, file_deletion等
- `event_category`: authentication, process, network等

> **当前状态**: 如需底层Kafka消息查询，建议使用 `/api/v1/services/kafka/topics/{topic}/messages` 接口，功能更完整。

---

### 4. Kafka 管理 (`/services/kafka`)

#### 连接与集群管理
```http
GET /api/v1/services/kafka/test-connection       # 测试 Kafka 连接
GET /api/v1/services/kafka/clusters              # 获取集群信息
GET /api/v1/services/kafka/brokers               # 获取 Brokers 信息
GET /api/v1/services/kafka/brokers/overview      # 获取 Brokers 概览
```

#### Topic 管理
```http
GET    /api/v1/services/kafka/topics             # 获取 Topics 列表（增强版，包含Prometheus指标）
GET    /api/v1/services/kafka/topics/overview    # 获取 Topics 概览统计
POST   /api/v1/services/kafka/topics             # 创建新 Topic
GET    /api/v1/services/kafka/topics/{topic}     # 获取 Topic 详细信息（增强版）
DELETE /api/v1/services/kafka/topics/{topic}     # 删除 Topic（支持force参数）
GET    /api/v1/services/kafka/topics/{topic}/messages  # 获取 Topic 消息
```

**Topics 列表查询参数**:
- `page`: 页码（默认1）
- `limit`: 每页数量（默认20，最大100）
- `search`: 搜索关键词

**创建 Topic 请求示例**:
```json
{
  "name": "new-topic",
  "partitions": 3,
  "replication_factor": 1,
  "configs": {
    "retention.ms": "604800000",
    "segment.ms": "86400000"
  }
}
```

**获取消息查询参数**:
- `limit`: 消息数量限制（默认10，最大100）
- `partition`: 指定分区（-1表示所有分区）
- `offset`: 起始偏移量（数字或'earliest'/'latest'，默认'latest'）

**删除 Topic 参数**:
- `force`: 强制删除，忽略错误（默认false）

#### Topic 配置管理
```http
GET /api/v1/services/kafka/topics/{topic}/config     # 获取 Topic 配置
PUT /api/v1/services/kafka/topics/{topic}/config     # 更新 Topic 配置
GET /api/v1/services/kafka/topics/{topic}/metrics    # 获取 Topic 指标
```

**更新配置请求示例**:
```json
{
  "retention.ms": "1209600000",
  "segment.ms": "172800000",
  "cleanup.policy": "delete"
}
```

#### Consumer Group 管理
```http
GET /api/v1/services/kafka/consumer-groups           # 获取 Consumer Groups
GET /api/v1/services/kafka/consumer-groups/{group}   # 获取特定 Consumer Group 详情
```

---

### 5. Flink 管理 (`/services/flink`)

#### 连接与集群管理
```http
GET /api/v1/services/flink/test-connection       # 测试 Flink 连接
GET /api/v1/services/flink/overview              # 获取集群概览
GET /api/v1/services/flink/config                # 获取 Flink 配置
GET /api/v1/services/flink/health                # 获取集群健康状态
```

#### 作业管理
```http
GET /api/v1/services/flink/jobs                  # 获取作业列表
GET /api/v1/services/flink/jobs/overview         # 获取作业概览
GET /api/v1/services/flink/jobs/{job_id}         # 获取作业详细信息
GET /api/v1/services/flink/jobs/{job_id}/metrics # 获取作业指标
```

#### TaskManager 管理
```http
GET /api/v1/services/flink/taskmanagers          # 获取 TaskManager 信息
GET /api/v1/services/flink/taskmanagers/overview # 获取 TaskManager 概览
```

**集群健康状态响应示例**:
```json
{
  "success": true,
  "data": {
    "healthy": true,
    "status": "healthy",
    "cluster_overview": {
      "slots_total": 4,
      "slots_available": 2,
      "jobs_running": 1,
      "jobs_finished": 0,
      "jobs_cancelled": 0,
      "jobs_failed": 0
    },
    "taskmanager_overview": {
      "total_taskmanagers": 1,
      "healthy_taskmanagers": 1,
      "unhealthy_taskmanagers": 0
    },
    "issues": [],
    "checked_at": "2025-01-01T12:00:00Z"
  }
}
```

---

### 6. OpenSearch 管理 (`/services/opensearch`)

#### 集群管理
```http
GET /api/v1/services/opensearch/cluster/health       # 获取集群健康状态
GET /api/v1/services/opensearch/cluster/stats        # 获取集群统计信息
GET /api/v1/services/opensearch/indices              # 获取索引列表
```

#### 事件搜索与查询
```http
GET /api/v1/services/opensearch/events/search        # 搜索安全事件
GET /api/v1/services/opensearch/events/time-range    # 根据时间范围获取事件
GET /api/v1/services/opensearch/events/high-risk     # 获取高风险事件
GET /api/v1/services/opensearch/events/by-source     # 根据数据源获取事件
GET /api/v1/services/opensearch/events/threats       # 获取威胁事件
GET /api/v1/services/opensearch/events/recent        # 获取最近事件
GET /api/v1/services/opensearch/events/aggregations  # 获取事件聚合统计
```

**搜索事件查询参数**:
- `index`: 索引模式（默认 `sysarmor-events-*`）
- `q`: 搜索查询字符串
- `size`: 返回结果数量（默认10，最大100）
- `from`: 结果偏移量（默认0）

**时间范围查询参数**:
- `from`: 开始时间（RFC3339格式，必需）
- `to`: 结束时间（RFC3339格式，必需）
- `size`: 返回结果数量（默认10，最大100）
- `page`: 页码（默认1）

**高风险事件查询参数**:
- `min_score`: 最小风险评分（必需）
- `size`: 返回结果数量（默认10，最大100）

**数据源查询参数**:
- `source`: 数据源名称（必需）
- `size`: 返回结果数量（默认10，最大100）

**最近事件查询参数**:
- `hours`: 时间范围（小时，默认24小时）
- `size`: 返回结果数量（默认10，最大100）

---

### 7. Prometheus 管理 (`/services/prometheus`)

#### 指标查询
```http
GET /api/v1/services/prometheus/test-connection     # 测试 Prometheus 连接
GET /api/v1/services/prometheus/metrics             # 获取系统指标
GET /api/v1/services/prometheus/query               # 执行 PromQL 查询
GET /api/v1/services/prometheus/targets             # 获取监控目标
```

**查询参数**:
- `query`: PromQL 查询语句
- `time`: 查询时间点
- `start`: 开始时间（范围查询）
- `end`: 结束时间（范围查询）
- `step`: 查询步长

---

### 8. 文档与监控

#### API 文档
```http
GET /swagger/index.html                 # Swagger API 文档
GET /docs                              # 文档重定向
```

## 🔄 服务间通信

Manager 作为控制平面，与以下服务进行通信：

### 与 Middleware 通信
- **Vector API**: `http://middleware-vector:8686` - 健康检查和指标收集
- **Kafka 管理**: `middleware-kafka:9092` - Topic管理、消息查询
- **Prometheus**: `http://middleware-prometheus:9090` - 指标数据收集

### 与 Processor 通信
- **Flink JobManager**: `http://processor-jobmanager:8081`
- **Flink REST API**: 作业状态、指标、配置查询
- **Flink Web UI**: 作业监控和管理界面

### 与 Indexer 通信
- **OpenSearch**: `http://indexer-opensearch:9200`
- **OpenSearch API**: 集群管理、索引操作、事件搜索

### Worker 健康检查
- **动态 Worker 发现**: 基于环境变量 `WORKER_URLS` 配置
- **负载均衡**: 自动选择健康的 Worker 进行 Collector 注册
- **健康监控**: 定期检查 Worker 状态和组件健康度

### 内部服务
- **PostgreSQL**: `manager-postgres:5432` - Collector 数据持久化
- **数据库自动初始化**: 支持自动迁移和表结构管理

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
      "environment": "production",
      "owner": "ops-team",
      "tags": ["nginx", "api-server"]
    }
  }'
```

### 查询事件（当前实现）
```bash
# 查询特定 Topic 的事件（需要指定topic）
curl "http://localhost:8080/api/v1/events/query?topic=sysarmor-agentless-558c01dd&limit=50&latest=true"

# 查询特定 Collector 的事件
curl "http://localhost:8080/api/v1/events/collectors/558c01dd-xxxx-xxxx-xxxx-xxxxxxxxxxxx?limit=100&event_type=syslog"

# 获取所有 Collector 相关的 Topics
curl "http://localhost:8080/api/v1/events/collectors/topics"

# 搜索包含关键词的事件
curl -X POST http://localhost:8080/api/v1/events/search \
  -H "Content-Type: application/json" \
  -d '{
    "topic": "sysarmor-agentless-558c01dd",
    "keyword": "sudo",
    "limit": 50,
    "latest": true
  }'

# 获取最新事件
curl "http://localhost:8080/api/v1/events/latest?topic=sysarmor-agentless-558c01dd&limit=50"

# 列出所有 Topics（分类显示）
curl "http://localhost:8080/api/v1/events/topics"

# 获取 Topic 信息
curl "http://localhost:8080/api/v1/events/topics/sysarmor-agentless-558c01dd/info"
```

> **注意**: 当前events接口基于Kafka底层实现。如需更完整的Kafka管理功能，建议使用services/kafka接口。

### 获取系统健康状态
```bash
# 获取综合健康状态
curl http://localhost:8080/api/v1/health/comprehensive

# 获取系统健康摘要
curl http://localhost:8080/api/v1/health/system

# 选择健康的 Worker
curl http://localhost:8080/api/v1/health/workers/select
```

### Kafka 管理操作
```bash
# 测试 Kafka 连接
curl http://localhost:8080/api/v1/services/kafka/test-connection

# 获取 Topics 概览
curl http://localhost:8080/api/v1/services/kafka/topics/overview

# 创建新 Topic
curl -X POST http://localhost:8080/api/v1/services/kafka/topics \
  -H "Content-Type: application/json" \
  -d '{
    "name": "test-topic",
    "partitions": 3,
    "replication_factor": 1
  }'

# 获取 Topic 消息
curl "http://localhost:8080/api/v1/services/kafka/topics/sysarmor-agentless-558c01dd/messages?limit=10&offset=latest"
```

### Flink 作业管理
```bash
# 测试 Flink 连接
curl http://localhost:8080/api/v1/services/flink/test-connection

# 获取集群概览
curl http://localhost:8080/api/v1/services/flink/overview

# 获取作业列表
curl http://localhost:8080/api/v1/services/flink/jobs

# 获取集群健康状态
curl http://localhost:8080/api/v1/services/flink/health
```

### OpenSearch 事件搜索
```bash
# 搜索安全事件
curl "http://localhost:8080/api/v1/services/opensearch/events/search?q=sudo&size=20"

# 获取高风险事件
curl "http://localhost:8080/api/v1/services/opensearch/events/high-risk?min_score=80&size=10"

# 获取最近24小时的事件
curl "http://localhost:8080/api/v1/services/opensearch/events/recent?hours=24&size=50"

# 根据时间范围获取事件
curl "http://localhost:8080/api/v1/services/opensearch/events/time-range?from=2025-01-01T00:00:00Z&to=2025-01-01T23:59:59Z&size=100"
```

### Prometheus 指标查询
```bash
# 测试 Prometheus 连接
curl http://localhost:8080/api/v1/services/prometheus/test-connection

# 获取系统指标
curl http://localhost:8080/api/v1/services/prometheus/metrics

# 执行 PromQL 查询
curl "http://localhost:8080/api/v1/services/prometheus/query?query=up&time=2025-01-01T12:00:00Z"

# 获取监控目标
curl http://localhost:8080/api/v1/services/prometheus/targets
```

### Collector 管理操作
```bash
# 列出所有 Collectors（支持过滤）
curl "http://localhost:8080/api/v1/collectors?page=1&limit=20&environment=production&tags=nginx,api-server"

# 更新 Collector 元数据
curl -X PUT http://localhost:8080/api/v1/collectors/{collector_id}/metadata \
  -H "Content-Type: application/json" \
  -d '{
    "metadata": {
      "group": "updated-group",
      "environment": "staging",
      "tags": ["updated-tag"]
    }
  }'

# 删除 Collector（软删除）
curl -X DELETE http://localhost:8080/api/v1/collectors/{collector_id}

# 强制删除 Collector
curl -X DELETE "http://localhost:8080/api/v1/collectors/{collector_id}?force=true"

# 下载安装脚本
curl -O "http://localhost:8080/api/v1/scripts/setup-terminal.sh?collector_id={collector_id}"
```

## 🔧 配置说明

Manager 服务通过以下环境变量进行配置：

### 核心服务配置
- `MANAGER_PORT`: 服务端口（默认8080）
- `MANAGER_LOG_LEVEL`: 日志级别（默认info）
- `MANAGER_DB_URL`: PostgreSQL 连接字符串
- `DATABASE_URL`: 数据库连接URL（别名）

### 外部服务配置
- `KAFKA_BOOTSTRAP_SERVERS`: Kafka 服务器地址
- `OPENSEARCH_URL`: OpenSearch 服务地址
- `OPENSEARCH_USERNAME`: OpenSearch 用户名
- `OPENSEARCH_PASSWORD`: OpenSearch 密码
- `PROMETHEUS_URL`: Prometheus 服务地址
- `WORKER_URLS`: Worker 服务地址列表

### 服务发现配置
- `MANAGER_HOST`: Manager 服务主机名
- `FLINK_JOBMANAGER_HOST`: Flink JobManager 主机名
- `FLINK_JOBMANAGER_PORT`: Flink JobManager 端口

### 配置示例
```bash
# .env 文件示例
MANAGER_PORT=8080
MANAGER_LOG_LEVEL=info
MANAGER_DB_URL=postgres://sysarmor:password@manager-postgres:5432/sysarmor?sslmode=disable
KAFKA_BOOTSTRAP_SERVERS=middleware-kafka:9092
OPENSEARCH_URL=http://indexer-opensearch:9200
OPENSEARCH_USERNAME=admin
OPENSEARCH_PASSWORD=admin
PROMETHEUS_URL=http://middleware-prometheus:9090
WORKER_URLS=middleware-vector:http://middleware-vector:6000:http://middleware-vector:8686/health
```

## 📈 监控与指标

Manager 提供以下监控能力：

### 健康检查体系
1. **多层次健康检查**: 
   - 基础服务健康检查 (`/health`)
   - 综合系统健康检查 (`/api/v1/health/comprehensive`)
   - Worker 健康状态监控
   - 外部服务连接检查

2. **服务发现与负载均衡**:
   - 动态 Worker 发现和健康检查
   - 自动选择健康的 Worker 进行 Collector 注册
   - 支持多 Worker 负载均衡

3. **指标收集与监控**:
   - Prometheus 指标集成
   - Kafka 集群监控（通过 JMX Exporter）
   - Flink 作业状态和性能指标
   - OpenSearch 集群健康状态

4. **日志管理**:
   - 结构化日志输出
   - 请求/响应日志记录
   - 错误和异常跟踪
   - 操作审计日志

### 监控端点汇总
```bash
# 服务健康检查
GET /health                              # 基础健康检查
GET /api/v1/health                       # 健康状态概览
GET /api/v1/health/comprehensive         # 综合健康状态
GET /api/v1/health/system               # 系统健康摘要

# 外部服务连接测试
GET /api/v1/services/kafka/test-connection       # Kafka 连接测试
GET /api/v1/services/flink/test-connection       # Flink 连接测试
GET /api/v1/services/opensearch/cluster/health   # OpenSearch 健康检查
GET /api/v1/services/prometheus/test-connection  # Prometheus 连接测试

# 指标和统计
GET /api/v1/services/kafka/brokers/overview      # Kafka Brokers 概览
GET /api/v1/services/kafka/topics/overview       # Kafka Topics 概览
GET /api/v1/services/flink/overview              # Flink 集群概览
GET /api/v1/services/opensearch/cluster/stats    # OpenSearch 集群统计
GET /api/v1/services/prometheus/metrics          # Prometheus 指标
```

## 🚨 错误处理

### 常见错误码
- `400 Bad Request`: 请求参数错误或格式不正确
- `401 Unauthorized`: 认证失败或缺少认证信息
- `404 Not Found`: 请求的资源不存在
- `409 Conflict`: 资源冲突（如 Topic 已存在）
- `500 Internal Server Error`: 服务器内部错误
- `503 Service Unavailable`: 服务不可用或依赖服务异常

### 错误响应格式
```json
{
  "success": false,
  "error": "简短错误描述",
  "message": "详细错误信息",
  "code": "ERROR_CODE",
  "timestamp": "2025-01-01T12:00:00Z"
}
```

### 故障排查建议
1. **连接问题**: 检查网络连接和服务状态
2. **认证问题**: 验证 API Key 或 Bearer Token
3. **参数错误**: 检查请求参数格式和必需字段
4. **资源不存在**: 确认资源 ID 或名称正确
5. **服务依赖**: 检查 Kafka、OpenSearch、Flink 等服务状态

## 🔄 版本兼容性

### API 版本策略
- **当前版本**: v1
- **向后兼容**: 保证同一主版本内的向后兼容性
- **废弃通知**: 废弃的接口会提前通知并保留一个版本周期
- **版本升级**: 主版本升级时会提供迁移指南

### 支持的客户端
- **HTTP 客户端**: 任何支持 HTTP/1.1 的客户端
- **认证方式**: API Key、Bearer Token
- **内容类型**: `application/json`
- **字符编码**: UTF-8

---

**文档版本**: v1.2  
**最后更新**: 2025-01-01  
**维护团队**: SysArmor Team  
**技术支持**: support@sysarmor.com
