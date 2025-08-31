#!/bin/bash
set -e

echo "🔧 设置 SysArmor Middleware 监控..."

# 创建监控目录
mkdir -p configs/monitoring/jmx-exporter
mkdir -p configs/monitoring/prometheus

# 下载 JMX Exporter (如果不存在)
JMX_JAR="configs/monitoring/jmx-exporter/jmx_prometheus_javaagent-0.19.0.jar"
if [ ! -f "$JMX_JAR" ]; then
    echo "📥 下载 JMX Prometheus Java Agent..."
    wget -O "$JMX_JAR" \
        "https://repo1.maven.org/maven2/io/prometheus/jmx/jmx_prometheus_javaagent/0.19.0/jmx_prometheus_javaagent-0.19.0.jar"
    echo "✅ JMX Agent 下载完成"
else
    echo "✅ JMX Agent 已存在"
fi

# 验证配置文件
if [ ! -f "configs/monitoring/jmx-exporter/kafka-metrics.yml" ]; then
    echo "❌ Kafka JMX 配置文件不存在"
    exit 1
fi

if [ ! -f "configs/monitoring/prometheus/prometheus.yml" ]; then
    echo "❌ Prometheus 配置文件不存在"
    exit 1
fi

echo "✅ 监控设置完成"
echo ""
echo "📊 指标端点:"
echo "  - Vector:     http://localhost:9598/metrics"
echo "  - Kafka:      http://localhost:7071/metrics"  
echo "  - Prometheus: http://localhost:9090"
