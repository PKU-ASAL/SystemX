# SysArmor EDR Monorepo Makefile
.PHONY: help init migrate-repos up deploy down deploy-distributed restart status logs health build test

# Default target
help: ## Show this help message
	@echo "SysArmor EDR Monorepo Management"
	@echo "================================"
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ 初始化和迁移
init: ## 初始化Monorepo
	@echo "🚀 初始化SysArmor EDR Monorepo..."
	@cp .env.example .env
	@echo "✅ 环境变量文件已创建，请根据需要修改 .env 文件"
	@echo "📁 目录结构已就绪"

migrate-repos: ## 迁移现有分散仓库到Monorepo
	@echo "🔄 开始迁移现有仓库..."
	@echo "⚠️  请手动将以下仓库的代码迁移到对应目录："
	@echo "   - sysarmor-manager → services/manager/"
	@echo "   - sysarmor-middleware → services/middleware/"
	@echo "   - sysarmor-processor → services/processor/"
	@echo "   - sysarmor-indexer → services/indexer/"

##@ 部署管理
up: ## 启动所有服务 (开发模式)
	@echo "🚀 启动SysArmor EDR服务..."
	@if [ ! -f .env ]; then cp .env.example .env; fi
	docker compose up -d
	@echo "✅ 服务启动完成"
	@echo "🌐 Manager API: http://localhost:8080"
	@echo "🔍 Consul UI: http://localhost:8500"

deploy: ## 重新构建镜像并部署所有服务
	@echo "🔄 重新构建并部署SysArmor EDR服务..."
	@if [ ! -f .env ]; then cp .env.example .env; fi
	@echo "🛑 停止现有服务..."
	docker compose down
	@echo "🔨 重新构建镜像..."
	docker compose build --no-cache
	@echo "🚀 启动服务..."
	docker compose up -d
	@echo "✅ 部署完成"
	@echo "🌐 Manager API: http://localhost:8080"
	@echo "🔍 Consul UI: http://localhost:8500"

down: ## 停止所有服务
	@echo "🛑 停止SysArmor EDR服务..."
	docker compose down
	@echo "✅ 服务已停止"

deploy-distributed: ## 分布式部署
	@echo "🌐 分布式部署模式"
	@echo "⚠️  请参考 examples/development/README.md 进行分布式部署配置"

##@ 服务管理
restart: ## 重启所有服务
	@echo "🔄 重启SysArmor EDR服务..."
	docker compose restart
	@echo "✅ 服务重启完成"

status: ## 查看服务状态
	@echo "📊 SysArmor EDR服务状态："
	docker compose ps

logs: ## 查看日志
	@echo "📋 SysArmor EDR服务日志："
	docker compose logs -f

health: ## 健康检查
	@echo "🏥 SysArmor EDR健康检查..."
	@echo "检查Manager服务..."
	@curl -s http://localhost:8080/health > /dev/null && echo "✅ Manager: 健康" || echo "❌ Manager: 异常"
	@echo "检查Consul服务..."
	@curl -s http://localhost:8500/v1/status/leader > /dev/null && echo "✅ Consul: 健康" || echo "❌ Consul: 异常"

##@ 开发工具
build: build-manager build-images ## 构建所有组件

build-manager: ## 构建Manager服务
	@echo "🔨 构建Manager服务..."
	@mkdir -p bin
	@if [ -f services/manager/go.mod ]; then cd services/manager && go build -o ../../bin/manager ./cmd/manager; fi
	@echo "✅ Manager构建完成"

build-images: ## 构建所有Docker镜像
	@echo "🐳 构建Docker镜像..."
	@echo "构建Manager镜像..."
	@docker build -t sysarmor/manager:latest -f services/manager/Dockerfile services/manager/
	@echo "构建Middleware镜像..."
	@docker build -t sysarmor/middleware:latest -f services/middleware/Dockerfile services/middleware/
	@echo "构建Processor镜像..."
	@docker build -t sysarmor/processor:latest -f services/processor/Dockerfile services/processor/
	@echo "构建Indexer镜像..."
	@docker build -t sysarmor/indexer:latest -f services/indexer/Dockerfile services/indexer/
	@echo "✅ 所有镜像构建完成"

test: test-manager test-services ## 运行所有测试

test-manager: ## 测试Manager服务
	@echo "🧪 测试Manager服务..."
	@if [ -f services/manager/go.mod ]; then cd services/manager && go test ./...; fi

test-services: ## 测试其他服务
	@echo "🧪 测试Middleware服务..."
	@if [ -f services/middleware/tests/test_agentless_rsyslog_format.sh ]; then cd services/middleware && bash tests/test_agentless_rsyslog_format.sh; fi
	@echo "🧪 测试Processor服务..."
	@if [ -f services/processor/tests/test_collect_kafka_samples.py ]; then cd services/processor && python3 tests/test_collect_kafka_samples.py; fi
	@echo "✅ 所有测试完成"

##@ 配置管理
config-validate: ## 验证环境变量配置
	@echo "🔍 验证配置文件..."
	@if [ ! -f .env ]; then echo "❌ .env 文件不存在，请运行 make init"; exit 1; fi
	@echo "✅ 配置文件验证通过"

##@ 清理
clean: ## 清理构建文件和容器
	@echo "🧹 清理构建文件..."
	@rm -rf bin/
	@echo "🐳 清理容器..."
	docker compose down -v --remove-orphans
	@echo "✅ 清理完成"

##@ 信息
info: ## 显示项目信息
	@echo "SysArmor EDR Monorepo"
	@echo "===================="
	@echo "架构: 控制平面 + 数据平面"
	@echo "控制平面: Manager (Go + Gin)"
	@echo "数据平面: Middleware (Vector+Kafka) + Processor (PyFlink) + Indexer (OpenSearch)"
	@echo "配置模式: 12-Factor App (环境变量驱动)"
	@echo "容器编排: Docker Compose"
	@echo ""
	@echo "核心服务端口:"
	@echo "  Manager:    8080"
	@echo "  Consul:     8500"
	@echo "  Vector:     6000"
	@echo "  Kafka:      9092/9093"
	@echo "  Flink:      8081"
	@echo "  OpenSearch: 9200"
