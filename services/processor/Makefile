# SysArmor Processor - Makefile
# PyFlink 实时威胁检测处理器

.PHONY: help up down clean logs status submit-datastream submit-configurable list-jobs cancel-job setup

# 默认目标 - 显示帮助信息
default: help

#==============================================================================
# Flink 集群管理
#==============================================================================

# 启动 Flink 集群
up:
	@echo "🚀 Starting SysArmor Flink Cluster..."
	docker compose up -d --build
	@echo "✅ Flink cluster started!"
	@echo "🌐 Web UI: http://localhost:8081"

# 停止 Flink 集群
down:
	@echo "🛑 Stopping SysArmor Flink Cluster..."
	docker compose down
	@echo "✅ Flink cluster stopped!"

# 清理 Flink 集群和数据
clean:
	@echo "🗑️  Cleaning up SysArmor Flink Cluster..."
	docker compose down -v
	docker system prune -f
	@echo "✅ Cleanup completed!"

# 查看日志
logs:
	@echo "📋 SysArmor Flink Logs:"
	docker compose logs -f

# 查看服务状态
status:
	@echo "📊 SysArmor Flink Status:"
	docker compose ps

#==============================================================================
# 作业管理
#==============================================================================

# 提交基础威胁检测作业 (DataStream API)
submit-datastream:
	@echo "🌊 Submitting DataStream API Threat Detection Job..."
	docker compose exec flink-jobmanager flink run -py /opt/flink/usr_jobs/job_rules_filter_datastream.py
	@echo "✅ DataStream threat detection job submitted!"
	@echo "🔧 Using DataStream API for better error handling"
	@echo "🎯 Stateful consecutive sudo detection with memory"
	@echo "🚨 Enhanced debugging and logging capabilities"
	@echo "🖨️  All threats printed to console with 'THREAT_DETECTED'"
	@echo "🌐 Monitor at: http://localhost:8081"

# 提交基于配置文件的威胁检测作业 (推荐)
submit-configurable:
	@echo "⚙️  Submitting Configurable Threat Detection Job..."
	docker compose exec flink-jobmanager flink run -py /opt/flink/usr_jobs/job_rules_configuration_datastream.py
	@echo "✅ Configurable threat detection job submitted!"
	@echo "🔧 基于 threat_detection_rules.yaml 配置文件"
	@echo "🎯 动态规则加载和热重载支持"
	@echo "🚨 频率基础威胁检测与时间窗口"
	@echo "📋 灵活模式匹配 (关键词 + 正则 + 条件)"
	@echo "🎚️  可配置风险评分和严重程度"
	@echo "📊 规则分组和分类支持"
	@echo "🖨️  All threats printed with 'CONFIG_THREAT_DETECTED' prefix"
	@echo "🌐 Monitor at: http://localhost:8081"

# 提交简单控制台测试作业 (用于基本功能验证)
submit-console-test:
	@echo "🖥️  Submitting Simple Console Test Job..."
	docker compose exec flink-jobmanager flink run -py /opt/flink/usr_jobs/job_simple_console_test.py
	@echo "✅ Simple console test job submitted!"
	@echo "🔍 读取指定 topic prefix 的数据并输出到控制台"
	@echo "📋 默认监听 topics: sysarmor-events-test, sysarmor-agentless-*"
	@echo "🎯 单并行度，便于观察输出顺序"
	@echo "📝 消息格式: '🔍 MESSAGE #N | Time | Host | Collector | Content'"
	@echo "🖨️  All messages printed to TaskManager console"
	@echo "🌐 Monitor at: http://localhost:8081"
	@echo "📊 Check TaskManager logs: docker logs processor-taskmanager"

# 列出所有作业
list-jobs:
	@echo "📋 Active Flink Jobs:"
	docker compose exec flink-jobmanager flink list

# 取消作业 (需要指定 JOB_ID)
cancel-job:
	@if [ -z "$(JOB_ID)" ]; then \
		echo "❌ Please specify JOB_ID: make cancel-job JOB_ID=your_job_id"; \
		exit 1; \
	fi
	@echo " Cancelling job $(JOB_ID)..."
	docker compose exec flink-jobmanager flink cancel $(JOB_ID)

#==============================================================================
# 开发和测试
#==============================================================================

# 设置开发环境
setup:
	@echo "⚙️  Setting up SysArmor Processor..."
	@mkdir -p jobs
	@echo "✅ Setup completed!"
	@echo "📝 Please configure environment variables in .env"

# 帮助信息
help:
	@echo "🚀 SysArmor Processor - PyFlink 实时威胁检测处理器"
	@echo "=================================================="
	@echo ""
	@echo "🛠️  Cluster Management:"
	@echo "  make up             - 启动 Flink 集群"
	@echo "  make down           - 停止 Flink 集群"
	@echo "  make clean          - 清理集群和数据"
	@echo "  make status         - 查看服务状态"
	@echo "  make logs           - 查看所有日志"
	@echo ""
	@echo "📋 Job Management:"
	@echo "  make submit-console-test - 提交简单控制台测试作业 (基本功能验证)"
	@echo "  make submit-datastream   - 提交基础威胁检测作业 (DataStream API)"
	@echo "  make submit-configurable - 提交基于配置文件的威胁检测作业 (推荐)"
	@echo "  make list-jobs           - 列出所有作业"
	@echo "  make cancel-job JOB_ID=xxx - 取消指定作业"
	@echo ""
	@echo "⚙️  Development:"
	@echo "  make setup          - 初始化开发环境"
	@echo ""
	@echo "🌐 Web Interfaces:"
	@echo "  Flink Web UI:       http://localhost:8081"
	@echo ""
	@echo "📖 Quick Start:"
	@echo "  1. make setup            # 初始化环境"
	@echo "  2. make up               # 启动集群"
	@echo "  3. make submit-console-test # 提交简单测试作业 (验证基本功能)"
	@echo "  4. make status           # 检查状态"
	@echo "  5. docker logs processor-taskmanager # 查看控制台输出"
	@echo ""
	@echo "💡 Available Jobs:"
	@echo "   - job_simple_console_test.py           # 简单控制台测试 (推荐先测试)"
	@echo "   - job_rules_filter_datastream.py       # 基础威胁检测"
	@echo "   - job_rules_configuration_datastream.py # 配置化威胁检测"
