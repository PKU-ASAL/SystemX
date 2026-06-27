#!/usr/bin/env bash
set -euo pipefail

REPEAT="${REPEAT:-1}"
COUNT="${COUNT:-80}"
DURATION="${DURATION:-0}"

root=/tmp/sysarmor-workload/host-activity-heavy
mkdir -p "$root/files" "$root/readable"
printf "sysarmor-workload-token\n" > "$root/readable/token"

start_ts="$(date +%s)"
i=0
while :; do
  i=$((i + 1))
  echo "[workload] host-activity-heavy iteration=$i count=$COUNT"
  for n in $(seq 1 "$COUNT"); do
    /bin/true
    /bin/echo sysarmor-workload-host >/dev/null
    /usr/bin/env true
    cat /etc/passwd >/dev/null 2>&1 || true
    cat "$root/readable/token" >/dev/null 2>&1 || true
    printf "sysarmor-workload-%s-%s\n" "$i" "$n" > "$root/files/artifact-$n.txt"
    cp "$root/files/artifact-$n.txt" "$root/files/copy-$n.txt" 2>/dev/null || true
  done
  if [ "$REPEAT" != "0" ] && [ "$i" -ge "$REPEAT" ]; then
    break
  fi
  now="$(date +%s)"
  if [ "$DURATION" != "0" ] && [ $((now - start_ts)) -ge "$DURATION" ]; then
    break
  fi
done
