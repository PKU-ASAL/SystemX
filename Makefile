# SysArmor EDR Monorepo Makefile
.PHONY: help init up down restart status logs health build docs clean up-dev down-dev

# Default target
help: ## Show this help message
	@echo "SysArmor EDR Monorepo Management"
	@echo "================================"
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-25s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ 基础操作
init: ## 初始化项目环境
	@echo "🚀 初始化SysArmor EDR项目..."
	@echo "1️⃣  创建数据目录..."
	@mkdir -p data/kafka-exports data/logs data/backups
	@echo "✅ 数据目录已创建: data/"
	@echo "2️⃣  创建环境配置文件..."
	@if [ ! -f .env ]; then cp .env.example .env; echo "✅ 环境配置文件已创建: .env"; else echo "ℹ️  .env 文件已存在，跳过"; fi
	@echo "3️⃣  生成OpenSearch SSL证书..."
	@if [ ! -f services/indexer/configs/opensearch/certs/node.pem ]; then \
		cd services/indexer && chmod +x scripts/generate-certs.sh && ./scripts/generate-certs.sh; \
		echo "✅ OpenSearch SSL证书已生成"; \
	else \
		echo "ℹ️  SSL证书已存在，跳过生成"; \
	fi
	@echo "📁 项目初始化完成"
	@echo "   data/            - 数据存储目录"
	@echo "   data/kafka-exports/ - Kafka 数据导出目录"
	@echo "   data/logs/       - 日志文件目录"
	@echo "   data/backups/    - 备份文件目录"
	@echo "   .env             - 单机部署配置"
	@echo "   .env.dev         - 开发环境配置 (连接远程middleware)"
	@echo "   services/indexer/configs/opensearch/certs/ - SSL证书文件"

up: ## 启动所有服务 (单机部署)
	@echo "🚀 启动SysArmor EDR服务..."
	@if [ ! -f .env ]; then cp .env.example .env; fi
	docker compose up -d
	@echo "✅ 所有服务启动完成"
	@echo "🌐 Manager API: http://localhost:8080"
	@echo "📖 API文档: http://localhost:8080/swagger/index.html"

deploy: ## 构建并启动所有服务 (单机部署)
	@echo "🔨 构建并启动SysArmor EDR服务..."
	@if [ ! -f .env ]; then cp .env.example .env; fi
	docker compose build --no-cache
	docker compose up -d
	@echo "✅ 所有服务构建并启动完成"
	@echo "🌐 Manager API: http://localhost:8080"
	@echo "📖 API文档: http://localhost:8080/swagger/index.html"

down: ## 停止所有服务
	@echo "🛑 停止SysArmor EDR服务..."
	docker compose down -v --remove-orphans
	@echo "✅ 所有服务已停止，数据卷和网络已清理"

up-dev: ## 启动开发环境 (连接远程middleware)
	@echo "🚀 启动SysArmor EDR开发环境..."
	@if [ ! -f .env.dev ]; then echo "❌ .env.dev 文件不存在"; exit 1; fi
	docker compose -f docker-compose.dev.yml up -d
	@echo "✅ 开发环境启动完成 (连接到远程middleware: 49.232.13.155)"
	@echo "🌐 Manager API: http://localhost:8080"
	@echo "📖 API文档: http://localhost:8080/swagger/index.html"
	@echo "🔧 Flink监控: http://localhost:8081"
	@echo "🔍 OpenSearch: http://localhost:9200"
	@echo "📊 远程Prometheus: http://49.232.13.155:9090"

deploy-dev: ## 构建并启动开发环境 (连接远程middleware)
	@echo "🔨 构建并启动SysArmor EDR开发环境..."
	@if [ ! -f .env.dev ]; then echo "❌ .env.dev 文件不存在"; exit 1; fi
	docker compose -f docker-compose.dev.yml build --no-cache
	docker compose -f docker-compose.dev.yml up -d
	@echo "✅ 开发环境构建并启动完成 (连接到远程middleware: 49.232.13.155)"
	@echo "🌐 Manager API: http://localhost:8080"
	@echo "📖 API文档: http://localhost:8080/swagger/index.html"
	@echo "🔧 Flink监控: http://localhost:8081"
	@echo "🔍 OpenSearch: http://localhost:9200"
	@echo "📊 远程Prometheus: http://49.232.13.155:9090"

down-dev: ## 停止并清理开发环境
	@echo "🛑 停止并清理SysArmor EDR开发环境..."
	docker compose -f docker-compose.dev.yml down -v --remove-orphans
	@echo "🧹 清理开发环境镜像..."
	docker image prune -f --filter "label=sysarmor.module"
	@echo "✅ 开发环境已清理"

up-middleware: ## 启动middleware服务 (单独部署middleware)
	@echo "🚀 启动SysArmor EDR middleware服务..."
	@if [ ! -f .env.middleware ]; then echo "❌ .env.middleware 文件不存在"; exit 1; fi
	docker compose -f docker-compose.middleware.yml up -d
	@echo "✅ Middleware服务启动完成"

deploy-middleware: ## 构建并启动middleware服务 (单独部署middleware)
	@echo "� 构建并启动SysArmor EDR middleware服务..."
	@if [ ! -f .env.middleware ]; then echo "❌ .env.middleware 文件不存在"; exit 1; fi
	docker compose -f docker-compose.middleware.yml build --no-cache
	docker compose -f docker-compose.middleware.yml up -d
	@echo "✅ Middleware服务构建并启动完成"

down-middleware: ## 停止并清理开发环境
	@echo "🛑 停止并清理SysArmor EDR开发环境..."
	docker compose -f docker-compose.middleware.yml down -v --remove-orphans
	@echo "🧹 清理开发环境镜像..."
	docker image prune -f --filter "label=sysarmor.module"
	@echo "✅ 开发环境已清理"

restart: ## 重启所有服务
	@echo "🔄 重启SysArmor EDR服务..."
	docker compose restart
	@echo "✅ 所有服务重启完成"

# 允许make命令接受参数
%:
	@:

##@ 监控运维
status: ## 查看服务状态
	@echo "📊 SysArmor EDR服务状态："
	@if [ -f .env ]; then \
		docker compose ps; \
	else \
		echo "⚠️  .env文件不存在，显示所有SysArmor容器:"; \
		docker ps --filter "label=sysarmor.module" --format "table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}"; \
	fi


health: ## 系统健康检查
	@echo "🏥 SysArmor EDR健康检查..."
	@curl -s http://localhost:8080/health > /dev/null && echo "✅ Manager: 健康" || echo "❌ Manager: 异常"
	@curl -s http://localhost:9090/-/healthy > /dev/null && echo "✅ Prometheus: 健康" || echo "❌ Prometheus: 异常"
	@curl -s http://localhost:8081/overview > /dev/null && echo "✅ Flink: 健康" || echo "❌ Flink: 异常"
	@curl -s http://localhost:9200/_cluster/health > /dev/null && echo "✅ OpenSearch: 健康" || echo "❌ OpenSearch: 异常"

##@ 服务管理 (格式: make <service> <command>)
# Middleware 服务管理
middleware: ## Middleware服务管理 (用法: make middleware <command>)
	@if [ -z "$(filter-out $@,$(MAKECMDGOALS))" ]; then \
		echo "📡 SysArmor Middleware 服务管理"; \
		echo "==============================="; \
		echo "用法: make middleware <command>"; \
		echo ""; \
		echo "可用命令:"; \
		echo "  status           - 查看Middleware服务状态"; \
		echo "  test-kafka       - 测试Kafka连接"; \
		echo "  topics           - 查看Kafka Topics"; \
		echo "  health           - 健康检查"; \
		echo ""; \
		echo "示例:"; \
		echo "  make middleware status"; \
		echo "  make middleware test-kafka"; \
		echo "  make middleware topics"; \
	else \
		$(MAKE) middleware-$(filter-out $@,$(MAKECMDGOALS)); \
	fi

middleware-status:
	@echo "📊 SysArmor Middleware - 服务状态："
	@docker ps --filter "label=sysarmor.module=middleware" --format "table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}"


middleware-test-kafka:
	@echo "📡 SysArmor Middleware - 测试Kafka连接..."
	@curl -s http://localhost:8080/api/v1/services/kafka/health | jq '.' || echo "❌ Kafka不可用"

middleware-topics:
	@echo "📋 SysArmor Middleware - Kafka Topics："
	@curl -s http://localhost:8080/api/v1/services/kafka/topics | jq '.data' || echo "❌ 无法获取Topics"

middleware-health:
	@echo "🏥 SysArmor Middleware - 健康检查..."
	@curl -s http://localhost:8080/api/v1/services/kafka/health | jq '.' || echo "❌ Kafka不可用"
	@curl -s http://localhost:9090/-/healthy > /dev/null && echo "✅ Prometheus: 健康" || echo "❌ Prometheus: 异常"

# Processor 服务管理
processor: ## Processor服务管理 (用法: make processor <command>)
	@if [ -z "$(filter-out $@,$(MAKECMDGOALS))" ]; then \
		echo "🔧 SysArmor Processor 服务管理"; \
		echo "=============================="; \
		echo "用法: make processor <command>"; \
		echo ""; \
		echo "核心命令:"; \
		echo "  init             - 智能初始化 (推荐: 等待所有服务就绪后自动提交作业)"; \
		echo "  jobs             - 查看作业状态"; \
		echo "  cancel JOB_ID=xxx - 取消指定作业"; \
		echo "  status           - 查看服务状态"; \
		echo ""; \
		echo "常用操作:"; \
		echo "  make processor init    # 智能初始化数据流"; \
		echo "  make processor jobs    # 查看运行中的作业"; \
		echo "  make processor status  # 检查服务状态"; \
	else \
		$(MAKE) processor-$(filter-out $@,$(MAKECMDGOALS)); \
	fi

processor-jobs:
	@echo "📋 SysArmor Processor - 作业状态："
	@curl -s http://localhost:8081/jobs 2>/dev/null | jq -r '.jobs[]? | "  🎯 \(.id[:8])... | \(.status) | \(.name // "未命名")"' 2>/dev/null || \
	echo "  ❌ 无法获取作业信息"

processor-init:
	@echo "🚀 SysArmor Processor - 智能初始化..."
	@./scripts/auto-init-processor.sh

processor-cancel:
	@if [ -z "$(JOB_ID)" ]; then \
		echo "❌ 请指定作业ID: make processor cancel JOB_ID=your_job_id"; \
		echo "💡 获取作业ID: make processor jobs"; \
		exit 1; \
	fi
	@echo "🛑 取消Flink作业 $(JOB_ID)..."
	@if docker ps --format "table {{.Names}}" | grep -q "flink-jobmanager"; then \
		docker compose exec flink-jobmanager flink cancel $(JOB_ID); \
		echo "✅ 作业已取消"; \
	else \
		echo "❌ Flink JobManager容器未运行"; \
	fi

processor-status:
	@echo "📊 SysArmor Processor - 服务状态："
	@docker ps --filter "label=sysarmor.module=processor" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

# Indexer 服务管理
indexer: ## Indexer服务管理 (用法: make indexer <command>)
	@if [ -z "$(filter-out $@,$(MAKECMDGOALS))" ]; then \
		echo "🔍 SysArmor Indexer 服务管理"; \
		echo "============================="; \
		echo "用法: make indexer <command>"; \
		echo ""; \
		echo "可用命令:"; \
		echo "  status           - 查看Indexer服务状态"; \
		echo "  health           - 健康检查"; \
		echo "  indices          - 查看索引列表"; \
		echo "  search           - 搜索威胁事件"; \
		echo "  cluster-info     - 查看集群信息"; \
		echo ""; \
		echo "示例:"; \
		echo "  make indexer status"; \
		echo "  make indexer health"; \
		echo "  make indexer indices"; \
	else \
		$(MAKE) indexer-$(filter-out $@,$(MAKECMDGOALS)); \
	fi

indexer-status:
	@echo "📊 SysArmor Indexer - 服务状态："
	@docker ps --filter "label=sysarmor.module=indexer" --format "table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}"


indexer-health:
	@echo "🏥 SysArmor Indexer - 健康检查..."
	@curl -s http://localhost:9200/_cluster/health | jq '.' || echo "❌ OpenSearch不可用"

indexer-indices:
	@echo "📋 SysArmor Indexer - 索引列表："
	@curl -s http://localhost:8080/api/v1/services/opensearch/indices | jq '.data' || \
	curl -s -u admin:admin http://localhost:9200/_cat/indices?v || echo "❌ 无法获取索引列表"

indexer-search:
	@echo "🔍 SysArmor Indexer - 搜索威胁事件 (最近1小时)："
	@curl -s "http://localhost:8080/api/v1/services/opensearch/events/recent?hours=1&size=5" | jq '.data.hits.hits[] | ._source | {timestamp, threat_type, risk_score, severity, host}' || echo "❌ 无法搜索事件"

indexer-cluster-info:
	@echo "📊 SysArmor Indexer - 集群信息："
	@curl -s -u admin:admin http://localhost:9200/_cluster/stats | jq '{cluster_name, status, nodes: .nodes.count, indices: .indices.count, docs: .indices.docs.count}' || echo "❌ 无法获取集群信息"

# Manager 服务管理
manager: ## Manager服务管理 (用法: make manager <command>)
	@if [ -z "$(filter-out $@,$(MAKECMDGOALS))" ]; then \
		echo "🎛️  SysArmor Manager 服务管理"; \
		echo "============================="; \
		echo "用法: make manager <command>"; \
		echo ""; \
		echo "可用命令:"; \
		echo "  status           - 查看Manager服务状态"; \
		echo "  health           - 健康检查"; \
		echo "  api-docs         - 打开API文档"; \
		echo "  collectors       - 查看设备列表"; \
		echo "  events           - 查看最近事件"; \
		echo ""; \
		echo "示例:"; \
		echo "  make manager status"; \
		echo "  make manager health"; \
		echo "  make manager collectors"; \
	else \
		$(MAKE) manager-$(filter-out $@,$(MAKECMDGOALS)); \
	fi

manager-status:
	@echo "📊 SysArmor Manager - 服务状态："
	@docker ps --filter "label=sysarmor.module=manager" --format "table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}"


manager-health:
	@echo "🏥 SysArmor Manager - 健康检查..."
	@curl -s http://localhost:8080/health | jq '.' || echo "❌ Manager不可用"

manager-api-docs:
	@echo "📖 SysArmor Manager - API文档："
	@echo "🌐 http://localhost:8080/swagger/index.html"
	@if command -v open >/dev/null 2>&1; then \
		open http://localhost:8080/swagger/index.html; \
	elif command -v xdg-open >/dev/null 2>&1; then \
		xdg-open http://localhost:8080/swagger/index.html; \
	fi

manager-collectors:
	@echo "📱 SysArmor Manager - 设备列表："
	@curl -s http://localhost:8080/api/v1/collectors | jq '.data[] | {id: .id[:8], hostname, status, last_active}' || echo "❌ 无法获取设备列表"

manager-events:
	@echo "📋 SysArmor Manager - 最近事件 (最近1小时)："
	@curl -s "http://localhost:8080/api/v1/events/recent?hours=1&size=5" | jq '.data[] | {timestamp, event_type, severity, host, message}' || echo "❌ 无法获取事件"

##@ 开发构建
build: ## 构建Manager应用
	@echo "🔨 构建Manager应用..."
	@mkdir -p bin
	@if [ -f apps/manager/go.mod ]; then cd apps/manager && go build -o ../../bin/manager ./main.go; fi
	@echo "✅ Manager构建完成"

docs: ## 生成API文档
	@echo "📚 生成Swagger API文档..."
	@if [ -f apps/manager/go.mod ]; then \
		cd apps/manager && \
		if command -v ~/go/bin/swag >/dev/null 2>&1; then \
			~/go/bin/swag init -g main.go -o docs --parseDependency --parseInternal; \
			echo "✅ API文档生成完成: http://localhost:8080/swagger/index.html"; \
		else \
			echo "❌ swag工具未安装，请运行: go install github.com/swaggo/swag/cmd/swag@latest"; \
		fi; \
	fi

##@ 清理维护
clean: ## 清理构建文件和容器
	@echo "🧹 清理构建文件和容器..."
	@rm -rf bin/
	docker compose down -v --remove-orphans
	@echo "✅ 清理完成"


##@ 信息帮助
info: ## 显示项目信息
	@echo "SysArmor EDR/HIDS 系统"
	@echo "====================="
	@echo "架构: Monorepo + 微服务 + 极简配置"
	@echo "控制平面: Manager (Go + Gin + Swagger)"
	@echo "数据平面: Middleware + Processor + Indexer"
	@echo "集成功能: Wazuh SIEM + 实时威胁检测"
	@echo ""
	@echo "极简配置架构:"
	@echo "  只需要设置4个服务HOST，其他配置自动派生"
	@echo "  Manager服务:    MANAGER_HOST (控制平面)"
	@echo "  Middleware服务: MIDDLEWARE_HOST (数据中间件)"
	@echo "  Processor服务:  PROCESSOR_HOST (数据处理)"
	@echo "  Indexer服务:    INDEXER_HOST (索引存储)"
	@echo ""
	@echo "核心端口:"
	@echo "  Manager:    8080  (API + Swagger UI)"
	@echo "  Vector:     6000  (数据收集)"
	@echo "  Kafka:      9094  (消息队列)"
	@echo "  Flink:      8081  (流处理)"
	@echo "  OpenSearch: 9200  (搜索引擎)"
	@echo "  Prometheus: 9090  (监控)"
	@echo ""
	@echo "配置文件 (按服务逻辑分类):"
	@echo "  .env     - 单机部署配置 (所有HOST=localhost)"
	@echo "  .env.dev - 开发环境配置 (MIDDLEWARE_HOST=远程IP)"
	@echo ""
	@echo "部署模式:"
	@echo "  单机部署: make up"
	@echo "  开发环境: make up-dev (连接远程middleware)"
	@echo ""
	@echo "配置优势:"
	@echo "  - 环境变量从55个减少到23个 (减少58%)"
	@echo "  - 只需要设置4个HOST，其他配置自动派生"
	@echo "  - 按Manager/Middleware/Processor/Indexer逻辑分类"
	@echo "  - 修改部署拓扑只需要改对应服务的HOST"
	@echo ""
	@echo "快速开始: make init && make up"
	@echo "API文档: http://localhost:8080/swagger/index.html"
	@echo "部署指南: docs/deployment/README.md"
