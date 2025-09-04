# SysArmor 分布式部署指南

## 📋 概述

本指南详细说明如何将 SysArmor EDR/HIDS 系统进行分布式部署，实现 Middleware 服务部署到远程服务器，Manager 及其他服务部署到本地的架构。

## 🏗️ 分布式架构

### 部署拓扑
```mermaid
graph TB
    subgraph "🌐 远程服务器 (Remote Server)"
        R1[Vector:6000<br/>数据收集]
        R2[Kafka:9092<br/>消息队列]
        R3[Prometheus:9090<br/>监控指标]
        R1 --> R2
        R1 --> R3
        R2 --> R3
    end
    
    subgraph "💻 本地环境 (Local Environment)"
        L1[Manager:8080<br/>控制平面]
        L2[Processor:8081<br/>数据处理]
        L3[Indexer:9200<br/>索引存储]
        L4[(PostgreSQL:5432<br/>数据库)]
        L1 --- L4
        L2 --> L3
    end
    
    subgraph "📱 终端设备 (Collectors)"
        C1[Agent/Collector]
        C2[Agent/Collector]
        C3[Agent/Collector]
    end
    
    C1 -->|TCP:6000| R1
    C2 -->|TCP:6000| R1
    C3 -->|TCP:6000| R1
    
    L1 -.->|管理API| R1
    L1 -.->|监控| R2
    L1 -.->|指标查询| R3
    L2 -.->|消费数据| R2
    
    classDef remote fill:#e8f5e8,stroke:#1b5e20,stroke-width:2px
    classDef local fill:#e1f5fe,stroke:#01579b,stroke-width:2px
    classDef collector fill:#fff3e0,stroke:#e65100,stroke-width:2px
    
    class R1,R2,R3 remote
    class L1,L2,L3,L4 local
    class C1,C2,C3 collector
```

### 网络通信
```mermaid
graph LR
    A[Collectors] -->|TCP:6000| B[Remote Vector]
    B -->|Internal| C[Remote Kafka]
    B -->|Metrics| D[Remote Prometheus]
    
    E[Local Manager] -.->|HTTP API| B
    E -.->|Kafka Admin| C
    E -.->|Metrics Query| D
    
    F[Local Processor] -->|Consumer| C
    F -->|Index| G[Local OpenSearch]
    
    classDef remote fill:#ffebee,stroke:#c62828,stroke-width:2px
    classDef local fill:#e3f2fd,stroke:#1565c0,stroke-width:2px
    
    class B,C,D remote
    class E,F,G local
```

## 🚀 部署方案

### 方案概述
用户的想法是**完全可行**的，这是一个标准的分布式微服务部署模式：

1. **远程服务器**: 部署 Middleware 服务 (Vector + Kafka + Prometheus)
2. **本地环境**: 部署 Manager + Processor + Indexer + PostgreSQL
3. **网络连接**: 通过环境变量配置服务间通信

### 优势分析
- ✅ **数据就近处理**: Middleware 靠近数据源，减少网络延迟
- ✅ **负载分离**: 数据收集和管理控制分离，提高性能
- ✅ **扩展性好**: 可以独立扩展 Middleware 和 Manager
- ✅ **故障隔离**: 远程和本地服务故障互不影响

## 📋 部署步骤

### 第一步: 远程服务器部署 Middleware

#### 1.1 克隆项目到远程服务器
```bash
# 在远程服务器上执行
git clone https://github.com/sysarmor/sysarmor-stack.git
cd sysarmor-stack/sysarmor
```

#### 1.2 配置远程服务器环境变量
```bash
# 复制并编辑环境配置
cp .env.example .env.remote
vim .env.remote
```

**远程服务器 .env.remote 配置**:
```bash
# =============================================================================
# SysArmor EDR 远程Middleware配置
# =============================================================================

# 部署模式
DEPLOYMENT_MODE=distributed
ENVIRONMENT=production

# 网络配置 (重要：设置为远程服务器的实际IP)
SYSARMOR_NETWORK=sysarmor-net
EXTERNAL_IP=YOUR_REMOTE_SERVER_IP    # 替换为实际IP地址

# Middleware服务 (Vector + Kafka + Prometheus)
VECTOR_HOST=middleware-vector
VECTOR_TCP_PORT=6000
VECTOR_API_PORT=8686
VECTOR_METRICS_PORT=9598

# Kafka配置
KAFKA_HOST=middleware-kafka
KAFKA_INTERNAL_PORT=9092
KAFKA_EXTERNAL_HOST=YOUR_REMOTE_SERVER_IP    # 替换为实际IP地址
KAFKA_EXTERNAL_PORT=9094
KAFKA_CONTROLLER_PORT=9093
KAFKA_JMX_PORT=9101
KAFKA_JMX_HOSTNAME=middleware-kafka
KAFKA_BOOTSTRAP_SERVERS=middleware-kafka:9092
KAFKA_CLUSTER_ID=0203ecef23a24688af6901b94ebafa80

# Prometheus配置
PROMETHEUS_HOST=middleware-prometheus
PROMETHEUS_PORT=9090
PROMETHEUS_URL=http://middleware-prometheus:9090

# Worker配置 (供本地Manager连接)
WORKER_URLS=YOUR_REMOTE_SERVER_IP:http://YOUR_REMOTE_SERVER_IP:6000:http://YOUR_REMOTE_SERVER_IP:8686/health
```

#### 1.3 启动远程 Middleware 服务
```bash
# 使用自定义环境文件启动
docker compose --env-file .env.remote up middleware-vector middleware-kafka middleware-prometheus -d

# 验证服务状态
docker compose ps
```

#### 1.4 配置防火墙规则
```bash
# 开放必要端口 (根据实际防火墙工具调整)
sudo ufw allow 6000/tcp    # Vector TCP端口
sudo ufw allow 8686/tcp    # Vector API端口
sudo ufw allow 9094/tcp    # Kafka外部端口
sudo ufw allow 9090/tcp    # Prometheus端口

# 或者使用iptables
sudo iptables -A INPUT -p tcp --dport 6000 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 8686 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 9094 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 9090 -j ACCEPT
```

### 第二步: 本地环境部署 Manager + Processor + Indexer

#### 2.1 配置本地环境变量
```bash
# 在本地环境编辑 .env 文件
cp .env.example .env
vim .env
```

**本地 .env 配置**:
```bash
# =============================================================================
# SysArmor EDR 本地Manager配置
# =============================================================================

# 部署模式
DEPLOYMENT_MODE=distributed
ENVIRONMENT=development

# 网络配置
SYSARMOR_NETWORK=sysarmor-net
EXTERNAL_IP=localhost

# 数据库配置 (本地)
POSTGRES_DB=sysarmor
POSTGRES_USER=sysarmor
POSTGRES_PASSWORD=password

# Manager服务 (本地)
MANAGER_HOST=manager
MANAGER_PORT=8080
MANAGER_DB_URL=postgres://sysarmor:password@manager-postgres:5432/sysarmor?sslmode=disable
MANAGER_LOG_LEVEL=info

# 远程Middleware服务连接配置
VECTOR_HOST=YOUR_REMOTE_SERVER_IP
VECTOR_TCP_PORT=6000
VECTOR_API_PORT=8686
KAFKA_HOST=YOUR_REMOTE_SERVER_IP
KAFKA_EXTERNAL_HOST=YOUR_REMOTE_SERVER_IP
KAFKA_EXTERNAL_PORT=9094
KAFKA_BOOTSTRAP_SERVERS=YOUR_REMOTE_SERVER_IP:9094
PROMETHEUS_HOST=YOUR_REMOTE_SERVER_IP
PROMETHEUS_PORT=9090
PROMETHEUS_URL=http://YOUR_REMOTE_SERVER_IP:9090

# Worker配置 (指向远程服务器)
WORKER_URLS=YOUR_REMOTE_SERVER_IP:http://YOUR_REMOTE_SERVER_IP:6000:http://YOUR_REMOTE_SERVER_IP:8686/health

# Processor服务 (本地)
FLINK_JOBMANAGER_HOST=processor-jobmanager
FLINK_JOBMANAGER_PORT=8081
FLINK_TASKMANAGER_SLOTS=2
FLINK_PARALLELISM=2
THREAT_RULES_PATH=/app/configs/rules.yaml

# Indexer服务 (本地)
OPENSEARCH_HOST=indexer-opensearch
OPENSEARCH_PORT=9200
OPENSEARCH_URL=http://indexer-opensearch:9200
OPENSEARCH_USERNAME=admin
OPENSEARCH_PASSWORD=admin
INDEX_PREFIX=sysarmor-events

# Wazuh集成配置 (可选)
WAZUH_ENABLED=false
```

#### 2.2 启动本地服务
```bash
# 启动除Middleware外的所有服务
docker compose up manager processor-jobmanager processor-taskmanager indexer-opensearch manager-postgres -d

# 验证服务状态
make status
```

#### 2.3 验证分布式连接
```bash
# 测试Manager到远程Middleware的连接
curl http://localhost:8080/api/v1/health/workers

# 测试Kafka连接
curl http://localhost:8080/api/v1/services/kafka/test-connection

# 测试Prometheus连接
curl http://YOUR_REMOTE_SERVER_IP:9090/api/v1/query?query=up
```

## 🔧 配置详解

### 关键配置项说明

#### 远程服务器配置
```bash
# 网络暴露配置
EXTERNAL_IP=YOUR_REMOTE_SERVER_IP           # 远程服务器公网IP
KAFKA_EXTERNAL_HOST=YOUR_REMOTE_SERVER_IP   # Kafka外部访问地址
KAFKA_EXTERNAL_PORT=9094                    # Kafka外部端口

# Worker健康检查配置
WORKER_URLS=YOUR_REMOTE_SERVER_IP:http://YOUR_REMOTE_SERVER_IP:6000:http://YOUR_REMOTE_SERVER_IP:8686/health
```

#### 本地环境配置
```bash
# 指向远程Middleware服务
KAFKA_BOOTSTRAP_SERVERS=YOUR_REMOTE_SERVER_IP:9094
PROMETHEUS_URL=http://YOUR_REMOTE_SERVER_IP:9090
VECTOR_HOST=YOUR_REMOTE_SERVER_IP

# 本地服务配置
MANAGER_PORT=8080                           # 本地Manager端口
OPENSEARCH_URL=http://indexer-opensearch:9200  # 本地OpenSearch
```

### Docker Compose 服务选择

#### 远程服务器启动的服务
```bash
# 只启动Middleware相关服务
docker compose up \
  middleware-vector \
  middleware-kafka \
  middleware-prometheus \
  -d
```

#### 本地环境启动的服务
```bash
# 启动除Middleware外的所有服务
docker compose up \
  manager \
  manager-postgres \
  processor-jobmanager \
  processor-taskmanager \
  indexer-opensearch \
  -d
```

## 🌐 网络配置

### 端口映射规划

#### 远程服务器端口
| 服务 | 内部端口 | 外部端口 | 用途 |
|------|----------|----------|------|
| Vector | 6000 | 6000 | 数据收集 |
| Vector API | 8686 | 8686 | 健康检查 |
| Kafka | 9092 | 9094 | 消息队列 |
| Prometheus | 9090 | 9090 | 监控指标 |

#### 本地环境端口
| 服务 | 端口 | 用途 |
|------|------|------|
| Manager | 8080 | API服务 |
| PostgreSQL | 5432 | 数据库 |
| Flink JobManager | 8081 | 作业管理 |
| OpenSearch | 9200 | 搜索引擎 |

### 安全配置

#### 防火墙规则
```bash
# 远程服务器防火墙配置
sudo ufw allow from YOUR_LOCAL_IP to any port 6000
sudo ufw allow from YOUR_LOCAL_IP to any port 8686
sudo ufw allow from YOUR_LOCAL_IP to any port 9094
sudo ufw allow from YOUR_LOCAL_IP to any port 9090

# 如果需要允许所有Collector连接
sudo ufw allow 6000/tcp
```

#### TLS/SSL 配置 (推荐)
```bash
# 为生产环境配置TLS
KAFKA_SSL_ENABLED=true
KAFKA_SSL_KEYSTORE_LOCATION=/path/to/kafka.server.keystore.jks
KAFKA_SSL_TRUSTSTORE_LOCATION=/path/to/kafka.server.truststore.jks

VECTOR_TLS_ENABLED=true
VECTOR_TLS_CERT_PATH=/path/to/vector.crt
VECTOR_TLS_KEY_PATH=/path/to/vector.key
```

## 📝 部署脚本

### 远程服务器部署脚本
创建 `deploy-remote-middleware.sh`:
```bash
#!/bin/bash
# SysArmor 远程Middleware部署脚本

set -e

echo "🚀 开始部署SysArmor Middleware到远程服务器..."

# 检查环境
if [ -z "$REMOTE_SERVER_IP" ]; then
    echo "❌ 请设置 REMOTE_SERVER_IP 环境变量"
    exit 1
fi

# 克隆项目 (如果不存在)
if [ ! -d "sysarmor-stack" ]; then
    git clone https://github.com/sysarmor/sysarmor-stack.git
fi

cd sysarmor-stack/sysarmor

# 创建远程配置
cat > .env.remote << EOF
DEPLOYMENT_MODE=distributed
ENVIRONMENT=production
EXTERNAL_IP=$REMOTE_SERVER_IP
KAFKA_EXTERNAL_HOST=$REMOTE_SERVER_IP
WORKER_URLS=$REMOTE_SERVER_IP:http://$REMOTE_SERVER_IP:6000:http://$REMOTE_SERVER_IP:8686/health
EOF

# 启动Middleware服务
echo "🔧 启动Middleware服务..."
docker compose --env-file .env.remote up middleware-vector middleware-kafka middleware-prometheus -d

# 等待服务启动
echo "⏳ 等待服务启动..."
sleep 30

# 验证服务状态
echo "🔍 验证服务状态..."
docker compose ps

# 测试端口连通性
echo "🌐 测试端口连通性..."
nc -zv localhost 6000 && echo "✅ Vector端口6000正常"
nc -zv localhost 9094 && echo "✅ Kafka端口9094正常"
nc -zv localhost 9090 && echo "✅ Prometheus端口9090正常"

echo "✅ 远程Middleware部署完成！"
echo "📋 服务访问地址："
echo "   Vector: http://$REMOTE_SERVER_IP:6000"
echo "   Kafka: $REMOTE_SERVER_IP:9094"
echo "   Prometheus: http://$REMOTE_SERVER_IP:9090"
```

### 本地环境部署脚本
创建 `deploy-local-manager.sh`:
```bash
#!/bin/bash
# SysArmor 本地Manager部署脚本

set -e

echo "🚀 开始部署SysArmor Manager到本地环境..."

# 检查环境
if [ -z "$REMOTE_SERVER_IP" ]; then
    echo "❌ 请设置 REMOTE_SERVER_IP 环境变量"
    exit 1
fi

# 进入项目目录
cd sysarmor-stack/sysarmor

# 创建本地配置
cat > .env.local << EOF
DEPLOYMENT_MODE=distributed
ENVIRONMENT=development
EXTERNAL_IP=localhost

# 远程Middleware连接配置
VECTOR_HOST=$REMOTE_SERVER_IP
KAFKA_HOST=$REMOTE_SERVER_IP
KAFKA_EXTERNAL_HOST=$REMOTE_SERVER_IP
KAFKA_EXTERNAL_PORT=9094
KAFKA_BOOTSTRAP_SERVERS=$REMOTE_SERVER_IP:9094
PROMETHEUS_HOST=$REMOTE_SERVER_IP
PROMETHEUS_URL=http://$REMOTE_SERVER_IP:9090
WORKER_URLS=$REMOTE_SERVER_IP:http://$REMOTE_SERVER_IP:6000:http://$REMOTE_SERVER_IP:8686/health

# 本地服务配置
MANAGER_PORT=8080
POSTGRES_DB=sysarmor
POSTGRES_USER=sysarmor
POSTGRES_PASSWORD=password
OPENSEARCH_URL=http://indexer-opensearch:9200
FLINK_JOBMANAGER_PORT=8081
EOF

# 启动本地服务
echo "🔧 启动本地服务..."
docker compose --env-file .env.local up \
  manager \
  manager-postgres \
  processor-jobmanager \
  processor-taskmanager \
  indexer-opensearch \
  -d

# 等待服务启动
echo "⏳ 等待服务启动..."
sleep 30

# 验证服务状态
echo "🔍 验证本地服务状态..."
docker compose ps

# 测试连接
echo "🌐 测试远程连接..."
curl -s http://localhost:8080/health && echo "✅ Manager健康检查正常"
curl -s http://localhost:8080/api/v1/services/kafka/test-connection && echo "✅ 远程Kafka连接正常"

echo "✅ 本地Manager部署完成！"
echo "📋 服务访问地址："
echo "   Manager API: http://localhost:8080"
echo "   API文档: http://localhost:8080/swagger/index.html"
echo "   Flink监控: http://localhost:8081"
echo "   OpenSearch: http://localhost:9200"
```

## 🔧 高级配置

### 自定义 Docker Compose 配置

#### 远程服务器 docker-compose.remote.yml
```yaml
# 远程Middleware专用配置
version: '3.8'

services:
  middleware-vector:
    image: timberio/vector:0.34.0-alpine
    container_name: sysarmor-vector-remote
    ports:
      - "6000:6000"    # 数据收集端口
      - "8686:8686"    # API端口
    volumes:
      - ./services/middleware/configs/vector/vector.toml:/etc/vector/vector.toml:ro
    networks:
      - sysarmor-net
    restart: unless-stopped

  middleware-kafka:
    image: apache/kafka:latest
    container_name: sysarmor-kafka-remote
    ports:
      - "9094:9094"    # 外部访问端口
    environment:
      KAFKA_EXTERNAL_HOST: ${KAFKA_EXTERNAL_HOST}
      KAFKA_EXTERNAL_PORT: ${KAFKA_EXTERNAL_PORT}
    networks:
      - sysarmor-net
    restart: unless-stopped

  middleware-prometheus:
    image: prom/prometheus:v2.45.0
    container_name: sysarmor-prometheus-remote
    ports:
      - "9090:9090"
    volumes:
      - ./services/middleware/configs/monitoring/prometheus/prometheus.yml:/etc/prometheus/prometheus.yml:ro
    networks:
      - sysarmor-net
    restart: unless-stopped

networks:
  sysarmor-net:
    driver: bridge
    name: sysarmor-net
```

#### 本地环境 docker-compose.local.yml
```yaml
# 本地Manager专用配置
version: '3.8'

services:
  manager:
    build:
      context: .
      dockerfile: deployments/docker/manager.Dockerfile
    container_name: sysarmor-manager-local
    ports:
      - "8080:8080"
    environment:
      KAFKA_BOOTSTRAP_SERVERS: ${REMOTE_SERVER_IP}:9094
      PROMETHEUS_URL: http://${REMOTE_SERVER_IP}:9090
    depends_on:
      - manager-postgres
    networks:
      - sysarmor-net
    restart: unless-stopped

  manager-postgres:
    image: postgres:15-alpine
    container_name: sysarmor-postgres-local
    ports:
      - "5432:5432"
    environment:
      POSTGRES_DB: ${POSTGRES_DB}
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
    volumes:
      - postgres_data:/var/lib/postgresql/data
    networks:
      - sysarmor-net
    restart: unless-stopped

  # Processor和Indexer服务...
  # (完整配置参考主docker-compose.yml)

volumes:
  postgres_data:

networks:
  sysarmor-net:
    driver: bridge
    name: sysarmor-net
```

### 使用自定义配置部署
```bash
# 远程服务器
docker compose -f docker-compose.remote.yml --env-file .env.remote up -d

# 本地环境
docker compose -f docker-compose.local.yml --env-file .env.local up -d
```

## 🧪 分布式部署验证

### 连通性测试
```bash
# 1. 测试远程Vector连接
curl http://YOUR_REMOTE_SERVER_IP:8686/health

# 2. 测试远程Kafka连接
curl http://localhost:8080/api/v1/services/kafka/test-connection

# 3. 测试远程Prometheus连接
curl http://YOUR_REMOTE_SERVER_IP:9090/api/v1/query?query=up

# 4. 测试Manager健康状态
curl http://localhost:8080/api/v1/health/comprehensive
```

### 数据流验证
```bash
# 1. 从本地向远程Vector发送数据
echo '{"collector_id":"test-distributed","timestamp":"2025-09-04T20:00:00Z","message":"test distributed deployment"}' | nc YOUR_REMOTE_SERVER_IP 6000

# 2. 通过本地Manager查看Kafka主题
curl http://localhost:8080/api/v1/services/kafka/topics

# 3. 验证数据流到本地Processor
curl http://localhost:8081/jobs
```

### 性能监控
```bash
# 监控远程服务器资源
ssh user@YOUR_REMOTE_SERVER_IP "docker stats --no-stream"

# 监控本地服务资源
docker stats --no-stream

# 网络延迟测试
ping YOUR_REMOTE_SERVER_IP
```

## 🚨 故障排查

### 常见问题和解决方案

#### 1. 连接超时问题
```bash
# 检查防火墙设置
sudo ufw status
sudo iptables -L

# 检查端口监听
netstat -tlnp | grep :6000
netstat -tlnp | grep :9094
```

#### 2. Kafka连接失败
```bash
# 检查Kafka配置
docker exec sysarmor-kafka-remote /opt/kafka/bin/kafka-broker-api-versions.sh --bootstrap-server localhost:9092

# 检查外部端口配置
docker logs sysarmor-kafka-remote | grep "started (kafka.server.KafkaServer)"
```

#### 3. 服务发现问题
```bash
# 检查Manager的Worker配置
curl http://localhost:8080/api/v1/health/workers

# 检查Vector健康状态
curl http://YOUR_REMOTE_SERVER_IP:8686/health
```

### 日志查看
```bash
# 远程服务器日志
ssh user@YOUR_REMOTE_SERVER_IP "cd sysarmor-stack/sysarmor && docker compose logs middleware-vector"

# 本地服务日志
docker compose logs manager
docker compose logs processor-jobmanager
```

## 📊 监控和运维

### 分布式监控配置
```bash
# 配置Prometheus监控远程和本地服务
# 在本地Prometheus配置中添加远程目标
scrape_configs:
  - job_name: 'remote-middleware'
    static_configs:
      - targets: ['YOUR_REMOTE_SERVER_IP:8686']  # Vector metrics
      - targets: ['YOUR_REMOTE_SERVER_IP:9090']  # Remote Prometheus
  
  - job_name: 'local-services'
    static_configs:
      - targets: ['localhost:8080']              # Manager
      - targets: ['localhost:8081']              # Flink
      - targets: ['localhost:9200']              # OpenSearch
```

### 健康检查脚本
创建 `health-check-distributed.sh`:
```bash
#!/bin/bash
# 分布式部署健康检查脚本

echo "🏥 SysArmor分布式部署健康检查"
echo "================================"

# 检查远程Middleware
echo "📡 检查远程Middleware服务..."
curl -s http://$REMOTE_SERVER_IP:8686/health > /dev/null && echo "✅ 远程Vector: 健康" || echo "❌ 远程Vector: 异常"
curl -s http://$REMOTE_SERVER_IP:9090/-/healthy > /dev/null && echo "✅ 远程Prometheus: 健康" || echo "❌ 远程Prometheus: 异常"

# 检查本地服务
echo "💻 检查本地服务..."
curl -s http://localhost:8080/health > /dev/null && echo "✅ 本地Manager: 健康" || echo "❌ 本地Manager: 异常"
curl -s http://localhost:8081/overview > /dev/null && echo "✅ 本地Flink: 健康" || echo "❌ 本地Flink: 异常"
curl -s http://localhost:9200/_cluster/health > /dev/null && echo "✅ 本地OpenSearch: 健康" || echo "❌ 本地OpenSearch: 异常"

# 检查服务间连接
echo "🔗 检查服务间连接..."
curl -s http://localhost:8080/api/v1/services/kafka/test-connection | grep -q "connected.*true" && echo "✅ Kafka连接: 正常" || echo "❌ Kafka连接: 异常"

echo "✅ 健康检查完成"
```

## 🔄 运维操作

### 服务重启
```bash
# 重启远程Middleware
ssh user@YOUR_REMOTE_SERVER_IP "cd sysarmor-stack/sysarmor && docker compose restart middleware-vector middleware-kafka"

# 重启本地Manager
docker compose restart manager
```

### 配置更新
```bash
# 更新远程配置
scp .env.remote user@YOUR_REMOTE_SERVER_IP:~/sysarmor-stack/sysarmor/.env
ssh user@YOUR_REMOTE_SERVER_IP "cd sysarmor-stack/sysarmor && docker compose up -d"

# 更新本地配置
vim .env.local
docker compose --env-file .env.local up -d
```

### 数据备份
```bash
# 备份本地数据库
docker exec sysarmor-postgres-local pg_dump -U sysarmor sysarmor > backup.sql

# 备份远程Kafka数据 (如需要)
ssh user@YOUR_REMOTE_SERVER_IP "docker exec sysarmor-kafka-remote /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list"
```

## 🎯 最佳实践

### 1. 网络优化
- **使用专用网络**: 配置VPN或专线连接
- **启用压缩**: Kafka消息压缩减少网络传输
- **连接池**: 配置合适的连接池大小

### 2. 安全加固
- **TLS加密**: 所有服务间通信启用TLS
- **访问控制**: 使用防火墙限制访问来源
- **认证机制**: 启用Kafka SASL认证

### 3. 监控告警
- **指标收集**: 配置完整的监控指标
- **告警规则**: 设置关键指标告警
- **日志聚合**: 统一日志收集和分析

### 4. 容灾备份
- **数据备份**: 定期备份数据库和配置
- **服务冗余**: 部署多个Middleware实例
- **故障切换**: 配置自动故障转移

## 📚 相关文档

- [SysArmor API参考手册](../sysarmor-api-reference.md)
- [系统更新日志](../../CHANGELOG.md)
- [Kafka集群配置指南](kafka-cluster-setup.md) (待创建)
- [监控配置指南](monitoring-setup.md) (待创建)

---

**SysArmor 分布式部署指南** - 企业级分布式架构部署  
**最后更新**: 2025-09-04  
**适用版本**: v1.0.0+  
**部署模式**: 生产就绪 ✅
