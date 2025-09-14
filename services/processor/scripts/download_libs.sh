#!/bin/bash

mkdir -p ./lib

# 优先使用国内镜像源加速下载
echo "📦 Downloading Flink connectors from Aliyun Mirror..."

# 使用阿里云镜像源（更快）
curl -L --connect-timeout 10 --max-time 300 -o ./lib/flink-sql-connector-kafka-3.1.0-1.18.jar https://maven.aliyun.com/repository/public/org/apache/flink/flink-sql-connector-kafka/3.1.0-1.18/flink-sql-connector-kafka-3.1.0-1.18.jar && \
curl -L --connect-timeout 10 --max-time 300 -o ./lib/flink-sql-connector-opensearch-1.2.0-1.18.jar https://maven.aliyun.com/repository/public/org/apache/flink/flink-sql-connector-opensearch/1.2.0-1.18/flink-sql-connector-opensearch-1.2.0-1.18.jar && \
curl -L --connect-timeout 10 --max-time 300 -o ./lib/flink-sql-connector-elasticsearch7-3.1.0-1.18.jar https://maven.aliyun.com/repository/public/org/apache/flink/flink-sql-connector-elasticsearch7/3.1.0-1.18/flink-sql-connector-elasticsearch7-3.1.0-1.18.jar

if [ $? -eq 0 ]; then
    echo "✅ Libs downloaded successfully"
    echo "📋 Downloaded connectors:"
    echo "   - Kafka Connector: flink-sql-connector-kafka-3.1.0-1.18.jar"
    echo "   - OpenSearch Connector: flink-sql-connector-opensearch-1.2.0-1.18.jar"
    echo "   - Elasticsearch7 Connector: flink-sql-connector-elasticsearch7-3.1.0-1.18.jar"
    ls -la ./lib/
else
    echo "❌ 阿里云镜像失败，尝试腾讯云镜像..."
    # 备用源：腾讯云镜像
    curl -L --connect-timeout 10 --max-time 300 --retry 3 -o ./lib/flink-sql-connector-kafka-3.1.0-1.18.jar https://mirrors.cloud.tencent.com/nexus/repository/maven-public/org/apache/flink/flink-sql-connector-kafka/3.1.0-1.18/flink-sql-connector-kafka-3.1.0-1.18.jar && \
    curl -L --connect-timeout 10 --max-time 300 --retry 3 -o ./lib/flink-sql-connector-opensearch-1.2.0-1.18.jar https://mirrors.cloud.tencent.com/nexus/repository/maven-public/org/apache/flink/flink-sql-connector-opensearch/1.2.0-1.18/flink-sql-connector-opensearch-1.2.0-1.18.jar && \
    curl -L --connect-timeout 10 --max-time 300 --retry 3 -o ./lib/flink-sql-connector-elasticsearch7-3.1.0-1.18.jar https://mirrors.cloud.tencent.com/nexus/repository/maven-public/org/apache/flink/flink-sql-connector-elasticsearch7/3.1.0-1.18/flink-sql-connector-elasticsearch7-3.1.0-1.18.jar
    
    if [ $? -eq 0 ]; then
        echo "✅ Libs downloaded successfully from backup source"
        ls -la ./lib/
    else
        echo "❌ 腾讯云镜像失败，尝试华为云镜像..."
        curl -L --connect-timeout 10 --max-time 300 --retry 3 -o ./lib/flink-sql-connector-kafka-3.1.0-1.18.jar https://repo.huaweicloud.com/repository/maven/org/apache/flink/flink-sql-connector-kafka/3.1.0-1.18/flink-sql-connector-kafka-3.1.0-1.18.jar && \
        curl -L --connect-timeout 10 --max-time 300 --retry 3 -o ./lib/flink-sql-connector-opensearch-1.2.0-1.18.jar https://repo.huaweicloud.com/repository/maven/org/apache/flink/flink-sql-connector-opensearch/1.2.0-1.18/flink-sql-connector-opensearch-1.2.0-1.18.jar && \
        curl -L --connect-timeout 10 --max-time 300 --retry 3 -o ./lib/flink-sql-connector-elasticsearch7-3.1.0-1.18.jar https://repo.huaweicloud.com/repository/maven/org/apache/flink/flink-sql-connector-elasticsearch7/3.1.0-1.18/flink-sql-connector-elasticsearch7-3.1.0-1.18.jar
        
        if [ $? -eq 0 ]; then
            echo "✅ Libs downloaded successfully from Maven Central"
            ls -la ./lib/
        else
            echo "❌ All download sources failed"
            exit 1
        fi
    fi
fi
