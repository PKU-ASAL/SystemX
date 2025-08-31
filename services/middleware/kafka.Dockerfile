# Kafka 定制镜像 - 仅包含 JMX Exporter
FROM docker.io/apache/kafka:latest

# 切换到 root 用户安装工具
USER root

# 创建 JMX Exporter 目录
RUN mkdir -p /opt/jmx-exporter

# 复制 JMX Exporter 文件
COPY services/middleware/configs/monitoring/jmx-exporter/ /opt/jmx-exporter/

# 设置权限
RUN chmod -R 755 /opt/jmx-exporter

# 暴露端口
EXPOSE 9092 9093 9094 7071

# 使用Apache Kafka官方镜像的默认启动方式
CMD ["/etc/kafka/docker/run"]
