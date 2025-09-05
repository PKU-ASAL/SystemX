#!/bin/bash

echo "🔍 测试真实rsyslog格式"
echo "======================"

VECTOR_HOST="162.105.126.246"
VECTOR_TCP_PORT="6000"

# 模拟真实的rsyslog JSON格式 - 基于SysArmorTemplate
echo "测试1: 发送标准rsyslog JSON格式"
JSON_MSG='{"timestamp":"2025-08-22T11:57:00+08:00","collector_id":"558c01dd-b545-41cb-ab17-0d4290615006","host":"test-host","program":"auditd","message":"type=SYSCALL msg=audit(1692691200.123:456): arch=c000003e syscall=2 success=yes exit=3 a0=7fff12345678 a1=241 a2=1b6 a3=0 items=1 ppid=1234 pid=5678 auid=1000 uid=0 gid=0 euid=0 suid=0 fsuid=0 egid=0 sgid=0 fsgid=0 tty=pts0 ses=1 comm=\"cat\" exe=\"/bin/cat\" key=\"file_access\"","event_type":"syslog"}'

echo "发送数据: $JSON_MSG"
echo "$JSON_MSG" | nc -w 5 $VECTOR_HOST $VECTOR_TCP_PORT
echo ""

# 测试2: 使用printf确保正确的换行符
echo "测试2: 使用printf发送"
printf '%s\n' "$JSON_MSG" | nc -w 5 $VECTOR_HOST $VECTOR_TCP_PORT
echo ""

# 测试3: 发送多条消息
echo "测试3: 发送多条消息"
for i in {1..3}; do
    MSG='{"timestamp":"2025-08-22T11:57:0'$i'+08:00","collector_id":"558c01dd-b545-41cb-ab17-0d4290615006","host":"test-host-'$i'","program":"auditd","message":"test message '$i'","event_type":"syslog"}'
    echo "$MSG" | nc -w 3 $VECTOR_HOST $VECTOR_TCP_PORT
    sleep 1
done
echo ""

# 测试4: 使用telnet发送
echo "测试4: 使用telnet发送"
{
    echo "$JSON_MSG"
    sleep 2
} | telnet $VECTOR_HOST $VECTOR_TCP_PORT 2>/dev/null
echo ""

echo "📋 测试完成，等待Vector处理..."
sleep 3

# 检查Kafka中是否有新的topic
echo "检查Kafka topics:"
curl -s "http://$VECTOR_HOST:8080/api/topics" | python3 -m json.tool 2>/dev/null || echo "无法连接到Kafka UI"
