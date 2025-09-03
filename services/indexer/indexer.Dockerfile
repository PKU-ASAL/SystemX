# Python 索引服务镜像 - 简化版本
FROM docker.io/python:3.11-slim

# 设置工作目录
WORKDIR /app

# 复制 requirements 文件
COPY ./requirements.txt .

# 安装 Python 依赖
RUN pip install --no-cache-dir -r requirements.txt

# 复制应用代码
COPY ./src/ ./src/
COPY ./templates/ ./templates/

# 创建非 root 用户
RUN addgroup --gid 1001 --system sysarmor && \
    adduser --uid 1001 --system --group sysarmor && \
    chown -R sysarmor:sysarmor /app

# 切换到非 root 用户
USER sysarmor

# 设置环境变量
ENV PYTHONPATH=/app/src:$PYTHONPATH
ENV INDEX_PREFIX=sysarmor-events

# 启动应用
CMD ["python", "src/main.py"]
