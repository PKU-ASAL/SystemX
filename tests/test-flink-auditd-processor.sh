#!/bin/bash

# SysArmor Flink Auditd 处理器测试脚本
# 测试 sysarmor.raw.audit → sysarmor.events.audit 数据转换

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 配置
MANAGER_API="http://localhost:8080"
FLINK_API="http://localhost:8081"
INPUT_TOPIC="sysarmor.raw.audit"
OUTPUT_TOPIC="sysarmor.events.audit"

echo "🚀 SysArmor Flink Auditd 处理器测试"
echo "=================================================="
echo "输入Topic: $INPUT_TOPIC"
echo "输出Topic: $OUTPUT_TOPIC"
echo ""

# 步骤1: 检查系统状态
echo -e "${YELLOW}🔍 步骤1: 检查系统状态${NC}"
echo "=================================================="

echo -n "Flink JobManager: "
if curl -s -f "$FLINK_API/overview" > /dev/null; then
    echo -e "${GREEN}✅ 正常${NC}"
else
    echo -e "${RED}❌ 不可用${NC}"
    exit 1
fi

echo -n "输入Topic数据: "
input_count=$(./scripts/kafka-tools.sh list 2>/dev/null | grep "$INPUT_TOPIC" | grep -o "消息数: [0-9]*" | cut -d' ' -f2 | tr -d '\n' || echo "0")
echo -e "${GREEN}$input_count 条${NC}"

echo -n "输出Topic数据: "
output_count=$(./scripts/kafka-tools.sh list 2>/dev/null | grep "$OUTPUT_TOPIC" | grep -o "消息数: [0-9]*" | cut -d' ' -f2 | tr -d '\n' || echo "0")
echo -e "${GREEN}$output_count 条${NC}"

echo ""

# 步骤2: 提交Flink作业
echo -e "${YELLOW}📤 步骤2: 提交Flink作业${NC}"
echo "=================================================="

echo "提交 Auditd Raw to Events 处理作业..."

# 进入processor目录并运行作业
cd services/processor

echo -n "启动Flink作业: "
if python3 jobs/job_auditd_raw_to_events.py > /tmp/flink-job.log 2>&1 &
then
    FLINK_PID=$!
    echo -e "${GREEN}✅ 已提交 (PID: $FLINK_PID)${NC}"
    
    # 等待作业启动
    echo -n "等待作业启动..."
    sleep 5
    echo -e " ${GREEN}✅${NC}"
    
    # 检查作业状态
    echo -n "检查作业状态: "
    job_count=$(curl -s "$FLINK_API/jobs" | jq -r '.jobs | length' 2>/dev/null || echo "0")
    if [[ "$job_count" -gt 0 ]]; then
        echo -e "${GREEN}✅ $job_count 个作业运行中${NC}"
    else
        echo -e "${YELLOW}⚠️  未检测到运行中的作业${NC}"
    fi
else
    echo -e "${RED}❌ 提交失败${NC}"
    exit 1
fi

cd ../..

echo ""

# 步骤3: 等待数据处理
echo -e "${YELLOW}⏳ 步骤3: 等待数据处理 (10秒)${NC}"
echo "=================================================="

for i in {10..1}; do
    echo -n "等待 $i 秒..."
    sleep 1
    echo -e "\r\033[K"
done
echo -e "${GREEN}✅ 等待完成${NC}"

echo ""

# 步骤4: 验证处理结果
echo -e "${YELLOW}📊 步骤4: 验证处理结果${NC}"
echo "=================================================="

echo -n "检查输出Topic: "
new_output_count=$(./scripts/kafka-tools.sh list 2>/dev/null | grep "$OUTPUT_TOPIC" | grep -o "消息数: [0-9]*" | cut -d' ' -f2 | tr -d '\n' || echo "0")
echo -e "${GREEN}$new_output_count 条${NC}"

processed_count=$((new_output_count - output_count))
echo "新处理的事件: $processed_count 条"

if [[ "$processed_count" -gt 0 ]]; then
    echo -e "${GREEN}✅ 数据处理成功${NC}"
    
    # 查看处理后的数据样例
    echo ""
    echo "📋 处理后的事件样例:"
    curl -s "$MANAGER_API/api/v1/events/latest?topic=$OUTPUT_TOPIC&limit=1" | jq -r '.data.events[0]' 2>/dev/null | jq . || echo "无法获取样例数据"
    
else
    echo -e "${YELLOW}⚠️  未检测到新的处理数据${NC}"
fi

echo ""

# 步骤5: 查看作业日志
echo -e "${YELLOW}📋 步骤5: 查看作业日志${NC}"
echo "=================================================="

echo "Flink TaskManager 日志 (最近10行):"
docker logs --tail 10 sysarmor-flink-taskmanager-1 2>/dev/null || echo "无法获取日志"

echo ""

# 步骤6: 测试总结
echo -e "${BLUE}📊 测试总结${NC}"
echo "=================================================="
echo "输入数据: $input_count 条"
echo "输出数据: $new_output_count 条"
echo "处理数据: $processed_count 条"

if [[ "$processed_count" -gt 0 ]]; then
    echo -e "${GREEN}✅ Flink处理器工作正常${NC}"
    echo -e "${GREEN}✅ 数据转换: Raw Audit → Structured Events${NC}"
else
    echo -e "${YELLOW}⚠️  处理器可能需要更多时间或调试${NC}"
fi

echo ""
echo -e "${BLUE}💡 后续操作:${NC}"
echo "1. 查看Flink作业: curl -s '$FLINK_API/jobs' | jq ."
echo "2. 查看处理后数据: curl -s '$MANAGER_API/api/v1/events/latest?topic=$OUTPUT_TOPIC&limit=5' | jq ."
echo "3. 导出处理后数据: ./scripts/kafka-tools.sh export $OUTPUT_TOPIC 10"
echo "4. 停止作业: kill $FLINK_PID"

echo ""
echo -e "${GREEN}🎉 SysArmor Flink Auditd 处理器测试完成！${NC}"
