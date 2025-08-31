#!/bin/bash
# Kafka 命令行工具包装脚本 - 清除 JMX 配置避免端口冲突

# 清除可能导致端口冲突的 JMX 相关环境变量
unset KAFKA_JVM_PERFORMANCE_OPTS
unset JMX_PORT
unset KAFKA_JMX_PORT
unset KAFKA_JMX_HOSTNAME

# 执行传入的 Kafka 命令
exec "$@"
