# OpenSearch 定制镜像 - 使用官方镜像
FROM docker.io/opensearchproject/opensearch:2.11.0

# 设置环境变量
ENV OPENSEARCH_JAVA_OPTS="-Xms512m -Xmx512m"
ENV discovery.type=single-node

# 暴露端口
EXPOSE 9200 9600

# 使用官方镜像的默认启动命令
