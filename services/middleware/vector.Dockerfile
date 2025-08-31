# Vector 数据收集器定制镜像
FROM docker.io/timberio/vector:0.34.0-alpine

# 安装额外工具
RUN apk add --no-cache curl wget netcat-openbsd

# 创建配置目录
RUN mkdir -p /etc/vector /var/lib/vector

# 设置工作目录
WORKDIR /var/lib/vector

# 设置环境变量
ENV VECTOR_CONFIG=/etc/vector/vector.toml
ENV VECTOR_LOG=info

# 暴露端口
EXPOSE 6000 8686 9598

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8686/health || exit 1

# 启动 Vector (配置文件通过 volume 挂载)
CMD ["--config", "/etc/vector/vector.toml"]
