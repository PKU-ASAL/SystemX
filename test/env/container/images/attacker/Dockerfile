# 攻击机：C2 服务器，在测试网内提供恶意载荷下载 + 反弹 shell 监听。
# 容器拓扑中 attacker 即"攻击节点"，http :8080 发 payload，nc :443 收反弹/exfil。
FROM docker.1panel.live/library/ubuntu:22.04
RUN sed -i 's|http://archive.ubuntu.com/ubuntu|https://mirrors.edge.kernel.org/ubuntu|g; s|http://security.ubuntu.com/ubuntu|https://mirrors.edge.kernel.org/ubuntu|g' /etc/apt/sources.list && \
    printf 'Acquire::https::Verify-Peer "false";\n' > /etc/apt/apt.conf.d/99no-verify && \
    apt-get update && apt-get install -y netcat-openbsd python3 tar bash && rm -rf /var/lib/apt/lists/*
COPY payloads /opt/c2/payloads
WORKDIR /opt/c2/payloads
RUN bash gen-deps.sh /opt/c2/payloads || true
EXPOSE 8080 443
CMD bash -c "python3 -m http.server 8080 & while true; do nc -lvn 443; done"
