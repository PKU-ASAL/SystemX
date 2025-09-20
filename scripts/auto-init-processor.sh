#!/bin/bash

# =============================================================================
# SysArmor 自动初始化处理器脚本
# 等待所有服务就绪后自动提交 Flink 作业
# =============================================================================

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 配置
MANAGER_API="http://localhost:8080"
FLINK_API="http://localhost:8081"
OPENSEARCH_API="http://localhost:9200"
OPENSEARCH_USERNAME="${OPENSEARCH_USERNAME:-admin}"
OPENSEARCH_PASSWORD="${OPENSEARCH_PASSWORD:-admin}"
MAX_WAIT_TIME=300  # 最大等待时间 5分钟
CHECK_INTERVAL=10  # 检查间隔 10秒

# 日志函数
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 等待服务就绪
wait_for_service() {
    local service_name="$1"
    local health_url="$2"
    local wait_time=0
    
    log_info "等待 $service_name 服务启动..."
    
    while [ $wait_time -lt $MAX_WAIT_TIME ]; do
        if curl -s -f "$health_url" > /dev/null 2>&1; then
            log_success "$service_name 服务已就绪"
            return 0
        fi
        
        log_info "$service_name 未就绪，等待 $CHECK_INTERVAL 秒... ($wait_time/$MAX_WAIT_TIME)"
        sleep $CHECK_INTERVAL
        wait_time=$((wait_time + CHECK_INTERVAL))
    done
    
    log_error "等待 $service_name 服务超时"
    return 1
}

# 等待 OpenSearch 健康
wait_for_opensearch_health() {
    local wait_time=0
    
    log_info "等待 OpenSearch 集群健康..."
    
    while [ $wait_time -lt $MAX_WAIT_TIME ]; do
        local health_response=$(curl -s -u "$OPENSEARCH_USERNAME:$OPENSEARCH_PASSWORD" "$OPENSEARCH_API/_cluster/health" 2>/dev/null)
        if [ -n "$health_response" ]; then
            if echo "$health_response" | grep -q '"status":"green"' || echo "$health_response" | grep -q '"status":"yellow"'; then
                log_success "OpenSearch 集群健康"
                return 0
            fi
        fi
        
        log_info "OpenSearch 集群未健康，等待 $CHECK_INTERVAL 秒... ($wait_time/$MAX_WAIT_TIME)"
        sleep $CHECK_INTERVAL
        wait_time=$((wait_time + CHECK_INTERVAL))
    done
    
    log_error "等待 OpenSearch 集群健康超时"
    return 1
}

# 主函数
main() {
    echo "🚀 SysArmor 自动初始化处理器"
    echo "=============================================="
    echo "等待所有服务就绪后自动提交 Flink 作业"
    echo ""
    
    # 1. 等待 Manager API
    if ! wait_for_service "Manager" "$MANAGER_API/health"; then
        log_error "Manager 服务未就绪，退出"
        exit 1
    fi
    
    # 2. 等待 Flink
    if ! wait_for_service "Flink" "$FLINK_API/overview"; then
        log_error "Flink 服务未就绪，退出"
        exit 1
    fi
    
    # 3. 等待 OpenSearch 健康
    if ! wait_for_opensearch_health; then
        log_error "OpenSearch 服务未健康，退出"
        exit 1
    fi
    
    # 4. 额外等待确保所有服务完全稳定
    log_info "所有服务已就绪，等待 30 秒确保系统稳定..."
    sleep 30
    
    # 5. 提交核心 Flink 作业
    log_info "开始提交核心 Flink 作业..."
    
    local success_count=0
    local total_jobs=2
    
    # 提交 job_01_audit_raw_to_events.py
    log_info "提交作业: job_01_audit_raw_to_events.py"
    if docker compose exec -T flink-jobmanager flink run -d -py /opt/flink/usr_jobs/job_01_audit_raw_to_events.py > /dev/null 2>&1; then
        log_success "job_01_audit_raw_to_events.py 提交成功"
        success_count=$((success_count + 1))
    else
        log_error "job_01_audit_raw_to_events.py 提交失败"
    fi
    
    # 等待第一个作业启动
    sleep 5
    
    # 提交 job_02_audit_events_to_alerts.py
    log_info "提交作业: job_02_audit_events_to_alerts.py"
    if docker compose exec -T flink-jobmanager flink run -d -py /opt/flink/usr_jobs/job_02_audit_events_to_alerts.py > /dev/null 2>&1; then
        log_success "job_02_audit_events_to_alerts.py 提交成功"
        success_count=$((success_count + 1))
    else
        log_error "job_02_audit_events_to_alerts.py 提交失败"
    fi
    
    # 等待作业启动
    log_info "等待作业启动..."
    sleep 10
    
    if [ $success_count -eq $total_jobs ]; then
        log_success "🎉 所有核心作业提交成功！数据流已启动！"
        echo ""
        echo "数据流已启动："
        echo "  📥 sysarmor.raw.audit → job_01_audit_raw_to_events.py → sysarmor.events.audit"
        echo "  🚨 sysarmor.events.audit → job_02_audit_events_to_alerts.py → sysarmor.alerts.audit"
        echo ""
        echo "监控地址："
        echo "  🔧 Flink Web UI: http://localhost:8081"
        echo "  📊 Manager API: http://localhost:8080/api/v1/services/flink/jobs"
        echo ""
        echo "测试数据流："
        echo "  ./tests/import-events-data.sh ./data/kafka-imports/sysarmor-agentless-samples.jsonl"
    else
        log_error "部分作业提交失败 ($success_count/$total_jobs)"
        exit 1
    fi
}

# 脚本入口
if [ "$1" = "--help" ] || [ "$1" = "-h" ]; then
    echo "SysArmor 自动初始化处理器脚本"
    echo ""
    echo "功能："
    echo "  - 等待 Manager、Flink、OpenSearch 服务就绪"
    echo "  - 自动提交核心 Flink 作业"
    echo "  - 启动完整的数据处理流程"
    echo ""
    echo "用法: $0 [--help]"
    echo ""
    echo "建议在系统启动后运行："
    echo "  make deploy && ./scripts/auto-init-processor.sh"
    exit 0
fi

# 检查依赖
if ! command -v curl &> /dev/null; then
    log_error "需要安装 curl"
    exit 1
fi

if ! command -v docker &> /dev/null; then
    log_error "需要安装 docker"
    exit 1
fi

# 执行主函数
main
