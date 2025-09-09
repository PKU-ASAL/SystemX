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

up-middleware: ## 构建并启动开发环境 (单独部署middleware)
	@echo "🚀 启动SysArmor EDR开发环境..."
	@if [ ! -f .env.middleware ]; then echo "❌ .env.middleware 文件不存在"; exit 1; fi
	docker compose -f docker-compose.middleware.yml build --no-cache
	docker compose -f docker-compose.middleware.yml up -d
	@echo "✅ 开发环境启动完成 (已部署middleware)"

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

logs: ## 查看服务日志
	@echo "📋 SysArmor EDR服务日志："
	docker compose logs -f

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
		echo "  logs-vector      - 查看Vector日志"; \
		echo "  logs-kafka       - 查看Kafka日志"; \
		echo "  logs-prometheus  - 查看Prometheus日志"; \
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

middleware-logs-vector:
	@echo "📋 SysArmor Middleware - Vector日志："
	@if docker ps --format "table {{.Names}}" | grep -q "vector"; then \
		docker logs $$(docker ps --format "table {{.Names}}" | grep vector | head -1) --tail 50 -f; \
	else \
		echo "❌ Vector容器未运行"; \
	fi

middleware-logs-kafka:
	@echo "📋 SysArmor Middleware - Kafka日志："
	@if docker ps --format "table {{.Names}}" | grep -q "kafka"; then \
		docker logs $$(docker ps --format "table {{.Names}}" | grep kafka | head -1) --tail 50 -f; \
	else \
		echo "❌ Kafka容器未运行"; \
	fi

middleware-logs-prometheus:
	@echo "📋 SysArmor Middleware - Prometheus日志："
	@if docker ps --format "table {{.Names}}" | grep -q "prometheus"; then \
		docker logs $$(docker ps --format "table {{.Names}}" | grep prometheus | head -1) --tail 50 -f; \
	else \
		echo "❌ Prometheus容器未运行"; \
	fi

middleware-test-kafka:
	@echo "📡 SysArmor Middleware - 测试Kafka连接..."
	@make test-kafka

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
		echo "可用命令:"; \
		echo "  list-jobs        - 查看Flink作业列表"; \
		echo "  submit-console   - 提交简单控制台测试作业"; \
		echo "  submit-auditd-sysdig - 提交Auditd到Sysdig转换测试作业"; \
		echo "  submit-multi-topic - 提交多Topic进程树构建作业 (开发中)"; \
		echo "  cancel-job JOB_ID=xxx - 取消指定作业"; \
		echo "  logs-jobmanager  - 查看JobManager日志"; \
		echo "  logs-taskmanager - 查看TaskManager日志 (控制台输出)"; \
		echo "  overview         - 查看Flink集群概览"; \
		echo "  status           - 查看Processor服务状态"; \
		echo "  test             - 快速测试Processor功能"; \
		echo ""; \
		echo "示例:"; \
		echo "  make processor list-jobs"; \
		echo "  make processor submit-console"; \
		echo "  make processor submit-auditd-sysdig"; \
		echo "  make processor submit-multi-topic"; \
		echo "  make processor logs-taskmanager"; \
	else \
		$(MAKE) processor-$(filter-out $@,$(MAKECMDGOALS)); \
	fi

processor-list-jobs:
	@echo "📋 SysArmor Processor - Flink作业列表："
	@echo "通过Manager API查询:"
	@curl -s http://localhost:8080/api/v1/services/flink/jobs 2>/dev/null | jq -r '.data.jobs[]? | "  🎯 作业名称: \(if .name == "" then "未知" else .name end) | 状态: \(if .state == "" then "未知" else .state end) | ID: \(.id)"' 2>/dev/null || \
	(echo "  ⚠️  Manager API不可用，尝试直接访问Flink..." && \
	 curl -s http://localhost:8081/jobs 2>/dev/null | jq -r '.jobs[]? | "  🎯 Job ID: \(.id) | 状态: \(.status)"' 2>/dev/null || \
	 echo "  ❌ Flink集群不可用")

processor-submit-console:
	@echo "🖥️  SysArmor Processor - 提交简单控制台测试作业..."
	@if docker ps --format "table {{.Names}}" | grep -q "flink-jobmanager"; then \
		docker compose exec flink-jobmanager flink run -py /opt/flink/usr_jobs/job_test_simple_console.py; \
		echo "✅ 简单控制台测试作业已提交!"; \
		echo "🔍 查看输出: make processor logs-taskmanager"; \
		echo "📊 监控: http://localhost:8081"; \
	else \
		echo "❌ Flink JobManager容器未运行，请先启动: make up-dev"; \
	fi

processor-submit-auditd-sysdig:
	@echo "🔄 SysArmor Processor - 提交Auditd到Sysdig转换测试作业..."
	@if docker ps --format "table {{.Names}}" | grep -q "flink-jobmanager"; then \
		docker compose exec flink-jobmanager flink run -py /opt/flink/usr_jobs/job_test_auditd_sysdig_console.py; \
		echo "✅ Auditd到Sysdig转换测试作业已提交!"; \
		echo "🔄 基于NODLINK管道处理逻辑"; \
		echo "📥 消费: sysarmor-events-test"; \
		echo "📤 输出: 控制台 (sysdig格式)"; \
		echo "🔍 查看输出: make processor logs-taskmanager"; \
		echo "📊 监控: http://localhost:8081"; \
	else \
		echo "❌ Flink JobManager容器未运行，请先启动: make up-dev"; \
	fi

processor-submit-multi-topic:
	@echo "🌐 SysArmor Processor - 提交多Topic进程树构建作业 (开发中)..."
	@if docker ps --format "table {{.Names}}" | grep -q "flink-jobmanager"; then \
		docker compose exec flink-jobmanager flink run -py /opt/flink/usr_jobs/job_multi_topic_process_tree_builder.py; \
		echo "✅ 多Topic进程树构建作业已提交!"; \
		echo "🌐 支持同时处理多个 sysarmor-agentless-* topics"; \
		echo "🔄 每个 collector 独立处理进程树重建"; \
		echo "📥 消费: sysarmor-agentless-*"; \
		echo "📤 输出: sysarmor-audit-unified (统一路由)"; \
		echo "🔍 查看输出: make processor logs-taskmanager"; \
		echo "📊 监控: http://localhost:8081"; \
		echo "⚠️  注意: 此作业仍在开发中，可能存在不稳定性"; \
	else \
		echo "❌ Flink JobManager容器未运行，请先启动: make up-dev"; \
	fi

processor-cancel-job:
	@if [ -z "$(JOB_ID)" ]; then \
		echo "❌ 请指定作业ID: make processor cancel-job JOB_ID=your_job_id"; \
		echo "💡 获取作业ID: make processor list-jobs"; \
		exit 1; \
	fi
	@echo "🛑 SysArmor Processor - 取消Flink作业 $(JOB_ID)..."
	@if docker ps --format "table {{.Names}}" | grep -q "flink-jobmanager"; then \
		docker compose exec flink-jobmanager flink cancel $(JOB_ID); \
		echo "✅ 作业 $(JOB_ID) 已取消"; \
	else \
		echo "❌ Flink JobManager容器未运行"; \
	fi

processor-logs-jobmanager:
	@echo "📋 SysArmor Processor - Flink JobManager日志："
	@if docker ps --format "table {{.Names}}" | grep -q "flink-jobmanager"; then \
		docker logs $$(docker ps --format "table {{.Names}}" | grep flink-jobmanager | head -1) --tail 50 -f; \
	else \
		echo "❌ Flink JobManager容器未运行"; \
	fi

processor-logs-taskmanager:
	@echo "📋 SysArmor Processor - Flink TaskManager日志 (控制台输出)："
	@if docker ps --format "table {{.Names}}" | grep -q "flink-taskmanager"; then \
		docker logs $$(docker ps --format "table {{.Names}}" | grep flink-taskmanager | head -1) --tail 50 -f; \
	else \
		echo "❌ Flink TaskManager容器未运行"; \
	fi

processor-overview:
	@echo "📊 SysArmor Processor - Flink集群概览："
	@echo "通过Manager API查询:"
	@curl -s http://localhost:8080/api/v1/services/flink/overview 2>/dev/null | jq '.' 2>/dev/null || \
	(echo "⚠️  Manager API不可用，尝试直接访问Flink..." && \
	 curl -s http://localhost:8081/overview 2>/dev/null | jq '.' 2>/dev/null || \
	 echo "❌ Flink集群不可用")

processor-status:
	@echo "📊 SysArmor Processor - 服务状态："
	@docker ps --filter "label=sysarmor.module=processor" --format "table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}"

processor-test:
	@echo "🚀 SysArmor Processor - 快速测试流程..."
	@echo "1️⃣  检查Processor服务状态..."
	@make processor-status
	@echo ""
	@echo "2️⃣  查看Flink集群概览..."
	@make processor-overview
	@echo ""
	@echo "3️⃣  提交简单控制台测试作业..."
	@make processor-submit-console
	@echo ""
	@echo "4️⃣  查看作业列表..."
	@sleep 3
	@make processor-list-jobs
	@echo ""
	@echo "✅ Processor快速测试完成!"
	@echo "🔍 查看实时输出: make processor logs-taskmanager"
	@echo "📊 Web监控: http://localhost:8081"

# Indexer 服务管理
indexer: ## Indexer服务管理 (用法: make indexer <command>)
	@if [ -z "$(filter-out $@,$(MAKECMDGOALS))" ]; then \
		echo "🔍 SysArmor Indexer 服务管理"; \
		echo "============================="; \
		echo "用法: make indexer <command>"; \
		echo ""; \
		echo "可用命令:"; \
		echo "  status           - 查看Indexer服务状态"; \
		echo "  logs-opensearch  - 查看OpenSearch日志"; \
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

indexer-logs-opensearch:
	@echo "📋 SysArmor Indexer - OpenSearch日志："
	@if docker ps --format "table {{.Names}}" | grep -q "opensearch"; then \
		docker logs $$(docker ps --format "table {{.Names}}" | grep opensearch | head -1) --tail 50 -f; \
	else \
		echo "❌ OpenSearch容器未运行"; \
	fi

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
		echo "  logs             - 查看Manager日志"; \
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

manager-logs:
	@echo "📋 SysArmor Manager - 日志："
	@if docker ps --format "table {{.Names}}" | grep -q "manager"; then \
		docker logs $$(docker ps --format "table {{.Names}}" | grep manager | head -1) --tail 50 -f; \
	else \
		echo "❌ Manager容器未运行"; \
	fi

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

##@ 快速测试
test-flink: ## 快速测试Flink功能 (一键测试流程)
	@echo "🚀 SysArmor Flink快速测试流程..."
	@echo "1️⃣  检查环境状态..."
	@make health
	@echo ""
	@echo "2️⃣  查看Flink集群概览..."
	@make processor-overview
	@echo ""
	@echo "3️⃣  提交简单控制台测试作业..."
	@make processor-submit-console
	@echo ""
	@echo "4️⃣  查看作业列表..."
	@sleep 3
	@make processor-list-jobs
	@echo ""
	@echo "✅ 快速测试完成!"
	@echo "🔍 查看实时输出: make processor logs-taskmanager"
	@echo "📊 Web监控: http://localhost:8081"
	@echo "💡 发送测试数据: make test-kafka"

test-kafka: ## 测试Kafka连接和发送消息
	@echo "📡 测试SysArmor Kafka连接..."
	@echo "1️⃣  检查Kafka健康状态..."
	@curl -s http://localhost:8080/api/v1/services/kafka/health | jq '.' || echo "❌ Kafka不可用"
	@echo ""
	@echo "2️⃣  查看可用Topics..."
	@curl -s http://localhost:8080/api/v1/services/kafka/topics | jq '.data' || echo "❌ 无法获取Topics"
	@echo ""
	@echo "3️⃣  发送测试消息到 sysarmor-events-test..."
	@if [ -f scripts/kafka-tools.sh ]; then \
		cd scripts && KAFKA_BROKERS=localhost:9094 ./kafka-tools.sh send sysarmor-events-test \
		"{\"timestamp\":\"$$(date -Iseconds)\",\"host\":\"test-host\",\"message\":\"Kafka test message from Makefile\",\"collector_id\":\"makefile-test\"}"; \
		echo "✅ 测试消息已发送!"; \
	else \
		echo "❌ kafka-tools.sh 脚本不存在"; \
	fi
	@echo ""
	@echo "4️⃣  验证消息..."
	@curl -s "http://localhost:8080/api/v1/services/kafka/topics/sysarmor-events-test/messages?limit=3" | jq '.data' || echo "❌ 无法读取消息"

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
