#!/bin/bash

# =============================================================================
# SysArmor Kafka 工具脚本
# 提供 Kafka 事件的导出、导入和 topic 管理功能
# =============================================================================

set -e

# 配置参数
DEFAULT_KAFKA_BROKERS="localhost:9094"
KAFKA_BOOTSTRAP_SERVERS="${KAFKA_BROKERS:-$DEFAULT_KAFKA_BROKERS}"
DEFAULT_OUTPUT_DIR="./data/kafka-exports"
DEFAULT_INPUT_DIR="./data/kafka-exports"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
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

# 检查依赖
check_dependencies() {
    if ! command -v docker &> /dev/null; then
        log_error "Docker 未安装或不在 PATH 中"
        exit 1
    fi
}

# 检查 Kafka 连接
check_kafka_connection() {
    docker run --rm confluentinc/cp-kafka:7.4.0 \
        kafka-topics --bootstrap-server ${KAFKA_BOOTSTRAP_SERVERS} --list &>/dev/null || {
        log_error "无法连接到 Kafka 服务器 ${KAFKA_BOOTSTRAP_SERVERS}"
        exit 1
    }
}

# 获取 Kafka topics
get_kafka_topics() {
    docker run --rm confluentinc/cp-kafka:7.4.0 \
        kafka-topics --bootstrap-server ${KAFKA_BOOTSTRAP_SERVERS} --list 2>/dev/null
}

# 列出 topics
list_topics() {
    echo "=============================================="
    echo "SysArmor Kafka Topics"
    echo "=============================================="
    echo "服务器: ${KAFKA_BOOTSTRAP_SERVERS}"
    echo "=============================================="
    
    check_dependencies
    check_kafka_connection
    
    log_info "获取 Kafka topics..."
    topics=$(get_kafka_topics)
    
    if [[ -z "$topics" ]]; then
        log_error "未找到任何 topics"
        exit 1
    fi
    
    log_success "可用 topics:"
    echo "$topics" | while IFS= read -r topic; do
        if [[ "$topic" =~ (sysarmor|agentless) ]]; then
            echo -e "  ${GREEN}★${NC} $topic"
        else
            echo "  - $topic"
        fi
    done
    echo "=============================================="
}

# 导出事件
export_events() {
    local topic="$1"
    local count="${2:-1000}"
    local output_dir="${3:-$DEFAULT_OUTPUT_DIR}"
    local output_file="${output_dir}/${topic}_${TIMESTAMP}.jsonl"
    
    # 检查是否导出全部数据
    local export_all=false
    local max_messages_param=""
    if [[ "$count" == "all" || "$count" == "ALL" || "$count" == "-1" ]]; then
        export_all=true
        count="全部"
        max_messages_param=""
    else
        max_messages_param="--max-messages $count"
    fi
    
    echo "=============================================="
    echo "导出 Kafka 事件"
    echo "=============================================="
    echo "Topic: $topic"
    echo "数量: $count"
    echo "输出: $output_file"
    echo "=============================================="
    
    check_dependencies
    check_kafka_connection
    
    # 创建输出目录
    mkdir -p "$output_dir"
    
    if [[ "$export_all" == "true" ]]; then
        log_info "从 topic '$topic' 导出全部事件..."
        log_warning "导出全部数据可能需要较长时间，请耐心等待..."
        
        # 导出全部事件 (不使用 --max-messages 参数)
        docker run --rm confluentinc/cp-kafka:7.4.0 \
            kafka-console-consumer \
            --bootstrap-server ${KAFKA_BOOTSTRAP_SERVERS} \
            --topic "$topic" \
            --from-beginning \
            --timeout-ms 60000 > "$output_file" 2>/dev/null || {
            log_error "导出失败"
            rm -f "$output_file"
            exit 1
        }
    else
        log_info "从 topic '$topic' 导出 $count 条事件..."
        
        # 导出指定数量的事件
        docker run --rm confluentinc/cp-kafka:7.4.0 \
            kafka-console-consumer \
            --bootstrap-server ${KAFKA_BOOTSTRAP_SERVERS} \
            --topic "$topic" \
            --from-beginning \
            $max_messages_param \
            --timeout-ms 30000 > "$output_file" 2>/dev/null || {
            log_error "导出失败"
            rm -f "$output_file"
            exit 1
        }
    fi
    
    # 检查结果
    if [[ -f "$output_file" && -s "$output_file" ]]; then
        local line_count=$(wc -l < "$output_file")
        local file_size=$(du -h "$output_file" | cut -f1)
        log_success "成功导出 $line_count 条事件"
        log_info "文件: $output_file ($file_size)"
        
        if [[ "$export_all" == "true" ]]; then
            log_info "已导出 topic '$topic' 的全部数据"
        fi
    else
        log_error "导出失败或文件为空"
        exit 1
    fi
}

# 导入事件
import_events() {
    local input_file="$1"
    local topic="$2"
    
    echo "=============================================="
    echo "导入 Kafka 事件"
    echo "=============================================="
    echo "文件: $input_file"
    echo "Topic: $topic"
    echo "=============================================="
    
    if [[ ! -f "$input_file" ]]; then
        log_error "输入文件不存在: $input_file"
        exit 1
    fi
    
    if [[ ! -s "$input_file" ]]; then
        log_error "输入文件为空: $input_file"
        exit 1
    fi
    
    check_dependencies
    check_kafka_connection
    
    local line_count=$(wc -l < "$input_file")
    log_info "准备导入 $line_count 条事件到 topic '$topic'..."
    
    # 检查 topic 是否存在，不存在则创建
    if ! docker run --rm confluentinc/cp-kafka:7.4.0 \
        kafka-topics --bootstrap-server ${KAFKA_BOOTSTRAP_SERVERS} \
        --describe --topic "$topic" &>/dev/null; then
        log_info "创建 topic '$topic'..."
        docker run --rm confluentinc/cp-kafka:7.4.0 \
            kafka-topics --bootstrap-server ${KAFKA_BOOTSTRAP_SERVERS} \
            --create --topic "$topic" \
            --partitions 3 --replication-factor 1 || {
            log_error "创建 topic 失败"
            exit 1
        }
        log_success "Topic '$topic' 创建成功"
    fi
    
    # 导入数据
    docker run --rm -i confluentinc/cp-kafka:7.4.0 \
        kafka-console-producer \
        --bootstrap-server ${KAFKA_BOOTSTRAP_SERVERS} \
        --topic "$topic" < "$input_file" || {
        log_error "导入失败"
        exit 1
    }
    
    log_success "成功导入 $line_count 条事件到 topic '$topic'"
}

# 显示帮助信息
show_help() {
    cat << EOF
SysArmor Kafka 工具

用法: $0 <命令> [选项]
      KAFKA_BROKERS=<brokers> $0 <命令> [选项]

命令:
  list                           列出所有可用的 topics
  export <topic> [count] [dir]   导出事件
  import <file> <topic>          导入事件

参数:
  topic    - Kafka topic 名称
  count    - 导出事件数量 (默认: 1000, 使用 'all' 或 '-1' 导出全部)
  dir      - 输出目录 (默认: ./data/kafka-exports)
  file     - 输入文件路径
  topic    - 目标 topic 名称

环境变量:
  KAFKA_BROKERS - Kafka 服务器地址 (默认: ${DEFAULT_KAFKA_BROKERS})

示例:
  $0 list                                           # 列出所有 topics (使用本地 Kafka)
  KAFKA_BROKERS=49.232.13.155:9094 $0 list         # 连接远程 Kafka
  $0 export sysarmor-agentless-b1de298c             # 导出 1000 条事件
  $0 export sysarmor-agentless-b1de298c 500         # 导出 500 条事件
  $0 export sysarmor-agentless-b1de298c all         # 导出全部事件
  $0 export sysarmor-agentless-b1de298c -1          # 导出全部事件 (另一种写法)
  $0 export sysarmor-agentless-b1de298c 1000 /tmp   # 导出到指定目录
  $0 import ./data/events.jsonl sysarmor-test       # 导入事件到指定 topic

配置:
  当前服务器: ${KAFKA_BOOTSTRAP_SERVERS}
  默认输出目录: ${DEFAULT_OUTPUT_DIR}

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
            list_topics
            ;;
        export|exp)
            if [[ $# -eq 0 ]]; then
                log_error "请指定 topic 名称"
                show_help
                exit 1
            fi
            export_events "$@"
            ;;
        import|imp)
            if [[ $# -lt 2 ]]; then
                log_error "请指定输入文件和目标 topic"
                show_help
                exit 1
            fi
            import_events "$@"
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
