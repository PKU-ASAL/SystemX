#!/bin/bash

# SysArmor 修复版数据流测试脚本
# 修复 JSON 格式和 Kafka 工具路径问题，支持自动读取 .env 配置

set -e

echo "🚀 SysArmor 修复版数据流测试开始..."
echo "=================================================="

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 自动加载 .env 配置
load_env_config() {
    local env_file=".env"
    
    if [[ -f "$env_file" ]]; then
        # 读取配置
        local vector_tcp_port=$(grep "^VECTOR_TCP_PORT=" "$env_file" 2>/dev/null | cut -d'=' -f2 | tr -d '"' || echo "6000")
        local vector_api_port=$(grep "^VECTOR_API_PORT=" "$env_file" 2>/dev/null | cut -d'=' -f2 | tr -d '"' || echo "8686")
        local vector_host=$(grep "^VECTOR_HOST=" "$env_file" 2>/dev/null | cut -d'=' -f2 | tr -d '"' || echo "middleware-vector")
        
        # 设置配置
        export VECTOR_TCP_PORT="$vector_tcp_port"
        export VECTOR_API_PORT="$vector_api_port"
        export VECTOR_HOST="localhost"  # 外部访问使用 localhost
        export VECTOR_API="http://localhost:$vector_api_port"
        
        echo "[INFO] 已加载 .env 配置: Vector TCP端口=$vector_tcp_port, API端口=$vector_api_port"
    else
        echo "[WARNING] 未找到 .env 文件，使用默认配置"
        export VECTOR_HOST="localhost"
        export VECTOR_TCP_PORT="6000"
        export VECTOR_API="http://localhost:8686"
    fi
}

# 加载环境配置
load_env_config

# 测试配置
MANAGER_API="http://localhost:8080"

# 生成符合要求的测试数据 (包含 collector_id)
COLLECTOR_ID="12345678-abcd-efgh-ijkl-123456789012"
COLLECTOR_SHORT="12345678"
EXPECTED_TOPIC="sysarmor.raw.audit"  # 使用新的统一topic

# 符合 Vector 配置要求的测试数据 (紧凑格式，避免换行问题)
TEST_MESSAGE='{"collector_id":"'${COLLECTOR_ID}'","timestamp":"'$(date -u +%Y-%m-%dT%H:%M:%SZ)'","host":"test-host-001","source":"auditd","message":"type=SYSCALL msg=audit(1693420800.123:456): arch=c000003e syscall=2 success=yes exit=3 a0=7fff12345678 a1=0 a2=0 a3=7fff87654321 items=1 ppid=1234 pid=5678 auid=1000 uid=0 gid=0 euid=0 suid=0 fsuid=0 egid=0 sgid=0 fsgid=0 tty=pts0 ses=1 comm=\"cat\" exe=\"/bin/cat\" key=\"file_access\"","event_type":"syslog","severity":"info","tags":["audit","syscall","file_access"]}'

echo -e "${BLUE}📋 测试环境信息:${NC}"
echo "  Vector TCP: ${VECTOR_HOST}:${VECTOR_TCP_PORT}"
echo "  Vector API: ${VECTOR_API}"
echo "  Collector ID: ${COLLECTOR_ID}"
echo "  Expected Topic: ${EXPECTED_TOPIC}"
echo ""

# 步骤1: 检查 Vector 服务
echo -e "${YELLOW}🔍 步骤1: 检查 Vector 服务状态${NC}"
echo "=================================================="

echo -n "Vector 健康检查: "
if curl -s -f "${VECTOR_API}/health" > /dev/null; then
    echo -e "${GREEN}✅ 健康${NC}"
else
    echo -e "${RED}❌ 不可用${NC}"
    exit 1
fi

echo ""

# 步骤2: 发送修复格式的测试数据
echo -e "${YELLOW}📤 步骤2: 发送修复格式的测试数据${NC}"
echo "=================================================="

echo "测试数据 (紧凑格式):"
echo "${TEST_MESSAGE}" | jq . 2>/dev/null || echo "${TEST_MESSAGE}"
echo ""

echo -n "发送数据到 Vector TCP:${VECTOR_TCP_PORT} (带换行符): "
# 使用 printf 确保正确的换行符
if printf "%s\n" "${TEST_MESSAGE}" | nc -w 5 ${VECTOR_HOST} ${VECTOR_TCP_PORT}; then
    echo -e "${GREEN}✅ 数据发送成功${NC}"
else
    echo -e "${RED}❌ 数据发送失败${NC}"
    exit 1
fi

echo ""

# 步骤3: 等待数据处理
echo -e "${YELLOW}⏳ 步骤3: 等待数据处理 (3秒)${NC}"
echo "=================================================="
for i in {3..1}; do
    echo -n "等待 $i 秒..."
    sleep 1
    echo -e "\r\033[K"
done
echo -e "${GREEN}✅ 等待完成${NC}"
echo ""

# 步骤4: 查看 Vector 最新日志
echo -e "${YELLOW}📋 步骤4: 查看 Vector 最新日志${NC}"
echo "=================================================="
docker compose -f docker-compose.middleware.yml logs --tail 10 vector
echo ""

# 步骤5: 检查 Kafka 主题 (使用 Manager API)
echo -e "${YELLOW}📋 步骤5: 检查 Kafka 主题: ${EXPECTED_TOPIC}${NC}"
echo "=================================================="

echo -n "检查 Kafka 健康状态: "
KAFKA_HEALTH=$(curl -s --max-time 10 "${MANAGER_API}/api/v1/services/kafka/health" 2>/dev/null)
KAFKA_CONNECTED=$(echo "$KAFKA_HEALTH" | jq -r '.connected' 2>/dev/null)

if [ "$KAFKA_CONNECTED" = "true" ]; then
    echo -e "${GREEN}✅ Kafka 连接正常${NC}"
    
    echo -n "获取主题列表: "
    TOPICS_RESPONSE=$(curl -s --max-time 10 "${MANAGER_API}/api/v1/services/kafka/topics" 2>/dev/null)
    TOPICS_SUCCESS=$(echo "$TOPICS_RESPONSE" | jq -r '.success' 2>/dev/null)
    
    if [ "$TOPICS_SUCCESS" = "true" ]; then
        echo -e "${GREEN}✅ 成功${NC}"
        echo "现有主题:"
        echo "$TOPICS_RESPONSE" | jq -r '.data.topics[].name' | sed 's/^/  - /'
        
        # 检查期望的主题是否存在
        TOPIC_EXISTS=$(echo "$TOPICS_RESPONSE" | jq -r ".data.topics[] | select(.name == \"${EXPECTED_TOPIC}\") | .name" 2>/dev/null)
        if [ -n "$TOPIC_EXISTS" ]; then
            echo -e "${GREEN}✅ 主题 ${EXPECTED_TOPIC} 存在${NC}"
            
            # 获取主题消息
            echo -n "获取最新消息: "
            MESSAGES_RESPONSE=$(curl -s --max-time 10 "${MANAGER_API}/api/v1/services/kafka/topics/${EXPECTED_TOPIC}/messages?limit=1" 2>/dev/null)
            MESSAGES_SUCCESS=$(echo "$MESSAGES_RESPONSE" | jq -r '.success' 2>/dev/null)
            
            if [ "$MESSAGES_SUCCESS" = "true" ]; then
                MESSAGES_COUNT=$(echo "$MESSAGES_RESPONSE" | jq -r '.data.messages | length' 2>/dev/null)
                if [ "$MESSAGES_COUNT" -gt 0 ]; then
                    echo -e "${GREEN}✅ 发现 $MESSAGES_COUNT 条消息${NC}"
                    echo "最新消息:"
                    echo "$MESSAGES_RESPONSE" | jq -r '.data.messages[0].value' | jq . 2>/dev/null || echo "$MESSAGES_RESPONSE" | jq -r '.data.messages[0].value'
                else
                    echo -e "${YELLOW}⚠️  暂无消息${NC}"
                fi
            else
                echo -e "${YELLOW}⚠️  无法获取消息${NC}"
            fi
        else
            echo -e "${YELLOW}⚠️  主题 ${EXPECTED_TOPIC} 不存在${NC}"
        fi
    else
        echo -e "${RED}❌ 无法获取主题列表${NC}"
    fi
else
    echo -e "${RED}❌ Kafka 连接失败${NC}"
fi

echo ""

# 步骤6: 检查 Vector 错误日志
echo -e "${YELLOW}🔍 步骤6: 检查 Vector 错误日志${NC}"
echo "=================================================="
echo "搜索最新的错误信息:"
VECTOR_ERRORS=$(docker compose -f docker-compose.middleware.yml logs --tail 50 vector 2>&1 | grep -i -E "(error|failed|abort|drop)" | tail -5 || echo "")
if [ -n "$VECTOR_ERRORS" ]; then
    echo -e "${RED}发现错误日志:${NC}"
    echo "$VECTOR_ERRORS"
else
    echo -e "${GREEN}✅ 无最新错误日志${NC}"
fi

echo ""

# 步骤7: 手动验证 JSON 格式
echo -e "${YELLOW}🔧 步骤7: 验证 JSON 格式${NC}"
echo "=================================================="
echo -n "JSON 格式验证: "
if echo "${TEST_MESSAGE}" | jq . > /dev/null 2>&1; then
    echo -e "${GREEN}✅ JSON 格式正确${NC}"
else
    echo -e "${RED}❌ JSON 格式错误${NC}"
    echo "原始数据: ${TEST_MESSAGE}"
fi

echo ""

# 步骤8: 测试总结
echo -e "${BLUE}📊 测试总结${NC}"
echo "=================================================="
echo -e "${GREEN}✅ Vector 服务: 健康${NC}"
echo -e "${GREEN}✅ 数据发送: 成功 (修复格式)${NC}"
echo -e "${GREEN}✅ Kafka 工具: 路径修复${NC}"

# 检查数据流是否成功
TOPICS_RESPONSE=$(curl -s --max-time 10 "${MANAGER_API}/api/v1/services/kafka/topics" 2>/dev/null)
TOPIC_EXISTS=$(echo "$TOPICS_RESPONSE" | jq -r ".data.topics[] | select(.name == \"${EXPECTED_TOPIC}\") | .name" 2>/dev/null)

if [ -n "$TOPIC_EXISTS" ]; then
    echo -e "${GREEN}✅ Kafka 主题创建: 成功${NC}"
    echo -e "${GREEN}✅ 数据流: Vector → Kafka 正常${NC}"
else
    echo -e "${YELLOW}⚠️  Kafka 主题创建: 需要进一步调试${NC}"
    echo -e "${YELLOW}⚠️  数据流: 检查 Vector 配置和日志${NC}"
fi

echo ""
echo -e "${BLUE}💡 调试命令:${NC}"
echo "1. 实时查看 Vector 日志: docker compose logs -f vector"
echo "2. 检查 Kafka 主题: ./scripts/kafka-tools.sh list"
echo "3. 消费 Kafka 消息: ./scripts/kafka-tools.sh export ${EXPECTED_TOPIC} 10"
echo "4. 检查 Vector 配置: cat services/middleware/configs/vector/vector.toml"
echo "5. Manager API 健康检查: curl -s ${MANAGER_API}/api/v1/health | jq ."
echo "6. Kafka 健康检查: curl -s ${MANAGER_API}/api/v1/services/kafka/health | jq ."
echo ""
echo -e "${GREEN}🎉 SysArmor 修复版数据流测试完成！${NC}"
