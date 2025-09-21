# SysArmor EDR Monorepo Makefile
.PHONY: help init deploy up down restart status health test clean info

# Default target
help: ## 显示帮助信息
	@echo "SysArmor EDR 系统管理"
	@echo "===================="
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ 🚀 部署操作
init: ## 初始化项目环境
	@echo "🚀 初始化SysArmor EDR项目..."
	@mkdir -p data/api-exports data/kafka-imports data/logs data/backups
	@if [ ! -f .env ]; then cp .env.example .env; echo "✅ 环境配置文件已创建"; fi
	@if [ ! -f services/indexer/configs/opensearch/certs/node.pem ]; then \
		cd services/indexer && chmod +x scripts/generate-certs.sh && ./scripts/generate-certs.sh; \
		echo "✅ OpenSearch SSL证书已生成"; \
	fi
	@echo "✅ 项目初始化完成"

deploy: ## 🎯 完整部署 (推荐)
	@echo "🔨 构建并启动SysArmor EDR系统..."
	@if [ ! -f .env ]; then cp .env.example .env; fi
	docker compose build --no-cache
	docker compose up -d
	@echo "✅ 所有服务构建并启动完成"
	@echo ""
	@echo "🚀 自动初始化数据处理流程..."
	@./scripts/auto-init-processor.sh
	@echo ""
	@echo "🎉 SysArmor EDR 系统完全就绪！"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "📋 系统访问地址:"
	@echo "   🌐 Manager API: http://localhost:8080"
	@echo "   📖 API文档: http://localhost:8080/swagger/index.html"
	@echo "   🔧 Flink监控: http://localhost:8081"
	@echo "   📊 Prometheus: http://localhost:9090"
	@echo "   🔍 OpenSearch: http://localhost:9200"
	@echo ""
	@echo "🧪 系统测试命令:"
	@echo "   ./tests/test-system-health.sh     # 快速健康检查"
	@echo "   ./tests/test-system-api.sh        # 完整API测试"
	@echo "   ./tests/import-events-data.sh     # 事件数据导入"
	@echo ""
	@echo "📊 数据流状态: auditd → events → alerts (已激活)"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

up: ## 启动服务 (不重新构建)
	@echo "🚀 启动SysArmor EDR服务..."
	@if [ ! -f .env ]; then cp .env.example .env; fi
	docker compose up -d
	@echo "✅ 服务启动完成"
	@echo "🌐 Manager API: http://localhost:8080"

down: ## 停止所有服务
	@echo "🛑 停止SysArmor EDR服务..."
	docker compose down -v --remove-orphans
	@echo "✅ 所有服务已停止"

restart: ## 重启所有服务
	@echo "🔄 重启SysArmor EDR服务..."
	docker compose restart
	@echo "✅ 服务重启完成"

##@ 🔍 监控测试
status: ## 查看服务状态
	@echo "📊 SysArmor EDR服务状态："
	@docker compose ps 2>/dev/null || docker ps --filter "label=sysarmor.module" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

health: ## 系统健康检查
	@echo "🏥 SysArmor EDR健康检查..."
	@./tests/test-system-health.sh

test: ## 运行完整系统测试
	@echo "🧪 运行SysArmor EDR完整测试..."
	@echo "1️⃣  系统健康检查..."
	@./tests/test-system-health.sh
	@echo ""
	@echo "2️⃣  API接口测试..."
	@./tests/test-system-api.sh
	@echo ""
	@echo "🎉 完整系统测试完成！"

##@ 🛠️ 开发环境
dev-up: ## 启动开发环境 (连接远程middleware)
	@echo "🚀 启动开发环境..."
	@if [ ! -f .env.dev ]; then echo "❌ .env.dev 文件不存在"; exit 1; fi
	docker compose -f docker-compose.dev.yml up -d
	@echo "✅ 开发环境启动完成"

dev-down: ## 停止开发环境
	@echo "🛑 停止开发环境..."
	docker compose -f docker-compose.dev.yml down -v --remove-orphans
	@echo "✅ 开发环境已停止"

build-ui: ## 构建UI Docker服务
	@echo "🔨 构建UI Docker服务..."
	@cd apps/ui && docker compose build
	@echo "✅ UI Docker服务构建完成"


##@ 🧹 清理维护
clean: ## 清理构建文件和容器
	@echo "🧹 清理构建文件和容器..."
	@rm -rf bin/ data/api-exports/* data/logs/*
	docker compose down -v --remove-orphans
	docker system prune -f
	@echo "✅ 清理完成"

##@ ℹ️ 信息帮助
info: ## 显示项目信息
	@echo "SysArmor EDR/HIDS 系统"
	@echo "====================="
	@echo "架构: Monorepo + 微服务"
	@echo "控制平面: Manager (Go + Gin + Swagger)"
	@echo "数据平面: Middleware + Processor + Indexer"
	@echo ""
	@echo "核心服务:"
	@echo "  Manager:    8080  (API + Swagger UI)"
	@echo "  Vector:     6000  (数据收集)"
	@echo "  Kafka:      9094  (消息队列)"
	@echo "  Flink:      8081  (流处理)"
	@echo "  OpenSearch: 9200  (搜索引擎)"
	@echo "  Prometheus: 9090  (监控)"
	@echo ""
	@echo "快速开始:"
	@echo "  make init    # 初始化环境"
	@echo "  make deploy  # 完整部署"
	@echo "  make test    # 系统测试"
	@echo ""
	@echo "常用命令:"
	@echo "  make status  # 查看服务状态"
	@echo "  make health  # 健康检查"
	@echo "  make test    # 完整测试"
	@echo "  make clean   # 清理环境"

# 允许make命令接受参数
%:
	@:
