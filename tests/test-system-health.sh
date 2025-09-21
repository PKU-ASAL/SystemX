#!/bin/bash

# SysArmor EDR 系统健康状态测试脚本
# 简化版本 - 专注于主要组件健康状态

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
MANAGER_HOST="${MANAGER_HOST:-localhost}"
MANAGER_PORT="${MANAGER_PORT:-8080}"
MANAGER_URL="http://${MANAGER_HOST}:${MANAGER_PORT}"
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

print_success() {
    echo -e "${GREEN}✅ $1${NC}"
    ((PASSED_TESTS++))
}

print_error() {
    echo -e "${RED}❌ $1${NC}"
    ((FAILED_TESTS++))
}

print_info() {
    echo -e "${PURPLE}ℹ️  $1${NC}"
}

# 测试组件健康状态
test_component_health() {
    local component_name="$1"
    local jq_path="$2"
    
    ((TOTAL_TESTS++))
    
    local response=$(curl -s --max-time $TIMEOUT "$MANAGER_URL/api/v1/health" 2>/dev/null)
    local component_status=$(echo "$response" | jq -r "$jq_path" 2>/dev/null)
    
    if [ "$component_status" = "true" ] || [ "$component_status" = "connected" ] || [ "$component_status" = "running" ] || [ "$component_status" = "healthy" ]; then
        print_success "$component_name: 健康"
    else
        print_error "$component_name: 异常 ($component_status)"
    fi
}

# 主测试函数
main() {
    print_header "🚀 SysArmor EDR 系统健康状态检查"
    
    echo -e "测试目标: ${MANAGER_URL}"
    echo -e "测试时间: $(date)"
    
    # 检查Manager API连通性
    print_section "系统连通性检查"
    ((TOTAL_TESTS++))
    if curl -s --max-time $TIMEOUT "$MANAGER_URL/health" > /dev/null 2>&1; then
        print_success "Manager API: 连通"
    else
        print_error "Manager API: 不可用"
        echo -e "\n${RED}⚠️  Manager API不可用，无法继续健康检查${NC}"
        exit 1
    fi
    
    # 获取系统健康状态
    print_section "核心组件健康状态"
    
    local health_response=$(curl -s --max-time $TIMEOUT "$MANAGER_URL/api/v1/health" 2>/dev/null)
    
    if [ -z "$health_response" ]; then
        print_error "无法获取系统健康状态"
        exit 1
    fi
    
    # 检查各个组件
    test_component_health "数据库 (PostgreSQL)" ".data.services.manager.components.database.healthy"
    test_component_health "索引器 (OpenSearch)" ".data.services.indexer.components.opensearch.healthy"
    test_component_health "消息队列 (Kafka)" ".data.services.middleware.components.kafka.healthy"
    test_component_health "监控系统 (Prometheus)" ".data.services.middleware.components.prometheus.healthy"
    test_component_health "数据收集 (Vector)" ".data.services.middleware.components.vector.healthy"
    test_component_health "流处理 (Flink)" ".data.services.processor.components.flink.healthy"
    
    # 检查整体健康状态
    print_section "系统整体状态"
    ((TOTAL_TESTS++))
    local overall_healthy=$(echo "$health_response" | jq -r ".data.healthy" 2>/dev/null)
    if [ "$overall_healthy" = "true" ]; then
        print_success "系统整体状态: 健康"
    else
        print_error "系统整体状态: 异常"
    fi
    
    # 显示服务摘要
    local healthy_services=$(echo "$health_response" | jq -r ".data.summary.healthy_services" 2>/dev/null)
    local total_services=$(echo "$health_response" | jq -r ".data.summary.total_services" 2>/dev/null)
    local healthy_components=$(echo "$health_response" | jq -r ".data.summary.healthy_components" 2>/dev/null)
    local total_components=$(echo "$health_response" | jq -r ".data.summary.total_components" 2>/dev/null)
    
    print_info "服务状态: $healthy_services/$total_services 健康"
    print_info "组件状态: $healthy_components/$total_components 健康"
    
    # 测试结果汇总
    print_header "📊 测试结果汇总"
    echo -e "总测试数: ${TOTAL_TESTS}"
    echo -e "${GREEN}通过测试: ${PASSED_TESTS}${NC}"
    echo -e "${RED}失败测试: ${FAILED_TESTS}${NC}"
    
    if [ $FAILED_TESTS -eq 0 ]; then
        echo -e "\n${GREEN}🎉 所有组件健康！系统运行正常！${NC}"
        exit 0
    else
        echo -e "\n${RED}⚠️  发现 $FAILED_TESTS 个问题，请检查系统状态${NC}"
        echo -e "${BLUE}💡 详细检查: ./tests/test-system-api.sh${NC}"
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

# 显示帮助信息
if [ "$1" = "--help" ] || [ "$1" = "-h" ]; then
    echo "SysArmor EDR 系统健康状态测试脚本 (简化版)"
    echo ""
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  -h, --help     显示帮助信息"
    echo "  --url URL      指定Manager API地址 (默认: http://localhost:8080)"
    echo "  --timeout SEC  设置请求超时时间 (默认: 10秒)"
    echo ""
    echo "功能:"
    echo "  - 检查Manager API连通性"
    echo "  - 验证核心组件健康状态"
    echo "  - 显示系统整体状态摘要"
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
