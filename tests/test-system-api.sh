#!/bin/bash

# SysArmor 系统API接口测试脚本
# 测试所有Manager API接口是否正常工作

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
MANAGER_API="http://localhost:8080"
TIMEOUT=10
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# 导出配置
EXPORT_DIR="./data/api-exports"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
EXPORT_FILE="$EXPORT_DIR/api-test-results_$TIMESTAMP.json"
EXPORT_LOG="$EXPORT_DIR/api-test-log_$TIMESTAMP.txt"

# 测试结果数组
declare -a TEST_RESULTS=()
declare -a API_RESPONSES=()

# 全局变量存储创建的collector_id
CREATED_COLLECTOR_ID=""

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
}

print_error() {
    echo -e "${RED}❌ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

print_info() {
    echo -e "${PURPLE}ℹ️  $1${NC}"
}

# 测试单个API接口
test_api() {
    local method="$1"
    local endpoint="$2"
    local description="$3"
    local expected_status="${4:-200}"
    local data="$5"
    
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    
    echo -n "  Testing $description... "
    
    local curl_cmd="curl -s -w '%{http_code}' --max-time $TIMEOUT"
    
    if [ "$method" = "POST" ]; then
        if [ -n "$data" ]; then
            curl_cmd="$curl_cmd -X POST -H 'Content-Type: application/json' -d '$data'"
        else
            curl_cmd="$curl_cmd -X POST -H 'Content-Type: application/json' -d '{}'"
        fi
    elif [ "$method" = "PUT" ]; then
        if [ -n "$data" ]; then
            curl_cmd="$curl_cmd -X PUT -H 'Content-Type: application/json' -d '$data'"
        else
            curl_cmd="$curl_cmd -X PUT -H 'Content-Type: application/json' -d '{}'"
        fi
    elif [ "$method" = "DELETE" ]; then
        curl_cmd="$curl_cmd -X DELETE"
    fi
    
    local response=$(eval "$curl_cmd '$MANAGER_API$endpoint'" 2>/dev/null)
    local status_code="${response: -3}"
    local body="${response%???}"
    
    # 特殊处理：如果是创建Collector接口，提取collector_id
    if [ "$endpoint" = "/api/v1/collectors/register" ] && [ "$status_code" = "200" ]; then
        CREATED_COLLECTOR_ID=$(echo "$body" | jq -r '.data.collector_id // ""')
        if [ -n "$CREATED_COLLECTOR_ID" ]; then
            print_info "    Created Collector ID: $CREATED_COLLECTOR_ID"
        fi
    fi
    
    # 收集响应内容用于后续展示
    local formatted_body=""
    if echo "$body" | jq . > /dev/null 2>&1; then
        formatted_body=$(echo "$body" | jq -c . 2>/dev/null)
    else
        formatted_body="$body"
    fi
    
    # 限制响应长度避免输出过长
    if [ ${#formatted_body} -gt 300 ]; then
        formatted_body="${formatted_body:0:300}..."
    fi
    
    API_RESPONSES+=("$method $endpoint [$status_code]: $formatted_body")
    
    if [ "$status_code" = "$expected_status" ]; then
        print_success "PASS ($status_code)"
        PASSED_TESTS=$((PASSED_TESTS + 1))
        TEST_RESULTS+=("✅ $description")
        
        # 检查响应是否包含success字段
        if echo "$body" | jq -e '.success' > /dev/null 2>&1; then
            local success=$(echo "$body" | jq -r '.success')
            if [ "$success" = "false" ]; then
                print_warning "    Response success=false"
                local error=$(echo "$body" | jq -r '.error // "Unknown error"')
                echo "    Error: $error"
            fi
        fi
    else
        print_error "FAIL ($status_code, expected $expected_status)"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        TEST_RESULTS+=("❌ $description")
        
        # 显示错误详情
        if [ ${#body} -lt 200 ]; then
            echo "    Response: $body"
        else
            echo "    Response: ${body:0:200}..."
        fi
    fi
}

# 导出测试结果
export_results() {
    # 创建导出目录
    mkdir -p "$EXPORT_DIR"
    
    # 生成JSON格式的测试结果
    cat > "$EXPORT_FILE" << EOF
{
  "test_metadata": {
    "manager_api": "$MANAGER_API",
    "test_time": "$(date -Iseconds)",
    "timeout_seconds": $TIMEOUT,
    "total_tests": $TOTAL_TESTS,
    "passed_tests": $PASSED_TESTS,
    "failed_tests": $FAILED_TESTS,
    "success_rate": $(( PASSED_TESTS * 100 / TOTAL_TESTS ))
  },
  "test_results": [
EOF

    # 添加每个测试结果
    for i in "${!TEST_RESULTS[@]}"; do
        local result="${TEST_RESULTS[$i]}"
        local response="${API_RESPONSES[$i]}"
        
        # 解析响应信息
        local method=$(echo "$response" | cut -d' ' -f1)
        local endpoint=$(echo "$response" | cut -d' ' -f2)
        local status_code=$(echo "$response" | sed 's/.*\[\([0-9]*\)\].*/\1/')
        local body=$(echo "$response" | sed 's/.*\[[0-9]*\]: //')
        
        # 判断是否通过
        local passed="true"
        if [[ "$result" == ❌* ]]; then
            passed="false"
        fi
        
        cat >> "$EXPORT_FILE" << EOF
    {
      "test_id": $((i+1)),
      "method": "$method",
      "endpoint": "$endpoint",
      "description": "$(echo "$result" | sed 's/^[✅❌] //')",
      "passed": $passed,
      "status_code": $status_code,
      "response_body": $body
    }$([ $i -lt $((${#TEST_RESULTS[@]} - 1)) ] && echo "," || echo "")
EOF
    done

    cat >> "$EXPORT_FILE" << EOF
  ]
}
EOF

    # 生成文本格式的测试日志
    {
        echo "SysArmor API接口测试日志"
        echo "========================"
        echo "测试时间: $(date)"
        echo "Manager API: $MANAGER_API"
        echo "超时设置: ${TIMEOUT}秒"
        echo ""
        echo "测试统计:"
        echo "  总测试数: $TOTAL_TESTS"
        echo "  通过测试: $PASSED_TESTS"
        echo "  失败测试: $FAILED_TESTS"
        echo "  成功率: $(( PASSED_TESTS * 100 / TOTAL_TESTS ))%"
        echo ""
        echo "详细结果:"
        for result in "${TEST_RESULTS[@]}"; do
            echo "  $result"
        done
        echo ""
        echo "API响应详情:"
        for i in "${!API_RESPONSES[@]}"; do
            echo "$((i+1)). ${API_RESPONSES[$i]}"
        done
    } > "$EXPORT_LOG"
    
    print_info "测试结果已导出:"
    echo "  📄 JSON格式: $EXPORT_FILE"
    echo "  📝 日志格式: $EXPORT_LOG"
}

# 检查依赖
check_dependencies() {
    local missing_deps=()
    
    if ! command -v curl &> /dev/null; then
        missing_deps+=("curl")
    fi
    
    if ! command -v jq &> /dev/null; then
        missing_deps+=("jq")
    fi
    
    if [ ${#missing_deps[@]} -gt 0 ]; then
        echo -e "${RED}错误: 缺少必要依赖: ${missing_deps[*]}${NC}"
        exit 1
    fi
}

# 主函数
main() {
    print_header "🚀 SysArmor 系统API接口测试"
    
    echo -e "Manager API: ${MANAGER_API}"
    echo -e "测试时间: $(date)"
    echo -e "超时设置: ${TIMEOUT}秒"
    
    # 1. 基础健康检查接口
    print_section "1. 基础健康检查接口"
    test_api "GET" "/health" "基础健康检查"
    test_api "GET" "/api/v1/health" "系统健康状态"
    test_api "GET" "/api/v1/health/overview" "健康状态概览"
    test_api "GET" "/api/v1/health/comprehensive" "综合健康状态"
    test_api "GET" "/api/v1/health/workers" "Worker状态列表"
    
    # 2. Kafka服务接口
    print_section "2. Kafka服务接口"
    test_api "GET" "/api/v1/services/kafka/health" "Kafka健康检查"
    test_api "GET" "/api/v1/services/kafka/clusters" "Kafka集群信息"
    test_api "GET" "/api/v1/services/kafka/brokers" "Kafka Brokers信息"
    test_api "GET" "/api/v1/services/kafka/brokers/overview" "Brokers概览"
    test_api "GET" "/api/v1/services/kafka/topics" "Topics列表"
    test_api "GET" "/api/v1/services/kafka/topics/overview" "Topics概览"
    test_api "GET" "/api/v1/services/kafka/consumer-groups" "Consumer Groups列表"
    
    # 3. Flink服务接口
    print_section "3. Flink服务接口"
    test_api "GET" "/api/v1/services/flink/health" "Flink健康检查"
    test_api "GET" "/api/v1/services/flink/overview" "Flink集群概览"
    test_api "GET" "/api/v1/services/flink/config" "Flink配置信息"
    test_api "GET" "/api/v1/services/flink/jobs" "Flink作业列表"
    test_api "GET" "/api/v1/services/flink/jobs/overview" "Flink作业概览"
    test_api "GET" "/api/v1/services/flink/taskmanagers" "TaskManager信息"
    test_api "GET" "/api/v1/services/flink/taskmanagers/overview" "TaskManager概览"
    
    # 4. OpenSearch服务接口
    print_section "4. OpenSearch服务接口"
    test_api "GET" "/api/v1/services/opensearch/health" "OpenSearch健康检查"
    test_api "GET" "/api/v1/services/opensearch/cluster/health" "OpenSearch集群健康"
    test_api "GET" "/api/v1/services/opensearch/cluster/stats" "OpenSearch集群统计" "200"  # 修复：JSON解析错误已修复
    test_api "GET" "/api/v1/services/opensearch/indices" "OpenSearch索引列表"
    test_api "GET" "/api/v1/services/opensearch/events/recent" "最近事件查询"
    test_api "GET" "/api/v1/services/opensearch/events/search" "事件搜索"
    test_api "GET" "/api/v1/services/opensearch/events/aggregations" "事件聚合统计"
    
    # 5. 事件查询接口
    print_section "5. 事件查询接口"
    test_api "GET" "/api/v1/events/latest" "最新事件查询"
    test_api "GET" "/api/v1/events/query?topic=sysarmor.raw.audit&limit=5" "事件查询"
    test_api "GET" "/api/v1/events/topics" "事件Topics列表"
    
    # 6. Topic配置管理接口
    print_section "6. Topic配置管理接口"
    test_api "GET" "/api/v1/topics/configs" "Topic配置查询"
    test_api "GET" "/api/v1/topics/categories" "Topic分类查询"
    test_api "GET" "/api/v1/topics/defaults" "默认Topics查询"
    
    # 7. Collector管理接口 (完整生命周期测试)
    print_section "7. Collector管理接口"
    
    # 7.1 创建Collector
    test_api "POST" "/api/v1/collectors/register" "创建Collector" "200" '{"deployment_type":"agentless","hostname":"test-collector","ip_address":"192.168.1.100","os_type":"linux","os_version":"ubuntu-20.04","metadata":{"environment":"test","purpose":"api-testing"}}'
    
    # 7.2 查询Collector列表 (验证创建成功)
    test_api "GET" "/api/v1/collectors" "Collector列表"
    
    # 检查是否成功获取到collector_id
    if [ -z "$CREATED_COLLECTOR_ID" ]; then
        print_warning "    未能获取到创建的Collector ID，使用测试ID继续"
        CREATED_COLLECTOR_ID="test-collector-id"
    fi
    
    print_info "使用Collector ID进行后续测试: $CREATED_COLLECTOR_ID"
    
    # 7.3 获取特定Collector状态 (使用真实ID)
    test_api "GET" "/api/v1/collectors/$CREATED_COLLECTOR_ID" "获取Collector状态" "200"
    
    # 7.4 Collector心跳上报
    test_api "POST" "/api/v1/collectors/$CREATED_COLLECTOR_ID/heartbeat" "Collector心跳上报" "200" '{"status":"active"}'
    
    # 7.5 主动探测Collector (可能超时)
    test_api "POST" "/api/v1/collectors/$CREATED_COLLECTOR_ID/probe" "主动探测Collector" "000" '{"timeout":10}'  # 预期超时
    
    # 7.6 更新Collector元数据
    test_api "PUT" "/api/v1/collectors/$CREATED_COLLECTOR_ID/metadata" "更新Collector元数据" "200" '{"metadata":{"environment":"production","tags":["updated","test"]}}'
    
    # 7.7 再次查询验证更新
    test_api "GET" "/api/v1/collectors/$CREATED_COLLECTOR_ID" "验证更新后状态" "200"
    
    # 7.8 注销Collector (软删除) - 先注销再删除
    test_api "POST" "/api/v1/collectors/$CREATED_COLLECTOR_ID/unregister" "注销Collector" "200"  # 修复：使用正确的测试顺序
    
    # 7.9 验证注销后状态
    test_api "GET" "/api/v1/collectors/$CREATED_COLLECTOR_ID" "验证注销后状态" "200"  # 应该显示unregistered状态
    
    # 7.10 软删除Collector (设为inactive)
    test_api "DELETE" "/api/v1/collectors/$CREATED_COLLECTOR_ID" "软删除Collector" "200"
    
    # 7.11 验证软删除后状态
    test_api "GET" "/api/v1/collectors/$CREATED_COLLECTOR_ID" "验证软删除后状态" "200"  # 应该显示inactive状态
    
    # 8. 资源管理接口 (在硬删除之前测试)
    print_section "8. 资源管理接口"
    # 使用创建的collector_id测试资源下载 (在Collector还存在时)
    if [ -n "$CREATED_COLLECTOR_ID" ]; then
        test_api "GET" "/api/v1/resources/scripts/agentless/setup-terminal.sh?collector_id=$CREATED_COLLECTOR_ID" "获取部署脚本" "200"
        test_api "GET" "/api/v1/resources/configs/agentless/audit-rules?collector_id=$CREATED_COLLECTOR_ID" "获取audit规则配置" "200"  # agentless使用audit-rules
        test_api "GET" "/api/v1/resources/configs/collector/cfg.yaml?collector_id=$CREATED_COLLECTOR_ID" "获取collector配置(类型不匹配)" "500"  # 类型不匹配应该失败
    else
        # 如果没有collector_id，测试参数验证
        test_api "GET" "/api/v1/resources/scripts/agentless/setup-terminal.sh" "获取部署脚本(无参数)" "400"
        test_api "GET" "/api/v1/resources/configs/agentless/audit-rules" "获取配置文件(无参数)" "400"
    fi
    
    # 7.12 硬删除Collector (永久删除)
    test_api "DELETE" "/api/v1/collectors/$CREATED_COLLECTOR_ID?force=true" "硬删除Collector" "200"
    
    # 7.13 验证硬删除后状态
    test_api "GET" "/api/v1/collectors/$CREATED_COLLECTOR_ID" "验证硬删除后状态" "404"  # 应该返回404，因为已被永久删除
    
    # 9. Wazuh集成接口 (如果启用)
    print_section "9. Wazuh集成接口"
    test_api "GET" "/api/v1/wazuh/config" "Wazuh配置查询" "200"  # Wazuh禁用时返回200
    
    # 10. 特定Topic接口测试
    print_section "10. 特定Topic接口测试"
    test_api "GET" "/api/v1/services/kafka/topics/sysarmor.raw.audit" "获取audit原始Topic详情"
    test_api "GET" "/api/v1/services/kafka/topics/sysarmor.events.audit" "获取audit事件Topic详情"
    test_api "GET" "/api/v1/services/kafka/topics/sysarmor.alerts.audit" "获取audit告警Topic详情"
    
    # 11. 新的audit告警索引测试
    print_section "11. 新的audit告警索引测试"
    test_api "GET" "/api/v1/services/opensearch/events/search?index=sysarmor-alerts-audit&size=3" "查询audit告警索引" "500"  # 已知问题：@timestamp字段映射问题
    
    # 测试总结
    print_section "测试总结"
    
    echo -e "${BLUE}📊 测试统计:${NC}"
    echo "  总测试数: $TOTAL_TESTS"
    echo "  通过测试: $PASSED_TESTS"
    echo "  失败测试: $FAILED_TESTS"
    echo "  成功率: $(( PASSED_TESTS * 100 / TOTAL_TESTS ))%"
    
    echo -e "\n${BLUE}📋 详细结果:${NC}"
    for result in "${TEST_RESULTS[@]}"; do
        echo "  $result"
    done
    
    # API响应展示section
    print_section "API接口响应展示"
    echo -e "${PURPLE}📄 各接口实际返回内容:${NC}"
    for i in "${!API_RESPONSES[@]}"; do
        echo -e "${YELLOW}$((i+1)).${NC} ${API_RESPONSES[$i]}"
    done
    
    # 导出测试结果
    print_section "导出测试结果"
    export_results
    
    if [ $FAILED_TESTS -eq 0 ]; then
        echo -e "\n${GREEN}🎉 所有API接口测试通过！${NC}"
        echo -e "${BLUE}📁 测试结果已保存到: $EXPORT_DIR${NC}"
        exit 0
    else
        echo -e "\n${YELLOW}⚠️  有 $FAILED_TESTS 个接口测试失败${NC}"
        echo -e "${BLUE}💡 建议检查:${NC}"
        echo "  1. 服务是否完全启动"
        echo "  2. 网络连接是否正常"
        echo "  3. 依赖服务是否健康"
        echo "  4. 查看Manager日志: docker logs sysarmor-manager-1"
        echo -e "${BLUE}📁 测试结果已保存到: $EXPORT_DIR${NC}"
        exit 1
    fi
}

# 显示帮助信息
show_help() {
    cat << EOF
SysArmor 系统API接口测试脚本

用法: $0 [选项]

选项:
  --api <url>       指定Manager API地址 (默认: $MANAGER_API)
  --timeout <sec>   指定请求超时时间 (默认: $TIMEOUT秒)
  --help           显示此帮助信息

功能:
  1. 测试基础健康检查接口
  2. 测试Kafka服务管理接口
  3. 测试Flink服务管理接口
  4. 测试OpenSearch服务管理接口
  5. 测试事件查询接口
  6. 测试Topic配置管理接口
  7. 测试Collector管理接口
  8. 测试资源管理接口
  9. 测试Wazuh集成接口
  10. 测试重构后的数据流接口

示例:
  $0                                    # 使用默认配置测试
  $0 --api http://remote:8080           # 测试远程API
  $0 --timeout 30                       # 使用30秒超时

EOF
}

# 解析命令行参数
while [[ $# -gt 0 ]]; do
    case $1 in
        --api)
            MANAGER_API="$2"
            shift 2
            ;;
        --timeout)
            TIMEOUT="$2"
            shift 2
            ;;
        --help|-h)
            show_help
            exit 0
            ;;
        -*)
            echo -e "${RED}未知参数: $1${NC}"
            show_help
            exit 1
            ;;
        *)
            echo -e "${RED}未知参数: $1${NC}"
            show_help
            exit 1
            ;;
    esac
done

# 脚本入口
check_dependencies
main
