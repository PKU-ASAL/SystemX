# SysArmor EDR/HIDS 系统

## 🎯 项目概述

SysArmor 是一个现代化的端点检测与响应(EDR/HIDS)系统，采用 **Monorepo + 微服务架构**，支持 agentless 数据采集、实时威胁检测和 Wazuh 生态集成，提供统一的 SIEM/EDR 管理平台。

## 🏗️ 系统架构

### 核心架构
```mermaid
graph TB
    subgraph "🎯 控制平面 (apps/)"
        A1[Manager API<br/>Go + Gin]
        A2[Web UI<br/>预留]
        A1 -.-> A2
    end
    
    subgraph "🔧 数据平面 (services/)"
        S1[Middleware<br/>Vector + Kafka]
        S2[Processor<br/>Flink]
        S3[Indexer<br/>OpenSearch]
        S1 --> S2
        S2 --> S3
    end
    
    A1 <==> S1
    A1 <==> S2
    A1 <==> S3
    
    classDef controlPlane fill:#e1f5fe,stroke:#01579b,stroke-width:2px
    classDef dataPlane fill:#f3e5f5,stroke:#4a148c,stroke-width:2px
    
    class A1,A2 controlPlane
    class S1,S2,S3 dataPlane
```

### 四大核心模块
- **Manager** (Go): 控制平面 - 设备管理、API 服务、健康监控、Wazuh集成
- **Middleware** (Vector+Kafka): 数据中间件 - 数据收集、消息队列、监控
- **Processor** (Flink): 数据处理 - 实时流处理、威胁检测、格式转换
- **Indexer** (OpenSearch): 索引存储 - 数据索引、搜索服务、事件查询

### 数据流向
```mermaid
graph LR
    A[Agent/Collector] -->|TCP:6000| B[Vector]
    B -->|消息队列| C[Kafka:9092]
    C -->|流处理| D[Flink:8081]
    D -->|索引存储| E[OpenSearch:9200]
    
    F[Manager:8080] -.->|管理| B
    F -.->|监控| C
    F -.->|作业管理| D
    F -.->|查询| E
    
    G[Prometheus:9090] -.->|指标收集| B
    G -.->|指标收集| C
    G -.->|指标收集| D
    G -.->|指标收集| E
    
    classDef dataFlow fill:#e8f5e8,stroke:#1b5e20,stroke-width:2px
    classDef management fill:#fff3e0,stroke:#e65100,stroke-width:2px
    classDef monitoring fill:#fce4ec,stroke:#880e4f,stroke-width:2px
    
    class A,B,C,D,E dataFlow
    class F management
    class G monitoring
```

## 📁 项目结构

```
sysarmor/
├── apps/                    # 🎯 应用层
│   ├── manager/            # 控制平面管理应用
│   └── ui/                 # Web UI 应用 (预留)
├── services/               # 🔧 服务层 (数据平面)
│   ├── middleware/         # 数据中间件 (Vector + Kafka)
│   ├── processor/          # 数据处理 (Flink)
│   └── indexer/           # 索引存储 (OpenSearch)
├── shared/                 # 🤝 共享层
│   ├── config/            # 共享配置库
│   ├── templates/         # 共享模板
│   └── migrations/        # 数据库迁移
├── deployments/           # 🚀 部署配置
│   ├── docker/           # Dockerfile 集中管理
│   └── compose/          # Docker Compose 配置
└── docs/                  # 📚 文档
```

## 🚀 快速开始

### 1. 一键启动
```bash
# 克隆项目
git clone https://git.pku.edu.cn/oslab/sysarmor.git
cd sysarmor

# 启动所有服务
make up
# 或者: docker compose up -d

# 验证部署
make health
```

### 2. 访问服务
- **Manager API**: http://localhost:8080
- **API 文档**: http://localhost:8080/swagger/index.html
- **Flink 监控**: http://localhost:8081
- **OpenSearch**: http://localhost:9200
- **Prometheus**: http://localhost:9090

### 3. 注册设备
```bash
# 注册新设备
curl -X POST http://localhost:8080/api/v1/collectors/register \
  -H "Content-Type: application/json" \
  -d '{
    "hostname": "web-server-01",
    "ip_address": "192.168.1.100",
    "os_type": "linux",
    "deployment_type": "agentless"
  }'

# 下载安装脚本
curl "http://localhost:8080/api/v1/resources/scripts/agentless/setup-terminal.sh?collector_id=xxx" -o install.sh
```

## ⚙️ 配置管理

### 环境配置
```bash
# 复制配置模板
cp .env.example .env

# 编辑配置 (12-Factor App 模式)
vim .env
```

### 核心配置项

#### 部署和网络配置
```bash
# 部署模式
DEPLOYMENT_MODE=single-node          # single-node, distributed
ENVIRONMENT=development              # development, production

# 网络配置
SYSARMOR_NETWORK=sysarmor-net
EXTERNAL_IP=localhost
```

#### 数据库配置
```bash
# PostgreSQL数据库
POSTGRES_DB=sysarmor
POSTGRES_USER=sysarmor
POSTGRES_PASSWORD=password
```

#### Manager服务 (控制平面)
```bash
MANAGER_HOST=manager
MANAGER_PORT=8080
MANAGER_DB_URL=postgres://sysarmor:password@manager-postgres:5432/sysarmor?sslmode=disable
MANAGER_LOG_LEVEL=info
```

#### Middleware服务 (数据中间件)
```bash
# Vector配置
VECTOR_HOST=middleware-vector
VECTOR_TCP_PORT=6000
VECTOR_API_PORT=8686
VECTOR_METRICS_PORT=9598

# Kafka配置
KAFKA_HOST=middleware-kafka
KAFKA_INTERNAL_PORT=9092
KAFKA_EXTERNAL_PORT=9094
KAFKA_BOOTSTRAP_SERVERS=middleware-kafka:9092
KAFKA_CLUSTER_ID=0203ecef23a24688af6901b94ebafa80

# Prometheus监控
PROMETHEUS_HOST=middleware-prometheus
PROMETHEUS_PORT=9090
PROMETHEUS_URL=http://middleware-prometheus:9090
```

#### Processor服务 (数据处理)
```bash
FLINK_JOBMANAGER_HOST=processor-jobmanager
FLINK_JOBMANAGER_PORT=8081
FLINK_TASKMANAGER_SLOTS=2
FLINK_PARALLELISM=2
THREAT_RULES_PATH=/app/configs/rules.yaml
```

#### Indexer服务 (索引存储)
```bash
OPENSEARCH_HOST=indexer-opensearch
OPENSEARCH_PORT=9200
OPENSEARCH_URL=http://indexer-opensearch:9200
OPENSEARCH_USERNAME=admin
OPENSEARCH_PASSWORD=admin
INDEX_PREFIX=sysarmor-events
```

#### Wazuh集成配置
```bash
# Wazuh功能开关
WAZUH_ENABLED=false

# Wazuh Manager
WAZUH_MANAGER_HOST=wazuh-manager
WAZUH_MANAGER_PORT=55000
WAZUH_MANAGER_USERNAME=wazuh
WAZUH_MANAGER_PASSWORD=wazuh
WAZUH_MANAGER_TLS=true
WAZUH_MANAGER_TLS_VERIFY=false

# Wazuh Indexer
WAZUH_INDEXER_HOST=wazuh-indexer
WAZUH_INDEXER_PORT=9200
WAZUH_INDEXER_USERNAME=admin
WAZUH_INDEXER_PASSWORD=admin
WAZUH_INDEXER_TLS=true
WAZUH_INDEXER_TLS_VERIFY=false

# Wazuh配置文件路径
WAZUH_CONFIG_PATH=/app/configs/wazuh.yaml
```

## 🔧 管理命令

### 服务管理
```bash
make up          # 启动所有服务
make down        # 停止所有服务
make restart     # 重启所有服务
make status      # 查看服务状态
make logs        # 查看日志
make health      # 健康检查
```

### 开发工具
```bash
make build       # 构建所有组件
make docs        # 生成所有文档
make docs-swagger # 生成Swagger API文档
make test        # 运行测试
make clean       # 清理资源
```

## 🌐 API 接口

### 核心业务 API
- **设备管理**: `/api/v1/collectors/*`
- **安全事件**: `/api/v1/events/*`
- **系统监控**: `/api/v1/health/*`
- **资源管理**: `/api/v1/resources/*`

### 服务管理 API
- **Kafka 管理**: `/api/v1/services/kafka/*`
- **Flink 管理**: `/api/v1/services/flink/*`
- **OpenSearch 管理**: `/api/v1/services/opensearch/*`

### Wazuh 集成 API
- **配置管理**: `/api/v1/services/wazuh/config/*`
- **Manager API**: `/api/v1/services/wazuh/manager/*`
- **Agent 管理**: `/api/v1/services/wazuh/agents/*`
- **组管理**: `/api/v1/services/wazuh/groups/*`
- **告警查询**: `/api/v1/services/wazuh/alerts/*`
- **Indexer API**: `/api/v1/services/wazuh/indexer/*`

## 🎯 核心特性

### ✅ **实时威胁检测**
- 基于 Flink 的毫秒级威胁检测
- 支持权限提升、命令注入、网络扫描等威胁类型
- 动态风险评分 (0-100) 和严重程度分级

### ✅ **Agentless 部署**
- 无需在目标主机安装 Agent
- 基于 rsyslog 和 auditd 的数据采集
- 自动生成安装/卸载脚本

### ✅ **数据格式转换**
- 实时 auditd 到 sysdig 格式转换
- 支持 NODLINK 算法标准
- 智能进程树重建

### ✅ **Wazuh 生态集成**
- 完整的 Wazuh Manager 和 Indexer 集成
- 30+ 个 Wazuh API 端点支持
- 统一的 SIEM/EDR 管理平台
- 动态配置管理和热重载

### ✅ **统一管理**
- Monorepo 架构，统一代码管理
- 完整的 REST API
- 一键部署和监控

## 🔍 故障排查

```bash
# 检查服务状态
make status

# 查看特定服务日志
docker compose logs manager
docker compose logs middleware-kafka

# 健康检查
make health

# 重启特定服务
docker compose restart manager
```

## 🧪 测试和验证

### 🔄 自动化测试套件
```bash
# 完整测试套件
make test                    # 运行所有测试
make test-manager           # Manager应用单元测试
make test-services          # 服务集成测试
```

**测试覆盖范围**:
- ✅ **单元测试**: Go代码单元测试
- ✅ **集成测试**: 服务间集成验证
- ✅ **API测试**: RESTful API功能测试

### 🌊 数据流验证测试
```bash
# 端到端数据流测试
./tests/test-auditd-data-flow.sh
```

**测试流程**:
1. **Vector服务检查** - 验证数据收集服务状态
2. **数据注入测试** - 发送模拟auditd数据
3. **Kafka主题验证** - 确认主题自动创建
4. **数据路由确认** - 验证数据正确流转

**预期结果**:
```
✅ Vector服务健康
✅ 数据成功注入
✅ Kafka主题已创建: sysarmor-agentless-12345678
✅ 数据正确路由到Kafka
```

### 🛡️ Wazuh集成测试
```bash
# Wazuh功能集成测试
./tests/test-hfw-wazuh-integration.sh
```

**测试模块**:
- **配置管理**: Wazuh配置CRUD操作
- **连接测试**: Manager和Indexer连通性
- **Agent管理**: 代理生命周期管理
- **告警查询**: 搜索和统计功能
- **索引管理**: Indexer索引操作

**测试输出示例**:
```
=== Wazuh配置管理测试 ===
[✅] GET /api/v1/wazuh/config - Status: 200
[✅] PUT /api/v1/wazuh/config - Status: 200

=== Wazuh Manager API测试 ===
[✅] GET /api/v1/wazuh/manager/info - Status: 200
[✅] GET /api/v1/wazuh/agents - Status: 200
```

### 🔧 手动验证测试

#### 数据注入测试
```bash
# 1. 向Vector发送测试数据
echo '{
  "collector_id": "12345678-abcd-efgh-ijkl-123456789012",
  "timestamp": "2025-09-04T15:30:00Z",
  "host": "test-host",
  "source": "auditd",
  "message": "type=SYSCALL msg=audit(1693420800.123:456): arch=c000003e syscall=2 success=yes exit=3",
  "event_type": "audit",
  "severity": "info",
  "tags": ["audit", "syscall"]
}' | nc localhost 6000
```

#### Kafka消息验证
```bash
# 2. 消费Kafka消息 (禁用JMX避免端口冲突)
docker exec -e KAFKA_OPTS= sysarmor-kafka-1 \
  /opt/kafka/bin/kafka-console-consumer.sh \
  --bootstrap-server localhost:9092 \
  --topic sysarmor-agentless-12345678 \
  --from-beginning
```

#### API功能测试
```bash
# 3. 系统健康检查
curl -s http://localhost:8080/health | jq .

# 4. Collector管理测试
curl -s -H "Accept: application/json" \
     http://localhost:8080/api/v1/collectors | jq .

# 5. Wazuh配置测试
curl -s -H "Accept: application/json" \
     http://localhost:8080/api/v1/wazuh/config | jq .

# 6. Kafka连接测试
curl -s -H "Accept: application/json" \
     http://localhost:8080/api/v1/services/kafka/test-connection | jq .

# 7. Flink集群测试
curl -s -H "Accept: application/json" \
     http://localhost:8080/api/v1/services/flink/overview | jq .

# 8. OpenSearch健康测试
curl -s -H "Accept: application/json" \
     http://localhost:8080/api/v1/services/opensearch/cluster/health | jq .
```

### 📊 性能基准测试
```bash
# API响应时间测试
time curl -s http://localhost:8080/api/v1/collectors > /dev/null

# 并发测试 (使用ab工具)
ab -n 100 -c 10 http://localhost:8080/health

# 内存和CPU监控
docker stats sysarmor-manager-1
```

### 🔍 集成验证清单
- [ ] **服务启动**: 所有容器正常运行
- [ ] **健康检查**: 所有组件状态健康
- [ ] **API响应**: 所有端点正常响应
- [ ] **数据流**: 端到端数据流通畅
- [ ] **Wazuh集成**: Wazuh API功能正常
- [ ] **错误处理**: 错误响应格式正确
- [ ] **文档访问**: Swagger UI正常显示

## 📚 文档

### 核心文档
- [SysArmor API参考手册](docs/sysarmor-api-reference.md) - 🆕 完整系统API手册
- [Manager API参考](docs/manager-api-reference.md) - Manager服务API文档
- [v0.1功能特性](docs/v0.1-release-features.md) - 版本功能说明
- [系统更新日志](CHANGELOG.md) - 版本历史和功能迁移记录

### 部署指南
- [部署指南总览](docs/deployment/README.md) - 各种部署方案对比
- [分布式部署指南](docs/deployment/distributed-deployment.md) - 🆕 完整分布式架构部署
- [快速分布式部署](docs/deployment/quick-distributed-setup.md) - 🆕 5分钟快速部署

## 🚀 开发指南

### 本地开发
```bash
# 进入 Manager 应用
cd apps/manager

# 本地运行
go run main.go

# 构建
go build -o manager main.go
```

### 构建镜像
```bash
# 构建 Manager 镜像
docker build -f deployments/docker/manager.Dockerfile -t sysarmor/manager:latest .
```

---

**SysArmor EDR/HIDS** - 现代化端点检测与响应系统

**🔗 快速开始**: `git clone https://git.pku.edu.cn/oslab/sysarmor.git && cd sysarmor && make up`  
**📖 架构文档**: [MONOREPO_DESIGN.md](MONOREPO_DESIGN.md)  
**🐛 问题反馈**: https://git.pku.edu.cn/oslab/sysarmor/-/issues
