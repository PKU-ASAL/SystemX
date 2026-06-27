# Manager 容器：运行从仓库 bin/ 挂载进来的 sysarmor-manager。
FROM docker.1panel.live/library/ubuntu:22.04
RUN sed -i 's|http://archive.ubuntu.com/ubuntu|https://mirrors.edge.kernel.org/ubuntu|g; s|http://security.ubuntu.com/ubuntu|https://mirrors.edge.kernel.org/ubuntu|g' /etc/apt/sources.list && \
    printf 'Acquire::https::Verify-Peer "false";\n' > /etc/apt/apt.conf.d/99no-verify && \
    apt-get update && apt-get install -y curl bash && rm -rf /var/lib/apt/lists/*
ENTRYPOINT ["bash", "-lc", "/opt/sysarmor/bin/sysarmor-manager --listen 0.0.0.0:9443 --grpc-listen 0.0.0.0:9444 --store-backend postgres --postgres-driver ${SYSARMOR_POSTGRES_DRIVER:-postgres} --postgres-dsn \"$SYSARMOR_POSTGRES_DSN\""]
