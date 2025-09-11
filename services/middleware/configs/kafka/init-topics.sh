#!/bin/bash
# SysArmor Kafka Topics 初始化脚本
# 基于代码中定义的topic常量，避免环境变量配置

set -e

# 等待Kafka服务启动
echo "⏳ 等待Kafka服务启动..."
while ! nc -z middleware-kafka 9092; do
    echo "  - Kafka未就绪，等待5秒..."
    sleep 5
done
echo "✅ Kafka服务已启动"

# 直接创建topics，不使用数组（简化脚本）

REPLICATION_FACTOR=${KAFKA_DEFAULT_REPLICATION_FACTOR:-1}

echo "🚀 开始创建SysArmor完整Topics架构..."
echo "  - 副本数: $REPLICATION_FACTOR"
echo ""

# 创建topics的函数
create_topic_if_not_exists() {
    local topic_name="$1"
    local partitions="$2"
    local retention_ms="$3"
    local purpose="$4"
    
    echo "📝 检查Topic: $topic_name"
    echo "  - 用途: $purpose"
    echo "  - 分区数: $partitions"
    echo "  - 保留期: $retention_ms ms ($(($retention_ms / 1000 / 60 / 60 / 24)) 天)"
    
    if /opt/kafka/bin/kafka-topics.sh --bootstrap-server middleware-kafka:9092 --list | grep -q "^$topic_name$"; then
        echo "  ✅ Topic $topic_name 已存在"
    else
        echo "  🔨 创建Topic: $topic_name"
        /opt/kafka/bin/kafka-topics.sh --bootstrap-server middleware-kafka:9092 \
            --create \
            --topic "$topic_name" \
            --partitions "$partitions" \
            --replication-factor "$REPLICATION_FACTOR" \
            --config retention.ms="$retention_ms" \
            --config compression.type=snappy \
            --config cleanup.policy=delete
        echo "  ✅ Topic $topic_name 创建成功"
    fi
    echo ""
}

# 按类别创建topics
echo "🔸 创建原始数据Topics (sysarmor.raw.*)..."
create_topic_if_not_exists "sysarmor.raw.audit" "32" "259200000" "auditd通过rsyslog发回的原始事件"
create_topic_if_not_exists "sysarmor.raw.other" "16" "86400000" "Vector解析失败的数据预留"

echo "🔸 创建处理后事件Topics (sysarmor.events.*)..."
create_topic_if_not_exists "sysarmor.events.audit" "32" "604800000" "auditd经过convert后形成的类sysdig事件"
create_topic_if_not_exists "sysarmor.events.sysdig" "32" "604800000" "Sysdig直接发过来的事件"

echo "🔸 创建告警Topics (sysarmor.alerts.*)..."
create_topic_if_not_exists "sysarmor.alerts" "16" "2592000000" "消费sysarmor.events.*后生成的一般预警事件"
create_topic_if_not_exists "sysarmor.alerts.high" "8" "7776000000" "消费sysarmor.events.*后生成的高危预警事件"

# 验证创建结果
echo "🔍 验证Topics创建结果:"
echo "=== 所有SysArmor Topics ==="
/opt/kafka/bin/kafka-topics.sh --bootstrap-server middleware-kafka:9092 --list | grep "sysarmor" | sort

echo ""
echo "=== 主要Topics详情 ==="
for topic in "sysarmor.raw.audit" "sysarmor.events.audit" "sysarmor.alerts"; do
    echo "--- $topic ---"
    /opt/kafka/bin/kafka-topics.sh --bootstrap-server middleware-kafka:9092 --describe --topic "$topic"
    echo ""
done

echo "🎉 SysArmor完整Topics架构初始化完成！"
echo ""
echo "📋 创建的Topics总结:"
echo "  🔸 Raw Data Layer (原始数据层) - sysarmor.raw.*:"
echo "    - sysarmor.raw.audit (32分区, 3天保留) - auditd通过rsyslog发回的原始事件"
echo "    - sysarmor.raw.other (16分区, 1天保留) - Vector解析失败的数据预留"
echo ""
echo "  🔸 Events Layer (处理后事件层) - sysarmor.events.*:"
echo "    - sysarmor.events.audit (32分区, 7天保留) - auditd经过convert后形成的类sysdig事件"
echo "    - sysarmor.events.sysdig (32分区, 7天保留) - Sysdig直接发过来的事件"
echo ""
echo "  🔸 Alert Layer (告警层) - sysarmor.alerts.*:"
echo "    - sysarmor.alerts (16分区, 30天保留) - 消费sysarmor.events.*后生成的一般预警事件"
echo "    - sysarmor.alerts.high (8分区, 90天保留) - 消费sysarmor.events.*后生成的高危预警事件"
echo ""
echo "✨ 所有Topics配置与代码中的常量定义保持一致，无需环境变量配置！"
