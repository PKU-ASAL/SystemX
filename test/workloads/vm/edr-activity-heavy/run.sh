#!/usr/bin/env bash
set -euo pipefail

REPEAT="${REPEAT:-1}"
COUNT="${COUNT:-50}"
DURATION="${DURATION:-0}"

mkdir -p /tmp/sysarmor-workload/files

start_ts="$(date +%s)"
i=0
while :; do
  i=$((i + 1))
  echo "[workload] edr-activity-heavy iteration=$i count=$COUNT"
  for n in $(seq 1 "$COUNT"); do
    /bin/true
    /bin/echo sysarmor-workload-edr >/dev/null
    printf "sysarmor-workload-%s-%s\n" "$i" "$n" > "/tmp/sysarmor-workload/files/artifact-$n.txt"
    cp "/tmp/sysarmor-workload/files/artifact-$n.txt" "/tmp/sysarmor-workload/files/copy-$n.txt" 2>/dev/null || true
    timeout 1 bash -c "cat < /dev/null > /dev/tcp/127.0.0.1/9" >/dev/null 2>&1 || true
  done
  if [ "$REPEAT" != "0" ] && [ "$i" -ge "$REPEAT" ]; then
    break
  fi
  now="$(date +%s)"
  if [ "$DURATION" != "0" ] && [ $((now - start_ts)) -ge "$DURATION" ]; then
    break
  fi
done
