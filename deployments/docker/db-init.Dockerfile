# 数据库初始化 Dockerfile
FROM docker.io/postgres:15-alpine

# 安装必要的工具
RUN apk add --no-cache curl

# 复制初始化脚本
COPY shared/migrations/ /docker-entrypoint-initdb.d/

# 设置权限
RUN chmod +x /docker-entrypoint-initdb.d/*.sql

# 创建初始化脚本
RUN echo '#!/bin/bash' > /docker-entrypoint-initdb.d/00-init.sh && \
    echo 'set -e' >> /docker-entrypoint-initdb.d/00-init.sh && \
    echo 'echo "Initializing SysArmor database..."' >> /docker-entrypoint-initdb.d/00-init.sh && \
    echo 'psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" < /docker-entrypoint-initdb.d/001_initial.sql' >> /docker-entrypoint-initdb.d/00-init.sh && \
    echo 'echo "Database initialization completed."' >> /docker-entrypoint-initdb.d/00-init.sh && \
    chmod +x /docker-entrypoint-initdb.d/00-init.sh

# 设置环境变量
ENV POSTGRES_DB=sysarmor
ENV POSTGRES_USER=sysarmor
ENV POSTGRES_PASSWORD=password

# 暴露端口
EXPOSE 5432
