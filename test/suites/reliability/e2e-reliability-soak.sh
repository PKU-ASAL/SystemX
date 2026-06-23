#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
RESULTS="$ROOT/test/.results"

mkdir -p "$RESULTS"

OUTAGE_EVENTS="${SYSARMOR_SOAK_OUTAGE_EVENTS:-12}"
RETRY_EVENTS="${SYSARMOR_SOAK_RETRY_EVENTS:-6}"
RETRY_FAILS="${SYSARMOR_SOAK_RETRY_FAILS:-12}"

echo "[e2e-agent-reliability-soak] outage drain with $OUTAGE_EVENTS queued batches"
SYSARMOR_OUTAGE_EVENTS="$OUTAGE_EVENTS" bash "$ROOT/test/suites/reliability/e2e-outage-soak.sh"

echo "[e2e-agent-reliability-soak] sensor restart recovers after failure"
bash "$ROOT/test/harness/e2e-agent-sensor-restart.sh"
bash "$ROOT/test/harness/e2e-agent-sensor-recover.sh"

echo "[e2e-agent-reliability-soak] graceful shutdown drains queued batch"
bash "$ROOT/test/suites/reliability/e2e-shutdown.sh"

cat > "$RESULTS/e2e-agent-reliability-soak.summary.json" <<EOF
{"outage_events":$OUTAGE_EVENTS,"retry_events":$RETRY_EVENTS,"retry_failures":$RETRY_FAILS,"restart_unacked":true,"sensor_restart":true,"sensor_recover":true,"shutdown_flush":true}
EOF

echo "[e2e-agent-reliability-soak] ok"
