# 多阶段构建 Dockerfile for SysArmor Manager

# 构建阶段
FROM docker.io/golang:1.24-alpine AS builder

# 设置工作目录
WORKDIR /app

# 设置 Go 代理为大陆镜像
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=sum.golang.google.cn

# 安装必要的工具
RUN apk add --no-cache git ca-certificates tzdata make

# 复制 Go workspace 配置
COPY go.work go.work.sum ./

# 复制共享配置模块
COPY shared/ ./shared/

# 复制 manager 应用
COPY apps/manager/ ./apps/manager/

# 设置工作目录到 manager 应用
WORKDIR /app/apps/manager

# 下载依赖（使用 workspace 模式）
RUN go mod download

# 安装 swag 工具
RUN go install github.com/swaggo/swag/cmd/swag@latest

# 生成 Swagger 文档
RUN /go/bin/swag init -g main.go -o docs --parseDependency --parseInternal

# 构建应用
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o manager ./main.go

# 运行阶段
FROM docker.io/alpine:latest

# 安装运行时依赖
RUN apk --no-cache add ca-certificates curl tzdata && \
    update-ca-certificates

# 创建非 root 用户
RUN addgroup -g 1001 -S sysarmor && \
    adduser -u 1001 -S sysarmor -G sysarmor

# 设置工作目录
WORKDIR /app

# 从构建阶段复制二进制文件
COPY --from=builder /app/apps/manager/manager .

# 复制 Swagger 文档
COPY --from=builder /app/apps/manager/docs ./docs

# 复制共享模板
COPY shared/templates ./shared/templates
COPY shared/binaries ./shared/binaries

# 创建必要的目录
RUN mkdir -p ./configs ./logs && \
    chown -R sysarmor:sysarmor /app

# 切换到非 root 用户
USER sysarmor

# 暴露端口
EXPOSE 8080

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

# 启动应用
CMD ["./manager"]
