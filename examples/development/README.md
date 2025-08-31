# SysArmor EDR 开发环境部署指南

## 🎯 概述

本指南介绍如何在开发环境中快速部署和运行SysArmor EDR系统。开发环境采用单机模式，所有组件运行在同一台机器上，便于开发和调试。

## 🚀 快速开始

### 1. 环境准备

确保系统已安装以下软件：

- **Podman** 4.0+ (推荐) 或 **Docker** 20.0+
- **Podman Compose** 或 **Docker Compose**
- **Make** 4.0+
- **curl** (用于健康检查)

### 2. 初始化项目

```bash
# 进入项目根目录
cd stack/sysarmor

# 初始化Monorepo
make init

# 使用开发环境配置
cp examples/development/.env.dev .env
```

### 3. 启动服务

```bash
# 一键启动所有服务
make up

# 查看服务状态
make status

# 查看日志
make logs
```

### 4. 验证部署

```bash
# 健康检查
make health

# 手动验证各服务
curl http://localhost:8080/health    # Manager API
curl http://localhost:8500/v1/status/leader  # Consul
curl http://localhost:8686/health    # Vector API
curl http://localhost:9200/_cluster/health   # OpenSearch
```

## 🌐 服务访问地址

| 服务 | 地址 | 用途 |
|------|------|------|
| Manager API | http://localhost:8080 | 控制平面API |
| Consul UI | http://localhost:8500 | 服务发现管理 |
| Vector API | http://localhost:8686 | 事件收集状态 |
| Flink Web UI | http://localhost:8081 | 流处理作业管理 |
| OpenSearch | http://localhost:9200 | 事件存储和搜索 |
| Kafka | localhost:9093 | 消息队列 (外部访问) |

## 🔧 开发配置

### 环境变量说明

开发环境使用 `.env.dev` 配置文件，主要特点：

- `MANAGER_LOG_LEVEL=debug` - 启用详细日志
- `DEBUG=true` - 开启调试模式
- `HOT_RELOAD=true` - 支持热重载 (如果服务支持)

### 服务配置

#### Manager服务 (控制平面)
- 端口: 8080
- 数据库: PostgreSQL (自动创建)
- 日志级别: debug
- 服务发现: 自动注册到Consul

#### Middleware服务 (事件分发)
- Vector TCP端口: 6000 (接收rsyslog数据)
- Vector API端口: 8686 (管理和监控)
- Kafka内部端口: 9092
- Kafka外部端口: 9093

#### Processor服务 (流处理)
- Flink JobManager端口: 8081
- TaskManager槽位: 2
- 并行度: 2
- 威胁规则: `/app/configs/rules.yaml`

#### Indexer服务 (事件存储)
- OpenSearch端口: 9200
- 索引前缀: `sysarmor-events`
- 安全插件: 禁用 (开发环境)

## 🛠️ 开发工作流

### 代码修改和测试

```bash
# 构建服务
make build

# 运行测试
make test

# 重启特定服务 (修改代码后)
podman-compose restart manager

# 查看特定服务日志
podman-compose logs -f manager
```

### 调试技巧

1. **查看服务日志**
   ```bash
   # 查看所有服务日志
   make logs
   
   # 查看特定服务日志
   podman-compose logs -f manager
   podman-compose logs -f middleware
   ```

2. **进入容器调试**
   ```bash
   # 进入Manager容器
   podman exec -it sysarmor-manager-1 /bin/bash
   
   # 进入Processor容器
   podman exec -it sysarmor-processor-1 /bin/bash
   ```

3. **检查服务发现**
   ```bash
   # 查看Consul中注册的服务
   curl http://localhost:8500/v1/catalog/services
   
   # 查看特定服务的健康状态
   curl http://localhost:8500/v1/health/service/sysarmor-manager
   ```

### 数据流测试

1. **模拟Agentless客户端**
   ```bash
   # 发送测试事件到Vector
   echo '{"timestamp":"2024-01-01T10:00:00Z","collector_id":"test-001","host":"test-host","message":"test event"}' | \
   nc localhost 6000
   ```

2. **查看Kafka消息**
   ```bash
   # 列出Kafka Topics
   podman exec sysarmor-kafka-1 kafka-topics --list --bootstrap-server localhost:9092
   
   # 消费消息
   podman exec sysarmor-kafka-1 kafka-console-consumer --bootstrap-server localhost:9092 --topic sysarmor-agentless-test --from-beginning
   ```

3. **查看OpenSearch数据**
   ```bash
   # 查看索引
   curl http://localhost:9200/_cat/indices
   
   # 搜索事件
   curl "http://localhost:9200/sysarmor-events-*/_search?pretty"
   ```

## 🔄 常用操作

### 重置环境

```bash
# 停止所有服务
make down

# 清理数据和容器
make clean

# 重新启动
make up
```

### 更新配置

```bash
# 修改 .env 文件后重启服务
make restart

# 或者重新启动
make down && make up
```

### 性能监控

```bash
# 查看容器资源使用
podman stats

# 查看服务状态
make status

# 健康检查
make health
```

## 🐛 故障排查

### 常见问题

1. **端口冲突**
   ```bash
   # 检查端口占用
   netstat -tlnp | grep 8080
   
   # 修改 .env 文件中的端口配置
   ```

2. **服务启动失败**
   ```bash
   # 查看详细日志
   make logs
   
   # 检查服务依赖
   make status
   ```

3. **数据库连接失败**
   ```bash
   # 检查PostgreSQL状态
   podman-compose logs postgres
   
   # 重启数据库
   podman-compose restart postgres
   ```

4. **Kafka连接问题**
   ```bash
   # 检查Kafka和Zookeeper状态
   podman-compose logs kafka
   podman-compose logs zookeeper
   
   # 重启Kafka集群
   podman-compose restart zookeeper kafka
   ```

### 日志分析

```bash
# 查看错误日志
make logs | grep -i error

# 查看特定时间段的日志
podman-compose logs --since="1h" manager

# 实时监控日志
make logs
```

## 📚 下一步

- 查看 [架构文档](../../docs/architecture.md) 了解系统设计
- 查看 [部署文档](../../docs/deployment.md) 了解生产部署
- 查看 [开发文档](../../docs/development.md) 了解开发规范

## 🆘 获取帮助

如果遇到问题：

1. 查看服务日志: `make logs`
2. 检查服务状态: `make status`
3. 运行健康检查: `make health`
4. 查看Makefile帮助: `make help`

---

**开发愉快！** 🚀
