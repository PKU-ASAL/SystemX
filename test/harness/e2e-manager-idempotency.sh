#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RESULTS="$ROOT/test/.results"
BIN="$ROOT/bin"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-manager-idempotency.XXXXXX")"
PORT_BASE="${PORT_BASE:-$((31000 + RANDOM % 5000))}"
MANAGER_PORT="${MANAGER_PORT:-$PORT_BASE}"
GRPC_PORT="${GRPC_PORT:-$((PORT_BASE + 1))}"
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
MGR_URL="http://127.0.0.1:$MANAGER_PORT"

mkdir -p "$RESULTS" "$BIN"

cleanup() {
  if [[ -n "${MGR_PID:-}" ]]; then kill "$MGR_PID" 2>/dev/null || true; fi
  rm -rf "$TMP"
}
trap cleanup EXIT

echo "[e2e-manager-idempotency] building binaries"
make -C "$ROOT" build >/dev/null

"$BIN/sysarmor-manager" \
  --listen "127.0.0.1:$MANAGER_PORT" \
  --grpc-listen "127.0.0.1:$GRPC_PORT" \
  --store "$TMP/store.json" \
  --dev-token "$TOKEN" \
  >"$TMP/manager.log" 2>&1 &
MGR_PID=$!

wait_contains() {
  local name="$1"
  local needle="$2"
  local out="$3"
  shift 3
  local deadline=$((SECONDS + 10))
  until "$@" >"$out" && grep -Fq "$needle" "$out"; do
    if (( SECONDS >= deadline )); then
      echo "[e2e-manager-idempotency][ERROR] timeout waiting for $needle via $name" >&2
      echo "--- last response ---" >&2
      cat "$out" >&2 2>/dev/null || true
      echo "--- manager log ---" >&2
      cat "$TMP/manager.log" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 0.1
  done
}

wait_contains "manager healthz" '"ok":true' "$TMP/healthz.json" curl -sf "$MGR_URL/healthz"

cat > "$TMP/batch.json" <<'JSON'
{
  "batch_id": "00000000000000000099",
  "agent": {
    "agent_id": "e2e-manager-idempotency",
    "host_id": "e2e-host",
    "tenant_id": "default",
    "version": "e2e"
  },
  "events": [
    {
      "id": "ev-idempotency",
      "agent_id": "e2e-manager-idempotency",
      "host_id": "e2e-host",
      "scenario": "apt-fileless-c2",
      "behavior": "process.exec",
      "lineage_id": "lin-idem"
    }
  ],
  "signals": [
    {
      "id": "sig-web-shell",
      "name": "web_runtime_spawns_shell",
      "where": "SIGNAL_WHERE_ENDPOINT",
      "base_risk": 50,
      "global_rarity": 1,
      "lineage_id": "lin-idem",
      "scenario": "apt-fileless-c2",
      "entities": [
        {"kind": "process", "key": "p-web", "role": "subject"}
      ]
    },
    {
      "id": "sig-payload",
      "name": "payload_dropped",
      "where": "SIGNAL_WHERE_ENDPOINT",
      "base_risk": 50,
      "global_rarity": 1,
      "lineage_id": "lin-idem",
      "scenario": "apt-fileless-c2",
      "entities": [
        {"kind": "file", "key": "file:/dev/shm/x.sh", "role": "object"}
      ]
    },
    {
      "id": "sig-reverse-shell",
      "name": "reverse_shell_pattern",
      "where": "SIGNAL_WHERE_ENDPOINT",
      "base_risk": 80,
      "global_rarity": 1,
      "lineage_id": "lin-idem",
      "terminal": true,
      "scenario": "apt-fileless-c2",
      "entities": [
        {"kind": "process", "key": "p-bash", "role": "subject"},
        {"kind": "socket", "key": "socket:10.66.0.99:443", "role": "object"}
      ]
    }
  ]
}
JSON

curl -sf \
  -H "content-type: application/json" \
  -H "X-SysArmor-Agent-Token: $TOKEN" \
  --data-binary "@$TMP/batch.json" \
  "$MGR_URL/api/v1/upload" \
  > "$RESULTS/e2e-manager-idempotency.ack.first.json"

curl -sf \
  -H "content-type: application/json" \
  -H "X-SysArmor-Agent-Token: $TOKEN" \
  --data-binary "@$TMP/batch.json" \
  "$MGR_URL/api/v1/upload" \
  > "$RESULTS/e2e-manager-idempotency.ack.second.json"

curl -sf "$MGR_URL/api/v1/metrics" > "$RESULTS/e2e-manager-idempotency.metrics.json"
curl -sf "$MGR_URL/api/v1/events?scenario=apt-fileless-c2" > "$RESULTS/e2e-manager-idempotency.events.json"
curl -sf "$MGR_URL/api/v1/signals?scenario=apt-fileless-c2&layer=endpoint" > "$RESULTS/e2e-manager-idempotency.endpoint-signals.json"
curl -sf "$MGR_URL/api/v1/signals?scenario=apt-fileless-c2&layer=cloud" > "$RESULTS/e2e-manager-idempotency.cloud-signals.json"
curl -sf "$MGR_URL/api/v1/incidents?scenario=apt-fileless-c2" > "$RESULTS/e2e-manager-idempotency.incidents.json"

python3 - "$RESULTS" <<'PY'
import json
import pathlib
import sys

results = pathlib.Path(sys.argv[1])
first = json.loads((results / "e2e-manager-idempotency.ack.first.json").read_text())
second = json.loads((results / "e2e-manager-idempotency.ack.second.json").read_text())
metrics = json.loads((results / "e2e-manager-idempotency.metrics.json").read_text())
events = json.loads((results / "e2e-manager-idempotency.events.json").read_text())
endpoint = json.loads((results / "e2e-manager-idempotency.endpoint-signals.json").read_text())
cloud = json.loads((results / "e2e-manager-idempotency.cloud-signals.json").read_text())
incidents = json.loads((results / "e2e-manager-idempotency.incidents.json").read_text())

def as_int(value):
    if value is None:
        return 0
    if isinstance(value, int):
        return value
    if isinstance(value, str) and value.isdigit():
        return int(value)
    return value

if first.get("batch_id") != "00000000000000000099":
    raise SystemExit(f"first ack batch_id mismatch: {first}")
if as_int(first.get("accepted_events")) != 1 or as_int(first.get("accepted_signals")) != 3:
    raise SystemExit(f"first ack should accept one event and three signals: {first}")
if second.get("batch_id") != "00000000000000000099":
    raise SystemExit(f"second ack batch_id mismatch: {second}")
if as_int(second.get("accepted_events")) != 0 or as_int(second.get("accepted_signals")) != 0:
    raise SystemExit(f"retry ack should accept zero new records: {second}")
want_metrics = {
    "upload_batches": 2,
    "events_ingested": 1,
    "endpoint_signals_ingested": 3,
    "cloud_signals_emitted": 2,
    "signals_emitted": 5,
    "incidents_created": 1,
}
for key, want in want_metrics.items():
    if metrics.get(key) != want:
        raise SystemExit(f"metric {key}={metrics.get(key)}, want {want}: {metrics}")
if len(events) != 1:
    raise SystemExit(f"events count={len(events)}, want 1: {events}")
if len(endpoint) != 3:
    raise SystemExit(f"endpoint signal count={len(endpoint)}, want 3: {endpoint}")
if len(cloud) != 2:
    raise SystemExit(f"cloud signal count={len(cloud)}, want 2: {cloud}")
if len(incidents) != 1:
    raise SystemExit(f"incident count={len(incidents)}, want 1: {incidents}")
PY

cp "$TMP/manager.log" "$RESULTS/e2e-manager-idempotency.manager.log"

echo "[e2e-manager-idempotency] ok"
