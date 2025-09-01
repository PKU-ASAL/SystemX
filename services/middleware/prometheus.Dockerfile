# Prometheus 定制镜像 - 简化版本，直接使用官方镜像
FROM docker.io/prom/prometheus:v2.45.0

# 设置环境变量
ENV PROMETHEUS_CONFIG_FILE=/etc/prometheus/prometheus.yml
ENV PROMETHEUS_STORAGE_PATH=/prometheus
ENV PROMETHEUS_RETENTION_TIME=15d

# 暴露端口
EXPOSE 9090

# 使用官方镜像的默认启动命令
