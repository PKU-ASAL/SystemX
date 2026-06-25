#!/usr/bin/env bash
# S4 benign-ci-noise（容器拓扑版）：完全合法的本地 CI 构建噪声。
# 断言：Incident=0。该场景不能触碰 C2/IoC、攻击落盘路径或敏感凭据。
set -euo pipefail
CYCLES="${CYCLES:-20}"

for i in $(seq 1 "$CYCLES"); do
  docker exec node-a bash /usr/local/bin/build.sh
done
echo "[benign-ci-noise] ran $CYCLES benign build cycles (container)"
