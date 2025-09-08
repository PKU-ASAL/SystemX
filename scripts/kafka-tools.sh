#!/bin/bash

# =============================================================================
# SysArmor Kafka 工具脚本
# 提供 Kafka 事件的导出、导入和 topic 管理功能
# 支持 Docker 网络连接和批量导出
# =============================================================================

set -e

# 配置参数
DEFAULT_KAFKA_BROKERS="localhost:9094"
KAFKA_BOOTSTRAP_SERVERS="${KAFKA_BROKERS:-$DEFAULT_KAFKA_BROKERS}"
DEFAULT_OUTPUT_DIR="./data/kafka-exports"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")

# Docker 网络配置
DOCKER_NETWORK="${DOCKER_NETWORK:-}"
KAFKA_CONTAINER_NAME="${KAFKA_CONTAINER_NAME:-sysarmor-kafka-1}"

# 批量导出配置
DEFAULT_BATCH_SIZE=1000000  # 默认每批100万条
LOG_FILE="$HOME/kafka-export-${TIMESTAMP}.log"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 日志函数
log_info() {
    local msg="[$(date '+%Y-%m-%d %H:%M:%S')] [INFO] $1"
    echo -e "${BLUE}${msg}${NC}"
    if [[ -n "$LOG_FILE" ]]; then
        echo "$msg" >> "$LOG_FILE"
    fi
}

log_success() {
    local msg="[$(date '+%Y-%m-%d %H:%M:%S')] [SUCCESS] $1"
    echo -e "${GREEN}${msg}${NC}"
    if [[ -n "$LOG_FILE" ]]; then
        echo "$msg" >> "$LOG_FILE"
    fi
}

log_warning() {
    local msg="[$(date '+%Y-%m-%d %H:%M:%S')] [WARNING] $1"
    echo -e "${YELLOW}${msg}${NC}"
    if [[ -n "$LOG_FILE" ]]; then
        echo "$msg" >> "$LOG_FILE"
    fi
}

log_error() {
    local msg="[$(date '+%Y-%m-%d %H:%M:%S')] [ERROR] $1"
    echo -e "${RED}${msg}${NC}"
    if [[ -n "$LOG_FILE" ]]; then
        echo "$msg" >> "$LOG_FILE"
    fi
}

# 解析命令行参数
parse_args() {
    OUTPUT_DIR="$DEFAULT_OUTPUT_DIR"
    BATCH_SIZE="$DEFAULT_BATCH_SIZE"
    COLLECTOR_ID=""
    
    while [[ $# -gt 0 ]]; do
        case $1 in
            --out)
                OUTPUT_DIR="$2"
                shift 2
                ;;
            --batch-size)
                BATCH_SIZE="$2"
                shift 2
                ;;
            --collector-id)
                COLLECTOR_ID="$2"
                shift 2
                ;;
            *)
                # 保留其他参数
                REMAINING_ARGS+=("$1")
                shift
                ;;
        esac
    done
}

# 检查依赖
check_dependencies() {
    if ! command -v docker &> /dev/null; then
        log_error "Docker 未安装或不在 PATH 中"
        exit 1
    fi
    
    # 检查 Docker 网络是否存在
    if [[ -n "$DOCKER_NETWORK" ]] && ! docker network ls | grep -q "$DOCKER_NETWORK"; then
        log_warning "Docker 网络 '$DOCKER_NETWORK' 不存在，将使用默认连接"
        DOCKER_NETWORK=""
    fi
}

# 执行 Kafka 命令
run_kafka_command() {
    local cmd="$1"
    local network_param=""
    local kafka_server=""
    
    if [[ -n "$DOCKER_NETWORK" ]]; then
        network_param="--network $DOCKER_NETWORK"
        kafka_server="$KAFKA_CONTAINER_NAME:9092"
    else
        kafka_server="$KAFKA_BOOTSTRAP_SERVERS"
    fi
    
    docker run --rm $network_param confluentinc/cp-kafka:7.4.0 \
        $cmd --bootstrap-server "$kafka_server"
}

# 检查 Kafka 连接
check_kafka_connection() {
    run_kafka_command "kafka-topics --list" &>/dev/null || {
        log_error "无法连接到 Kafka 服务器"
        exit 1
    }
}

# 获取 Kafka topics
get_kafka_topics() {
    run_kafka_command "kafka-topics --list" 2>/dev/null
}

# 获取 topic 消息数量
get_topic_message_count() {
    local topic="$1"
    local network_param=""
    local kafka_server=""
    
    if [[ -n "$DOCKER_NETWORK" ]]; then
        network_param="--network $DOCKER_NETWORK"
        kafka_server="$KAFKA_CONTAINER_NAME:9092"
    else
        kafka_server="$KAFKA_BOOTSTRAP_SERVERS"
    fi
    
    local offset_info=$(docker run --rm $network_param confluentinc/cp-kafka:7.4.0 \
        kafka-run-class kafka.tools.GetOffsetShell \
        --broker-list "$kafka_server" \
        --topic "$topic" \
        --time -1 2>/dev/null | awk -F: '{sum+=$3} END {print sum}')
    
    echo "${offset_info:-0}"
}

# 列出 topics
list_topics() {
    echo "=============================================="
    echo "SysArmor Kafka Topics"
    echo "=============================================="
    if [[ -n "$DOCKER_NETWORK" ]]; then
        echo "网络: $DOCKER_NETWORK"
        echo "服务器: $KAFKA_CONTAINER_NAME:9092"
    else
        echo "服务器: $KAFKA_BOOTSTRAP_SERVERS"
    fi
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
            local message_count=$(get_topic_message_count "$topic")
            echo -e "  ${GREEN}★${NC} $topic (消息数: $message_count)"
        else
            echo "  - $topic"
        fi
    done
    echo "=============================================="
}

# 导出事件（支持批量和过滤）
export_events() {
    local topic="$1"
    local count="${2:-1000}"
    
    # 检查是否导出全部数据
    local export_all=false
    if [[ "$count" == "all" || "$count" == "ALL" || "$count" == "-1" ]]; then
        export_all=true
        count="全部"
    fi
    
    echo "=============================================="
    echo "SysArmor Kafka 数据导出"
    echo "=============================================="
    echo "Topic: $topic"
    echo "数量: $count"
    echo "输出目录: $OUTPUT_DIR"
    echo "批次大小: $BATCH_SIZE"
    if [[ -n "$COLLECTOR_ID" ]]; then
        echo "Collector 过滤: $COLLECTOR_ID"
    fi
    if [[ -n "$DOCKER_NETWORK" ]]; then
        echo "网络: $DOCKER_NETWORK"
        echo "服务器: $KAFKA_CONTAINER_NAME:9092"
    else
        echo "服务器: $KAFKA_BOOTSTRAP_SERVERS"
    fi
    echo "日志文件: $LOG_FILE"
    echo "=============================================="
    
    check_dependencies
    check_kafka_connection
    
    # 创建输出目录
    mkdir -p "$OUTPUT_DIR"
    
    # 初始化日志文件
    echo "SysArmor Kafka 数据导出开始 - $(date)" > "$LOG_FILE"
    
    # 创建 topic 专用目录
    local topic_suffix=""
    if [[ -n "$COLLECTOR_ID" ]]; then
        topic_suffix="_${COLLECTOR_ID}"
    fi
    local topic_dir="$OUTPUT_DIR/${topic}${topic_suffix}_${TIMESTAMP}"
    mkdir -p "$topic_dir"
    
    # 获取总消息数
    local total_messages=$(get_topic_message_count "$topic")
    log_info "Topic $topic 总消息数: $total_messages"
    
    if [[ "$total_messages" -eq 0 ]]; then
        log_warning "Topic $topic 无消息，跳过"
        return 0
    fi
    
    local network_param=""
    local kafka_server=""
    
    if [[ -n "$DOCKER_NETWORK" ]]; then
        network_param="--network $DOCKER_NETWORK"
        kafka_server="$KAFKA_CONTAINER_NAME:9092"
    else
        kafka_server="$KAFKA_BOOTSTRAP_SERVERS"
    fi
    
    # 判断是否需要批量导出
    if [[ "$export_all" == "true" && "$total_messages" -gt "$BATCH_SIZE" ]]; then
        # 批量导出模式
        log_info "数据量较大，启用批量导出模式"
        export_in_batches "$topic" "$topic_dir" "$total_messages" "$network_param" "$kafka_server"
    else
        # 标准导出模式
        export_standard "$topic" "$count" "$topic_dir" "$network_param" "$kafka_server"
    fi
    
    # 创建汇总信息
    create_export_summary "$topic" "$topic_dir"
    
    log_success "=== 数据导出完成 ==="
    log_info "输出目录: $topic_dir"
    log_info "日志文件: $LOG_FILE"
}

# 批量导出模式
export_in_batches() {
    local topic="$1"
    local topic_dir="$2"
    local total_messages="$3"
    local network_param="$4"
    local kafka_server="$5"
    
    # 计算需要的批次数
    local total_batches=$(( (total_messages + BATCH_SIZE - 1) / BATCH_SIZE ))
    log_info "计划分 $total_batches 个批次导出，每批 $BATCH_SIZE 条消息"
    
    # 使用 consumer group 确保连续读取
    local consumer_group="sysarmor-export-${topic}-${TIMESTAMP}-$$"
    
    local batch_num=1
    local total_exported=0
    
    while [[ $batch_num -le $total_batches ]]; do
        local batch_file="$topic_dir/${topic}_batch_${batch_num}_${TIMESTAMP}.jsonl"
        local temp_file="$topic_dir/.temp_batch_${batch_num}.jsonl"
        
        # 计算当前批次应该导出的消息数量
        local remaining_messages=$(( total_messages - total_exported ))
        local current_batch_size=$BATCH_SIZE
        
        # 如果是最后一批次，调整批次大小为剩余消息数量
        if [[ $batch_num -eq $total_batches ]] && [[ $remaining_messages -lt $BATCH_SIZE ]]; then
            current_batch_size=$remaining_messages
            log_info "最后批次，调整批次大小为: $current_batch_size"
        fi
        
        log_info "导出批次 $batch_num/$total_batches (预期: $current_batch_size 条消息)..."
        
        # 使用 consumer group 进行批量导出
        local from_beginning_flag=""
        if [[ $batch_num -eq 1 ]]; then
            from_beginning_flag="--from-beginning"
        fi
        
        docker run --rm $network_param confluentinc/cp-kafka:7.4.0 \
            kafka-console-consumer \
            --bootstrap-server "$kafka_server" \
            --topic "$topic" \
            --group "$consumer_group" \
            $from_beginning_flag \
            --max-messages $current_batch_size \
            --timeout-ms 30000 > "$temp_file" 2>/dev/null || {
            log_warning "批次 $batch_num 导出完成或超时"
        }
        
        # 检查是否有数据
        if [[ ! -f "$temp_file" || ! -s "$temp_file" ]]; then
            log_info "批次 $batch_num 无数据，导出完成"
            rm -f "$temp_file"
            break
        fi
        
        # 如果指定了 collector_id，进行过滤
        if [[ -n "$COLLECTOR_ID" ]]; then
            local filtered_file="$topic_dir/.filtered_batch_${batch_num}.jsonl"
            
            if command -v jq &> /dev/null; then
                jq -c "select(.collector_id | contains(\"$COLLECTOR_ID\"))" "$temp_file" > "$filtered_file" 2>/dev/null || {
                    grep "$COLLECTOR_ID" "$temp_file" > "$filtered_file" 2>/dev/null || true
                }
            else
                grep "$COLLECTOR_ID" "$temp_file" > "$filtered_file" 2>/dev/null || true
            fi
            
            if [[ -f "$filtered_file" && -s "$filtered_file" ]]; then
                mv "$filtered_file" "$batch_file"
                rm -f "$temp_file"
            else
                log_info "批次 $batch_num 无匹配的 collector 数据"
                rm -f "$temp_file" "$filtered_file"
                batch_num=$((batch_num + 1))
                continue
            fi
        else
            mv "$temp_file" "$batch_file"
        fi
        
        # 统计当前批次
        local batch_count=$(wc -l < "$batch_file")
        total_exported=$((total_exported + batch_count))
        
        log_success "批次 $batch_num 完成: $batch_count 条记录"
        log_info "累计导出: $total_exported 条记录"
        
        # 计算进度
        local progress=$(( batch_num * 100 / total_batches ))
        log_info "导出进度: $progress% ($batch_num/$total_batches)"
        
        # 如果当前批次少于批次大小，说明已经导出完毕
        if [[ $batch_count -lt $BATCH_SIZE ]]; then
            log_info "最后一批数据导出完成"
            break
        fi
        
        batch_num=$((batch_num + 1))
        
        # 短暂休息，避免过度占用资源
        sleep 2
    done
    
    # 清理 consumer group
    docker run --rm $network_param confluentinc/cp-kafka:7.4.0 \
        kafka-consumer-groups \
        --bootstrap-server "$kafka_server" \
        --delete --group "$consumer_group" 2>/dev/null || {
        log_warning "Consumer Group 清理失败或已不存在"
    }
    
    log_success "批量导出完成: $total_exported 条记录，$((batch_num - 1)) 个批次"
}

# 标准导出模式
export_standard() {
    local topic="$1"
    local count="$2"
    local topic_dir="$3"
    local network_param="$4"
    local kafka_server="$5"
    
    local output_file="$topic_dir/${topic}_${TIMESTAMP}.jsonl"
    
    local max_messages_param=""
    if [[ "$count" != "全部" ]]; then
        max_messages_param="--max-messages $count"
    fi
    
    log_info "标准导出模式: 从 topic '$topic' 导出 $count 条事件..."
    
    docker run --rm $network_param confluentinc/cp-kafka:7.4.0 \
        kafka-console-consumer \
        --bootstrap-server "$kafka_server" \
        --topic "$topic" \
        --from-beginning \
        $max_messages_param \
        --timeout-ms 60000 > "$output_file" 2>/dev/null || {
        log_error "导出失败"
        rm -f "$output_file"
        exit 1
    }
    
    # 如果指定了 collector_id，进行过滤
    if [[ -n "$COLLECTOR_ID" ]]; then
        local filtered_file="$topic_dir/${topic}_filtered_${TIMESTAMP}.jsonl"
        
        log_info "过滤 collector_id: $COLLECTOR_ID"
        
        if command -v jq &> /dev/null; then
            jq -c "select(.collector_id | contains(\"$COLLECTOR_ID\"))" "$output_file" > "$filtered_file" 2>/dev/null || {
                grep "$COLLECTOR_ID" "$output_file" > "$filtered_file" 2>/dev/null || true
            }
        else
            grep "$COLLECTOR_ID" "$output_file" > "$filtered_file" 2>/dev/null || true
        fi
        
        if [[ -f "$filtered_file" && -s "$filtered_file" ]]; then
            rm -f "$output_file"
            mv "$filtered_file" "$output_file"
            log_success "过滤完成"
        else
            log_warning "未找到匹配的 collector 数据"
        fi
    fi
    
    # 检查结果
    if [[ -f "$output_file" && -s "$output_file" ]]; then
        local line_count=$(wc -l < "$output_file")
        local file_size=$(du -h "$output_file" | cut -f1)
        log_success "成功导出 $line_count 条事件 ($file_size)"
    else
        log_error "导出失败或文件为空"
        exit 1
    fi
}

# 创建导出汇总
create_export_summary() {
    local topic="$1"
    local topic_dir="$2"
    
    local summary_file="$topic_dir/export_summary.txt"
    
    # 统计所有导出文件
    local total_files=$(ls -1 "$topic_dir"/*.jsonl 2>/dev/null | wc -l || echo "0")
    local total_records=0
    local total_size="0B"
    
    if [[ $total_files -gt 0 ]]; then
        total_records=$(find "$topic_dir" -name "*.jsonl" -exec wc -l {} + 2>/dev/null | tail -1 | awk '{print $1}' || echo "0")
        total_size=$(du -sh "$topic_dir" 2>/dev/null | cut -f1 || echo "0B")
    fi
    
    cat > "$summary_file" << EOF
SysArmor Kafka 数据导出汇总
===========================
Topic: $topic
Collector ID: ${COLLECTOR_ID:-所有}
导出时间: $(date)
导出目录: $topic_dir
批次大小: $BATCH_SIZE
总文件数: $total_files
总记录数: $total_records
总文件大小: $total_size

导出文件:
$(ls -la "$topic_dir"/*.jsonl 2>/dev/null | awk '{print "  " $9 ": " $5 " bytes (" int($5/1024/1024) " MB)"}' || echo "  无数据文件")

配置信息:
  Docker 网络: ${DOCKER_NETWORK:-直连}
  Kafka 服务器: ${KAFKA_CONTAINER_NAME:-$KAFKA_BOOTSTRAP_SERVERS}
  日志文件: $LOG_FILE
EOF
    
    log_success "汇总文件创建: $summary_file"
}

# 监控导出进度
monitor_export_progress() {
    local log_file="${1:-$LOG_FILE}"
    
    if [[ ! -f "$log_file" ]]; then
        # 尝试找到最新的日志文件
        log_file=$(ls -t ~/kafka-export-*.log 2>/dev/null | head -1)
        if [[ -z "$log_file" ]]; then
            log_error "未找到日志文件"
            exit 1
        fi
        log_info "使用日志文件: $log_file"
    fi
    
    echo "=============================================="
    echo "SysArmor Kafka 导出进度监控"
    echo "=============================================="
    echo "日志文件: $log_file"
    echo "=============================================="
    
    # 实时监控日志
    tail -f "$log_file" | while IFS= read -r line; do
        if [[ "$line" =~ "导出进度:" ]]; then
            echo -e "${GREEN}$line${NC}"
        elif [[ "$line" =~ "SUCCESS" ]]; then
            echo -e "${GREEN}$line${NC}"
        elif [[ "$line" =~ "WARNING" ]]; then
            echo -e "${YELLOW}$line${NC}"
        elif [[ "$line" =~ "ERROR" ]]; then
            echo -e "${RED}$line${NC}"
        else
            echo "$line"
        fi
    done
}

# 显示导出状态
show_export_status() {
    echo "=============================================="
    echo "SysArmor Kafka 导出状态"
    echo "=============================================="
    
    # 检查正在运行的导出进程
    local running_exports=$(ps aux | grep -E "(kafka-tools|kafka-console-consumer)" | grep -v grep | wc -l)
    echo "正在运行的导出进程: $running_exports"
    
    if [[ $running_exports -gt 0 ]]; then
        echo ""
        echo "活跃进程:"
        ps aux | grep -E "(kafka-tools|kafka-console-consumer)" | grep -v grep | awk '{print "  PID " $2 ": " $11 " " $12 " " $13}'
    fi
    
    # 显示最近的导出目录
    echo ""
    echo "最近的导出目录:"
    find "$DEFAULT_OUTPUT_DIR" -maxdepth 1 -type d -name "*_*" 2>/dev/null | sort -r | head -5 | while read dir; do
        local size=$(du -sh "$dir" 2>/dev/null | cut -f1 || echo "0B")
        echo "  $(basename "$dir"): $size"
    done || echo "  无导出目录"
    
    # 显示最近的日志文件
    echo ""
    echo "最近的日志文件:"
    ls -lt ~/kafka-export-*.log 2>/dev/null | head -3 | awk '{print "  " $9 ": " $5 " bytes"}' || echo "  无日志文件"
    
    echo "=============================================="
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
    
    local network_param=""
    local kafka_server=""
    
    if [[ -n "$DOCKER_NETWORK" ]]; then
        network_param="--network $DOCKER_NETWORK"
        kafka_server="$KAFKA_CONTAINER_NAME:9092"
    else
        kafka_server="$KAFKA_BOOTSTRAP_SERVERS"
    fi
    
    # 检查 topic 是否存在，不存在则创建
    if ! docker run --rm $network_param confluentinc/cp-kafka:7.4.0 \
        kafka-topics --bootstrap-server "$kafka_server" \
        --describe --topic "$topic" &>/dev/null; then
        log_info "创建 topic '$topic'..."
        docker run --rm $network_param confluentinc/cp-kafka:7.4.0 \
            kafka-topics --bootstrap-server "$kafka_server" \
            --create --topic "$topic" \
            --partitions 3 --replication-factor 1 || {
            log_error "创建 topic 失败"
            exit 1
        }
        log_success "Topic '$topic' 创建成功"
    fi
    
    # 导入数据
    docker run --rm -i $network_param confluentinc/cp-kafka:7.4.0 \
        kafka-console-producer \
        --bootstrap-server "$kafka_server" \
        --topic "$topic" < "$input_file" || {
        log_error "导入失败"
        exit 1
    }
    
    log_success "成功导入 $line_count 条事件到 topic '$topic'"
}

# 显示帮助信息
show_help() {
    cat << EOF
SysArmor Kafka 工具 (增强版)

用法: $0 <命令> <topic> [count] [选项]

命令:
  list                           列出所有可用的 topics
  export <topic> [count]         导出事件 (支持批量和过滤)
  import <file> <topic>          导入事件
  monitor [log_file]             监控导出进度
  status                         显示导出状态

参数:
  topic    - Kafka topic 名称
  count    - 导出事件数量 (默认: 1000, 使用 'all' 导出全部)
  file     - 输入文件路径

选项:
  --out <dir>                    输出目录 (默认: ./data/kafka-exports)
  --batch-size <size>            批次大小 (默认: 1000000, 仅在导出全部数据时生效)
  --collector-id <id>            过滤特定 collector 的数据

环境变量:
  KAFKA_BROKERS         - Kafka 服务器地址 (默认: ${DEFAULT_KAFKA_BROKERS})
  DOCKER_NETWORK        - Docker 网络名称 (默认: 未设置，使用直连)
  KAFKA_CONTAINER_NAME  - Kafka 容器名称 (默认: sysarmor-kafka-1)

示例:
  # 基础操作
  $0 list                                                    # 列出所有 topics
  $0 export sysarmor-agentless-b1de298c                      # 导出 1000 条事件
  $0 export sysarmor-agentless-b1de298c 500                  # 导出 500 条事件
  $0 export sysarmor-agentless-b1de298c all                  # 导出全部事件
  
  # 使用选项
  $0 export sysarmor-agentless-b1de298c all --out ~/exports  # 指定输出目录
  $0 export sysarmor-agentless-b1de298c all --batch-size 500000  # 指定批次大小
  $0 export sysarmor-agentless-b1de298c all --collector-id b1de298c  # 过滤特定 collector
  
  # 组合使用
  $0 export sysarmor-agentless-b1de298c all --out ~/exports --batch-size 500000 --collector-id b1de298c
  
  # Docker 网络模式
  DOCKER_NETWORK=sysarmor-net $0 list                        # 使用 Docker 网络
  DOCKER_NETWORK=sysarmor-net $0 export sysarmor-agentless-b1de298c all --out ~/exports
  
  # 后台运行和监控
  nohup $0 export sysarmor-agentless-b1de298c all --collector-id b1de298c > export.out 2>&1 &
  $0 monitor                                                 # 监控最新的导出进度
  $0 status                                                  # 查看导出状态

配置:
  当前服务器: ${KAFKA_BOOTSTRAP_SERVERS}
  Docker 网络: ${DOCKER_NETWORK:-未设置}
  Kafka 容器: ${KAFKA_CONTAINER_NAME}
  默认输出目录: ${DEFAULT_OUTPUT_DIR}
  默认批次大小: ${DEFAULT_BATCH_SIZE}

注意:
  - 当导出全部数据且数据量 > batch_size 时，自动启用批量导出模式
  - 批量导出会创建多个文件: topic_batch_1.jsonl, topic_batch_2.jsonl, ...
  - 使用 --collector-id 可以过滤特定 collector 的数据
  - 支持 Docker 网络连接，适用于容器化部署

EOF
}

# 主函数
main() {
    # 解析参数
    REMAINING_ARGS=()
    parse_args "$@"
    set -- "${REMAINING_ARGS[@]}"
    
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
        monitor|mon)
            monitor_export_progress "$@"
            ;;
        status|stat)
            show_export_status
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
