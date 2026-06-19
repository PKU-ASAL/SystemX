#!/usr/bin/env bash
set -euo pipefail

REPEAT="${REPEAT:-1}"
COUNT="${COUNT:-80}"

mkdir -p /tmp/sysarmor-workload-secrets /run/secrets
printf "sysarmor-secret\n" >/tmp/sysarmor-workload-secrets/token
printf "sysarmor-secret\n" >/run/secrets/sysarmor-token 2>/dev/null || true

for i in $(seq 1 "$REPEAT"); do
  echo "[workload] file-read-storm iteration=$i count=$COUNT"
  for _ in $(seq 1 "$COUNT"); do
    cat /etc/passwd >/dev/null 2>&1 || true
    cat /tmp/sysarmor-workload-secrets/token >/dev/null 2>&1 || true
    cat /run/secrets/sysarmor-token >/dev/null 2>&1 || true
  done
done
