#!/usr/bin/env bash
set -euo pipefail

REPEAT="${REPEAT:-1}"
COUNT="${COUNT:-80}"

mkdir -p /tmp/sysarmor-workload /var/lib/app/plugins

for i in $(seq 1 "$REPEAT"); do
  echo "[workload] file-write-storm iteration=$i count=$COUNT"
  for n in $(seq 1 "$COUNT"); do
    printf "sysarmor-workload-%s-%s\n" "$i" "$n" > "/tmp/sysarmor-workload/payload-$n.sh"
    chmod +x "/tmp/sysarmor-workload/payload-$n.sh"
    cp "/tmp/sysarmor-workload/payload-$n.sh" "/var/lib/app/plugins/workload-$n" 2>/dev/null || true
  done
done
