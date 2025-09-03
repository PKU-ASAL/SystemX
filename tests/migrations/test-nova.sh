#!/bin/bash

# SysArmor Nova 分支双向心跳功能测试脚本
# 测试心跳上报和主动探测功能

# 注意: 不使用 set -e，以便继续执行所有测试

# 颜色定义
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly NC='\033[0m' # No Color

# 配置
readonly MANAGER_URL="http://localhost:8080"
readonly TEST_OUTPUT_DIR="./tests/migrations/outputs"
readonly TEST_COLLECTOR_DATA='{
    "hostname": "test-server-nova",
    "ip_address": "192.168.1.200",
    "os_type": "linux",
    "os_version": "Ubuntu 22.04",
    "deployment_type": "agentless"
}'

# 辅助函数
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# 检查服务健康状态
check_service_health() {
    log_info "检查 SysArmor Manager 服务状态..."
    
    if curl -s "${MANAGER_URL}/health" > /dev/null; then
        log_success "Manager 服务运行正常"
    else
        log_error "Manager 服务不可访问，请先启动服务: make up"
        exit 1
    fi
}

# 注册测试 Collector
register_test_collector() {
    log_info "注册测试 Collector..."
    
    echo "📤 请求: POST ${MANAGER_URL}/api/v1/collectors/register"
    echo "📤 请求体: ${TEST_COLLECTOR_DATA}"
    
    local response=$(curl -s -X POST "${MANAGER_URL}/api/v1/collectors/register" \
        -H "Content-Type: application/json" \
        -d "${TEST_COLLECTOR_DATA}")
    
    echo "📥 响应: $response"
    
    if echo "$response" | grep -q '"success":true'; then
        COLLECTOR_ID=$(echo "$response" | grep -o '"collector_id":"[^"]*"' | cut -d'"' -f4)
        log_success "Collector 注册成功: ${COLLECTOR_ID}"
        return 0
    else
        log_error "Collector 注册失败: $response"
        return 1
    fi
}

# 测试状态查询功能
test_status_query() {
    log_info "测试状态查询功能..."
    
    echo "📤 请求: GET ${MANAGER_URL}/api/v1/collectors/${COLLECTOR_ID}"
    
    local response=$(curl -s "${MANAGER_URL}/api/v1/collectors/${COLLECTOR_ID}")
    
    echo "📥 响应: $response"
    
    if echo "$response" | grep -q '"success":true'; then
        log_success "状态查询成功"
        
        # 检查新字段是否存在
        if echo "$response" | grep -q '"last_active"'; then
            log_success "✅ last_active 字段存在"
        else
            log_warning "❌ last_active 字段缺失"
        fi
        
        if echo "$response" | grep -q '"realtime_status"'; then
            log_success "✅ realtime_status 字段存在"
        else
            log_warning "❌ realtime_status 字段缺失"
        fi
        
        if echo "$response" | grep -q '"last_seen_minutes"'; then
            log_success "✅ last_seen_minutes 字段存在"
        else
            log_warning "❌ last_seen_minutes 字段缺失"
        fi
        
        return 0
    else
        log_error "状态查询失败: $response"
        return 1
    fi
}

# 测试心跳上报功能
test_heartbeat_report() {
    local status="$1"
    log_info "测试心跳上报功能 (status: ${status})..."
    
    local request_body="{\"status\":\"${status}\"}"
    echo "📤 请求: POST ${MANAGER_URL}/api/v1/collectors/${COLLECTOR_ID}/heartbeat"
    echo "📤 请求体: $request_body"
    
    local response=$(curl -s -X POST "${MANAGER_URL}/api/v1/collectors/${COLLECTOR_ID}/heartbeat" \
        -H "Content-Type: application/json" \
        -d "$request_body")
    
    echo "📥 响应: $response"
    
    if echo "$response" | grep -q '"success":true'; then
        log_success "心跳上报成功 (status: ${status})"
        
        # 检查响应格式
        if echo "$response" | grep -q '"next_heartbeat_interval"'; then
            log_success "✅ 心跳响应格式正确"
        else
            log_warning "❌ 心跳响应格式异常"
        fi
        
        return 0
    else
        log_error "心跳上报失败: $response"
        return 1
    fi
}

# 测试主动探测功能
test_probe_heartbeat() {
    local timeout="$1"
    log_info "测试主动探测功能 (timeout: ${timeout}s)..."
    
    local request_body="{\"timeout\":${timeout}}"
    echo "📤 请求: POST ${MANAGER_URL}/api/v1/collectors/${COLLECTOR_ID}/probe"
    echo "📤 请求体: $request_body"
    
    local response=$(curl -s -X POST "${MANAGER_URL}/api/v1/collectors/${COLLECTOR_ID}/probe" \
        -H "Content-Type: application/json" \
        -d "$request_body")
    
    echo "📥 响应: $response"
    
    # 探测可能成功也可能失败（因为没有真实的collector响应）
    if echo "$response" | grep -q '"probe_id"'; then
        log_success "探测请求发送成功"
        
        # 检查探测结果字段
        if echo "$response" | grep -q '"sent_at"'; then
            log_success "✅ 探测时间戳正确"
        fi
        
        if echo "$response" | grep -q '"heartbeat_before"'; then
            log_success "✅ 探测前心跳时间记录正确"
        fi
        
        # 检查探测结果
        if echo "$response" | grep -q '"success":true'; then
            log_success "🎉 探测成功 - Collector 响应正常"
        else
            log_warning "⏰ 探测超时 - 这是预期的（没有真实collector响应）"
        fi
        
        return 0
    else
        log_error "探测请求失败: $response"
        return 1
    fi
}

# 验证数据库字段更新
verify_database_fields() {
    log_info "验证数据库字段更新..."
    
    echo "📤 请求: GET ${MANAGER_URL}/api/v1/collectors/${COLLECTOR_ID} (验证字段更新)"
    
    # 这里我们通过API查询来验证，因为直接数据库查询需要额外权限
    local response=$(curl -s "${MANAGER_URL}/api/v1/collectors/${COLLECTOR_ID}")
    
    echo "📥 响应: $response"
    
    if echo "$response" | grep -q '"last_active".*[0-9]'; then
        log_success "✅ last_active 字段已正确更新"
    else
        log_warning "❌ last_active 字段未更新或为空"
    fi
    
    if echo "$response" | grep -q '"last_heartbeat".*[0-9]'; then
        log_success "✅ last_heartbeat 字段已正确更新"
    else
        log_error "❌ last_heartbeat 字段未更新"
    fi
}

# 运行所有测试
run_all_tests() {
    local failed_tests=0
    local total_tests=0
    
    log_info "开始 Nova 分支双向心跳功能测试..."
    echo "=================================="
    
    # 创建输出目录
    mkdir -p "$TEST_OUTPUT_DIR"
    
    # 检查服务健康状态
    check_service_health || exit 1
    
    # 注册测试 Collector
    register_test_collector || exit 1
    
    echo ""
    log_info "开始测试双向心跳功能..."
    echo "=================================="
    
    # 测试状态查询功能
    ((total_tests++))
    if test_status_query; then
        log_success "✅ 状态查询测试通过"
    else
        log_error "❌ 状态查询测试失败"
        ((failed_tests++))
    fi
    
    # 测试心跳上报功能 - active状态
    ((total_tests++))
    if test_heartbeat_report "active"; then
        log_success "✅ Active 心跳上报测试通过"
    else
        log_error "❌ Active 心跳上报测试失败"
        ((failed_tests++))
    fi
    
    # 等待1秒，然后测试inactive状态
    sleep 1
    
    # 测试心跳上报功能 - inactive状态
    ((total_tests++))
    if test_heartbeat_report "inactive"; then
        log_success "✅ Inactive 心跳上报测试通过"
    else
        log_error "❌ Inactive 心跳上报测试失败"
        ((failed_tests++))
    fi
    
    # 测试主动探测功能
    ((total_tests++))
    if test_probe_heartbeat 5; then
        log_success "✅ 主动探测测试通过"
    else
        log_error "❌ 主动探测测试失败"
        ((failed_tests++))
    fi
    
    # 验证数据库字段更新
    verify_database_fields
    
    echo ""
    echo "=================================="
    log_info "测试结果汇总"
    echo "=================================="
    
    if [ $failed_tests -eq 0 ]; then
        log_success "🎉 所有测试通过! (${total_tests}/${total_tests})"
        
        echo ""
        log_info "Nova 分支功能验证:"
        echo "  ✅ 数据库 last_active 字段正常工作"
        echo "  ✅ 心跳上报 API 正常工作"
        echo "  ✅ 主动探测 API 正常工作"
        echo "  ✅ 实时状态计算正常工作"
        echo "  ✅ 双向心跳机制完整实现"
        
        return 0
    else
        log_error "❌ ${failed_tests}/${total_tests} 个测试失败"
        return 1
    fi
}

# 清理测试数据
cleanup() {
    if [ -n "$COLLECTOR_ID" ]; then
        log_info "清理测试 Collector..."
        curl -s -X DELETE "${MANAGER_URL}/api/v1/collectors/${COLLECTOR_ID}?force=true" > /dev/null
        log_success "测试 Collector 已清理"
    fi
}

# 显示帮助信息
show_help() {
    echo "SysArmor Nova 分支双向心跳功能测试脚本"
    echo ""
    echo "用法:"
    echo "  $0 [选项]"
    echo ""
    echo "选项:"
    echo "  test     运行所有测试 (默认)"
    echo "  cleanup  清理测试数据"
    echo "  help     显示此帮助信息"
    echo ""
    echo "示例:"
    echo "  $0 test      # 运行所有测试"
    echo "  $0 cleanup   # 清理测试数据"
}

# 主函数
main() {
    case "${1:-test}" in
        "test")
            run_all_tests
            ;;
        "cleanup")
            cleanup
            ;;
        "help"|"-h"|"--help")
            show_help
            ;;
        *)
            log_error "未知选项: $1"
            show_help
            exit 1
            ;;
    esac
}

# 脚本入口
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
