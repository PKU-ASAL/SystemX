#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
RESULTS="$ROOT/.results"
VM_ENV="${SYSARMOR_VM_ENV:-${ENV:-vm-endpoint}}"
RUN_ID="${SYSARMOR_BENCH_RUN_ID:-$(date -u +%Y%m%dT%H%M%SZ)}"
SCENARIO="${SCENARIO:-apt-fileless-c2}"
OUT_DIR="$RESULTS/bench-endpoint-e2e/$RUN_ID"
REC_RUN_ID="bench-endpoint-e2e/$RUN_ID"
REC_DIR="$RESULTS/recordings/$REC_RUN_ID"

mkdir -p "$OUT_DIR"

recorder() {
  RUN_ID="$REC_RUN_ID" SYSARMOR_VM_ENV="$VM_ENV" bash "$ROOT/shared/recorder/recorder-vm.sh" "$@"
}

mark() {
  PHASE="$1" DETAIL="${2:-}" recorder mark
}

echo "[bench-endpoint-e2e] output: $OUT_DIR env=$VM_ENV scenario=$SCENARIO"
SYSARMOR_RECORDER_DURATION=3600 recorder start

mark e2e_start "$SCENARIO"
SYSARMOR_VM_ENV="$VM_ENV" bash "$ROOT/e2e/local-agent/capture-vm.sh" "$SCENARIO" "${DUR:-30}" > "$OUT_DIR/e2e.out" 2>"$OUT_DIR/e2e.err"
mark e2e_done "$SCENARIO"

recorder stop
recorder report

cp "$REC_DIR/timeline.csv" "$OUT_DIR/timeline.csv"
cp "$REC_DIR/markers.ndjson" "$OUT_DIR/markers.ndjson"
cp "$REC_DIR/summary.json" "$OUT_DIR/summary.json"

echo "[bench-endpoint-e2e] summary written to $OUT_DIR/summary.json"
