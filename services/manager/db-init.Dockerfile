# 数据库初始化服务镜像
FROM docker.io/postgres:15-alpine

# 安装必要工具
RUN apk add --no-cache bash curl

# 复制初始化脚本和迁移文件
COPY services/manager/scripts/init-db.sh /usr/local/bin/init-db.sh
COPY services/manager/migrations/ /app/migrations/

# 设置执行权限
RUN chmod +x /usr/local/bin/init-db.sh

# 设置工作目录
WORKDIR /app

# 设置环境变量
ENV POSTGRES_HOST=manager-postgres
ENV POSTGRES_PORT=5432
ENV POSTGRES_DB=sysarmor
ENV POSTGRES_USER=sysarmor
ENV POSTGRES_PASSWORD=password

# 启动初始化脚本
CMD ["/usr/local/bin/init-db.sh"]
