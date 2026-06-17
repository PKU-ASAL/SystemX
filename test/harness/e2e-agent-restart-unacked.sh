#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RESULTS="$ROOT/test/.results"
BIN="$ROOT/bin"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-agent-restart-unacked.XXXXXX")"
PORT_BASE="${PORT_BASE:-$((30000 + RANDOM % 5000))}"
MANAGER_PORT="${MANAGER_PORT:-$PORT_BASE}"
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"

mkdir -p "$RESULTS" "$BIN"

cleanup() {
  if [[ -n "${AGENT_PID:-}" ]]; then kill "$AGENT_PID" 2>/dev/null || true; fi
  if [[ -n "${MGR_PID:-}" ]]; then kill "$MGR_PID" 2>/dev/null || true; fi
  rm -rf "$TMP"
}
trap cleanup EXIT

echo "[e2e-agent-restart-unacked] building binaries"
make -C "$ROOT" build >/dev/null

cat > "$TMP/policy.yaml" <<'POLICY'
{"behaviors":["process.exec"],"observe_only":true}
POLICY

cat > "$TMP/agent.yaml" <<EOF
agent:
  id: e2e-agent-restart-unacked
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
  retry_initial: 1h
  retry_max: 1h
  request_timeout: 2s

health:
  interval: 1h
EOF

python3 -u - "$TMP" "$MANAGER_PORT" "$TOKEN" <<'PY' &
import json
import pathlib
import sys
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

tmp = pathlib.Path(sys.argv[1])
port = int(sys.argv[2])
token = sys.argv[3]
upload_log = tmp / "uploads.jsonl"
health_body = tmp / "health.body.json"
lock = threading.Lock()
upload_count = 0

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
        self.send_response(404)
        self.end_headers()

    def do_POST(self):
        global upload_count
        if self.headers.get("X-SysArmor-Agent-Token") != token:
            self.send_response(401)
            self.end_headers()
            return
        length = int(self.headers.get("content-length", "0"))
        body = self.rfile.read(length)
        if self.path == "/api/v1/agent-health":
            health_body.write_bytes(body)
            self.send_response(200)
            self.send_header("content-type", "application/json")
            self.end_headers()
            self.wfile.write(b'{"ok":true}')
            return
        if self.path != "/api/v1/upload":
            self.send_response(404)
            self.end_headers()
            return
        batch = json.loads(body.decode())
        with lock:
            upload_count += 1
            attempt = upload_count
            with upload_log.open("a", encoding="utf-8") as f:
                f.write(json.dumps({
                    "attempt": attempt,
                    "batch_id": batch.get("batch_id", ""),
                    "events": len(batch.get("events", [])),
                    "signals": len(batch.get("endpoint_signals", [])),
                }, separators=(",", ":")) + "\n")
        ack_batch_id = batch.get("batch_id", "")
        if attempt == 1:
            ack_batch_id = "wrong-" + ack_batch_id
        self.send_response(200)
        self.send_header("content-type", "application/json")
        self.end_headers()
        self.wfile.write(json.dumps({"ok": True, "batch_id": ack_batch_id}, separators=(",", ":")).encode())

httpd = ThreadingHTTPServer(("127.0.0.1", port), Handler)
httpd.serve_forever()
PY
MGR_PID=$!

wait_for_file_lines() {
  local file="$1"
  local want="$2"
  local deadline=$((SECONDS + 10))
  until [[ -f "$file" ]] && [[ "$(wc -l < "$file")" -ge "$want" ]]; do
    if (( SECONDS >= deadline )); then
      echo "[e2e-agent-restart-unacked][ERROR] timeout waiting for $want lines in $file" >&2
      echo "--- uploads ---" >&2
      cat "$file" >&2 2>/dev/null || true
      echo "--- agent first log ---" >&2
      cat "$TMP/agent.first.log" >&2 2>/dev/null || true
      echo "--- agent second log ---" >&2
      cat "$TMP/agent.second.log" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 0.1
  done
}

wait_for_healthz() {
  local deadline=$((SECONDS + 10))
  until curl -sf "http://127.0.0.1:$MANAGER_PORT/healthz" >"$TMP/healthz.json"; do
    if (( SECONDS >= deadline )); then
      echo "[e2e-agent-restart-unacked][ERROR] timeout waiting for healthz" >&2
      exit 1
    fi
    sleep 0.1
  done
}

wait_for_healthz

"$BIN/sysarmor-agent" run --config "$TMP/agent.yaml" >"$TMP/agent.first.log" 2>&1 &
AGENT_PID=$!

wait_for_file_lines "$TMP/uploads.jsonl" 1

if ! compgen -G "$TMP/spool/*.batch.json" >/dev/null; then
  echo "[e2e-agent-restart-unacked][ERROR] spool batch was deleted after mismatched ack" >&2
  cat "$TMP/uploads.jsonl" >&2
  cat "$TMP/agent.first.log" >&2
  exit 1
fi

cp "$TMP"/spool/*.batch.json "$RESULTS/e2e-agent-restart-unacked.before-restart.batch.json"

# Simulate an ungraceful agent restart. SIGTERM would run shutdown drain and
# could legitimately ack/delete the batch before the restart path is exercised.
kill -KILL "$AGENT_PID"
set +e
wait "$AGENT_PID" 2>/dev/null
set -e
AGENT_PID=""

"$BIN/sysarmor-agent" run --config "$TMP/agent.yaml" >"$TMP/agent.second.log" 2>&1 &
AGENT_PID=$!

wait_for_file_lines "$TMP/uploads.jsonl" 2

deadline=$((SECONDS + 10))
while compgen -G "$TMP/spool/*.batch.json" >/dev/null; do
  if (( SECONDS >= deadline )); then
    echo "[e2e-agent-restart-unacked][ERROR] spool did not drain after restart and valid ack" >&2
    ls -la "$TMP/spool" >&2
    cat "$TMP/uploads.jsonl" >&2
    exit 1
  fi
  sleep 0.1
done

python3 - "$TMP/uploads.jsonl" "$RESULTS/e2e-agent-restart-unacked.uploads.jsonl" <<'PY'
import json
import pathlib
import sys

src = pathlib.Path(sys.argv[1])
dst = pathlib.Path(sys.argv[2])
rows = [json.loads(line) for line in src.read_text().splitlines() if line.strip()]
if len(rows) < 2:
    raise SystemExit(f"upload attempts = {len(rows)}, want at least 2")
if rows[0]["batch_id"] == "" or rows[0]["batch_id"] != rows[1]["batch_id"]:
    raise SystemExit(f"batch ids differ across restart: {rows}")
if rows[0]["events"] != 1 or rows[1]["events"] != 1:
    raise SystemExit(f"unexpected event counts: {rows}")
dst.write_text(src.read_text())
PY

cp "$TMP/agent.first.log" "$RESULTS/e2e-agent-restart-unacked.agent.first.log"
cp "$TMP/agent.second.log" "$RESULTS/e2e-agent-restart-unacked.agent.second.log"
[[ -f "$TMP/health.body.json" ]] && cp "$TMP/health.body.json" "$RESULTS/e2e-agent-restart-unacked.health.body.json"

echo "[e2e-agent-restart-unacked] ok"
