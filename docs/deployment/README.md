# SysArmor 部署指南

## 📋 概述

本目录包含 SysArmor EDR/HIDS 系统的各种部署方案和配置指南，支持从单机部署到分布式集群的多种部署模式。

## 📚 部署文档

### 🚀 快速开始
- [快速分布式部署](quick-distributed-setup.md) - 5分钟完成分布式部署
- [单机部署指南](../README.md#快速开始) - 一键启动所有服务

### 🏗️ 分布式部署
- [分布式部署详细指南](distributed-deployment.md) - 完整的分布式架构部署
- [网络配置指南](network-configuration.md) (待创建)
- [安全配置指南](security-configuration.md) (待创建)

### ☁️ 云平台部署
- [AWS部署指南](cloud/aws-deployment.md) (待创建)
- [阿里云部署指南](cloud/aliyun-deployment.md) (待创建)
- [Kubernetes部署指南](k8s/kubernetes-deployment.md) (待创建)

### 🔧 高级配置
- [TLS/SSL配置](advanced/tls-configuration.md) (待创建)
- [高可用配置](advanced/high-availability.md) (待创建)
- [性能调优指南](advanced/performance-tuning.md) (待创建)

## 🎯 部署模式对比

| 部署模式 | 适用场景 | 优势 | 劣势 |
|----------|----------|------|------|
| **单机部署** | 开发测试、小规模环境 | 简单快速、资源占用少 | 性能有限、无法扩展 |
| **分布式部署** | 生产环境、中大规模 | 性能好、可扩展、故障隔离 | 配置复杂、网络依赖 |
| **云平台部署** | 企业级、全球化部署 | 弹性扩展、托管服务 | 成本较高、厂商绑定 |
| **Kubernetes** | 容器化、微服务架构 | 自动化运维、服务发现 | 学习成本高、复杂度高 |

## 🚀 推荐部署方案

### 开发环境
```bash
# 单机部署 - 最简单
cd sysarmor
make up
```

### 测试环境
```bash
# 分布式部署 - 模拟生产
# 1. 远程服务器部署Middleware
# 2. 本地部署Manager等服务
```

### 生产环境
```bash
# 高可用分布式部署
# 1. 多个Middleware实例
# 2. Kafka集群
# 3. OpenSearch集群
# 4. 负载均衡和故障转移
```

## 🔧 配置模板

### 环境变量模板

#### 单机部署 (.env)
```bash
DEPLOYMENT_MODE=single-node
ENVIRONMENT=development
EXTERNAL_IP=localhost
# ... 其他默认配置
```

#### 分布式部署 - 远程服务器 (.env.remote)
```bash
DEPLOYMENT_MODE=distributed
ENVIRONMENT=production
EXTERNAL_IP=YOUR_REMOTE_SERVER_IP
KAFKA_EXTERNAL_HOST=YOUR_REMOTE_SERVER_IP
# ... 远程服务配置
```

#### 分布式部署 - 本地环境 (.env.local)
```bash
DEPLOYMENT_MODE=distributed
ENVIRONMENT=development
KAFKA_BOOTSTRAP_SERVERS=YOUR_REMOTE_SERVER_IP:9094
PROMETHEUS_URL=http://YOUR_REMOTE_SERVER_IP:9090
# ... 本地服务配置
```

## 🧪 部署验证

### 验证清单
- [ ] **服务启动**: 所有容器正常运行
- [ ] **端口监听**: 必要端口正确监听
- [ ] **网络连通**: 服务间网络通信正常
- [ ] **API响应**: 所有API端点正常响应
- [ ] **数据流**: 端到端数据流通畅
- [ ] **健康检查**: 系统健康状态正常
- [ ] **监控指标**: Prometheus指标收集正常

### 验证命令
```bash
# 基础验证
make health
curl http://localhost:8080/health

# 服务连接验证
curl http://localhost:8080/api/v1/services/kafka/test-connection
curl http://localhost:8080/api/v1/services/flink/overview
curl http://localhost:8080/api/v1/services/opensearch/cluster/health

# 数据流验证
echo '{"test": "data"}' | nc $REMOTE_SERVER_IP 6000
curl http://localhost:8080/api/v1/services/kafka/topics
```

## 📊 性能基准

### 单机部署性能
- **CPU使用**: 2-4核心
- **内存使用**: 4-8GB
- **磁盘空间**: 20GB+
- **网络带宽**: 100Mbps+

### 分布式部署性能
- **远程服务器**: 2-4核心, 4-8GB内存 (Middleware)
- **本地环境**: 4-8核心, 8-16GB内存 (Manager+Processor+Indexer)
- **网络延迟**: <50ms (推荐)
- **带宽要求**: 1Gbps+ (高负载场景)

## 🚨 注意事项

### 安全考虑
- **防火墙配置**: 只开放必要端口
- **TLS加密**: 生产环境启用TLS
- **访问控制**: 限制管理端口访问
- **密码安全**: 使用强密码和密钥

### 网络要求
- **稳定连接**: 确保网络连接稳定
- **带宽充足**: 根据数据量配置带宽
- **延迟控制**: 网络延迟影响性能
- **DNS解析**: 确保域名解析正常

### 资源规划
- **CPU资源**: 根据负载规划CPU
- **内存资源**: 预留足够内存
- **存储空间**: 规划数据存储需求
- **网络带宽**: 评估网络传输需求

## 📚 相关资源

### 官方文档
- [SysArmor主文档](../../README.md)
- [API参考手册](../sysarmor-api-reference.md)
- [系统更新日志](../../CHANGELOG.md)

### 社区资源
- [GitHub Issues](https://github.com/sysarmor/sysarmor-stack/issues)
- [部署最佳实践](https://github.com/sysarmor/sysarmor-stack/wiki)
- [常见问题解答](https://github.com/sysarmor/sysarmor-stack/wiki/FAQ)

### 技术支持
- **邮件支持**: support@sysarmor.com
- **技术论坛**: https://forum.sysarmor.com
- **企业支持**: enterprise@sysarmor.com

---

**SysArmor 部署指南目录** - 选择适合的部署方案  
**最后更新**: 2025-09-04  
**维护状态**: 活跃维护 ✅
