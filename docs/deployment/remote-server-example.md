# 远程服务器部署示例 - 49.232.13.155

## 📋 场景说明

本示例演示如何在远程服务器 `49.232.13.155` 上部署 Middleware 服务，并配置正确的 Kafka 外部访问地址。

## 🔧 关键配置要点

### Kafka 外部访问配置
Kafka 需要正确的 `KAFKA_EXTERNAL_HOST` 配置，让外部客户端（本地Manager、Processor等）能够连接：

```bash
# 错误配置 (外部客户端无法连接)
KAFKA_EXTERNAL_HOST=localhost
KAFKA_EXTERNAL_HOST=162.105.126.246  # 默认示例IP

# 正确配置 (使用实际服务器IP)
KAFKA_EXTERNAL_HOST=49.232.13.155
```

## 🚀 部署步骤

### 第一步: 远程服务器配置

#### 1.1 SSH到远程服务器
```bash
ssh user@49.232.13.155
```

#### 1.2 克隆项目
```bash
git clone https://github.com/sysarmor/sysarmor-stack.git
cd sysarmor-stack/sysarmor
```

#### 1.3 配置环境变量
```bash
# 复制配置模板
cp .env.example .env

# 编辑配置文件
vim .env
```

**关键配置修改**:
```bash
# =============================================================================
# 远程服务器 49.232.13.155 配置
# =============================================================================

# 部署模式
DEPLOYMENT_MODE=distributed
ENVIRONMENT=production

# 网络配置 (重要!)
SYSARMOR_NETWORK=sysarmor-net
EXTERNAL_IP=49.232.13.155

# Kafka配置 (关键!)
KAFKA_HOST=middleware-kafka
KAFKA_INTERNAL_PORT=9092
KAFKA_EXTERNAL_HOST=49.232.13.155    # 必须设置为实际服务器IP
KAFKA_EXTERNAL_PORT=9094
KAFKA_CONTROLLER_PORT=9093
KAFKA_BOOTSTRAP_SERVERS=middleware-kafka:9092

# Vector配置
VECTOR_HOST=middleware-vector
VECTOR_TCP_PORT=6000
VECTOR_API_PORT=8686

# Prometheus配置
PROMETHEUS_HOST=middleware-prometheus
PROMETHEUS_PORT=9090

# Worker配置 (供外部Manager连接)
WORKER_URLS=49.232.13.155:http://49.232.13.155:6000:http://49.232.13.155:8686/health
```

#### 1.4 启动Middleware服务
```bash
# 使用智能检查的make命令
make up middleware
```

**预期输出**:
```
🚀 启动SysArmor EDR服务...
📡 启动Middleware服务...
✅ Middleware启动完成: Vector:6000, Kafka:9092, Prometheus:9090
📋 外部连接地址: 49.232.13.155:9094 (Kafka)
```

#### 1.5 配置防火墙
```bash
# 开放必要端口
sudo ufw allow 6000/tcp    # Vector数据收集
sudo ufw allow 8686/tcp    # Vector API
sudo ufw allow 9094/tcp    # Kafka外部端口
sudo ufw allow 9090/tcp    # Prometheus
```

### 第二步: 本地环境配置

#### 2.1 配置本地.env文件
```bash
# 在本地环境编辑.env
vim .env
```

**本地配置**:
```bash
# =============================================================================
# 本地环境配置 - 连接到远程Middleware
# =============================================================================

# 部署模式
DEPLOYMENT_MODE=distributed
ENVIRONMENT=development

# 网络配置
EXTERNAL_IP=localhost

# 远程Middleware连接配置
VECTOR_HOST=49.232.13.155
KAFKA_HOST=49.232.13.155
KAFKA_EXTERNAL_HOST=49.232.13.155
KAFKA_EXTERNAL_PORT=9094
KAFKA_BOOTSTRAP_SERVERS=49.232.13.155:9094    # 指向远程Kafka
PROMETHEUS_HOST=49.232.13.155
PROMETHEUS_URL=http://49.232.13.155:9090

# Worker配置 (指向远程Vector)
WORKER_URLS=49.232.13.155:http://49.232.13.155:6000:http://49.232.13.155:8686/health

# 本地服务配置
MANAGER_HOST=manager
MANAGER_PORT=8080
POSTGRES_DB=sysarmor
POSTGRES_USER=sysarmor
POSTGRES_PASSWORD=password
OPENSEARCH_HOST=indexer-opensearch
OPENSEARCH_PORT=9200
OPENSEARCH_URL=http://indexer-opensearch:9200
FLINK_JOBMANAGER_HOST=processor-jobmanager
FLINK_JOBMANAGER_PORT=8081
```

#### 2.2 启动本地服务
```bash
# 启动Manager
make up manager

# 启动Processor
make up processor

# 启动Indexer
make up indexer
```

## 🧪 验证部署

### 连通性测试
```bash
# 1. 测试远程Vector连接
curl http://49.232.13.155:8686/health

# 2. 测试远程Kafka连接 (从本地)
curl http://localhost:8080/api/v1/services/kafka/test-connection

# 3. 测试远程Prometheus
curl http://49.232.13.155:9090/api/v1/query?query=up
```

### 数据流测试
```bash
# 1. 从本地向远程Vector发送数据
echo '{"collector_id":"test-remote","timestamp":"2025-09-04T21:00:00Z","message":"test from local to remote"}' | nc 49.232.13.155 6000

# 2. 通过本地Manager查看Kafka主题
curl http://localhost:8080/api/v1/services/kafka/topics

# 3. 验证数据流到本地Processor
curl http://localhost:8081/jobs
```

## 🚨 常见问题和解决方案

### 问题1: Kafka连接超时
**症状**: 本地Manager无法连接到远程Kafka
**原因**: `KAFKA_EXTERNAL_HOST` 配置错误
**解决**:
```bash
# 检查远程服务器.env配置
grep KAFKA_EXTERNAL_HOST .env

# 应该显示
KAFKA_EXTERNAL_HOST=49.232.13.155

# 如果不正确，修改后重启
vim .env
make restart middleware
```

### 问题2: 防火墙阻止连接
**症状**: 端口无法访问
**解决**:
```bash
# 检查端口监听
netstat -tlnp | grep :9094

# 检查防火墙状态
sudo ufw status

# 开放端口
sudo ufw allow 9094/tcp
```

### 问题3: Vector健康检查失败
**症状**: Vector API无法访问
**解决**:
```bash
# 检查Vector容器状态
docker compose logs vector

# 检查端口绑定
docker compose ps vector

# 重启Vector服务
make restart middleware
```

## 📊 配置验证清单

### 远程服务器 (49.232.13.155)
- [ ] **环境变量**: `KAFKA_EXTERNAL_HOST=49.232.13.155`
- [ ] **端口开放**: 6000, 8686, 9094, 9090
- [ ] **服务启动**: `make up middleware` 成功
- [ ] **健康检查**: `curl localhost:8686/health` 返回正常

### 本地环境
- [ ] **Kafka连接**: `KAFKA_BOOTSTRAP_SERVERS=49.232.13.155:9094`
- [ ] **Prometheus**: `PROMETHEUS_URL=http://49.232.13.155:9090`
- [ ] **Worker配置**: 指向远程Vector地址
- [ ] **连通性**: `curl http://localhost:8080/api/v1/services/kafka/test-connection` 成功

## 🔧 Makefile 智能提醒

当运行 `make up middleware` 时，Makefile 会自动检查配置：

```bash
⚠️  警告: KAFKA_EXTERNAL_HOST 使用默认值，外部客户端可能无法连接
   当前配置: 162.105.126.246
   服务器IP: 49.232.13.155
   建议修改 .env 中的 KAFKA_EXTERNAL_HOST=49.232.13.155
```

## 🎯 最佳实践

### 1. 配置检查脚本
创建 `check-kafka-config.sh`:
```bash
#!/bin/bash
# Kafka配置检查脚本

CURRENT_IP=$(curl -s ifconfig.me)
KAFKA_EXT_HOST=$(grep "^KAFKA_EXTERNAL_HOST=" .env | cut -d'=' -f2)

echo "当前服务器IP: $CURRENT_IP"
echo "Kafka外部地址: $KAFKA_EXT_HOST"

if [ "$KAFKA_EXT_HOST" != "$CURRENT_IP" ]; then
    echo "❌ 配置不匹配，建议修改:"
    echo "sed -i 's/KAFKA_EXTERNAL_HOST=.*/KAFKA_EXTERNAL_HOST=$CURRENT_IP/' .env"
else
    echo "✅ Kafka配置正确"
fi
```

### 2. 自动配置脚本
创建 `setup-remote-middleware.sh`:
```bash
#!/bin/bash
# 远程Middleware自动配置脚本

set -e

echo "🚀 配置远程Middleware服务..."

# 获取当前服务器IP
CURRENT_IP=$(curl -s ifconfig.me || hostname -I | awk '{print $1}')
echo "检测到服务器IP: $CURRENT_IP"

# 更新.env配置
cp .env.example .env
sed -i "s/KAFKA_EXTERNAL_HOST=.*/KAFKA_EXTERNAL_HOST=$CURRENT_IP/" .env
sed -i "s/EXTERNAL_IP=.*/EXTERNAL_IP=$CURRENT_IP/" .env
sed -i "s/DEPLOYMENT_MODE=.*/DEPLOYMENT_MODE=distributed/" .env
sed -i "s/ENVIRONMENT=.*/ENVIRONMENT=production/" .env

echo "✅ 环境配置已更新"

# 启动服务
make up middleware

echo "🎉 远程Middleware部署完成!"
echo "📋 外部连接信息:"
echo "   Vector: http://$CURRENT_IP:6000"
echo "   Kafka: $CURRENT_IP:9094"
echo "   Prometheus: http://$CURRENT_IP:9090"
```

---

**远程服务器部署示例** - 49.232.13.155 配置指南  
**最后更新**: 2025-09-04  
**关键要点**: 正确配置 KAFKA_EXTERNAL_HOST ⚠️
