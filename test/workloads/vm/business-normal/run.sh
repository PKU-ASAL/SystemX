#!/usr/bin/env bash
set -euo pipefail

DURATION="${DURATION:-30}"
REPEAT="${REPEAT:-1}"
COUNT="${COUNT:-20}"

root=/var/tmp/sysarmor-workload
mkdir -p "$root/src" "$root/out" "$root/cache"

start_ts="$(date +%s)"
iter=0
while :; do
  iter=$((iter + 1))
  echo "[workload] business-normal iteration=$iter count=$COUNT duration=$DURATION"
  for n in $(seq 1 "$COUNT"); do
    printf "build-input-%s-%s\n" "$iter" "$n" > "$root/src/input-$n.txt"
    cp "$root/src/input-$n.txt" "$root/cache/cache-$n.txt"
    sha256sum "$root/cache/cache-$n.txt" > "$root/out/artifact-$n.sha256"
    cat "$root/out/artifact-$n.sha256" >/dev/null
    /bin/true
  done
  if (( iter % 3 == 0 )); then
    date > "$root/out/rotation.log"
    cat /etc/hostname >> "$root/out/rotation.log" 2>/dev/null || true
  fi
  if [ "$REPEAT" != "0" ] && [ "$iter" -ge "$REPEAT" ]; then
    break
  fi
  now="$(date +%s)"
  if [ "$DURATION" != "0" ] && [ $((now - start_ts)) -ge "$DURATION" ]; then
    break
  fi
  sleep 1
done
