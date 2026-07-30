#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TMP="$(mktemp -d)"
WEB_PORT="${WEB_PORT:-23000}"
DOWNLOAD_PORT="${DOWNLOAD_PORT:-23080}"
CONTROL_PORT="${CONTROL_PORT:-23443}"
PIDS=()

cleanup() {
  local pid
  for pid in "${PIDS[@]}"; do
    kill "$pid" >/dev/null 2>&1 || true
    wait "$pid" 2>/dev/null || true
  done
  rm -rf "$TMP" /tmp/.sysarmor-attack/sysarmor-fixture-payload \
    /tmp/.sysarmor-attack/sysarmor-fixture-exec-connect
}
trap cleanup EXIT

wait_ready() {
  local url="$1" deadline=$((SECONDS + 10))
  until curl -fsS "$url" >/dev/null 2>&1; do
    (( SECONDS < deadline )) || return 1
    sleep 0.1
  done
}

CONTROL_HOST=127.0.0.1 DOWNLOAD_PORT="$DOWNLOAD_PORT" CONTROL_PORT="$CONTROL_PORT" \
  node "$HERE/payload-server/server.js" >"$TMP/payload-server.log" 2>&1 &
PIDS+=("$!")
ATTACK_HOST=127.0.0.1 WEB_PORT="$WEB_PORT" DOWNLOAD_PORT="$DOWNLOAD_PORT" CONTROL_PORT="$CONTROL_PORT" \
  node "$HERE/web-app/server.js" >"$TMP/web-app.log" 2>&1 &
PIDS+=("$!")

wait_ready "http://127.0.0.1:$DOWNLOAD_PORT/healthz"
wait_ready "http://127.0.0.1:$WEB_PORT/healthz"
curl -fsS "http://127.0.0.1:$WEB_PORT/healthz" | jq -e '.status == "ok"' >/dev/null

if curl -fsS "http://127.0.0.1:$WEB_PORT/rce?marker=bad%20marker" >/dev/null 2>&1; then
  echo "invalid marker accepted" >&2
  exit 1
fi

rce_response="$TMP/rce-response.json"
curl -fsS "http://127.0.0.1:$WEB_PORT/rce?marker=sysarmor-fixture-rce" >"$rce_response" &
rce_pid="$!"
rce_deadline=$((SECONDS + 2))
until pgrep -f 'sysarmor-rce sysarmor-fixture-rce' >/dev/null; do
  if (( SECONDS >= rce_deadline )); then
    wait "$rce_pid" || true
    echo "web runtime shell was not observable" >&2
    exit 1
  fi
  sleep 0.05
done
wait "$rce_pid"
jq -e '.status == "ok"' "$rce_response" >/dev/null
curl -fsS "http://127.0.0.1:$WEB_PORT/download?marker=sysarmor-fixture-download" | jq -e '.status == "ok"' >/dev/null
curl -fsS "http://127.0.0.1:$WEB_PORT/reverse-shell?marker=sysarmor-fixture-reverse" | jq -e '.status == "ok"' >/dev/null
curl -fsS "http://127.0.0.1:$WEB_PORT/exec-connect?marker=sysarmor-fixture-exec-connect" | jq -e '.status == "ok"' >/dev/null
curl -fsS "http://127.0.0.1:$WEB_PORT/payload?marker=sysarmor-fixture-payload" | jq -e '.status == "ok"' >/dev/null

grep -Fq 'sysarmor-fixture-download' "$TMP/payload-server.log"
grep -Fq 'sysarmor-fixture-payload' "$TMP/payload-server.log"
grep -Fq 'sysarmor-fixture-reverse' "$TMP/payload-server.log"
grep -Fq 'sysarmor-fixture-exec-connect' "$TMP/payload-server.log"
test -f /tmp/.sysarmor-attack/sysarmor-fixture-payload

echo "[release-fixtures] ok"
