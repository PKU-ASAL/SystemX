#!/usr/bin/env bash
# S4 benign-ci-noise（容器拓扑版）：完全合法的 CI 构建，行为与攻击高度相似。
# 断言：Incident=0（罕见度而非裸加）。
set -euo pipefail
C2="${C2:-10.66.0.99}"
CYCLES="${CYCLES:-20}"

for i in $(seq 1 "$CYCLES"); do
  docker exec -e C2="$C2" node-a bash /usr/local/bin/build.sh
done
echo "[benign-ci-noise] ran $CYCLES benign build cycles (container)"
