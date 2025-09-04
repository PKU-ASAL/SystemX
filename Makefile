# SysArmor EDR Monorepo Makefile
.PHONY: help init up down restart status logs health build docs clean

# Default target
help: ## Show this help message
	@echo "SysArmor EDR Monorepo Management"
	@echo "================================"
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ 基础操作
init: ## 初始化项目环境
	@echo "🚀 初始化SysArmor EDR项目..."
	@if [ ! -f .env ]; then cp .env.example .env; echo "✅ 环境配置文件已创建"; fi
	@echo "📁 项目初始化完成，请根据需要编辑各服务的专用配置文件:"
	@echo "   .env.middleware - Middleware服务配置"
	@echo "   .env.manager    - Manager服务配置"
	@echo "   .env.processor  - Processor服务配置"
	@echo "   .env.indexer    - Indexer服务配置"

up: ## 启动服务 (支持参数: make up [service])
	@echo "🚀 启动SysArmor EDR服务..."
	@if [ "$(filter-out $@,$(MAKECMDGOALS))" ]; then \
		SERVICE="$(filter-out $@,$(MAKECMDGOALS))"; \
		case $$SERVICE in \
			middleware) \
				echo "📡 启动Middleware服务..."; \
				if [ ! -f .env.middleware ]; then echo "❌ .env.middleware 文件不存在"; exit 1; fi; \
				cd services/middleware && docker compose --env-file ../../.env.middleware up -d; \
				echo "✅ Middleware启动完成: Vector:6000, Kafka:9092, Prometheus:9090"; \
				;; \
			manager) \
				echo "🔧 启动Manager服务..."; \
				if [ ! -f .env.manager ]; then echo "❌ .env.manager 文件不存在"; exit 1; fi; \
				docker compose -f deployments/compose/manager.yml --env-file .env.manager up -d; \
				echo "✅ Manager启动完成: http://localhost:8080"; \
				;; \
			processor) \
				echo "⚡ 启动Processor服务..."; \
				if [ ! -f .env.processor ]; then echo "❌ .env.processor 文件不存在"; exit 1; fi; \
				cd services/processor && docker compose --env-file ../../.env.processor up -d; \
				echo "✅ Processor启动完成: http://localhost:8081"; \
				;; \
			indexer) \
				echo "🔍 启动Indexer服务..."; \
				if [ ! -f .env.indexer ]; then echo "❌ .env.indexer 文件不存在"; exit 1; fi; \
				cd services/indexer && docker compose --env-file ../../.env.indexer up -d; \
				echo "✅ Indexer启动完成: http://localhost:9200"; \
				;; \
			*) \
				echo "❌ 未知服务: $$SERVICE"; \
				echo "支持的服务: middleware, manager, processor, indexer"; \
				exit 1; \
				;; \
		esac; \
	else \
		if [ ! -f .env ]; then cp .env.example .env; fi; \
		docker compose up -d; \
		echo "✅ 所有服务启动完成"; \
		echo "🌐 Manager API: http://localhost:8080"; \
		echo "📖 API文档: http://localhost:8080/swagger/index.html"; \
	fi

down: ## 停止服务 (支持参数: make down [service])
	@echo "🛑 停止SysArmor EDR服务..."
	@if [ "$(filter-out $@,$(MAKECMDGOALS))" ]; then \
		SERVICE="$(filter-out $@,$(MAKECMDGOALS))"; \
		case $$SERVICE in \
			middleware) \
				echo "📡 停止Middleware服务..."; \
				cd services/middleware && docker compose --env-file ../../.env.middleware down; \
				echo "✅ Middleware已停止"; \
				;; \
			manager) \
				echo "🔧 停止Manager服务..."; \
				docker compose -f deployments/compose/manager.yml --env-file .env.manager down; \
				echo "✅ Manager已停止"; \
				;; \
			processor) \
				echo "⚡ 停止Processor服务..."; \
				cd services/processor && docker compose --env-file ../../.env.processor down; \
				echo "✅ Processor已停止"; \
				;; \
			indexer) \
				echo "🔍 停止Indexer服务..."; \
				cd services/indexer && docker compose --env-file ../../.env.indexer down; \
				echo "✅ Indexer已停止"; \
				;; \
			*) \
				echo "❌ 未知服务: $$SERVICE"; \
				echo "支持的服务: middleware, manager, processor, indexer"; \
				exit 1; \
				;; \
		esac; \
	else \
		docker compose down; \
		echo "✅ 所有服务已停止"; \
	fi

restart: ## 重启服务 (支持参数: make restart [service])
	@echo "🔄 重启SysArmor EDR服务..."
	@if [ "$(filter-out $@,$(MAKECMDGOALS))" ]; then \
		SERVICE="$(filter-out $@,$(MAKECMDGOALS))"; \
		case $$SERVICE in \
			middleware) \
				echo "📡 重启Middleware服务..."; \
				cd services/middleware && docker compose --env-file ../../.env.middleware restart; \
				echo "✅ Middleware重启完成"; \
				;; \
			manager) \
				echo "🔧 重启Manager服务..."; \
				docker compose -f deployments/compose/manager.yml --env-file .env.manager restart; \
				echo "✅ Manager重启完成"; \
				;; \
			processor) \
				echo "⚡ 重启Processor服务..."; \
				cd services/processor && docker compose --env-file ../../.env.processor restart; \
				echo "✅ Processor重启完成"; \
				;; \
			indexer) \
				echo "🔍 重启Indexer服务..."; \
				cd services/indexer && docker compose --env-file ../../.env.indexer restart; \
				echo "✅ Indexer重启完成"; \
				;; \
			*) \
				echo "❌ 未知服务: $$SERVICE"; \
				echo "支持的服务: middleware, manager, processor, indexer"; \
				exit 1; \
				;; \
		esac; \
	else \
		docker compose restart; \
		echo "✅ 所有服务重启完成"; \
	fi

# 允许make命令接受参数
%:
	@:

##@ 监控运维
status: ## 查看服务状态
	@echo "📊 SysArmor EDR服务状态："
	docker compose ps

logs: ## 查看服务日志
	@echo "📋 SysArmor EDR服务日志："
	docker compose logs -f

health: ## 系统健康检查
	@echo "🏥 SysArmor EDR健康检查..."
	@curl -s http://localhost:8080/health > /dev/null && echo "✅ Manager: 健康" || echo "❌ Manager: 异常"
	@curl -s http://localhost:9090/-/healthy > /dev/null && echo "✅ Prometheus: 健康" || echo "❌ Prometheus: 异常"

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
	@echo "架构: Monorepo + 微服务"
	@echo "控制平面: Manager (Go + Gin + Swagger)"
	@echo "数据平面: Middleware + Processor + Indexer"
	@echo "集成功能: Wazuh SIEM + 实时威胁检测"
	@echo ""
	@echo "核心端口:"
	@echo "  Manager:    8080  (API + Swagger UI)"
	@echo "  Vector:     6000  (数据收集)"
	@echo "  Kafka:      9092  (消息队列)"
	@echo "  Flink:      8081  (流处理)"
	@echo "  OpenSearch: 9200  (搜索引擎)"
	@echo "  Prometheus: 9090  (监控)"
	@echo ""
	@echo "专用配置文件:"
	@echo "  .env.middleware - Middleware服务专用配置"
	@echo "  .env.manager    - Manager服务专用配置"
	@echo "  .env.processor  - Processor服务专用配置"
	@echo "  .env.indexer    - Indexer服务专用配置"
	@echo ""
	@echo "分布式部署:"
	@echo "  远程服务器: make up middleware  (使用 .env.middleware)"
	@echo "  本地环境:   make up manager processor indexer"
	@echo ""
	@echo "快速开始: make init && make up"
	@echo "API文档: http://localhost:8080/swagger/index.html"
	@echo "部署指南: docs/deployment/README.md"
