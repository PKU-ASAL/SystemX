#!/bin/bash

# SysArmor 数据流测试脚本 - 简洁高效版本
# 支持发送测试数据和导入JSONL数据

set -e

# 显示帮助信息
show_help() {
    cat << EOF
SysArmor 数据流测试脚本

用法: $0 [选项] [模式]

模式:
  test        发送单条测试数据 (默认)
  import      导入JSONL数据文件
  
选项:
  --file <file>     指定要导入的JSONL文件 (必需，import模式)
  --topic <topic>   指定目标topic (默认: sysarmor.raw.audit)
  --limit <num>     限制导入的消息数量 (默认: 全部)
  --help           显示此帮助信息

示例:
  $0                                           # 发送单条测试数据
  $0 import --file data/sample.jsonl          # 导入指定文件
  $0 import --file data/sample.jsonl --limit 100  # 导入前100条

EOF
}

# 解析命令行参数
MODE="test"
IMPORT_FILE=""
TARGET_TOPIC="sysarmor.raw.audit"
IMPORT_LIMIT=""

while [[ $# -gt 0 ]]; do
    case $1 in
        test|import)
            MODE="$1"
            shift
            ;;
        --file)
            IMPORT_FILE="$2"
            shift 2
            ;;
        --topic)
            TARGET_TOPIC="$2"
            shift 2
            ;;
        --limit)
            IMPORT_LIMIT="$2"
            shift 2
            ;;
        --help|-h)
            show_help
            exit 0
            ;;
        *)
            echo "未知参数: $1"
            show_help
            exit 1
            ;;
    esac
done

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 加载配置
VECTOR_HOST="localhost"
VECTOR_TCP_PORT="6000"
VECTOR_API="http://localhost:8686"
MANAGER_API="http://localhost:8080"

echo "🚀 SysArmor 数据流测试"
echo "模式: $MODE | Topic: $TARGET_TOPIC"
[[ -n "$IMPORT_FILE" ]] && echo "文件: $IMPORT_FILE"
[[ -n "$IMPORT_LIMIT" ]] && echo "限制: $IMPORT_LIMIT 条"
echo ""

# 导入模式
if [[ "$MODE" == "import" ]]; then
    # 检查文件
    if [[ -z "$IMPORT_FILE" ]]; then
        echo -e "${RED}❌ 请指定导入文件: --file <path>${NC}"
        exit 1
    fi
    
    if [[ ! -f "$IMPORT_FILE" ]]; then
        echo -e "${RED}❌ 文件不存在: $IMPORT_FILE${NC}"
        exit 1
    fi
    
    # 文件信息
    total_lines=$(wc -l < "$IMPORT_FILE")
    file_size=$(du -h "$IMPORT_FILE" | cut -f1)
    echo "📁 文件: $(basename "$IMPORT_FILE") ($total_lines 条, $file_size)"
    
    # 应用限制
    import_count=$total_lines
    if [[ -n "$IMPORT_LIMIT" ]] && [[ "$IMPORT_LIMIT" -lt "$total_lines" ]]; then
        import_count=$IMPORT_LIMIT
        echo "📊 导入限制: $import_count 条"
    fi
    
    # 预处理数据
    echo -n "🔧 预处理数据..."
    temp_file="/tmp/sysarmor-import-$(date +%s).jsonl"
    head -n "$import_count" "$IMPORT_FILE" | while IFS= read -r line; do
        # 移除旧的topic字段，确保event_type为syslog
        echo "$line" | jq -c 'del(.topic) | .event_type = "syslog"' 2>/dev/null || echo "$line"
    done > "$temp_file"
    echo -e " ${GREEN}✅${NC}"
    
    # 批量导入
    echo "📤 开始导入 $import_count 条数据..."
    success_count=0
    error_count=0
    batch_size=100  # 增大批次
    current_batch=0
    
    while IFS= read -r line; do
        current_batch=$((current_batch + 1))
        
        # 发送数据到Vector
        if printf "%s\n" "$line" | nc -w 1 ${VECTOR_HOST} ${VECTOR_TCP_PORT} 2>/dev/null; then
            success_count=$((success_count + 1))
        else
            error_count=$((error_count + 1))
        fi
        
        # 每100条显示进度
        if [[ $((current_batch % batch_size)) -eq 0 ]]; then
            progress=$((current_batch * 100 / import_count))
            echo "  进度: $progress% ($current_batch/$import_count) 成功: $success_count"
        fi
        
        if [[ $current_batch -ge $import_count ]]; then
            break
        fi
    done < "$temp_file"
    
    # 清理
    rm -f "$temp_file"
    
    echo -e "${GREEN}✅ 导入完成${NC}"
    echo "  发送: $success_count 成功, $error_count 失败"
    
    # 等待处理
    echo -n "⏳ 等待数据处理..."
    sleep 3
    echo -e " ${GREEN}✅${NC}"
    
    # 验证结果
    echo -n "📊 验证结果..."
    kafka_count=$(./scripts/kafka-tools.sh list 2>/dev/null | grep "$TARGET_TOPIC" | grep -o "消息数: [0-9]*" | cut -d' ' -f2 | tr -d '\n' || echo "0")
    api_count=$(curl -s "$MANAGER_API/api/v1/events/latest?topic=$TARGET_TOPIC&limit=1" | jq -r '.data.total' 2>/dev/null || echo "0")
    echo -e " ${GREEN}✅${NC}"
    
    echo ""
    echo -e "${BLUE}📊 导入结果${NC}"
    echo "发送成功: $success_count 条"
    echo "Kafka存储: $kafka_count 条"
    echo "API可查询: $api_count 条"
    
    if [[ "$success_count" -eq "$import_count" ]]; then
        echo -e "${GREEN}🎉 导入完全成功！${NC}"
    else
        echo -e "${YELLOW}⚠️  部分导入成功${NC}"
    fi

# 测试模式
else
    # 生成测试数据
    COLLECTOR_ID="test-$(date +%s)"
    TEST_MESSAGE='{"collector_id":"'${COLLECTOR_ID}'","timestamp":"'$(date -u +%Y-%m-%dT%H:%M:%SZ)'","host":"test-host","source":"auditd","message":"type=SYSCALL msg=audit(1693420800.123:456): arch=c000003e syscall=2 success=yes exit=3 items=1 ppid=1234 pid=5678 auid=1000 uid=0 comm=\"test\" exe=\"/bin/test\"","event_type":"syslog","severity":"info","tags":["audit","test"]}'
    
    echo "📤 发送测试数据..."
    echo "Collector ID: $COLLECTOR_ID"
    
    # 发送数据
    if printf "%s\n" "${TEST_MESSAGE}" | nc -w 3 ${VECTOR_HOST} ${VECTOR_TCP_PORT}; then
        echo -e "${GREEN}✅ 发送成功${NC}"
        
        # 等待处理
        sleep 2
        
        # 验证
        api_count=$(curl -s "$MANAGER_API/api/v1/events/latest?topic=$TARGET_TOPIC&collector_id=$COLLECTOR_ID&limit=1" | jq -r '.data.total' 2>/dev/null || echo "0")
        if [[ "$api_count" -gt 0 ]]; then
            echo -e "${GREEN}🎉 测试成功！API查询到 $api_count 条事件${NC}"
        else
            echo -e "${YELLOW}⚠️  API暂未查询到数据，请稍后重试${NC}"
        fi
    else
        echo -e "${RED}❌ 发送失败${NC}"
        exit 1
    fi
fi

echo ""
echo -e "${BLUE}💡 后续操作:${NC}"
echo "1. 查看数据: curl -s '$MANAGER_API/api/v1/events/latest?topic=$TARGET_TOPIC&limit=5' | jq ."
echo "2. 检查Kafka: ./scripts/kafka-tools.sh list"
