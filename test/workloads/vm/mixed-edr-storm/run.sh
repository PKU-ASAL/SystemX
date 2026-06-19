#!/usr/bin/env bash
set -euo pipefail

REPEAT="${REPEAT:-1}"
COUNT="${COUNT:-50}"

mkdir -p /tmp/sysarmor-workload /var/lib/app/plugins

for i in $(seq 1 "$REPEAT"); do
  echo "[workload] mixed-edr-storm iteration=$i count=$COUNT"
  for n in $(seq 1 "$COUNT"); do
    /bin/true
    /bin/echo sysarmor-workload-mixed >/dev/null
    printf "sysarmor-workload-%s-%s\n" "$i" "$n" > "/tmp/sysarmor-workload/payload-$n.sh"
    chmod +x "/tmp/sysarmor-workload/payload-$n.sh"
    cp "/tmp/sysarmor-workload/payload-$n.sh" "/var/lib/app/plugins/workload-$n" 2>/dev/null || true
    timeout 1 bash -c "cat < /dev/null > /dev/tcp/127.0.0.1/9" >/dev/null 2>&1 || true
  done
done
