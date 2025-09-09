#!/bin/bash

# =============================================================================
# SysArmor EDR 系统健康状态测试脚本
# 测试所有组件的健康状态和连接性
# =============================================================================

# set -e  # 注释掉，避免单个测试失败导致整个脚本退出

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
TIMEOUT=10

# 统计变量
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# 辅助函数
print_header() {
    echo -e "\n${BLUE}===============================================${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}===============================================${NC}"
}

print_section() {
    echo -e "\n${CYAN}📋 $1${NC}"
    echo -e "${CYAN}-----------------------------------------------${NC}"
}

print_test() {
    echo -e "${YELLOW}🔍 测试: $1${NC}"
}

print_success() {
    echo -e "${GREEN}✅ $1${NC}"
    ((PASSED_TESTS++))
}

print_error() {
    echo -e "${RED}❌ $1${NC}"
    ((FAILED_TESTS++))
}

print_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

print_info() {
    echo -e "${PURPLE}ℹ️  $1${NC}"
}

# 测试API端点
test_api_endpoint() {
    local endpoint="$1"
    local description="$2"
    local expected_field="$3"
    
    ((TOTAL_TESTS++))
    print_test "$description"
    
    local response=$(curl -s --max-time $TIMEOUT "$MANAGER_URL$endpoint" 2>/dev/null)
    local http_code=$(curl -s --max-time $TIMEOUT -o /dev/null -w "%{http_code}" "$MANAGER_URL$endpoint" 2>/dev/null)
    
    if [ "$http_code" = "200" ]; then
        if [ -n "$expected_field" ]; then
            local field_value=$(echo "$response" | jq -r ".$expected_field" 2>/dev/null)
            if [ "$field_value" != "null" ] && [ "$field_value" != "" ]; then
                print_success "$description - HTTP 200, $expected_field: $field_value"
                return 0
            else
                print_error "$description - HTTP 200 但缺少字段: $expected_field"
                return 1
            fi
        else
            print_success "$description - HTTP 200"
            return 0
        fi
    else
        print_error "$description - HTTP $http_code"
        return 1
    fi
}

# 测试JSON响应结构
test_json_structure() {
    local endpoint="$1"
    local description="$2"
    local jq_filter="$3"
    
    ((TOTAL_TESTS++))
    print_test "$description"
    
    local response=$(curl -s --max-time $TIMEOUT "$MANAGER_URL$endpoint" 2>/dev/null)
    local result=$(echo "$response" | jq -r "$jq_filter" 2>/dev/null)
    
    if [ "$result" != "null" ] && [ "$result" != "" ]; then
        print_success "$description - $result"
        return 0
    else
        print_error "$description - 结构验证失败"
        echo "响应: $response" | head -c 200
        return 1
    fi
}

# 显示详细信息
show_detailed_info() {
    local endpoint="$1"
    local description="$2"
    
    print_info "获取 $description 详细信息..."
    local response=$(curl -s --max-time $TIMEOUT "$MANAGER_URL$endpoint" 2>/dev/null)
    echo "$response" | jq . 2>/dev/null || echo "$response"
}

# 主测试函数
main() {
    print_header "🚀 SysArmor EDR 系统健康状态测试"
    
    echo -e "测试目标: ${MANAGER_URL}"
    echo -e "测试时间: $(date)"
    echo -e "超时设置: ${TIMEOUT}秒"
    
    # 1. 基础健康检查
    print_section "1. 基础健康检查"
    test_api_endpoint "/health" "Manager基础健康检查" "status"
    test_api_endpoint "/api/v1/health" "Manager API健康检查" "success"
    
    # 2. 数据库连接测试
    print_section "2. 数据库连接测试"
    test_json_structure "/health" "数据库连接状态" ".database"
    test_json_structure "/api/v1/health" "API数据库连接状态" ".data.services.manager.components.database.status"
    
    # 3. Kafka服务测试
    print_section "3. Kafka服务测试"
    test_api_endpoint "/api/v1/services/kafka/health" "Kafka健康检查" "connected"
    test_json_structure "/api/v1/services/kafka/health" "Kafka集群信息" ".cluster_info[0].health_status"
    test_json_structure "/api/v1/services/kafka/health" "Kafka Broker数量" ".broker_count"
    
    # 4. Kafka Topics测试
    print_section "4. Kafka Topics测试"
    test_api_endpoint "/api/v1/services/kafka/topics" "Kafka Topics列表" "success"
    test_json_structure "/api/v1/services/kafka/topics" "Topics数据结构" ".data.topics | length"
    
    # 5. Kafka Brokers测试
    print_section "5. Kafka Brokers测试"
    test_api_endpoint "/api/v1/services/kafka/brokers" "Kafka Brokers信息" "success"
    test_json_structure "/api/v1/services/kafka/brokers" "Brokers数据结构" ".data | length"
    
    # 6. Flink服务测试
    print_section "6. Flink服务测试"
    test_api_endpoint "/api/v1/services/flink/health" "Flink健康检查" "connected"
    test_api_endpoint "/api/v1/services/flink/overview" "Flink集群概览" "success"
    test_json_structure "/api/v1/services/flink/overview" "Flink TaskManager数量" ".data.taskmanagers"
    
    # 7. OpenSearch服务测试
    print_section "7. OpenSearch服务测试"
    test_api_endpoint "/api/v1/services/opensearch/health" "OpenSearch健康检查" "connected"
    test_api_endpoint "/api/v1/services/opensearch/cluster/health" "OpenSearch集群健康" "success"
    test_json_structure "/api/v1/services/opensearch/cluster/health" "OpenSearch状态" ".data.status"
    
    # 8. Collectors管理测试
    print_section "8. Collectors管理测试"
    test_api_endpoint "/api/v1/collectors" "Collectors列表" "success"
    test_json_structure "/api/v1/collectors" "Collectors数据结构" ".data | length"
    
    # 9. 系统资源测试
    print_section "9. 系统资源测试"
    test_api_endpoint "/api/v1/resources/scripts/agentless/setup-terminal.sh" "安装脚本资源" ""
    
    # 10. 详细信息展示
    print_section "10. 详细系统信息"
    show_detailed_info "/api/v1/health" "系统健康状态"
    show_detailed_info "/api/v1/services/kafka/health" "Kafka健康信息"
    
    # 测试结果汇总
    print_header "📊 测试结果汇总"
    echo -e "总测试数: ${TOTAL_TESTS}"
    echo -e "${GREEN}通过测试: ${PASSED_TESTS}${NC}"
    echo -e "${RED}失败测试: ${FAILED_TESTS}${NC}"
    
    if [ $FAILED_TESTS -eq 0 ]; then
        echo -e "\n${GREEN}🎉 所有测试通过！系统健康状态良好！${NC}"
        exit 0
    else
        echo -e "\n${RED}⚠️  发现 $FAILED_TESTS 个问题，请检查系统状态${NC}"
        exit 1
    fi
}

# 检查依赖
check_dependencies() {
    if ! command -v curl &> /dev/null; then
        echo -e "${RED}错误: 需要安装 curl${NC}"
        exit 1
    fi
    
    if ! command -v jq &> /dev/null; then
        echo -e "${RED}错误: 需要安装 jq${NC}"
        exit 1
    fi
}

# 脚本入口
if [ "$1" = "--help" ] || [ "$1" = "-h" ]; then
    echo "SysArmor EDR 系统健康状态测试脚本"
    echo ""
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  -h, --help     显示帮助信息"
    echo "  --url URL      指定Manager API地址 (默认: http://localhost:8080)"
    echo "  --timeout SEC  设置请求超时时间 (默认: 10秒)"
    echo ""
    echo "示例:"
    echo "  $0                                    # 使用默认配置"
    echo "  $0 --url http://192.168.1.100:8080   # 指定远程Manager"
    echo "  $0 --timeout 30                      # 设置30秒超时"
    exit 0
fi

# 解析命令行参数
while [[ $# -gt 0 ]]; do
    case $1 in
        --url)
            MANAGER_URL="$2"
            shift 2
            ;;
        --timeout)
            TIMEOUT="$2"
            shift 2
            ;;
        *)
            echo -e "${RED}未知参数: $1${NC}"
            echo "使用 --help 查看帮助信息"
            exit 1
            ;;
    esac
done

# 执行测试
check_dependencies
main
