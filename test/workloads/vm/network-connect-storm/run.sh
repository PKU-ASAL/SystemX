#!/usr/bin/env bash
set -euo pipefail

REPEAT="${REPEAT:-1}"
COUNT="${COUNT:-40}"
C2="${C2:-10.66.0.99}"

for i in $(seq 1 "$REPEAT"); do
  echo "[workload] network-connect-storm iteration=$i count=$COUNT"
  for _ in $(seq 1 "$COUNT"); do
    timeout 1 bash -c "cat < /dev/null > /dev/tcp/127.0.0.1/9" >/dev/null 2>&1 || true
    timeout 1 bash -c "cat < /dev/null > /dev/tcp/$C2/443" >/dev/null 2>&1 || true
  done
done
