#!/usr/bin/env bash
# node-a VM：植入假凭据文件，让后渗透阶段的 open/read 系统调用真实发生。
set -euo pipefail

mkdir -p /var/run/secrets/kubernetes.io/serviceaccount /root/.ssh
printf 'FAKE-SA-TOKEN.for-test.%s' "$(date +%s)" \
  > /var/run/secrets/kubernetes.io/serviceaccount/token
printf -- '-----BEGIN OPENSSH PRIVATE KEY-----\nFAKE-TEST-KEY-DO-NOT-USE\n-----END OPENSSH PRIVATE KEY-----\n' \
  > /root/.ssh/id_rsa
chmod 600 /root/.ssh/id_rsa

# staged-drop 的共享落盘目录
mkdir -p /var/lib/app/plugins

echo "[credentials] 假凭据已植入"
