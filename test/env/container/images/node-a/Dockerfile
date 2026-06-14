# 受害端点：模拟 K8s 节点上跑的工作负载容器。
# 长驻 web 运行时（可被 RCE）+ CI 构建脚本 + 植入假凭据。
# 攻击通过 `docker exec node-a bash -c '...'` 触发谱系下的命令执行。
FROM docker.1panel.live/library/ubuntu:22.04
# 官方源被墙 → 换可达镜像 + 关 https 校验（容器无 ca 证书，测试环境可接受）
RUN sed -i 's|http://archive.ubuntu.com/ubuntu|https://mirrors.edge.kernel.org/ubuntu|g; s|http://security.ubuntu.com/ubuntu|https://mirrors.edge.kernel.org/ubuntu|g' /etc/apt/sources.list && \
    printf 'Acquire::https::Verify-Peer "false";\n' > /etc/apt/apt.conf.d/99no-verify && \
    apt-get update && apt-get install -y curl bash coreutils procps iproute2 strace tar && \
    rm -rf /var/lib/apt/lists/*
# 共享落盘目录（apt-staged-drop 跨 lineage 共享 FILE 实体）
RUN mkdir -p /var/lib/app/plugins
# 植入"假但可读"的凭据，让后渗透阶段的 open/read 系统调用真实发生
RUN mkdir -p /var/run/secrets/kubernetes.io/serviceaccount /root/.ssh && \
    printf 'FAKE-SA-TOKEN.for-test.%s' "$(date +%s)" \
      > /var/run/secrets/kubernetes.io/serviceaccount/token && \
    printf -- '-----BEGIN OPENSSH PRIVATE KEY-----\nFAKE-TEST-KEY-DO-NOT-USE\n-----END OPENSSH PRIVATE KEY-----\n' \
      > /root/.ssh/id_rsa && \
    chmod 600 /root/.ssh/id_rsa
COPY entrypoint.sh /entrypoint.sh
COPY build.sh /usr/local/bin/build.sh
RUN chmod +x /entrypoint.sh /usr/local/bin/build.sh
ENTRYPOINT ["/entrypoint.sh"]
