#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../../harness/lib/common.sh"

sa_init_repo_paths
TMP="$(sa_make_tmp sysarmor-manager-idempotency)"
sa_pick_ports 31000 5000
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
SA_TEST_NAME="e2e-manager-idempotency"
SA_WAIT_LOGS=("$TMP/manager.log")

cleanup() {
  sa_kill_pid_ref MGR_PID
  sa_cleanup_tmp "$TMP"
}
trap cleanup EXIT

echo "[e2e-manager-idempotency] building binaries"
sa_build_go_bins sysarmor-manager sysarmor-databatch-append sysarmorctl

sa_start_memory_manager --local-ingest --dev-token "$TOKEN"

sa_wait_url_contains "$MGR_URL/healthz" '"ok":true' "$TMP/healthz.json"

cat > "$TMP/batch.json" <<'JSON'
{
  "header": {
    "batchId": "00000000000000000099",
    "agentId": "e2e-manager-idempotency",
    "hostId": "e2e-host",
    "tenantId": "default",
    "eventCount": 1,
    "signalCount": 3,
    "labels": {"agent_version": "e2e"}
  },
  "events": [
    {
      "sequence": 1,
      "event": {
        "id": "ev-idempotency",
        "agentId": "e2e-manager-idempotency",
        "hostId": "e2e-host",
        "tenantId": "default",
        "scenario": "apt-fileless-c2",
        "behavior": "process.exec",
        "lineageId": "lin-idem"
      }
    }
  ],
  "signals": [
    {
      "sequence": 1,
      "signal": {
        "id": "sig-web-shell",
        "name": "web_runtime_spawns_shell",
        "where": "SIGNAL_WHERE_ENDPOINT",
        "baseRisk": 50,
        "globalRarity": 1,
        "lineageId": "lin-idem",
        "scenario": "apt-fileless-c2",
        "entities": [{"kind": "process", "key": "p-web", "role": "subject"}]
      }
    },
    {
      "sequence": 2,
      "signal": {
        "id": "sig-payload",
        "name": "payload_dropped",
        "where": "SIGNAL_WHERE_ENDPOINT",
        "baseRisk": 50,
        "globalRarity": 1,
        "lineageId": "lin-idem",
        "scenario": "apt-fileless-c2",
        "entities": [{"kind": "file", "key": "file:/dev/shm/x.sh", "role": "object"}]
      }
    },
    {
      "sequence": 3,
      "signal": {
        "id": "sig-reverse-shell",
        "name": "reverse_shell_pattern",
        "where": "SIGNAL_WHERE_ENDPOINT",
        "baseRisk": 80,
        "globalRarity": 1,
        "lineageId": "lin-idem",
        "terminal": true,
        "scenario": "apt-fileless-c2",
        "entities": [
          {"kind": "process", "key": "p-bash", "role": "subject"},
          {"kind": "socket", "key": "socket:10.66.0.99:443", "role": "object"}
        ]
      }
    }
  ]
}
JSON

"$BIN/sysarmor-databatch-append" --manager "127.0.0.1:$GRPC_PORT" --token "$TOKEN" --input "$TMP/batch.json" > "$RESULTS/e2e-manager-idempotency.ack.first.json"

"$BIN/sysarmor-databatch-append" --manager "127.0.0.1:$GRPC_PORT" --token "$TOKEN" --input "$TMP/batch.json" > "$RESULTS/e2e-manager-idempotency.ack.second.json"

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
    "data_batches_appended": 1,
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
