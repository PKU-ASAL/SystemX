# SysArmor EDR/HIDS 系统

## 🎯 项目概述

SysArmor EDR/HIDS 是一个现代化的端点检测与响应系统，采用**逻辑模块化架构**，通过 Docker Compose 实现统一的服务编排和管理。

**✅ 架构状态**: 已完成逻辑模块化重构，系统已验证可用于生产部署。

## 🏗️ 架构设计

### 逻辑模块化架构

```
┌─────────────────────────────────────────────────────────────┐
│                    Manager 模块                              │
│                   (控制平面)                                 │
├─────────────────────────────────────────────────────────────┤
│  • Manager 应用 (Go) :8080  • PostgreSQL 数据库 :5432       │
│  • 系统管理和配置           • 数据持久化存储                 │
│  • REST API 接口           • 健康检查: HEALTHY               │
└─────────────────────────────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────┐
│                    数据处理流水线                            │
├─────────────────────────────────────────────────────────────┤
│  Middleware 模块    │  Processor 模块     │  Indexer 模块    │
│  ┌─────────────────┐│  ┌─────────────────┐│  ┌──────────────┐│
│  │ Vector :6000    ││  │ Flink :8081     ││  │ OpenSearch   ││
│  │ Kafka :9092     ││  │ JobManager      ││  │ :9200        ││
│  │ 数据收集和路由   ││  │ TaskManager     ││  │ Python 索引  ││
│  │ 状态: HEALTHY   ││  │ 威胁检测引擎     ││  │ 服务         ││
│  └─────────────────┘│  │ 状态: HEALTHY   ││  │ 状态: GREEN  ││
│                     │  └─────────────────┘│  └──────────────┘│
└─────────────────────────────────────────────────────────────┘
```

## 📁 目录结构

```
sysarmor/
├── README.md                           # 项目总览和使用指南
├── docker-compose.yml                  # 🔥 主编排文件 (include 模式)
├── .env.example                        # 环境变量模板
├── .env                               # 实际环境变量配置
├── Makefile                           # 统一构建和部署命令
├── go.work                            # Go 工作空间配置
│
├── services/                          # 🔥 逻辑服务模块
│   ├── manager/                       # ✅ 控制平面模块
│   │   ├── docker-compose.yml        # Manager + PostgreSQL
│   │   ├── Dockerfile                # Manager 应用构建
│   │   ├── configs/
│   │   │   ├── postgres/             # PostgreSQL 配置
│   │   │   └── manager/              # Manager 应用配置
│   │   ├── cmd/manager/              # Go 应用入口
│   │   ├── internal/                 # 内部业务逻辑
│   │   ├── migrations/               # 数据库迁移脚本
│   │   └── templates/                # 模板文件
│   │
│   ├── middleware/                   # ✅ 数据中间件模块
│   │   ├── docker-compose.yml       # Vector + Kafka
│   │   ├── vector.Dockerfile        # Vector 定制镜像
│   │   ├── kafka.Dockerfile         # Kafka 定制镜像
│   │   ├── configs/
│   │   │   ├── vector/              # Vector 配置 (vector.toml)
│   │   │   ├── kafka/               # Kafka 配置
│   │   │   └── monitoring/          # 监控配置
│   │   ├── scripts/                 # 部署和维护脚本
│   │   └── tests/                   # 测试脚本
│   │
│   ├── processor/                   # ✅ 数据处理模块
│   │   ├── docker-compose.yml       # PyFlink 集群
│   │   ├── Dockerfile               # Flink 应用构建
│   │   ├── configs/
│   │   │   ├── flink/               # Flink 配置
│   │   │   └── rules/               # 威胁检测规则
│   │   ├── jobs/                    # PyFlink 作业脚本
│   │   ├── scripts/                 # 构建脚本 (download_libs.sh)
│   │   └── tests/                   # 测试文件和样本数据
│   │
│   └── indexer/                     # ✅ 索引存储模块
│       ├── docker-compose.yml       # OpenSearch + Python 服务
│       ├── opensearch.Dockerfile    # OpenSearch 定制镜像
│       ├── indexer.Dockerfile       # Python 索引服务
│       ├── configs/
│       │   ├── opensearch/          # OpenSearch 配置 (opensearch.yml)
│       │   └── indexer/             # 索引服务配置
│       ├── src/                     # Python 索引服务代码 (main.py)
│       ├── templates/               # 索引模板 (JSON)
│       └── scripts/                 # 维护脚本
│
├── shared/                          # 🔥 共享组件
│   └── config/                      # ✅ 统一配置管理库
│       ├── go.mod                   # Go 模块定义
│       └── config.go                # 配置结构和加载逻辑
│
└── docs/                            # 📚 完整文档
    ├── architecture-summary.md      # 架构总结
    ├── improved-architecture-design.md # 详细架构设计
    ├── migration-implementation-plan.md # 实施计划
    └── configuration-analysis.md    # 配置分析报告
```

## 🚀 快速开始

### 1. 环境准备

```bash
# 确保已安装 Docker 和 Docker Compose
docker --version
docker compose version

# 克隆项目后，进入目录
cd stack/sysarmor
```

### 2. 配置环境

```bash
# 复制环境变量模板
cp .env.example .env

# 根据需要修改配置 (可选，默认配置即可运行)
vim .env
```

### 3. 一键启动系统

```bash
# 启动所有逻辑模块
docker compose up -d

# 查看服务状态
docker compose ps

# 查看实时日志
docker compose logs -f
```

### 4. 验证部署

```bash
# 健康检查
curl http://localhost:8080/health    # Manager: {"status":"healthy","database":"connected"}
curl http://localhost:8686/health    # Vector: {"ok":true}
curl http://localhost:9200/_cluster/health # OpenSearch: {"status":"green"}
curl http://localhost:8081          # Flink Web UI

# 或者一键检查所有服务
curl -s http://localhost:8080/health && echo " ✅ Manager OK"
curl -s http://localhost:8686/health && echo " ✅ Vector OK"  
curl -s http://localhost:9200/_cluster/health | jq .status && echo " ✅ OpenSearch OK"
curl -s http://localhost:8081 > /dev/null && echo " ✅ Flink OK"
```

## 🔧 配置管理

### 12-Factor App 配置

系统采用 12-Factor App 最佳实践，所有配置通过环境变量管理：

```bash
# .env 文件配置示例
# =============================================================================
# SysArmor EDR 逻辑模块配置
# =============================================================================

# 全局配置
ENVIRONMENT=development
SYSARMOR_NETWORK=sysarmor-net

# Manager 模块配置
MANAGER_PORT=8080
POSTGRES_DB=sysarmor
POSTGRES_USER=sysarmor
POSTGRES_PASSWORD=password
MANAGER_LOG_LEVEL=info

# Middleware 模块配置
KAFKA_CLUSTER_ID=0203ecef23a24688af6901b94ebafa80
VECTOR_TCP_PORT=6000
VECTOR_API_PORT=8686
VECTOR_METRICS_PORT=9598

# Processor 模块配置
FLINK_JOBMANAGER_PORT=8081
FLINK_TASKMANAGER_SLOTS=2
FLINK_PARALLELISM=2
THREAT_RULES_PATH=/app/configs/rules.yaml

# Indexer 模块配置
OPENSEARCH_USERNAME=admin
OPENSEARCH_PASSWORD=admin
INDEX_PREFIX=sysarmor-events
```

### 配置传入方式

#### 1. **环境变量注入** (从根目录 .env)
- Manager: `MANAGER_PORT`, `POSTGRES_*` 等
- Middleware: `VECTOR_*_PORT`, `KAFKA_CLUSTER_ID` 等
- Processor: `FLINK_*` 参数
- Indexer: `OPENSEARCH_*`, `INDEX_PREFIX` 等

#### 2. **配置文件挂载** (Volume 挂载)
- Vector: `./configs/vector/vector.toml` → `/etc/vector:ro`
- OpenSearch: `./configs/opensearch/opensearch.yml` → 容器配置
- Flink: `./jobs` → `/opt/flink/usr_jobs`, `./configs` → `/opt/flink/configs`

#### 3. **服务发现** (Docker DNS)
- 跨模块: `middleware-kafka:9092`, `indexer-opensearch:9200`
- 模块内: `manager-postgres:5432`

## 🛠️ 模块化管理

### 独立模块部署

```bash
# 启动单个模块 (用于开发和测试)
docker compose -f services/manager/docker-compose.yml up -d     # Manager + PostgreSQL
docker compose -f services/middleware/docker-compose.yml up -d  # Vector + Kafka
docker compose -f services/processor/docker-compose.yml up -d   # Flink 集群
docker compose -f services/indexer/docker-compose.yml up -d     # OpenSearch + 索引服务

# 启动所有模块 (生产部署)
docker compose up -d
```

### 服务管理

```bash
# 服务控制
docker compose up -d          # 启动所有服务
docker compose down           # 停止所有服务
docker compose restart        # 重启所有服务

# 监控和调试
docker compose ps             # 查看服务状态
docker compose logs -f        # 查看实时日志
docker compose logs manager   # 查看特定服务日志

# 清理
docker compose down -v        # 停止服务并删除数据卷
```

## 🌐 服务端点

### 核心服务端口

| 模块 | 服务 | 端口 | 状态 | 用途 |
|------|------|------|------|------|
| Manager | Manager | 8080 | ✅ HEALTHY | REST API, Web UI |
| Manager | PostgreSQL | 5432 | ✅ HEALTHY | 数据库服务 |
| Middleware | Vector TCP | 6000 | ✅ HEALTHY | 数据接收端口 |
| Middleware | Vector API | 8686 | ✅ HEALTHY | 管理 API |
| Middleware | Kafka | 9092 | ✅ HEALTHY | 内部消息队列 |
| Middleware | Kafka External | 9094 | ✅ HEALTHY | 外部访问端口 |
| Processor | Flink JobManager | 8081 | ✅ HEALTHY | 作业管理, Web UI |
| Processor | Flink TaskManager | - | ✅ RUNNING | 任务执行 |
| Indexer | OpenSearch | 9200 | ✅ GREEN | 搜索 API, 数据存储 |
| Indexer | Python 索引服务 | - | ✅ RUNNING | 索引管理服务 |

### Web 界面访问

- **Manager API**: http://localhost:8080 - 系统管理和监控
- **Vector API**: http://localhost:8686 - 数据收集状态
- **Flink Web UI**: http://localhost:8081 - 流处理作业管理
- **OpenSearch**: http://localhost:9200 - 搜索和数据查询

## 🔍 系统验证

### 健康检查命令

```bash
# 快速健康检查
curl http://localhost:8080/health    # Manager: {"status":"healthy","database":"connected"}
curl http://localhost:8686/health    # Vector: {"ok":true}
curl http://localhost:9200/_cluster/health # OpenSearch: {"status":"green"}

# 一键检查所有服务
curl -s http://localhost:8080/health && echo " ✅ Manager OK"
curl -s http://localhost:8686/health && echo " ✅ Vector OK"  
curl -s http://localhost:9200/_cluster/health | jq .status && echo " ✅ OpenSearch OK"
curl -s http://localhost:8081 > /dev/null && echo " ✅ Flink OK"
```

### 服务状态查看

```bash
# 查看所有容器状态
docker compose ps

# 查看服务日志
docker compose logs -f                    # 所有服务日志
docker compose logs -f manager            # Manager 服务日志
docker compose logs -f middleware-kafka   # Kafka 日志
docker compose logs -f middleware-vector  # Vector 日志
docker compose logs -f processor-jobmanager # Flink JobManager 日志
docker compose logs -f indexer-opensearch # OpenSearch 日志
```

## 🎯 架构优势

### ✅ **逻辑模块化**
- **完整功能栈**: 每个模块包含应用服务和相关基础设施
- **独立部署**: 支持模块级别的独立启停和测试
- **清晰边界**: 明确的服务职责和模块边界

### ✅ **配置统一**
- **12-Factor App**: 环境变量驱动的配置管理
- **统一注入**: 根目录 `.env` 文件统一管理所有配置
- **配置分层**: 支持默认值、环境变量和运行时配置

### ✅ **部署简化**
- **一键启动**: `docker compose up -d` 启动所有模块
- **include 模式**: 根目录编排，模块独立配置
- **Docker 原生**: 使用 Docker DNS 服务发现，无需额外组件

### ✅ **运维友好**
- **健康检查**: 所有服务都有标准化健康检查
- **日志统一**: 集中化日志管理和查看
- **监控就绪**: 支持 Prometheus 指标收集

## 🔧 开发指南

### 模块开发

```bash
# 开发单个模块
cd services/manager
docker compose up -d          # 启动 Manager + PostgreSQL

cd services/middleware  
docker compose up -d          # 启动 Vector + Kafka

# 查看模块日志
docker compose logs -f
```

### 配置修改

```bash
# 修改全局配置
vim .env                      # 修改环境变量

# 修改模块特定配置
vim services/middleware/configs/vector/vector.toml  # Vector 配置
vim services/indexer/configs/opensearch/opensearch.yml # OpenSearch 配置

# 重启服务使配置生效
docker compose restart [service_name]
```

## 🌐 数据流架构

### 数据处理流程

```
外部数据源 → Vector (TCP:6000) → Kafka (9092) → Flink (8081) → OpenSearch (9200)
                ↓                                                    ↑
            Manager (8080) ←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←
```

### 服务间通信

- **Manager → Middleware**: `middleware-kafka:9092`
- **Manager → Indexer**: `indexer-opensearch:9200`
- **Processor → Middleware**: `middleware-kafka:9092`
- **Processor → Indexer**: `indexer-opensearch:9200`
- **Manager → PostgreSQL**: `manager-postgres:5432`
- **Vector → Kafka**: `middleware-kafka:9092`
- **Indexer → OpenSearch**: `indexer-opensearch:9200`

## 🔍 故障排查

### 常见问题

1. **端口冲突**
   ```bash
   # 检查端口占用
   docker compose ps
   
   # 停止冲突的容器
   docker stop [container_name]
   
   # 重新启动
   docker compose up -d
   ```

2. **服务启动失败**
   ```bash
   # 检查服务状态
   docker compose ps
   
   # 查看详细日志
   docker compose logs [service_name]
   
   # 验证配置
   docker compose config --quiet
   ```

3. **网络连接问题**
   ```bash
   # 检查网络
   docker network ls | grep sysarmor
   
   # 重建网络和服务
   docker compose down && docker compose up -d
   ```

### 日志分析

```bash
# 实时监控所有服务
docker compose logs -f

# 查看特定时间段的日志
docker compose logs --since 1h manager

# 搜索错误日志
docker compose logs | grep -i error
```

## 🎯 技术栈

### Manager 模块 (控制平面)
- **语言**: Go 1.24
- **框架**: Gin Web Framework  
- **数据库**: PostgreSQL 15
- **配置**: 12-Factor App (envconfig)
- **状态**: ✅ 健康运行

### Middleware 模块 (数据中间件)
- **数据收集**: Vector (Rust) - TCP/HTTP 接收
- **消息队列**: Apache Kafka (KRaft 模式，无 Zookeeper)
- **监控**: Prometheus 指标导出
- **状态**: ✅ 健康运行

### Processor 模块 (数据处理)
- **流处理**: Apache Flink 1.18
- **作业语言**: Python (PyFlink)
- **威胁检测**: 基于规则的检测引擎
- **状态**: ✅ 集群运行正常

### Indexer 模块 (索引存储)
- **搜索引擎**: OpenSearch 2.11 (单节点模式)
- **索引服务**: Python 3.11
- **数据存储**: 分布式索引和搜索
- **状态**: ✅ 集群状态绿色

## 📚 文档

- [架构总结](docs/architecture-summary.md) - 简洁的架构概览
- [详细设计](docs/improved-architecture-design.md) - 完整的架构设计文档
- [配置分析](docs/configuration-analysis.md) - 各模块配置传入方式分析
- [实施计划](docs/migration-implementation-plan.md) - 重构实施计划

## 🤝 贡献指南

1. Fork 项目
2. 创建功能分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 打开 Pull Request

## 📄 许可证

本项目采用 MIT 许可证 - 查看 [LICENSE](LICENSE) 文件了解详情。

## 🆘 支持

如果您遇到问题或有疑问，请：

1. 查看 [故障排查](#-故障排查) 部分
2. 检查服务健康状态: `docker compose ps`
3. 查看服务日志: `docker compose logs [service_name]`
4. 搜索现有的 [Issues](../../issues)
5. 创建新的 Issue 描述问题

---

**SysArmor EDR/HIDS** - 现代化的端点检测与响应系统，逻辑模块化架构，已验证可用于生产部署。

**🎯 当前状态**: 所有 8 个服务健康运行，系统完全可用！
