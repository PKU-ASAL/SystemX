#!/bin/bash

# =============================================================================
# SysArmor HFW分支 - Wazuh生态系统集成测试脚本
# =============================================================================

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# 配置
MANAGER_URL="http://localhost:8080"
API_BASE="${MANAGER_URL}/api/v1"

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

log_section() {
    echo -e "\n${PURPLE}=== $1 ===${NC}"
}

# HTTP请求函数
make_request() {
    local method=$1
    local endpoint=$2
    local data=$3
    local expected_status=${4:-200}
    
    log_info "Testing: $method $endpoint"
    
    if [ -n "$data" ]; then
        response=$(curl -s -w "\n%{http_code}" -X "$method" \
            -H "Content-Type: application/json" \
            -d "$data" \
            "$endpoint")
    else
        response=$(curl -s -w "\n%{http_code}" -X "$method" "$endpoint")
    fi
    
    # 分离响应体和状态码
    body=$(echo "$response" | head -n -1)
    status=$(echo "$response" | tail -n 1)
    
    if [ "$status" -eq "$expected_status" ]; then
        log_success "Status: $status (Expected: $expected_status)"
        if [ -n "$body" ] && [ "$body" != "null" ]; then
            echo "$body" | jq . 2>/dev/null || echo "$body"
        fi
        return 0
    else
        log_error "Status: $status (Expected: $expected_status)"
        if [ -n "$body" ]; then
            echo "$body" | jq . 2>/dev/null || echo "$body"
        fi
        return 1
    fi
}

# 检查依赖
check_dependencies() {
    log_section "检查依赖"
    
    # 检查curl
    if ! command -v curl &> /dev/null; then
        log_error "curl is required but not installed"
        exit 1
    fi
    
    # 检查jq
    if ! command -v jq &> /dev/null; then
        log_warning "jq is not installed, JSON output will not be formatted"
    fi
    
    log_success "Dependencies check passed"
}

# 检查Manager服务状态
check_manager_health() {
    log_section "检查Manager服务健康状态"
    
    if make_request "GET" "${MANAGER_URL}/health"; then
        log_success "Manager service is healthy"
    else
        log_error "Manager service is not healthy"
        exit 1
    fi
}

# 测试Wazuh配置管理
test_wazuh_config() {
    log_section "测试Wazuh配置管理"
    
    # 获取当前配置
    log_info "获取当前Wazuh配置..."
    make_request "GET" "${API_BASE}/wazuh/config"
    
    # 验证配置
    log_info "验证Wazuh配置..."
    local config_data='{
        "manager": {
            "host": "wazuh-manager",
            "port": 55000,
            "username": "wazuh",
            "password": "wazuh"
        },
        "indexer": {
            "host": "wazuh-indexer",
            "port": 9200,
            "username": "admin",
            "password": "admin"
        }
    }'
    make_request "POST" "${API_BASE}/wazuh/config/validate" "$config_data"
    
    # 更新配置
    log_info "更新Wazuh配置..."
    local update_data='{
        "manager": {
            "timeout": 30
        },
        "indexer": {
            "timeout": 60
        }
    }'
    make_request "PUT" "${API_BASE}/wazuh/config" "$update_data"
    
    # 重新加载配置
    log_info "重新加载Wazuh配置..."
    make_request "POST" "${API_BASE}/wazuh/config/reload"
}

# 测试Wazuh Manager API
test_wazuh_manager() {
    log_section "测试Wazuh Manager API"
    
    # 获取Manager信息
    log_info "获取Manager信息..."
    make_request "GET" "${API_BASE}/wazuh/manager/info"
    
    # 获取Manager状态
    log_info "获取Manager状态..."
    make_request "GET" "${API_BASE}/wazuh/manager/status"
    
    # 获取Manager统计信息
    log_info "获取Manager统计信息..."
    make_request "GET" "${API_BASE}/wazuh/manager/stats"
    
    # 获取Manager日志
    log_info "获取Manager日志..."
    make_request "GET" "${API_BASE}/wazuh/manager/logs?limit=10"
    
    # 获取Manager配置
    log_info "获取Manager配置..."
    make_request "GET" "${API_BASE}/wazuh/manager/configuration"
}

# 测试Wazuh Agent管理
test_wazuh_agents() {
    log_section "测试Wazuh Agent管理"
    
    # 获取Agent列表
    log_info "获取Agent列表..."
    make_request "GET" "${API_BASE}/wazuh/agents?limit=10"
    
    # 添加新Agent
    log_info "添加新Agent..."
    local agent_data='{
        "name": "test-agent-001",
        "ip": "192.168.1.100",
        "groups": ["default"]
    }'
    if make_request "POST" "${API_BASE}/wazuh/agents" "$agent_data" 201; then
        AGENT_ID=$(echo "$body" | jq -r '.data.id' 2>/dev/null || echo "001")
        log_success "Agent created with ID: $AGENT_ID"
        
        # 获取单个Agent信息
        log_info "获取Agent详细信息..."
        make_request "GET" "${API_BASE}/wazuh/agents/$AGENT_ID"
        
        # 获取Agent密钥
        log_info "获取Agent密钥..."
        make_request "GET" "${API_BASE}/wazuh/agents/$AGENT_ID/key"
        
        # 获取Agent配置
        log_info "获取Agent配置..."
        make_request "GET" "${API_BASE}/wazuh/agents/$AGENT_ID/config"
        
        # 更新Agent
        log_info "更新Agent信息..."
        local update_data='{
            "name": "test-agent-001-updated",
            "groups": ["default", "web-servers"]
        }'
        make_request "PUT" "${API_BASE}/wazuh/agents/$AGENT_ID" "$update_data"
        
        # 重启Agent
        log_info "重启Agent..."
        make_request "POST" "${API_BASE}/wazuh/agents/$AGENT_ID/restart"
        
        # 删除Agent
        log_info "删除Agent..."
        make_request "DELETE" "${API_BASE}/wazuh/agents/$AGENT_ID"
    fi
}

# 测试Wazuh组管理
test_wazuh_groups() {
    log_section "测试Wazuh组管理"
    
    # 获取组列表
    log_info "获取组列表..."
    make_request "GET" "${API_BASE}/wazuh/groups"
    
    # 创建新组
    log_info "创建新组..."
    local group_data='{"name": "test-group"}'
    if make_request "POST" "${API_BASE}/wazuh/groups" "$group_data" 201; then
        GROUP_NAME="test-group"
        
        # 获取组信息
        log_info "获取组信息..."
        make_request "GET" "${API_BASE}/wazuh/groups/$GROUP_NAME"
        
        # 获取组配置
        log_info "获取组配置..."
        make_request "GET" "${API_BASE}/wazuh/groups/$GROUP_NAME/configuration"
        
        # 获取组内Agent
        log_info "获取组内Agent..."
        make_request "GET" "${API_BASE}/wazuh/groups/$GROUP_NAME/agents"
        
        # 删除组
        log_info "删除组..."
        make_request "DELETE" "${API_BASE}/wazuh/groups/$GROUP_NAME"
    fi
}

# 测试Wazuh规则管理
test_wazuh_rules() {
    log_section "测试Wazuh规则管理"
    
    # 获取规则列表
    log_info "获取规则列表..."
    make_request "GET" "${API_BASE}/wazuh/rules?limit=10"
    
    # 获取规则文件列表
    log_info "获取规则文件列表..."
    make_request "GET" "${API_BASE}/wazuh/rules/files"
    
    # 获取特定规则
    log_info "获取特定规则..."
    make_request "GET" "${API_BASE}/wazuh/rules/1001"
}

# 测试Wazuh解码器管理
test_wazuh_decoders() {
    log_section "测试Wazuh解码器管理"
    
    # 获取解码器列表
    log_info "获取解码器列表..."
    make_request "GET" "${API_BASE}/wazuh/decoders?limit=10"
    
    # 获取解码器文件列表
    log_info "获取解码器文件列表..."
    make_request "GET" "${API_BASE}/wazuh/decoders/files"
}

# 测试Wazuh CDB列表管理
test_wazuh_lists() {
    log_section "测试Wazuh CDB列表管理"
    
    # 获取CDB列表
    log_info "获取CDB列表..."
    make_request "GET" "${API_BASE}/wazuh/lists"
    
    # 创建新CDB列表
    log_info "创建新CDB列表..."
    local list_data='{
        "filename": "test-list.cdb",
        "content": "192.168.1.1:suspicious_ip\n192.168.1.2:malicious_ip"
    }'
    if make_request "POST" "${API_BASE}/wazuh/lists" "$list_data" 201; then
        # 获取CDB列表内容
        log_info "获取CDB列表内容..."
        make_request "GET" "${API_BASE}/wazuh/lists/test-list.cdb"
        
        # 更新CDB列表
        log_info "更新CDB列表..."
        local update_data='{
            "content": "192.168.1.1:suspicious_ip\n192.168.1.2:malicious_ip\n192.168.1.3:blocked_ip"
        }'
        make_request "PUT" "${API_BASE}/wazuh/lists/test-list.cdb" "$update_data"
        
        # 删除CDB列表
        log_info "删除CDB列表..."
        make_request "DELETE" "${API_BASE}/wazuh/lists/test-list.cdb"
    fi
}

# 测试Wazuh Indexer API
test_wazuh_indexer() {
    log_section "测试Wazuh Indexer API"
    
    # 获取Indexer健康状态
    log_info "获取Indexer健康状态..."
    make_request "GET" "${API_BASE}/wazuh/indexer/health"
    
    # 获取Indexer信息
    log_info "获取Indexer信息..."
    make_request "GET" "${API_BASE}/wazuh/indexer/info"
    
    # 获取索引列表
    log_info "获取索引列表..."
    make_request "GET" "${API_BASE}/wazuh/indexer/indices"
    
    # 获取索引模板
    log_info "获取索引模板..."
    make_request "GET" "${API_BASE}/wazuh/indexer/templates"
    
    # 创建测试索引
    log_info "创建测试索引..."
    local index_data='{
        "name": "test-index",
        "settings": {
            "number_of_shards": 1,
            "number_of_replicas": 0
        }
    }'
    if make_request "POST" "${API_BASE}/wazuh/indexer/indices" "$index_data" 201; then
        # 删除测试索引
        log_info "删除测试索引..."
        make_request "DELETE" "${API_BASE}/wazuh/indexer/indices/test-index"
    fi
}

# 测试Wazuh告警查询
test_wazuh_alerts() {
    log_section "测试Wazuh告警查询"
    
    # 搜索告警
    log_info "搜索告警..."
    local search_data='{
        "size": 10,
        "query": {
            "match_all": {}
        },
        "sort": "@timestamp:desc"
    }'
    make_request "POST" "${API_BASE}/wazuh/alerts/search" "$search_data"
    
    # 根据Agent获取告警
    log_info "根据Agent获取告警..."
    make_request "GET" "${API_BASE}/wazuh/alerts/agent/001?limit=5"
    
    # 根据规则获取告警
    log_info "根据规则获取告警..."
    make_request "GET" "${API_BASE}/wazuh/alerts/rule/1001?limit=5"
    
    # 根据级别获取告警
    log_info "根据级别获取告警..."
    make_request "GET" "${API_BASE}/wazuh/alerts/level/10?limit=5"
    
    # 聚合告警统计
    log_info "聚合告警统计..."
    local agg_data='{
        "type": "terms",
        "field": "rule.level",
        "size": 10
    }'
    make_request "POST" "${API_BASE}/wazuh/alerts/aggregate" "$agg_data"
    
    # 获取告警统计
    log_info "获取告警统计..."
    make_request "GET" "${API_BASE}/wazuh/alerts/stats"
}

# 测试Wazuh监控和统计
test_wazuh_monitoring() {
    log_section "测试Wazuh监控和统计"
    
    # 获取监控概览
    log_info "获取监控概览..."
    make_request "GET" "${API_BASE}/wazuh/monitoring/overview"
    
    # 获取Agent摘要
    log_info "获取Agent摘要..."
    make_request "GET" "${API_BASE}/wazuh/monitoring/agents/summary"
    
    # 获取告警摘要
    log_info "获取告警摘要..."
    make_request "GET" "${API_BASE}/wazuh/monitoring/alerts/summary"
    
    # 获取系统统计
    log_info "获取系统统计..."
    make_request "GET" "${API_BASE}/wazuh/monitoring/system/stats"
}

# 性能测试
performance_test() {
    log_section "性能测试"
    
    log_info "执行并发请求测试..."
    
    # 并发获取配置
    for i in {1..5}; do
        (make_request "GET" "${API_BASE}/wazuh/config" "" 200 > /dev/null 2>&1) &
    done
    wait
    log_success "并发配置请求测试完成"
    
    # 并发获取Agent列表
    for i in {1..3}; do
        (make_request "GET" "${API_BASE}/wazuh/agents?limit=5" "" 200 > /dev/null 2>&1) &
    done
    wait
    log_success "并发Agent查询测试完成"
}

# 错误处理测试
error_handling_test() {
    log_section "错误处理测试"
    
    # 测试无效端点
    log_info "测试无效端点..."
    make_request "GET" "${API_BASE}/wazuh/invalid-endpoint" "" 404
    
    # 测试无效Agent ID
    log_info "测试无效Agent ID..."
    make_request "GET" "${API_BASE}/wazuh/agents/invalid-id" "" 404
    
    # 测试无效请求体
    log_info "测试无效请求体..."
    make_request "POST" "${API_BASE}/wazuh/agents" '{"invalid": "data"}' 400
    
    # 测试无效配置
    log_info "测试无效配置验证..."
    make_request "POST" "${API_BASE}/wazuh/config/validate" '{"invalid": "config"}' 400
}

# 清理函数
cleanup() {
    log_section "清理测试数据"
    
    # 清理可能创建的测试资源
    log_info "清理测试Agent..."
    make_request "DELETE" "${API_BASE}/wazuh/agents/test-agent-001" "" 200 2>/dev/null || true
    
    log_info "清理测试组..."
    make_request "DELETE" "${API_BASE}/wazuh/groups/test-group" "" 200 2>/dev/null || true
    
    log_info "清理测试CDB列表..."
    make_request "DELETE" "${API_BASE}/wazuh/lists/test-list.cdb" "" 200 2>/dev/null || true
    
    log_info "清理测试索引..."
    make_request "DELETE" "${API_BASE}/wazuh/indexer/indices/test-index" "" 200 2>/dev/null || true
    
    log_success "清理完成"
}

# 生成测试报告
generate_report() {
    log_section "测试报告"
    
    local end_time=$(date)
    local duration=$(($(date +%s) - start_time))
    
    echo -e "\n${CYAN}=== HFW分支 Wazuh集成测试报告 ===${NC}"
    echo -e "${BLUE}开始时间:${NC} $start_time_str"
    echo -e "${BLUE}结束时间:${NC} $end_time"
    echo -e "${BLUE}测试时长:${NC} ${duration}秒"
    echo -e "${BLUE}Manager URL:${NC} $MANAGER_URL"
    
    echo -e "\n${GREEN}✅ 测试完成！${NC}"
    echo -e "${YELLOW}注意: 某些测试可能因为Wazuh服务未运行而失败，这是正常的。${NC}"
    echo -e "${YELLOW}在生产环境中，请确保Wazuh Manager和Indexer服务正常运行。${NC}"
}

# 主函数
main() {
    local start_time=$(date +%s)
    local start_time_str=$(date)
    
    echo -e "${CYAN}"
    echo "============================================================================="
    echo "                    SysArmor HFW分支 - Wazuh集成测试"
    echo "============================================================================="
    echo -e "${NC}"
    
    # 设置错误处理
    trap cleanup EXIT
    
    # 执行测试
    check_dependencies
    check_manager_health
    
    # 核心功能测试
    test_wazuh_config
    test_wazuh_manager
    test_wazuh_agents
    test_wazuh_groups
    test_wazuh_rules
    test_wazuh_decoders
    test_wazuh_lists
    test_wazuh_indexer
    test_wazuh_alerts
    test_wazuh_monitoring
    
    # 高级测试
    performance_test
    error_handling_test
    
    # 生成报告
    generate_report
}

# 检查参数
if [ "$1" = "--help" ] || [ "$1" = "-h" ]; then
    echo "SysArmor HFW分支 - Wazuh集成测试脚本"
    echo ""
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  -h, --help     显示帮助信息"
    echo "  --manager-url  指定Manager URL (默认: http://localhost:8080)"
    echo ""
    echo "示例:"
    echo "  $0"
    echo "  $0 --manager-url http://192.168.1.100:8080"
    exit 0
fi

# 解析参数
while [[ $# -gt 0 ]]; do
    case $1 in
        --manager-url)
            MANAGER_URL="$2"
            API_BASE="${MANAGER_URL}/api/v1"
            shift 2
            ;;
        *)
            log_error "未知参数: $1"
            exit 1
            ;;
    esac
done

# 执行主函数
main "$@"
