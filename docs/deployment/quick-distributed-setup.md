# SysArmor 分布式部署快速指南

## 🚀 5分钟快速部署

### 前提条件
- 远程服务器: 具有公网IP，已安装Docker和Docker Compose
- 本地环境: 已安装Docker和Docker Compose
- 网络连通: 本地能访问远程服务器的指定端口

## 📋 快速部署步骤

### 第一步: 远程服务器 (部署Middleware)

```bash
# 1. 克隆项目
git clone https://github.com/sysarmor/sysarmor-stack.git
cd sysarmor-stack/sysarmor

# 2. 设置环境变量 (替换YOUR_REMOTE_SERVER_IP为实际IP)
export REMOTE_SERVER_IP="192.168.1.100"  # 替换为实际IP

# 3. 创建远程配置
cat > .env << EOF
DEPLOYMENT_MODE=distributed
ENVIRONMENT=production
EXTERNAL_IP=$REMOTE_SERVER_IP
KAFKA_EXTERNAL_HOST=$REMOTE_SERVER_IP
KAFKA_EXTERNAL_PORT=9094
WORKER_URLS=$REMOTE_SERVER_IP:http://$REMOTE_SERVER_IP:6000:http://$REMOTE_SERVER_IP:8686/health
EOF

# 4. 启动Middleware服务
docker compose up middleware-vector middleware-kafka middleware-prometheus -d

# 5. 开放防火墙端口
sudo ufw allow 6000/tcp    # Vector数据收集
sudo ufw allow 8686/tcp    # Vector API
sudo ufw allow 9094/tcp    # Kafka外部端口
sudo ufw allow 9090/tcp    # Prometheus

# 6. 验证服务
docker compose ps
curl http://localhost:8686/health
```

### 第二步: 本地环境 (部署Manager等)

```bash
# 1. 进入项目目录 (如果没有则先克隆)
cd sysarmor-stack/sysarmor

# 2. 设置远程服务器IP (替换为实际IP)
export REMOTE_SERVER_IP="192.168.1.100"  # 替换为实际IP

# 3. 创建本地配置
cat > .env << EOF
DEPLOYMENT_MODE=distributed
ENVIRONMENT=development
EXTERNAL_IP=localhost

# 远程Middleware连接
VECTOR_HOST=$REMOTE_SERVER_IP
KAFKA_HOST=$REMOTE_SERVER_IP
KAFKA_BOOTSTRAP_SERVERS=$REMOTE_SERVER_IP:9094
PROMETHEUS_HOST=$REMOTE_SERVER_IP
PROMETHEUS_URL=http://$REMOTE_SERVER_IP:9090
WORKER_URLS=$REMOTE_SERVER_IP:http://$REMOTE_SERVER_IP:6000:http://$REMOTE_SERVER_IP:8686/health

# 本地服务
MANAGER_PORT=8080
POSTGRES_DB=sysarmor
POSTGRES_USER=sysarmor
POSTGRES_PASSWORD=password
OPENSEARCH_URL=http://indexer-opensearch:9200
FLINK_JOBMANAGER_PORT=8081
FLINK_PARALLELISM=2
EOF

# 4. 启动本地服务
docker compose up manager manager-postgres processor-jobmanager processor-taskmanager indexer-opensearch -d

# 5. 验证部署
make health
curl http://localhost:8080/api/v1/services/kafka/test-connection
```

## ✅ 验证部署成功

### 检查服务状态
```bash
# 远程服务器
ssh user@$REMOTE_SERVER_IP "docker compose ps"

# 本地环境
docker compose ps
```

### 测试API连接
```bash
# 1. Manager健康检查
curl http://localhost:8080/health

# 2. 远程Kafka连接测试
curl http://localhost:8080/api/v1/services/kafka/test-connection

# 3. 访问API文档
open http://localhost:8080/swagger/index.html
```

### 测试数据流
```bash
# 1. 发送测试数据到远程Vector
echo '{"collector_id":"test-001","message":"distributed test"}' | nc $REMOTE_SERVER_IP 6000

# 2. 查看Kafka主题
curl http://localhost:8080/api/v1/services/kafka/topics

# 3. 注册测试Collector
curl -X POST http://localhost:8080/api/v1/collectors/register \
  -H "Content-Type: application/json" \
  -d '{
    "hostname": "test-server",
    "ip_address": "192.168.1.200",
    "os_type": "linux",
    "deployment_type": "agentless"
  }'
```

## 🎯 成功标志

当看到以下输出时，说明分布式部署成功：

### 远程服务器
```
✅ middleware-vector    Up    0.0.0.0:6000->6000/tcp, 0.0.0.0:8686->8686/tcp
✅ middleware-kafka     Up    0.0.0.0:9094->9094/tcp
✅ middleware-prometheus Up   0.0.0.0:9090->9090/tcp
```

### 本地环境
```
✅ manager              Up    0.0.0.0:8080->8080/tcp
✅ manager-postgres     Up    0.0.0.0:5432->5432/tcp
✅ processor-jobmanager Up    0.0.0.0:8081->8081/tcp
✅ indexer-opensearch   Up    0.0.0.0:9200->9200/tcp
```

### API测试结果
```json
{
  "success": true,
  "connected": true,
  "message": "Successfully connected to Kafka",
  "broker_count": 1
}
```

## 🔧 故障排查

### 常见问题
1. **连接超时**: 检查防火墙和网络连通性
2. **Kafka连接失败**: 确认KAFKA_EXTERNAL_HOST配置正确
3. **Vector无法访问**: 检查6000端口是否开放
4. **Prometheus查询失败**: 确认9090端口访问权限

### 快速修复
```bash
# 重启远程服务
ssh user@$REMOTE_SERVER_IP "cd sysarmor-stack/sysarmor && docker compose restart"

# 重启本地服务
docker compose restart

# 检查网络连通性
telnet $REMOTE_SERVER_IP 6000
telnet $REMOTE_SERVER_IP 9094
```

---

**SysArmor 分布式部署快速指南** - 5分钟完成分布式部署  
**最后更新**: 2025-09-04  
**难度等级**: 中级 ⭐⭐⭐  
**预计时间**: 5-10分钟 ⏱️
