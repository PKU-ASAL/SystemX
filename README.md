# SysArmor EDR/HIDS 系统

## 🎯 项目概述

SysArmor 是一个现代化的端点检测与响应(EDR/HIDS)系统，采用微服务架构，实现 agentless 数据采集、实时威胁检测和告警存储的完整数据流。

### 系统架构
```mermaid
graph LR
    A[终端设备<br/>auditd数据] -->|TCP:6000| B[Middleware<br/>Vector + Kafka]
    B -->|消息队列| C[Processor<br/>Flink + NODLINK]
    C -->|告警事件| D[Indexer<br/>OpenSearch]
    E[Manager<br/>控制平面] -.->|管理| B
    E -.->|查询| D
    E --- F[(PostgreSQL)]
```

### 核心特性
- 🚀 **Agentless 部署** - 基于 rsyslog + auditd，无需安装 Agent
- 🔧 **微服务架构** - Manager + Middleware + Processor + Indexer 四大模块
- 📊 **实时威胁检测** - Flink 流处理 + NODLINK 算法
- 🔍 **告警存储查询** - OpenSearch 索引和 REST API
- 🐳 **容器化部署** - Docker Compose 一键部署


## 🚀 快速开始

### 单机部署 (开发测试)
```bash
git clone https://git.pku.edu.cn/oslab/sysarmor.git
cd sysarmor
make up          # 启动所有服务
make health      # 验证部署
```

### 分布式部署 (生产推荐)
```bash
# 远程服务器 - 数据收集层
make up-middleware    # Vector + Kafka + Prometheus

# 本地环境 - 管理控制层  
make up-dev          # Manager + Processor + Indexer
```

### 访问服务
- **Manager API**: http://localhost:8080
- **API 文档**: http://localhost:8080/swagger/index.html
- **Flink 监控**: http://localhost:8081
- **OpenSearch**: http://localhost:9200

### 设备接入
1. **注册设备**: 通过 Manager API 注册新设备
2. **下载脚本**: 获取 agentless 安装脚本
3. **部署采集**: 在目标服务器执行脚本，配置 rsyslog + auditd


## 🔧 管理命令

```bash
# 单机部署
make up          # 启动所有服务
make down        # 停止所有服务
make health      # 健康检查

# 分布式部署
make up-middleware    # 启动middleware (远程)
make up-dev          # 启动开发环境 (本地)
make down-dev        # 停止开发环境

# 开发工具
make build           # 构建应用
make docs           # 生成API文档
make test           # 运行测试
```

## 🌐 API 接口

SysArmor 提供完整的 REST API 接口，支持设备管理、系统监控和服务管理：

- **设备管理**: `/api/v1/collectors/*` - 设备注册、状态管理
- **告警查询**: `/api/v1/events/*` - 威胁告警查询和分析
- **系统监控**: `/api/v1/health/*` - 健康检查和指标
- **API 文档**: http://localhost:8080/swagger/index.html


## 📚 文档

- [部署指南](docs/deployment.md) - 分布式部署配置
- [Flink 测试指南](docs/flink-testing.md) - 集群测试和验证
- [系统更新日志](CHANGELOG.md) - 版本历史

## 🧪 快速测试

### 系统健康检查
```bash
# 基础健康检查
make health

# 详细系统健康测试 (20项测试)
./tests/test-system-health.sh

# 查看按逻辑服务分组的健康状态
curl -s http://localhost:8080/api/v1/health | jq '.data.services'
```

### 数据流测试
```bash
# 导入测试数据到 Kafka
./scripts/kafka-tools.sh import docs/draft/sysarmor-agentless-b1de298c_20250905_225242.jsonl sysarmor-events-test

# 查看 Kafka topics 和消息数量
./scripts/kafka-tools.sh list

# 导出验证数据
./scripts/kafka-tools.sh export sysarmor-events-test 5

# 完整数据流测试
./tests/test-auditd-data-flow.sh
```

### 服务管理测试
```bash
# Kafka 服务管理
make middleware health
curl -s http://localhost:8080/api/v1/services/kafka/health | jq '.'

# Flink 服务管理  
make processor overview
curl -s http://localhost:8080/api/v1/services/flink/health | jq '.'

# OpenSearch 服务管理
make indexer health
curl -s http://localhost:8080/api/v1/services/opensearch/health | jq '.'
```

### 开发构建
```bash
# 本地开发
cd apps/manager && go run main.go

# 构建并启动 (重新构建镜像)
make deploy

# 生成 API 文档
make docs
```

---

**SysArmor EDR/HIDS** - 现代化端点检测与响应系统

**🔗 快速开始**: `git clone https://git.pku.edu.cn/oslab/sysarmor.git && cd sysarmor && make up`
