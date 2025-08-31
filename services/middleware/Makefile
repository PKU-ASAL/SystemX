# SysArmor Middleware Makefile
# 管理 Kafka + Vector 中间件服务

# 加载 .env 文件
include .env
export

.PHONY: help up down restart logs status health clean

# 从环境变量读取配置，提供默认值 (KRaft模式，无需Zookeeper)
KAFKA_PORT ?= 9092
KAFKA_EXTERNAL_PORT ?= 9094
VECTOR_API_PORT ?= 8686
VECTOR_METRICS_PORT ?= 9598
VECTOR_TCP_PORT ?= 514
KAFKA_UI_PORT ?= 8080
PROMETHEUS_PORT ?= 9090
YOUR_PUBLIC_IP ?= localhost

# 自动检测 Docker Compose 命令 (兼容新旧版本)
DOCKER_COMPOSE := $(shell if command -v docker-compose >/dev/null 2>&1; then echo "docker-compose"; elif docker compose version >/dev/null 2>&1; then echo "docker compose"; else echo "docker-compose"; fi)

# 默认目标
help:
	@echo "SysArmor Middleware 管理工具"
	@echo "============================="
	@echo ""
	@echo "🚀 基础命令:"
	@echo "  up                       - 启动所有中间件服务"
	@echo "  down                     - 停止所有中间件服务"
	@echo "  restart                  - 重启所有中间件服务"
	@echo "  logs                     - 查看所有服务日志"
	@echo "  status                   - 查看服务状态"
	@echo "  health                   - 综合健康检查"
	@echo "  clean                    - 清理容器"
	@echo ""
	@echo "📨 Kafka 管理:"
	@echo "  kafka-topics             - 列出所有 topics"
	@echo "  kafka-create TOPIC=name  - 创建 topic"
	@echo "  kafka-consume TOPIC=name - 消费消息"
	@echo ""
	@echo "📊 Vector 管理:"
	@echo "  vector-health            - Vector 健康检查"
	@echo "  vector-metrics           - 查看 Vector 指标"
	@echo ""
	@echo "📋 监控管理:"
	@echo "  setup-monitoring         - 设置监控组件"
	@echo "  monitoring-health        - 监控系统健康检查"
	@echo "  key-metrics              - 查看关键指标"
	@echo "  prometheus-ui            - 打开 Prometheus UI"
	@echo "  query-examples           - 常用指标查询示例"
	@echo ""
	@echo "📋 服务访问:"
	@echo "  - Vector API:    http://$(YOUR_PUBLIC_IP):$(VECTOR_API_PORT)"
	@echo "  - Kafka UI:      http://$(YOUR_PUBLIC_IP):$(KAFKA_UI_PORT)"
	@echo "  - Kafka ex-port: $(YOUR_PUBLIC_IP):$(KAFKA_EXTERNAL_PORT)"
	@echo "  - Prometheus:    http://$(YOUR_PUBLIC_IP):$(PROMETHEUS_PORT)"
	@echo "  - Agent TCP:     $(YOUR_PUBLIC_IP):$(VECTOR_TCP_PORT)"

# 启动所有服务
up:
	@echo "🚀 启动 SysArmor Middleware..."
	@$(DOCKER_COMPOSE) --env-file .env up -d
	@echo "⏳ 等待服务启动..."
	@sleep 15
	@echo "✅ SysArmor Middleware 启动完成!"
	@echo ""
	@echo "📊 服务访问地址:"
	@echo "  - Vector API:    http://$(YOUR_PUBLIC_IP):$(VECTOR_API_PORT)"
	@echo "  - Kafka UI:      http://$(YOUR_PUBLIC_IP):$(KAFKA_UI_PORT)"
	@echo "  - Prometheus:    http://$(YOUR_PUBLIC_IP):$(PROMETHEUS_PORT)"
	@echo "  - Agent TCP:     $(YOUR_PUBLIC_IP):$(VECTOR_TCP_PORT)"
	@echo ""
	@echo "📈 监控指标端点:"
	@echo "  - Vector 指标:   	http://$(YOUR_PUBLIC_IP):$(VECTOR_METRICS_PORT)/metrics"
	@echo "  - Kafka JMX 指标:  http://$(YOUR_PUBLIC_IP):7071/metrics"
	@echo "  - Prometheus API: http://$(YOUR_PUBLIC_IP):$(PROMETHEUS_PORT)/api/v1/query"
	@echo "🔧 监控管理命令:"
	@echo "  - 监控健康检查:    make monitoring-health"
	@echo "  - 查看关键指标:    make key-metrics"
	@echo "  - 打开 Prometheus: make prometheus-ui"

# 停止所有服务
down:
	@echo "🛑 停止 SysArmor Middleware..."
	@$(DOCKER_COMPOSE) down
	@echo "✅ 所有服务已停止"

# 重启所有服务
restart:
	@echo "🔄 重启 SysArmor Middleware..."
	@$(MAKE) down
	@sleep 3
	@$(MAKE) up

# 查看所有服务日志
logs:
	@echo "📋 查看所有服务日志..."
	@$(DOCKER_COMPOSE) logs -f

# 查看服务状态
status:
	@echo "🔍 检查 SysArmor Middleware 服务状态..."
	@$(DOCKER_COMPOSE) ps 2>/dev/null || echo "❌ 服务未运行"
	@echo ""
	@$(MAKE) health

# 综合健康检查 (KRaft模式，无需Zookeeper)
health:
	@echo "🏥 SysArmor Middleware 健康检查 (KRaft模式)..."
	@echo ""
	@echo "=== Kafka (KRaft模式) ==="
	@if docker exec sysarmor-middleware-kafka kafka-broker-api-versions --bootstrap-server localhost:$(KAFKA_PORT) >/dev/null 2>&1; then \
		echo "✅ Kafka: 健康运行 (KRaft模式，端口 $(KAFKA_PORT))"; \
	else \
		echo "❌ Kafka: 异常或未启动"; \
	fi
	@echo ""
	@echo "=== Vector ==="
	@if curl -s http://$(YOUR_PUBLIC_IP):$(VECTOR_API_PORT)/health >/dev/null 2>&1; then \
		echo "✅ Vector: 健康运行 (http://$(YOUR_PUBLIC_IP):$(VECTOR_API_PORT))"; \
	else \
		echo "❌ Vector: 异常或未启动"; \
	fi
	@echo ""
	@echo "=== Kafka UI ==="
	@if curl -s http://$(YOUR_PUBLIC_IP):$(KAFKA_UI_PORT) >/dev/null 2>&1; then \
		echo "✅ Kafka UI: 健康运行 (http://$(YOUR_PUBLIC_IP):$(KAFKA_UI_PORT))"; \
	else \
		echo "❌ Kafka UI: 异常或未启动"; \
	fi
	@echo ""
	@echo "=== Prometheus ==="
	@if curl -s http://$(YOUR_PUBLIC_IP):$(PROMETHEUS_PORT)/-/healthy >/dev/null 2>&1; then \
		echo "✅ Prometheus: 健康运行 (http://$(YOUR_PUBLIC_IP):$(PROMETHEUS_PORT))"; \
	else \
		echo "❌ Prometheus: 异常或未启动"; \
	fi

# 清理容器
clean:
	@echo "🧹 清理 SysArmor Middleware 容器..."
	@$(DOCKER_COMPOSE) down
	@$(DOCKER_COMPOSE) rm -f
	@echo "✅ 容器清理完成"

# 完全清理 (包括数据卷和缓存)
clean-all:
	@echo "🗑️  执行完全清理..."
	@echo "🛑 停止所有服务..."
	@$(DOCKER_COMPOSE) down --volumes --remove-orphans
	@echo "🧹 清理容器、网络和数据卷..."
	@$(DOCKER_COMPOSE) rm -f
	@echo "🗑️  清理未使用的镜像..."
	@docker image prune -f
	@echo "🗑️  清理未使用的数据卷..."
	@docker volume prune -f
	@echo "🗑️  清理构建缓存..."
	@docker builder prune -f
	@echo "✅ 完全清理完成"

# 重建并启动 (清理缓存)
rebuild:
	@echo "🔨 重建 SysArmor Middleware (清理缓存)..."
	@echo "🛑 停止现有服务..."
	@$(DOCKER_COMPOSE) down --volumes --remove-orphans
	@echo "🧹 清理构建缓存..."
	@docker builder prune -f
	@echo "🔨 重新构建镜像 (无缓存)..."
	@$(DOCKER_COMPOSE) build --no-cache
	@echo "🚀 启动服务..."
	@$(DOCKER_COMPOSE) up -d
	@echo "⏳ 等待服务启动..."
	@sleep 20
	@echo "✅ 重建完成!"
	@$(MAKE) health

# 快速重建 (保留缓存)
rebuild-fast:
	@echo "⚡ 快速重建 SysArmor Middleware..."
	@$(DOCKER_COMPOSE) down
	@$(DOCKER_COMPOSE) build
	@$(DOCKER_COMPOSE) up -d
	@echo "⏳ 等待服务启动..."
	@sleep 15
	@echo "✅ 快速重建完成!"
	@$(MAKE) health

# Vector 健康检查
vector-health:
	@echo "🏥 Vector 健康检查..."
	@curl -s http://$(YOUR_PUBLIC_IP):$(VECTOR_API_PORT)/health | jq . 2>/dev/null || echo "❌ 无法连接到 Vector API"

# Vector 指标
vector-metrics:
	@echo "📊 Vector 指标..."
	@curl -s http://$(YOUR_PUBLIC_IP):$(VECTOR_METRICS_PORT)/metrics 2>/dev/null || echo "❌ 无法获取 Vector 指标"

# Kafka Topics 列表
kafka-topics:
	@echo "📋 列出所有 Kafka topics..."
	@docker exec sysarmor-middleware-kafka kafka-topics --list --bootstrap-server localhost:$(KAFKA_PORT)

# 创建 Kafka Topic
kafka-create:
	@if [ -z "$(TOPIC)" ]; then \
		echo "❌ 请指定 TOPIC 名称: make kafka-create TOPIC=my-topic"; \
		exit 1; \
	fi
	@echo "📨 创建 Kafka topic: $(TOPIC)..."
	@docker exec sysarmor-middleware-kafka kafka-topics \
		--create --bootstrap-server localhost:$(KAFKA_PORT) \
		--topic $(TOPIC) --partitions 3 --replication-factor 1
	@echo "✅ Topic $(TOPIC) 创建成功"

# 消费 Kafka 消息
kafka-consume:
	@if [ -z "$(TOPIC)" ]; then \
		echo "❌ 请指定 TOPIC 名称: make kafka-consume TOPIC=my-topic"; \
		exit 1; \
	fi
	@echo "📨 消费 Kafka topic: $(TOPIC)..."
	@echo "按 Ctrl+C 停止消费"
	@docker exec -it sysarmor-middleware-kafka kafka-console-consumer \
		--bootstrap-server localhost:$(KAFKA_PORT) --topic $(TOPIC) --from-beginning

# === 监控相关命令 ===

# 设置监控
setup-monitoring:
	@echo "🔧 设置监控组件..."
	@./scripts/setup-monitoring.sh

# 监控健康检查
monitoring-health:
	@echo "🏥 监控系统健康检查..."
	@echo "=== Vector ==="
	@if curl -s http://localhost:$(VECTOR_METRICS_PORT)/health >/dev/null 2>&1; then \
		echo "✅ Vector: 健康运行"; \
	else \
		echo "❌ Vector: 异常或未启动"; \
	fi
	@echo ""
	@echo "=== Kafka JMX ==="
	@if curl -s http://localhost:7071/metrics >/dev/null 2>&1; then \
		echo "✅ Kafka JMX: 健康运行"; \
	else \
		echo "❌ Kafka JMX: 异常或未启动"; \
	fi
	@echo ""
	@echo "=== Prometheus ==="
	@if curl -s http://localhost:$(PROMETHEUS_PORT)/-/healthy >/dev/null 2>&1; then \
		echo "✅ Prometheus: 健康运行"; \
	else \
		echo "❌ Prometheus: 异常或未启动"; \
	fi

# 查看关键指标
key-metrics:
	@echo "📊 SysArmor Middleware 关键指标..."
	@echo ""
	@echo "=== Broker 整体指标 ==="
	@echo "总消息数:"
	@curl -s "http://localhost:$(PROMETHEUS_PORT)/api/v1/query?query=kafka_broker_messages_total" | jq -r '.data.result[0].value[1] // "N/A"' 2>/dev/null || echo "N/A"
	@echo "消息速率 (msg/sec):"
	@curl -s "http://localhost:$(PROMETHEUS_PORT)/api/v1/query?query=kafka_broker_messages_per_sec" | jq -r '.data.result[0].value[1] // "N/A"' 2>/dev/null || echo "N/A"
	@echo "总存储空间 (bytes):"
	@curl -s "http://localhost:$(PROMETHEUS_PORT)/api/v1/query?query=kafka_broker_log_size_total_bytes" | jq -r '.data.result[0].value[1] // "N/A"' 2>/dev/null || echo "N/A"
	@echo "JVM 堆内存使用 (bytes):"
	@curl -s "http://localhost:$(PROMETHEUS_PORT)/api/v1/query?query=kafka_jvm_memory_heap_used_bytes" | jq -r '.data.result[0].value[1] // "N/A"' 2>/dev/null || echo "N/A"
	@echo ""
	@echo "=== Topic 级别指标 (前5个) ==="
	@curl -s "http://localhost:$(PROMETHEUS_PORT)/api/v1/query?query=topk(5,kafka_topic_messages_total)" | jq -r '.data.result[]? | "\(.metric.topic): \(.value[1]) messages"' 2>/dev/null || echo "N/A"

# 打开 Prometheus UI
prometheus-ui:
	@echo "🌐 打开 Prometheus UI..."
	@open http://localhost:$(PROMETHEUS_PORT) 2>/dev/null || echo "访问: http://localhost:$(PROMETHEUS_PORT)"

# 常用 PromQL 查询示例
query-examples:
	@echo "📈 常用指标查询示例..."
	@echo ""
	@echo "1. 整体消息速率:"
	@echo "   kafka_broker_messages_per_sec"
	@echo ""
	@echo "2. 每个 Topic 的消息数:"
	@echo "   kafka_topic_messages_total"
	@echo ""
	@echo "3. 每个 Topic 的存储空间:"
	@echo "   sum by (topic) (kafka_topic_partition_log_size_bytes)"
	@echo ""
	@echo "4. Top 5 最活跃的 Topic:"
	@echo "   topk(5, rate(kafka_topic_messages_total[5m]))"
	@echo ""
	@echo "5. JVM 内存使用率:"
	@echo "   kafka_jvm_memory_heap_used_bytes / kafka_jvm_memory_heap_max_bytes * 100"
	@echo ""
	@echo "6. Vector 事件处理速率:"
	@echo "   rate(vector_events_in_total[5m])"
	@echo ""
	@echo "7. Kafka 字节速率:"
	@echo "   kafka_broker_bytes_in_per_sec"
