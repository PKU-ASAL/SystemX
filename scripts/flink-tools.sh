#!/bin/bash

# =============================================================================
# SysArmor Flink 工具脚本
# 提供 Flink 作业管理、监控和日志查看功能
# =============================================================================

set -e

# 默认配置
FLINK_API="http://localhost:8081"
MANAGER_API="http://localhost:8080"
FLINK_JOBMANAGER_CONTAINER="sysarmor-flink-jobmanager-1"
FLINK_TASKMANAGER_CONTAINER="sysarmor-flink-taskmanager-1"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
NC='\033[0m' # No Color

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

# 检查Flink服务状态
check_flink_status() {
    if ! docker ps --format "table {{.Names}}" | grep -q "$FLINK_JOBMANAGER_CONTAINER"; then
        log_error "Flink JobManager容器未运行"
        log_info "请先启动服务: make up 或 make up-dev"
        return 1
    fi
    return 0
}

# 列出所有作业
list_jobs() {
    echo "📋 SysArmor Flink 作业列表"
    echo "=============================================="
    
    log_info "通过Flink API查询作业..."
    
    # 尝试直接访问Flink API
    if jobs_response=$(curl -s "$FLINK_API/jobs" 2>/dev/null); then
        if echo "$jobs_response" | jq -e '.jobs' > /dev/null 2>&1; then
            echo "$jobs_response" | jq -r '.jobs[]? | "  🎯 Job ID: \(.id) | 名称: \(.name // "未知") | 状态: \(.status)"' 2>/dev/null
            job_count=$(echo "$jobs_response" | jq -r '.jobs | length' 2>/dev/null || echo "0")
            log_success "找到 $job_count 个作业"
        else
            log_warning "Flink API响应格式异常"
        fi
    else
        log_warning "Flink API不可用，尝试Manager API..."
        
        # 尝试通过Manager API
        if manager_response=$(curl -s "$MANAGER_API/api/v1/services/flink/jobs" 2>/dev/null); then
            if echo "$manager_response" | jq -e '.data.jobs' > /dev/null 2>&1; then
                echo "$manager_response" | jq -r '.data.jobs[]? | "  🎯 Job ID: \(.id) | 名称: \(.name // "未知") | 状态: \(.state // "未知")"' 2>/dev/null
                job_count=$(echo "$manager_response" | jq -r '.data.jobs | length' 2>/dev/null || echo "0")
                log_success "找到 $job_count 个作业"
            else
                log_error "Manager API响应格式异常"
            fi
        else
            log_error "所有API都不可用"
        fi
    fi
    
    echo "=============================================="
}

# 提交作业
submit_job() {
    local job_file="$1"
    
    if [[ -z "$job_file" ]]; then
        log_error "请指定作业文件"
        echo "用法: $0 submit <job_file>"
        echo "可用作业:"
        echo "  job_01_audit_raw_to_events.py"
        echo "  job_02_audit_events_to_alerts.py"
        return 1
    fi
    
    if ! check_flink_status; then
        return 1
    fi
    
    echo "🚀 SysArmor Flink - 提交作业"
    echo "=============================================="
    log_info "作业文件: $job_file"
    
    # 检查作业文件是否存在
    local job_path="/opt/flink/usr_jobs/$job_file"
    if ! docker exec "$FLINK_JOBMANAGER_CONTAINER" test -f "$job_path" 2>/dev/null; then
        log_error "作业文件不存在: $job_path"
        log_info "可用作业文件:"
        docker exec "$FLINK_JOBMANAGER_CONTAINER" ls -la /opt/flink/usr_jobs/ 2>/dev/null || echo "  无法列出作业文件"
        return 1
    fi
    
    log_info "提交作业到Flink集群..."
    
    if docker exec "$FLINK_JOBMANAGER_CONTAINER" flink run -py "$job_path"; then
        log_success "作业提交成功!"
        log_info "监控地址: $FLINK_API"
        
        # 等待作业启动
        sleep 3
        log_info "查看作业状态..."
        list_jobs
    else
        log_error "作业提交失败"
        return 1
    fi
    
    echo "=============================================="
}

# 取消作业
cancel_job() {
    local job_id="$1"
    
    if [[ -z "$job_id" ]]; then
        log_error "请指定作业ID"
        echo "用法: $0 cancel <job_id>"
        log_info "获取作业ID: $0 list"
        return 1
    fi
    
    if ! check_flink_status; then
        return 1
    fi
    
    echo "🛑 SysArmor Flink - 取消作业"
    echo "=============================================="
    log_info "作业ID: $job_id"
    
    if docker exec "$FLINK_JOBMANAGER_CONTAINER" flink cancel "$job_id"; then
        log_success "作业 $job_id 已取消"
    else
        log_error "取消作业失败"
        return 1
    fi
    
    echo "=============================================="
}

# 查看作业详情
job_details() {
    local job_id="$1"
    
    if [[ -z "$job_id" ]]; then
        log_error "请指定作业ID"
        echo "用法: $0 details <job_id>"
        return 1
    fi
    
    echo "📊 SysArmor Flink - 作业详情"
    echo "=============================================="
    log_info "作业ID: $job_id"
    
    # 通过Flink API获取作业详情
    if job_details=$(curl -s "$FLINK_API/jobs/$job_id" 2>/dev/null); then
        if echo "$job_details" | jq -e '.' > /dev/null 2>&1; then
            echo "$job_details" | jq '{
                id: .jid,
                name: .name,
                state: .state,
                "start-time": ."start-time",
                "end-time": ."end-time",
                duration: .duration,
                vertices: [.vertices[]? | {id: .id, name: .name, status: .status}]
            }'
        else
            log_error "作业详情响应格式异常"
        fi
    else
        log_error "无法获取作业详情"
    fi
    
    echo "=============================================="
}

# 查看集群概览
cluster_overview() {
    echo "📊 SysArmor Flink - 集群概览"
    echo "=============================================="
    
    # 尝试通过Manager API
    if overview=$(curl -s "$MANAGER_API/api/v1/services/flink/overview" 2>/dev/null); then
        if echo "$overview" | jq -e '.data' > /dev/null 2>&1; then
            echo "$overview" | jq '.data'
            log_success "集群状态正常"
        else
            log_warning "Manager API响应异常，尝试直接访问Flink..."
        fi
    else
        log_warning "Manager API不可用，尝试直接访问Flink..."
    fi
    
    # 尝试直接访问Flink API
    if flink_overview=$(curl -s "$FLINK_API/overview" 2>/dev/null); then
        if echo "$flink_overview" | jq -e '.' > /dev/null 2>&1; then
            echo "$flink_overview" | jq '{
                "flink-version": ."flink-version",
                "taskmanagers": .taskmanagers,
                "slots-total": ."slots-total",
                "slots-available": ."slots-available",
                "jobs-running": ."jobs-running",
                "jobs-finished": ."jobs-finished",
                "jobs-cancelled": ."jobs-cancelled",
                "jobs-failed": ."jobs-failed"
            }'
        else
            log_error "Flink集群不可用"
        fi
    else
        log_error "Flink集群不可用"
    fi
    
    echo "=============================================="
}

# 查看作业日志
job_logs() {
    local container="${1:-taskmanager}"
    local lines="${2:-50}"
    
    echo "📋 SysArmor Flink - 作业日志"
    echo "=============================================="
    log_info "容器: $container | 行数: $lines"
    
    case "$container" in
        "jobmanager"|"jm")
            log_info "查看JobManager日志..."
            docker logs --tail "$lines" "$FLINK_JOBMANAGER_CONTAINER" 2>/dev/null || log_error "无法获取JobManager日志"
            ;;
        "taskmanager"|"tm"|*)
            log_info "查看TaskManager日志..."
            docker logs --tail "$lines" "$FLINK_TASKMANAGER_CONTAINER" 2>/dev/null || log_error "无法获取TaskManager日志"
            ;;
    esac
    
    echo "=============================================="
}

# 服务状态
service_status() {
    echo "📊 SysArmor Flink - 服务状态"
    echo "=============================================="
    
    log_info "Flink容器状态:"
    docker ps --filter "name=flink" --format "table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}" 2>/dev/null || log_error "无法获取容器状态"
    
    echo ""
    log_info "Flink API连接测试:"
    if curl -s -f "$FLINK_API/overview" > /dev/null 2>&1; then
        log_success "Flink API: 可用 ($FLINK_API)"
    else
        log_error "Flink API: 不可用 ($FLINK_API)"
    fi
    
    if curl -s -f "$MANAGER_API/api/v1/services/flink/health" > /dev/null 2>&1; then
        log_success "Manager API: 可用 ($MANAGER_API)"
    else
        log_error "Manager API: 不可用 ($MANAGER_API)"
    fi
    
    echo "=============================================="
}

# 快速测试
quick_test() {
    echo "🚀 SysArmor Flink - 快速测试"
    echo "=============================================="
    
    log_info "1. 检查服务状态..."
    service_status
    
    echo ""
    log_info "2. 查看集群概览..."
    cluster_overview
    
    echo ""
    log_info "3. 查看当前作业..."
    list_jobs
    
    echo ""
    log_success "快速测试完成!"
    log_info "Web监控: $FLINK_API"
}

# 显示帮助信息
show_help() {
    cat << EOF
SysArmor Flink 工具

用法: $0 <命令> [参数]

命令:
  list                           列出所有Flink作业
  submit <job_file>              提交Flink作业
  cancel <job_id>                取消指定作业
  details <job_id>               查看作业详情
  overview                       查看集群概览
  logs [container] [lines]       查看作业日志
  status                         查看服务状态
  test                           快速测试流程

参数:
  job_file    - Python作业文件名 (位于/opt/flink/usr_jobs/)
  job_id      - Flink作业ID
  container   - 日志容器 (jobmanager|taskmanager, 默认: taskmanager)
  lines       - 日志行数 (默认: 50)

可用作业文件:
  job_01_audit_raw_to_events.py        - Auditd原始数据到事件转换
  job_02_audit_events_to_alerts.py     - 事件到告警转换

示例:
  # 基础操作
  $0 list                                    # 列出所有作业
  $0 overview                                # 查看集群状态
  $0 status                                  # 查看服务状态
  
  # 作业管理
  $0 submit job_01_audit_raw_to_events.py    # 提交原始数据转换作业
  $0 submit job_02_audit_events_to_alerts.py # 提交告警生成作业
  $0 cancel abc123def456                     # 取消指定作业
  $0 details abc123def456                    # 查看作业详情
  
  # 日志查看
  $0 logs                                    # 查看TaskManager日志 (默认50行)
  $0 logs taskmanager 100                    # 查看TaskManager日志 (100行)
  $0 logs jobmanager 20                      # 查看JobManager日志 (20行)
  
  # 快速测试
  $0 test                                    # 运行完整测试流程

配置:
  Flink API: $FLINK_API
  Manager API: $MANAGER_API
  JobManager容器: $FLINK_JOBMANAGER_CONTAINER
  TaskManager容器: $FLINK_TASKMANAGER_CONTAINER

EOF
}

# 主函数
main() {
    if [[ $# -eq 0 ]]; then
        show_help
        exit 1
    fi
    
    local command="$1"
    shift
    
    case "$command" in
        list|ls)
            list_jobs
            ;;
        submit|sub)
            submit_job "$@"
            ;;
        cancel|stop)
            cancel_job "$@"
            ;;
        details|detail|info)
            job_details "$@"
            ;;
        overview|cluster)
            cluster_overview
            ;;
        logs|log)
            job_logs "$@"
            ;;
        status|stat)
            service_status
            ;;
        test|quick-test)
            quick_test
            ;;
        help|-h|--help)
            show_help
            ;;
        *)
            log_error "未知命令: $command"
            show_help
            exit 1
            ;;
    esac
}

# 运行主函数
main "$@"
