# 简化版 PostgreSQL 镜像
FROM docker.io/postgres:15-alpine

# 设置标签
LABEL maintainer="SysArmor Team <support@sysarmor.com>"
LABEL description="SysArmor PostgreSQL Database"

# 安装必要的工具
RUN apk add --no-cache curl

# 设置默认环境变量
ENV POSTGRES_DB=sysarmor
ENV POSTGRES_USER=sysarmor
ENV POSTGRES_PASSWORD=password

# 优化 PostgreSQL 配置
ENV POSTGRES_INITDB_ARGS="--encoding=UTF-8 --lc-collate=C --lc-ctype=C"

# 暴露端口
EXPOSE 5432

# 健康检查
HEALTHCHECK --interval=10s --timeout=5s --start-period=30s --retries=5 \
    CMD pg_isready -U $POSTGRES_USER -d $POSTGRES_DB || exit 1
