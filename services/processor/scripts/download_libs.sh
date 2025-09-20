#!/bin/bash

mkdir -p ./lib

# 使用官方 Maven 中央仓库
echo "📦 Downloading Flink connectors from Maven Central..."

# 下载 Kafka 连接器和 OpenSearch 连接器
curl -L -o ./lib/flink-sql-connector-kafka-3.1.0-1.18.jar https://repo1.maven.org/maven2/org/apache/flink/flink-sql-connector-kafka/3.1.0-1.18/flink-sql-connector-kafka-3.1.0-1.18.jar && \

if [ $? -eq 0 ]; then
    echo "✅ Libs downloaded successfully"
    echo "📋 Downloaded connectors:"
    echo "   - Kafka Connector: flink-sql-connector-kafka-3.1.0-1.18.jar"
    ls -la ./lib/
else
    echo "❌ Failed to download libs, trying backup sources..."
    # 备用源：华为云镜像
    curl -L -o ./lib/flink-sql-connector-kafka-3.1.0-1.18.jar https://repo.huaweicloud.com/repository/maven/org/apache/flink/flink-sql-connector-kafka/3.1.0-1.18/flink-sql-connector-kafka-3.1.0-1.18.jar && \

    if [ $? -eq 0 ]; then
        echo "✅ Libs downloaded successfully from backup source"
        ls -la ./lib/
    else
        echo "❌ Trying original Maven Central..."
        curl -L -o ./lib/flink-sql-connector-kafka-3.1.0-1.18.jar https://repo.maven.apache.org/maven2/org/apache/flink/flink-sql-connector-kafka/3.1.0-1.18/flink-sql-connector-kafka-3.1.0-1.18.jar && \

        if [ $? -eq 0 ]; then
            echo "✅ Libs downloaded successfully from Maven Central"
            ls -la ./lib/
        else
            echo "❌ All download sources failed"
            exit 1
        fi
    fi
fi
