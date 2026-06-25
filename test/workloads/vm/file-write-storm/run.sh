#!/usr/bin/env bash
set -euo pipefail

REPEAT="${REPEAT:-1}"
COUNT="${COUNT:-80}"

mkdir -p /tmp/sysarmor-workload/files

for i in $(seq 1 "$REPEAT"); do
  echo "[workload] file-write-storm iteration=$i count=$COUNT"
  for n in $(seq 1 "$COUNT"); do
    printf "sysarmor-workload-%s-%s\n" "$i" "$n" > "/tmp/sysarmor-workload/files/artifact-$n.txt"
    cp "/tmp/sysarmor-workload/files/artifact-$n.txt" "/tmp/sysarmor-workload/files/copy-$n.txt" 2>/dev/null || true
  done
done
