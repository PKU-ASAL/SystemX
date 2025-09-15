#!/bin/bash

# SysArmor Kafka Producer 测试脚本
# 用于测试 Kafka 中间件健康状态和数据导入功能

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
KAFKA_BOOTSTRAP_SERVERS="localhost:9092"
DEFAULT_TOPIC="sysarmor.raw.audit"
DEFAULT_DATA_DIR="./data/kafka-imports"
TIMEOUT=10

# 显示帮助信息
show_help() {
    cat << EOF
SysArmor Kafka Producer 测试脚本

用法: $0 [选项] [文件名]

选项:
  --topic <topic>       指定目标 Topic (默认: $DEFAULT_TOPIC)
  --data-dir <dir>      指定数据目录 (默认: $DEFAULT_DATA_DIR)
  --bootstrap <servers> 指定 Kafka 服务器 (默认: $KAFKA_BOOTSTRAP_SERVERS)
  --help               显示此帮助信息

参数:
  文件名               要导入的 JSONL 文件名 (在数据目录中)

功能:
  1. 检测 Middleware 和 Kafka 健康状态
  2. 显示 Kafka 现有 Topics 和消息数量
  3. 导入指定的 JSONL 文件到 Kafka Topic
  4. 验证数据导入结果

示例:
  $0                                    # 交互式选择文件
  $0 sample.jsonl                       # 导入 sample.jsonl
  $0 --topic sysarmor.raw.other test.jsonl  # 导入到指定 Topic
  $0 --data-dir /path/to/data audit.jsonl   # 指定数据目录

EOF
}

# 解析命令行参数
TARGET_TOPIC="$DEFAULT_TOPIC"
DATA_DIR="$DEFAULT_DATA_DIR"
INPUT_FILE=""

while [[ $# -gt 0 ]]; do
    case $1 in
        --topic)
            TARGET_TOPIC="$2"
            shift 2
            ;;
        --data-dir)
            DATA_DIR="$2"
            shift 2
            ;;
        --bootstrap)
            KAFKA_BOOTSTRAP_SERVERS="$2"
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
            INPUT_FILE="$1"
            shift
            ;;
    esac
done

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

# 检查依赖
check_dependencies() {
    local missing_deps=()
    
    if ! command -v curl &> /dev/null; then
        missing_deps+=("curl")
    fi
    
    if ! command -v jq &> /dev/null; then
        missing_deps+=("jq")
    fi
    
    if ! command -v docker &> /dev/null; then
        missing_deps+=("docker")
    fi
    
    if [ ${#missing_deps[@]} -gt 0 ]; then
        echo -e "${RED}错误: 缺少必要依赖: ${missing_deps[*]}${NC}"
        exit 1
    fi
}

# 获取 Kafka Topics 信息 (使用 kafka-tools.sh)
get_kafka_topics_info() {
    echo -e "${YELLOW}📊 Kafka Topics 状态:${NC}"
    
    # 使用 kafka-tools.sh 获取 topics 信息
    if ./scripts/kafka-tools.sh list 2>/dev/null | grep -E "^\s*★\s*sysarmor" | while IFS= read -r line; do
        echo "  $line"
    done; then
        echo ""
    else
        print_warning "无法获取 Topics 信息"
        echo ""
    fi
}

# 获取指定 Topic 的消息数量 (直接使用 Docker 网络)
get_topic_message_count() {
    local topic="$1"
    
    # 直接使用 Docker 网络连接，避免复杂的脚本调用
    local offset_info=$(docker run --rm --network sysarmor-net confluentinc/cp-kafka:7.4.0 \
        kafka-run-class kafka.tools.GetOffsetShell \
        --broker-list sysarmor-kafka-1:9092 \
        --topic "$topic" \
        --time -1 2>/dev/null | awk -F: '{sum+=$3} END {print sum}')
    
    echo "${offset_info:-0}"
}

# 选择输入文件
select_input_file() {
    if [ -n "$INPUT_FILE" ]; then
        return 0
    fi
    
    echo -e "${YELLOW}📁 选择要导入的数据文件:${NC}"
    echo "数据目录: $DATA_DIR"
    
    if [ ! -d "$DATA_DIR" ]; then
        print_error "数据目录不存在: $DATA_DIR"
        exit 1
    fi
    
    local files=($(find "$DATA_DIR" -name "*.jsonl" -type f 2>/dev/null | sort))
    
    if [ ${#files[@]} -eq 0 ]; then
        print_error "在 $DATA_DIR 中未找到 .jsonl 文件"
        exit 1
    fi
    
    echo "可用文件:"
    for i in "${!files[@]}"; do
        local file="${files[$i]}"
        local filename=$(basename "$file")
        local filesize=$(du -h "$file" | cut -f1)
        local linecount=$(wc -l < "$file" 2>/dev/null || echo "?")
        echo "  $((i+1)). $filename ($filesize, $linecount 行)"
    done
    
    echo -n "请选择文件 (1-${#files[@]}): "
    read -r selection
    
    if [[ "$selection" =~ ^[0-9]+$ ]] && [ "$selection" -ge 1 ] && [ "$selection" -le ${#files[@]} ]; then
        INPUT_FILE=$(basename "${files[$((selection-1))]}")
        echo -e "${GREEN}✅ 已选择: $INPUT_FILE${NC}"
    else
        print_error "无效选择"
        exit 1
    fi
}

# 导入数据到 Kafka (使用 kafka-tools.sh)
import_data_to_kafka() {
    local file_path="$DATA_DIR/$INPUT_FILE"
    
    if [ ! -f "$file_path" ]; then
        print_error "文件不存在: $file_path"
        exit 1
    fi
    
    local line_count=$(wc -l < "$file_path" 2>/dev/null || echo "0")
    local file_size=$(du -h "$file_path" | cut -f1)
    
    print_info "准备导入数据:"
    echo "  📁 文件: $INPUT_FILE"
    echo "  📊 大小: $file_size"
    echo "  📝 行数: $line_count"
    echo "  🎯 目标 Topic: $TARGET_TOPIC"
    echo ""
    
    echo -n "确认导入? (y/N): "
    read -r confirm
    if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
        echo "导入取消"
        exit 0
    fi
    
    echo -e "${YELLOW}📤 开始导入数据...${NC}"
    
    # 使用 kafka-tools.sh 进行导入 (避免 JVM 冲突)
    if ./scripts/kafka-tools.sh import "$file_path" "$TARGET_TOPIC" > /tmp/kafka-import.log 2>&1; then
        print_success "数据导入完成"
        echo "  📊 已导入 $line_count 条消息到 $TARGET_TOPIC"
        # 显示导入日志的关键信息
        if grep -q "SUCCESS" /tmp/kafka-import.log; then
            grep "SUCCESS" /tmp/kafka-import.log | tail -1 | sed 's/.*SUCCESS]/  ✅/'
        fi
    else
        print_error "数据导入失败"
        echo "查看详细日志: cat /tmp/kafka-import.log"
        exit 1
    fi
}

# 主函数
main() {
    print_header "🚀 SysArmor Kafka Producer 测试"
    
    echo -e "Kafka 服务器: ${KAFKA_BOOTSTRAP_SERVERS}"
    echo -e "Manager API: ${MANAGER_API}"
    echo -e "目标 Topic: ${TARGET_TOPIC}"
    echo -e "数据目录: ${DATA_DIR}"
    echo -e "测试时间: $(date)"
    
    # 步骤1: 检查 Middleware 和 Kafka 健康状态
    print_section "1. Middleware 和 Kafka 健康检查"
    
    # 检查 Manager API
    echo -n "Manager API 连通性: "
    if curl -s --max-time $TIMEOUT "$MANAGER_API/health" > /dev/null 2>&1; then
        print_success "Manager API 正常"
    else
        print_error "Manager API 不可用"
        exit 1
    fi
    
    # 检查 Kafka 健康状态
    echo -n "Kafka 健康状态: "
    if kafka_health=$(curl -s --max-time $TIMEOUT "$MANAGER_API/api/v1/services/kafka/health" 2>/dev/null); then
        if echo "$kafka_health" | jq -e '.connected' > /dev/null 2>&1; then
            print_success "Kafka 集群正常"
            local broker_count=$(echo "$kafka_health" | jq -r '.broker_count // "N/A"' 2>/dev/null)
            echo "  📊 Broker 数量: $broker_count"
        else
            print_error "Kafka 集群异常"
            exit 1
        fi
    else
        print_error "无法获取 Kafka 健康状态"
        exit 1
    fi
    
    # 检查 Docker 容器状态
    echo -n "Kafka 容器状态: "
    if docker ps --filter "name=sysarmor-kafka-1" --format "table {{.Names}}\t{{.Status}}" | grep -q "Up"; then
        print_success "Kafka 容器运行正常"
    else
        print_error "Kafka 容器未运行"
        exit 1
    fi
    
    # 步骤2: 显示 Kafka 现有 Topics (导入前)
    print_section "2. Kafka Topics 状态 (导入前)"
    
    echo -e "${PURPLE}📊 记录导入前的 Topic 状态...${NC}"
    
    # 记录关键 topics 的消息数量
    local raw_audit_before=$(get_topic_message_count "sysarmor.raw.audit")
    local events_audit_before=$(get_topic_message_count "sysarmor.events.audit")
    local alerts_before=$(get_topic_message_count "sysarmor.alerts")
    
    echo "  📊 关键 Topics 消息数量 (导入前):"
    echo "    🎯 sysarmor.raw.audit: $raw_audit_before"
    echo "    🔄 sysarmor.events.audit: $events_audit_before"
    echo "    🚨 sysarmor.alerts: $alerts_before"
    echo ""
    
    get_kafka_topics_info
    
    # 步骤3: 选择和导入数据文件
    print_section "3. 数据文件导入"
    
    select_input_file
    import_data_to_kafka
    
    # 等待数据写入
    echo -n "等待数据写入..."
    sleep 3
    echo -e " ${GREEN}✅${NC}"
    
    # 步骤4: 显示 Kafka Topics (导入后) 并验证变化
    print_section "4. 数据导入结果验证"
    
    echo -e "${PURPLE}📊 验证导入后的 Topic 状态...${NC}"
    
    # 获取导入后的消息数量
    local raw_audit_after=$(get_topic_message_count "sysarmor.raw.audit")
    local events_audit_after=$(get_topic_message_count "sysarmor.events.audit")
    local alerts_after=$(get_topic_message_count "sysarmor.alerts")
    
    echo "  📊 关键 Topics 消息数量 (导入后):"
    echo "    🎯 sysarmor.raw.audit: $raw_audit_after"
    echo "    🔄 sysarmor.events.audit: $events_audit_after"
    echo "    🚨 sysarmor.alerts: $alerts_after"
    echo ""
    
    # 计算各个 topic 的变化
    echo -e "${CYAN}📈 消息数量变化对比:${NC}"
    
    # 原始数据 topic 变化
    if [[ "$raw_audit_before" != "N/A" && "$raw_audit_after" != "N/A" ]]; then
        local raw_diff=$((raw_audit_after - raw_audit_before))
        if [ $raw_diff -gt 0 ]; then
            echo "  🎯 sysarmor.raw.audit: +$raw_diff 条消息 ✅"
        else
            echo "  🎯 sysarmor.raw.audit: 无变化"
        fi
    fi
    
    # 处理后事件 topic 变化
    if [[ "$events_audit_before" != "N/A" && "$events_audit_after" != "N/A" ]]; then
        local events_diff=$((events_audit_after - events_audit_before))
        if [ $events_diff -gt 0 ]; then
            echo "  🔄 sysarmor.events.audit: +$events_diff 条消息 ✅ (Flink处理结果)"
        else
            echo "  🔄 sysarmor.events.audit: 无变化"
        fi
    fi
    
    # 告警 topic 变化
    if [[ "$alerts_before" != "N/A" && "$alerts_after" != "N/A" ]]; then
        local alerts_diff=$((alerts_after - alerts_before))
        if [ $alerts_diff -gt 0 ]; then
            echo "  🚨 sysarmor.alerts: +$alerts_diff 条消息 ✅ (告警生成)"
        else
            echo "  🚨 sysarmor.alerts: 无变化"
        fi
    fi
    
    echo ""
    get_kafka_topics_info
    
    # 步骤5: 测试总结
    print_section "5. 测试总结"
    
    print_success "Kafka Producer 测试完成"
    echo "  📁 导入文件: $INPUT_FILE"
    echo "  🎯 目标 Topic: $TARGET_TOPIC"
    echo ""
    echo "  📊 数据流处理统计:"
    echo "    📥 原始数据: $raw_audit_before → $raw_audit_after (+$((raw_audit_after - raw_audit_before)))"
    echo "    🔄 处理事件: $events_audit_before → $events_audit_after (+$((events_audit_after - events_audit_before)))"
    echo "    🚨 告警事件: $alerts_before → $alerts_after (+$((alerts_after - alerts_before)))"
    
    echo ""
    echo -e "${BLUE}💡 后续操作建议:${NC}"
    echo "1. 验证数据内容: ./scripts/kafka-tools.sh export $TARGET_TOPIC 5"
    echo "2. 启动 Flink 处理: ./tests/test-flink-processor.sh"
    echo "3. 查看处理结果: ./scripts/kafka-tools.sh export sysarmor.events.audit 5"
    echo "4. 系统健康检查: ./tests/test-system-health.sh"
    
    echo ""
    print_success "🎉 SysArmor Kafka Producer 测试完成！"
}

# 脚本入口
check_dependencies
main
