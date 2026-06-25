#!/usr/bin/env bash
set -euo pipefail

REPEAT="${REPEAT:-1}"
COUNT="${COUNT:-50}"

mkdir -p /tmp/sysarmor-workload/files

for i in $(seq 1 "$REPEAT"); do
  echo "[workload] mixed-edr-storm iteration=$i count=$COUNT"
  for n in $(seq 1 "$COUNT"); do
    /bin/true
    /bin/echo sysarmor-workload-mixed >/dev/null
    printf "sysarmor-workload-%s-%s\n" "$i" "$n" > "/tmp/sysarmor-workload/files/artifact-$n.txt"
    cp "/tmp/sysarmor-workload/files/artifact-$n.txt" "/tmp/sysarmor-workload/files/copy-$n.txt" 2>/dev/null || true
    timeout 1 bash -c "cat < /dev/null > /dev/tcp/127.0.0.1/9" >/dev/null 2>&1 || true
  done
done
