# SysArmor EDR/HIDS 系统

## 🎯 项目概述

SysArmor 是一个现代化的端点检测与响应(EDR/HIDS)系统，采用 **Monorepo + 微服务架构**，支持 agentless 数据采集、实时威胁检测和智能分析。

## 🏗️ 系统架构

### 核心架构
```
控制平面 (apps/)     +     数据平面 (services/)
      ↓                           ↓
  Manager API              Middleware + Processor + Indexer
      ↓                           ↓
   Web UI (预留)              实时数据处理流水线
```

### 四大核心模块
- **Manager** (Go): 控制平面 - 设备管理、API 服务、健康监控
- **Middleware** (Vector+Kafka): 数据中间件 - 数据收集、消息队列、监控
- **Processor** (Flink): 数据处理 - 实时流处理、威胁检测、格式转换
- **Indexer** (OpenSearch): 索引存储 - 数据索引、搜索服务、事件查询

### 数据流向
```
Agent/Collector → Vector:6000 → Kafka:9092 → Flink:8081 → OpenSearch:9200
                     ↓              ↓           ↓             ↓
                Manager:8080 ←→ Prometheus:9090 ←→ 威胁检测 ←→ 事件存储
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
curl "http://localhost:8080/api/v1/scripts/setup-terminal.sh?collector_id=xxx" -o install.sh
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
```bash
# 网络配置
SYSARMOR_NETWORK=sysarmor-net
EXTERNAL_IP=localhost

# Manager 服务
MANAGER_PORT=8080
POSTGRES_DB=sysarmor

# Middleware 服务
VECTOR_TCP_PORT=6000
KAFKA_BOOTSTRAP_SERVERS=middleware-kafka:9092

# Processor 服务
FLINK_JOBMANAGER_PORT=8081
FLINK_PARALLELISM=2

# Indexer 服务
OPENSEARCH_PORT=9200
INDEX_PREFIX=sysarmor-events
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
make test        # 运行测试
make clean       # 清理资源
```

## 🌐 API 接口

### 核心业务 API
- **设备管理**: `/api/v1/collectors/*`
- **安全事件**: `/api/v1/events/*`
- **系统监控**: `/api/v1/health/*`
- **脚本下载**: `/api/v1/scripts/*`

### 服务管理 API
- **Kafka 管理**: `/api/v1/services/kafka/*`
- **Flink 管理**: `/api/v1/services/flink/*`
- **OpenSearch 管理**: `/api/v1/services/opensearch/*`

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

### ✅ **统一管理**
- Monorepo 架构，统一代码管理
- 完整的 REST API
- 一键部署和监控

## 🧪 数据流测试

### 测试 Auditd 数据流
```bash
# 运行端到端数据流测试
./tests/test-auditd-data-flow.sh

# 测试内容：
# 1. 检查 Vector 服务状态
# 2. 发送模拟 auditd 数据到 Vector
# 3. 验证 Kafka 主题自动创建
# 4. 确认数据正确路由到 Kafka
```

### 手动测试数据发送
```bash
# 发送测试数据到 Vector
echo '{"collector_id":"12345678-abcd-efgh-ijkl-123456789012","timestamp":"2025-09-03T08:44:17Z","host":"test-host","source":"auditd","message":"type=SYSCALL msg=audit(1693420800.123:456): arch=c000003e syscall=2 success=yes exit=3","event_type":"audit","severity":"info","tags":["audit","syscall"]}' | nc localhost 6000

# 消费 Kafka 消息 (重要：禁用 JMX agent 避免端口冲突)
docker exec -e KAFKA_OPTS= sysarmor-kafka-1 /opt/kafka/bin/kafka-console-consumer.sh --bootstrap-server localhost:9092 --topic sysarmor-agentless-12345678 --from-beginning

# 或者使用 Manager API 查看主题
curl http://localhost:8080/api/v1/services/kafka/topics
```

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

## 📚 文档

- [API 参考](docs/manager-api-reference.md) - 完整 API 文档
- [功能特性](docs/v0.1-release-features.md) - 版本功能说明

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
