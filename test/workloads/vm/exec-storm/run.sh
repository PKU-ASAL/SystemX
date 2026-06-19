#!/usr/bin/env bash
set -euo pipefail

REPEAT="${REPEAT:-1}"
COUNT="${COUNT:-80}"

for i in $(seq 1 "$REPEAT"); do
  echo "[workload] exec-storm iteration=$i count=$COUNT"
  for _ in $(seq 1 "$COUNT"); do
    /bin/true
    /bin/echo sysarmor-workload-exec >/dev/null
    /usr/bin/env true
  done
done
