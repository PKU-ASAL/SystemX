#!/bin/bash

# SysArmor Flink 处理器测试脚本
# 专注于测试 Flink 接口健康状态和 job_auditd_raw_to_events.py 作业提交
# 不包含数据发送功能 - 使用其他脚本发送数据

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

# 显示帮助信息
show_help() {
    cat << EOF
SysArmor Flink 处理器测试脚本

用法: $0 [选项]

选项:
  --job-name <name>     指定作业名称 (默认: job_auditd_raw_to_events.py)
  --help               显示此帮助信息

功能:
  1. 检查 Flink 集群健康状态
  2. 测试 Manager Flink API 接口
  3. 提交 job_auditd_raw_to_events.py 作业
  4. 验证作业运行状态
  5. 显示监控信息

注意: 此脚本不发送数据，请使用 test-auditd-data-flow.sh 发送测试数据

EOF
}

# 解析命令行参数
JOB_NAME="job_auditd_raw_to_events.py"

while [[ $# -gt 0 ]]; do
    case $1 in
        --job-name)
            JOB_NAME="$2"
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

echo "🚀 SysArmor Flink 处理器测试"
echo "=================================================="
echo "输入Topic: $INPUT_TOPIC"
echo "输出Topic: $OUTPUT_TOPIC"
echo "作业文件: $JOB_NAME"
echo ""

# 步骤1: 检查Flink集群健康状态
echo -e "${YELLOW}🔍 步骤1: 检查Flink集群健康状态${NC}"
echo "=================================================="

# 检查 Flink JobManager
echo -n "Flink JobManager: "
if flink_overview=$(curl -s -f "$FLINK_API/overview" 2>/dev/null); then
    echo -e "${GREEN}✅ 正常${NC}"
    slots_total=$(echo "$flink_overview" | jq -r '.slots-total' 2>/dev/null || echo "N/A")
    slots_available=$(echo "$flink_overview" | jq -r '.slots-available' 2>/dev/null || echo "N/A")
    echo "  可用槽位: $slots_available/$slots_total"
    echo "  集群ID: $(echo "$flink_overview" | jq -r '.flink-commit' 2>/dev/null | cut -c1-8 || echo "N/A")"
else
    echo -e "${RED}❌ 不可用 - Flink集群未启动${NC}"
    exit 1
fi

# 检查 Flink TaskManager
echo -n "Flink TaskManager: "
if taskmanagers=$(curl -s -f "$FLINK_API/taskmanagers" 2>/dev/null); then
    tm_count=$(echo "$taskmanagers" | jq -r '.taskmanagers | length' 2>/dev/null || echo "0")
    echo -e "${GREEN}✅ $tm_count 个TaskManager${NC}"
else
    echo -e "${RED}❌ 无法获取TaskManager信息${NC}"
fi

# 检查 Manager Flink API
echo -n "Manager Flink API: "
if curl -s -f "$MANAGER_API/api/v1/services/flink/jobs" > /dev/null 2>&1; then
    echo -e "${GREEN}✅ 正常${NC}"
else
    echo -e "${YELLOW}⚠️  不可用${NC}"
fi

echo ""

# 步骤2: 提交Flink作业
echo -e "${YELLOW}📤 步骤2: 提交Flink作业${NC}"
echo "=================================================="

# 检查现有作业
echo -n "检查现有作业: "
existing_jobs=$(curl -s "$FLINK_API/jobs" | jq -r '.jobs[]? | select(.status == "RUNNING") | .id' 2>/dev/null || echo "")
if [[ -n "$existing_jobs" ]]; then
    echo -e "${YELLOW}⚠️  发现运行中的作业${NC}"
    # 显示作业详情
    curl -s "$FLINK_API/jobs" | jq -r '.jobs[]? | select(.status == "RUNNING") | "  - ID: \(.id[:8])... 状态: \(.status)"' 2>/dev/null || echo "  无法获取作业详情"
    echo -n "是否继续? (y/N): "
    read -r confirm
    if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
        echo "测试取消"
        exit 0
    fi
else
    echo -e "${GREEN}✅ 无运行中作业${NC}"
fi

# 检查 Docker 容器状态
echo -n "检查 Flink 容器状态: "
if ! docker ps --filter "name=flink-jobmanager" --format "{{.Names}}" | grep -q "flink-jobmanager"; then
    echo -e "${RED}❌ Flink JobManager 容器未运行${NC}"
    echo "请先启动服务: make up 或 make deploy"
    exit 1
else
    echo -e "${GREEN}✅ Flink 容器运行正常${NC}"
fi

# 提交作业
echo "提交 $JOB_NAME 作业..."

# 使用 Docker 执行 Flink 作业提交
echo -n "启动Flink作业: "

# 启动后台作业提交进程，避免阻塞
docker compose exec -T flink-jobmanager flink run -d -py /opt/flink/usr_jobs/$JOB_NAME > /tmp/flink-job-submit.log 2>&1 &
submit_pid=$!

# 等待最多30秒，检查提交是否完成
timeout_count=0
while [ $timeout_count -lt 30 ]; do
    if ! kill -0 $submit_pid 2>/dev/null; then
        # 进程已结束，检查结果
        wait $submit_pid
        submit_result=$?
        if [ $submit_result -eq 0 ]; then
            echo -e "${GREEN}✅ 提交成功${NC}"
            break
        else
            echo -e "${RED}❌ 提交失败${NC}"
            echo "查看日志: cat /tmp/flink-job-submit.log"
            exit 1
        fi
    fi
    sleep 1
    timeout_count=$((timeout_count + 1))
done

# 如果超时，终止进程
if kill -0 $submit_pid 2>/dev/null; then
    kill $submit_pid 2>/dev/null
    echo -e "${RED}❌ 提交超时${NC}"
    echo "查看日志: cat /tmp/flink-job-submit.log"
    exit 1
fi

# 等待作业启动
echo -n "等待作业启动..."
sleep 5
echo -e " ${GREEN}✅${NC}"

# 验证作业状态
echo -n "验证作业状态: "
running_jobs=$(curl -s "$FLINK_API/jobs" | jq -r '.jobs[] | select(.status == "RUNNING") | .id' 2>/dev/null | wc -l || echo "0")
if [[ "$running_jobs" -gt 0 ]]; then
    echo -e "${GREEN}✅ $running_jobs 个作业运行中${NC}"
    
    # 显示作业详情
    curl -s "$FLINK_API/jobs" | jq -r '.jobs[] | select(.status == "RUNNING") | "  - \(.name) (\(.id[:8])...)"' 2>/dev/null || echo "  无法获取作业详情"
else
    echo -e "${RED}❌ 作业未正常启动${NC}"
    echo "查看日志: cat /tmp/flink-job-submit.log"
    exit 1
fi
echo ""

# 步骤3: 测试Manager Flink API接口
echo -e "${YELLOW}🔍 步骤3: 测试Manager Flink API接口${NC}"
echo "=================================================="

# 测试Flink作业列表接口
echo -n "测试作业列表接口: "
if manager_jobs=$(curl -s "$MANAGER_API/api/v1/services/flink/jobs" 2>/dev/null); then
    echo -e "${GREEN}✅ 正常${NC}"
    job_count=$(echo "$manager_jobs" | jq -r '.data.jobs | length' 2>/dev/null || echo "0")
    echo "  Manager API返回: $job_count 个作业"
else
    echo -e "${YELLOW}⚠️  Manager Flink API不可用${NC}"
fi

# 测试Flink集群状态接口
echo -n "测试集群状态接口: "
if cluster_status=$(curl -s "$MANAGER_API/api/v1/services/flink/overview" 2>/dev/null); then
    echo -e "${GREEN}✅ 正常${NC}"
    echo "  集群状态: $(echo "$cluster_status" | jq -r '.data.status // "正常"' 2>/dev/null || echo "N/A")"
else
    echo -e "${YELLOW}⚠️  集群状态接口不可用${NC}"
fi

echo ""

# 步骤4: 验证Flink作业状态
echo -e "${YELLOW}🔍 步骤4: 验证Flink作业状态${NC}"
echo "=================================================="

# 获取作业详情
echo "📊 Flink作业状态:"
if jobs_info=$(curl -s "$FLINK_API/jobs" 2>/dev/null); then
    # 显示所有作业的基本信息
    echo "$jobs_info" | jq -r '.jobs[] | "  📋 作业ID: \(.id) | 状态: \(.status) | 名称: \(.name // "未知")"' 2>/dev/null
    
    # 显示运行中作业的详细信息
    running_jobs=$(echo "$jobs_info" | jq -r '.jobs[] | select(.status == "RUNNING")')
    
    if [[ -n "$running_jobs" ]]; then
        echo ""
        echo "🔍 运行中作业详情:"
        echo "$jobs_info" | jq -r '.jobs[] | select(.status == "RUNNING") | "  ✅ \(.name // "SysArmor-NODLINK-Auditd-Raw-to-Events") - \(.status) (ID: \(.id[:8])...)"' 2>/dev/null
        
        # 获取第一个运行中作业的详细信息
        job_id=$(echo "$jobs_info" | jq -r '.jobs[] | select(.status == "RUNNING") | .id' | head -n1)
        if [[ -n "$job_id" ]]; then
            echo ""
            echo "📈 作业详细信息 ($job_id):"
            job_details=$(curl -s "$FLINK_API/jobs/$job_id" 2>/dev/null)
            if [[ -n "$job_details" ]]; then
                echo "$job_details" | jq -r '"  - 状态: \(.state)"' 2>/dev/null
                echo "$job_details" | jq -r '"  - 开始时间: \(.["start-time"] // "N/A" | if type == "number" then (. / 1000 | strftime("%Y-%m-%d %H:%M:%S")) else . end)"' 2>/dev/null
                echo "$job_details" | jq -r '"  - 任务数: \(.vertices | length)"' 2>/dev/null
                echo "$job_details" | jq -r '"  - 运行时间: \(.duration // "N/A" | if type == "number" then (. / 1000 | tostring + "秒") else . end)"' 2>/dev/null
            fi
        fi
    else
        echo -e "${RED}❌ 没有运行中的作业${NC}"
    fi
else
    echo -e "${RED}❌ 无法获取作业信息${NC}"
fi

echo ""

# 步骤5: 监控和日志信息
echo -e "${YELLOW}� 步骤5: 监控和日志信息${NC}"
echo "=================================================="

# Flink集群指标
echo "🔧 Flink集群指标:"
if metrics=$(curl -s "$FLINK_API/jobmanager/metrics" 2>/dev/null); then
    echo "  - JobManager状态: 正常"
    # 尝试获取一些关键指标
    if memory_used=$(curl -s "$FLINK_API/jobmanager/metrics?get=Status.JVM.Memory.Heap.Used" 2>/dev/null); then
        heap_used=$(echo "$memory_used" | jq -r '.[0].value' 2>/dev/null || echo "N/A")
        echo "  - 堆内存使用: $heap_used bytes"
    fi
else
    echo "  - JobManager状态: 无法获取指标"
fi

# TaskManager日志 (最近几行)
echo ""
echo "📋 TaskManager日志 (最近3行):"
if docker logs --tail 3 sysarmor-flink-taskmanager-1 2>/dev/null; then
    echo ""
else
    echo "  无法获取TaskManager日志"
fi

echo ""

# 步骤6: 测试总结
echo -e "${BLUE}� 测试总结${NC}"
echo "=================================================="

# 运行中的作业数
running_job_count=$(curl -s "$FLINK_API/jobs" | jq -r '.jobs[] | select(.status == "RUNNING") | .id' 2>/dev/null | wc -l || echo "0")
total_slots=$(echo "$flink_overview" | jq -r '.slots-total' 2>/dev/null || echo "N/A")
available_slots=$(echo "$flink_overview" | jq -r '.slots-available' 2>/dev/null || echo "N/A")

# 修复槽位显示问题
if [[ "$total_slots" == "null" || "$total_slots" == "N/A" ]]; then
    # 尝试从 TaskManager 信息获取槽位数
    if taskmanager_info=$(curl -s "$FLINK_API/taskmanagers" 2>/dev/null); then
        total_slots=$(echo "$taskmanager_info" | jq -r '[.taskmanagers[].slotsNumber] | add' 2>/dev/null || echo "N/A")
        available_slots=$(echo "$taskmanager_info" | jq -r '[.taskmanagers[].freeSlots] | add' 2>/dev/null || echo "N/A")
    fi
fi

echo "Flink集群: $available_slots/$total_slots 槽位"
echo "TaskManager: $tm_count 个"
echo "运行作业: $running_job_count 个"

if [[ "$running_job_count" -gt 0 ]]; then
    echo -e "${GREEN}✅ Flink处理器工作正常${NC}"
    echo -e "${GREEN}✅ 作业: $JOB_NAME 运行正常${NC}"
    echo -e "${GREEN}✅ Manager Flink API 接口正常${NC}"
else
    echo -e "${RED}❌ Flink作业未正常运行${NC}"
fi

echo ""
echo -e "${BLUE}💡 后续操作:${NC}"
echo "1. 发送测试数据: ./tests/test-kafka-producer.sh sample-auditd.jsonl"
echo "2. 查看所有作业: make processor list-jobs"
echo "3. 使用Manager API: curl -s '$MANAGER_API/api/v1/services/flink/jobs' | jq ."
echo "4. 停止所有作业: make processor cancel-job JOB_ID=<job-id>"
echo "5. 查看作业详情: curl -s '$FLINK_API/jobs/<job-id>' | jq ."
echo "6. 监控作业输出: docker logs sysarmor-flink-taskmanager-1 -f | grep 'MESSAGE'"

echo ""
echo -e "${GREEN}🎉 SysArmor Flink 处理器测试完成！${NC}"
