#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RESULTS="$ROOT/test/.results"

mkdir -p "$RESULTS"

OUTAGE_EVENTS="${SYSARMOR_SOAK_OUTAGE_EVENTS:-12}"
RETRY_EVENTS="${SYSARMOR_SOAK_RETRY_EVENTS:-6}"
RETRY_FAILS="${SYSARMOR_SOAK_RETRY_FAILS:-12}"

echo "[e2e-agent-reliability-soak] outage drain with $OUTAGE_EVENTS queued batches"
SYSARMOR_OUTAGE_EVENTS="$OUTAGE_EVENTS" bash "$ROOT/test/harness/e2e-agent-outage-soak.sh"

echo "[e2e-agent-reliability-soak] retry/backoff with $RETRY_EVENTS batches and $RETRY_FAILS transient failures"
SYSARMOR_RETRY_EVENTS="$RETRY_EVENTS" SYSARMOR_RETRY_FAILS="$RETRY_FAILS" bash "$ROOT/test/harness/e2e-agent-retry-backoff.sh"

echo "[e2e-agent-reliability-soak] restart preserves unacked batch"
bash "$ROOT/test/harness/e2e-agent-restart-unacked.sh"

echo "[e2e-agent-reliability-soak] graceful shutdown drains queued batch"
bash "$ROOT/test/harness/e2e-agent-shutdown.sh"

cat > "$RESULTS/e2e-agent-reliability-soak.summary.json" <<EOF
{"outage_events":$OUTAGE_EVENTS,"retry_events":$RETRY_EVENTS,"retry_failures":$RETRY_FAILS,"restart_unacked":true,"shutdown_flush":true}
EOF

echo "[e2e-agent-reliability-soak] ok"
