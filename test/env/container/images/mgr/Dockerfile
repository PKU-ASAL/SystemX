# Manager 预留：sysarmor-manager 端（gateway + analytics + store）。
# 容器拓扑中占位，待 sysarmor-manager binary 就绪后替换。
FROM docker.1panel.live/library/ubuntu:22.04
RUN sed -i 's|http://archive.ubuntu.com/ubuntu|https://mirrors.edge.kernel.org/ubuntu|g; s|http://security.ubuntu.com/ubuntu|https://mirrors.edge.kernel.org/ubuntu|g' /etc/apt/sources.list && \
    printf 'Acquire::https::Verify-Peer "false";\n' > /etc/apt/apt.conf.d/99no-verify && \
    apt-get update && apt-get install -y curl bash && rm -rf /var/lib/apt/lists/*
ENTRYPOINT ["bash", "-c", "while true; do sleep 30; done"]
