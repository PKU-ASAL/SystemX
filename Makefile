# SysArmor EDR Monorepo Makefile
.PHONY: help init up down restart status logs health build docs clean up-dev down-dev

# Default target
help: ## Show this help message
	@echo "SysArmor EDR Monorepo Management"
	@echo "================================"
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ 基础操作
init: ## 初始化项目环境
	@echo "🚀 初始化SysArmor EDR项目..."
	@if [ ! -f .env ]; then cp .env.example .env; echo "✅ 环境配置文件已创建: .env"; fi
	@echo "📁 项目初始化完成"
	@echo "   .env     - 单机部署配置"
	@echo "   .env.dev - 开发环境配置 (连接远程middleware)"

up: ## 启动所有服务 (单机部署)
	@echo "🚀 启动SysArmor EDR服务..."
	@if [ ! -f .env ]; then cp .env.example .env; fi
	docker compose up -d
	@echo "✅ 所有服务启动完成"
	@echo "🌐 Manager API: http://localhost:8080"
	@echo "📖 API文档: http://localhost:8080/swagger/index.html"

down: ## 停止所有服务
	@echo "🛑 停止SysArmor EDR服务..."
	docker compose down
	@echo "✅ 所有服务已停止"

up-dev: ## 构建并启动开发环境 (连接远程middleware)
	@echo "🚀 启动SysArmor EDR开发环境..."
	@if [ ! -f .env.dev ]; then echo "❌ .env.dev 文件不存在"; exit 1; fi
	docker compose -f docker-compose.dev.yml build --no-cache
	docker compose -f docker-compose.dev.yml up -d
	@echo "✅ 开发环境启动完成 (连接到远程middleware: 49.232.13.155)"
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
	docker compose ps

logs: ## 查看服务日志
	@echo "📋 SysArmor EDR服务日志："
	docker compose logs -f

health: ## 系统健康检查
	@echo "🏥 SysArmor EDR健康检查..."
	@curl -s http://localhost:8080/health > /dev/null && echo "✅ Manager: 健康" || echo "❌ Manager: 异常"
	@curl -s http://localhost:9090/-/healthy > /dev/null && echo "✅ Prometheus: 健康" || echo "❌ Prometheus: 异常"
	@curl -s http://localhost:8081/overview > /dev/null && echo "✅ Flink: 健康" || echo "❌ Flink: 异常"
	@curl -s http://localhost:9200/_cluster/health > /dev/null && echo "✅ OpenSearch: 健康" || echo "❌ OpenSearch: 异常"

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
	@echo "配置文件:"
	@echo "  .env     - 单机部署配置"
	@echo "  .env.dev - 开发环境配置 (连接远程middleware)"
	@echo ""
	@echo "部署模式:"
	@echo "  单机部署: make up"
	@echo "  开发环境: make up-dev (连接远程middleware)"
	@echo ""
	@echo "快速开始: make init && make up"
	@echo "API文档: http://localhost:8080/swagger/index.html"
	@echo "部署指南: docs/deployment/README.md"
