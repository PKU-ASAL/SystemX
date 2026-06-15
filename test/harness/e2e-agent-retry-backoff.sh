#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RESULTS="$ROOT/test/.results"
BIN="$ROOT/bin"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-agent-retry-backoff.XXXXXX")"
PORT_BASE="${PORT_BASE:-$((29000 + RANDOM % 5000))}"
MANAGER_PORT="${MANAGER_PORT:-$PORT_BASE}"
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"

mkdir -p "$RESULTS" "$BIN"

cleanup() {
  if [[ -n "${AGENT_PID:-}" ]]; then kill "$AGENT_PID" 2>/dev/null || true; fi
  if [[ -n "${MGR_PID:-}" ]]; then kill "$MGR_PID" 2>/dev/null || true; fi
  rm -rf "$TMP"
}
trap cleanup EXIT

echo "[e2e-agent-retry-backoff] building binaries"
make -C "$ROOT" build >/dev/null

cat > "$TMP/policy.yaml" <<'POLICY'
kinds: [EXEC]
POLICY

cat > "$TMP/agent.yaml" <<EOF
agent:
  id: e2e-agent-retry-backoff
  host_id: e2e-host
  tenant_id: default
  token: $TOKEN

manager:
  address: http://127.0.0.1:$MANAGER_PORT
  transport: http

sensor:
  backend: fake
  mode: managed
  policy_path: $TMP/policy.yaml
  observe_only: true
  restart: always
  max_restarts: 1
  restart_window: 1h

spool:
  path: $TMP/spool
  max_bytes: 268435456
  batch_size: 256
  flush_interval: 5ms

upload:
  retry_initial: 50ms
  retry_max: 100ms
  request_timeout: 2s

health:
  interval: 100ms
EOF

python3 -u - "$TMP" "$MANAGER_PORT" "$TOKEN" <<'PY' &
import json
import pathlib
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

tmp = pathlib.Path(sys.argv[1])
port = int(sys.argv[2])
token = sys.argv[3]
upload_log = tmp / "upload.times"
health_body = tmp / "health.body.json"
upload_count = 0
success_count = 0
lock = threading.Lock()

class Handler(BaseHTTPRequestHandler):
    def log_message(self, fmt, *args):
        return

    def do_GET(self):
        if self.path == "/healthz":
            self.send_response(200)
            self.send_header("content-type", "application/json")
            self.end_headers()
            self.wfile.write(b'{"ok":true}')
            return
        if self.path == "/api/v1/metrics":
            with lock:
                count = success_count
            self.send_response(200)
            self.send_header("content-type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({"events_ingested": count}, separators=(",", ":")).encode())
            return
        if self.path.startswith("/api/v1/agent-health"):
            if health_body.exists():
                body = health_body.read_bytes()
            else:
                body = b'{}'
            self.send_response(200)
            self.send_header("content-type", "application/json")
            self.end_headers()
            self.wfile.write(body)
            return
        self.send_response(404)
        self.end_headers()

    def do_POST(self):
        global upload_count, success_count
        if self.path == "/api/v1/upload":
            if self.headers.get("X-SysArmor-Agent-Token") != token:
                self.send_response(401)
                self.end_headers()
                return
            _ = self.rfile.read(int(self.headers.get("content-length", "0")))
            now = time.time_ns()
            with lock:
                upload_count += 1
                attempt = upload_count
                upload_log.write_text(upload_log.read_text() + f"{now}\n" if upload_log.exists() else f"{now}\n")
            if attempt < 4:
                self.send_response(503)
                self.send_header("content-type", "application/json")
                self.end_headers()
                self.wfile.write(b'{"ok":false}')
                return
            with lock:
                success_count += 1
            self.send_response(200)
            self.send_header("content-type", "application/json")
            self.end_headers()
            self.wfile.write(b'{"ok":true}')
            return
        if self.path == "/api/v1/agent-health":
            if self.headers.get("X-SysArmor-Agent-Token") != token:
                self.send_response(401)
                self.end_headers()
                return
            length = int(self.headers.get("content-length", "0"))
            body = self.rfile.read(length)
            health_body.write_bytes(body)
            self.send_response(200)
            self.send_header("content-type", "application/json")
            self.end_headers()
            self.wfile.write(b'{"ok":true}')
            return
        self.send_response(404)
        self.end_headers()

httpd = ThreadingHTTPServer(("127.0.0.1", port), Handler)
httpd.serve_forever()
PY
MGR_PID=$!

wait_contains() {
  local name="$1"
  local url="$2"
  local needle="$3"
  local out="$4"
  local deadline=$((SECONDS + 10))
  until curl -sf "$url" >"$out" && grep -Fq "$needle" "$out"; do
    if (( SECONDS >= deadline )); then
      echo "[e2e-agent-retry-backoff][ERROR] timeout waiting for $needle via $name at $url" >&2
      echo "--- last response ---" >&2
      cat "$out" >&2 2>/dev/null || true
      echo "--- agent log ---" >&2
      cat "$TMP/agent.log" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 0.1
  done
}

wait_contains "manager healthz" "http://127.0.0.1:$MANAGER_PORT/healthz" '"ok":true' "$TMP/healthz.json"

"$BIN/sysarmor-agent" run --config "$TMP/agent.yaml" >"$TMP/agent.log" 2>&1 &
AGENT_PID=$!

wait_contains "agent-health ok" "http://127.0.0.1:$MANAGER_PORT/api/v1/agent-health?agent_id=e2e-agent-retry-backoff&tenant_id=default" '"status":"ok"' "$RESULTS/e2e-agent-retry-backoff.health.json"
wait_contains "metrics" "http://127.0.0.1:$MANAGER_PORT/api/v1/metrics" '"events_ingested":1' "$RESULTS/e2e-agent-retry-backoff.metrics.json"

if [[ ! -f "$TMP/upload.times" ]]; then
  echo "[e2e-agent-retry-backoff][ERROR] missing upload timing log" >&2
  cat "$TMP/agent.log" >&2
  exit 1
fi

mapfile -t TIMES < "$TMP/upload.times"
if [[ "${#TIMES[@]}" -lt 4 ]]; then
  echo "[e2e-agent-retry-backoff][ERROR] upload attempts = ${#TIMES[@]}, want at least 4" >&2
  cat "$TMP/upload.times" >&2
  exit 1
fi

DELTA_NS=$(( TIMES[3] - TIMES[0] ))
if [[ "$DELTA_NS" -lt 100000000 ]]; then
  echo "[e2e-agent-retry-backoff][ERROR] retry window too short: ${DELTA_NS}ns" >&2
  cat "$TMP/upload.times" >&2
  exit 1
fi

cp "$TMP/upload.times" "$RESULTS/e2e-agent-retry-backoff.upload.times"
cp "$TMP/agent.log" "$RESULTS/e2e-agent-retry-backoff.agent.log"
cp "$TMP/health.body.json" "$RESULTS/e2e-agent-retry-backoff.health.body.json"

echo "[e2e-agent-retry-backoff] ok"
