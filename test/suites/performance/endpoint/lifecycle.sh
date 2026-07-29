#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../../.." && pwd)"
REPO="$(cd "$ROOT/.." && pwd)"
VM_ENV="${SYSARMOR_VM_ENV:-${ENV:-vm-endpoint}}"
ENVDIR="$(cd "$ROOT/environments/$VM_ENV" && pwd)"
RESULTS="$ROOT/.results"
RUN_ID="${SYSARMOR_BENCH_RUN_ID:-$(date -u +%Y%m%dT%H%M%SZ)}"
OUT_DIR="$RESULTS/bench-edr-lifecycle-vm/$RUN_ID"
REC_RUN_ID="bench-edr-lifecycle-vm/$RUN_ID"
REC_DIR="$RESULTS/recordings/$REC_RUN_ID"
AGENT_SOCK="${SYSARMOR_AGENT_SOCK:-/run/sysarmor/agent/control.sock}"
AGENT_ID=""
TENANT_ID=""
POLICY="${POLICY:-test/data/policies/collection-balanced.json}"
CONTENT_DIR="${SYSARMOR_BENCH_CONTENT_DIR:-test/data/content}"
WORKLOAD="${DIAG_SCENARIO:-edr-activity-heavy}"
BASELINE_SECONDS="${SYSARMOR_BENCH_BASELINE_SECONDS:-8}"
SETTLE_SECONDS="${SYSARMOR_BENCH_SETTLE_SECONDS:-8}"
STEADY_SECONDS="${SYSARMOR_BENCH_STEADY_SECONDS:-8}"
WORKLOAD_SECONDS="${SYSARMOR_BENCH_WORKLOAD_SECONDS:-12}"
WORKLOAD_REPEAT="${SYSARMOR_BENCH_WORKLOAD_REPEAT:-0}"
WORKLOAD_C2="${SYSARMOR_DIAG_WORKLOAD_C2:-10.66.0.99}"

mkdir -p "$OUT_DIR"

recorder() {
  RUN_ID="$REC_RUN_ID" \
    SYSARMOR_RECORDER_AGENT_ID="$AGENT_ID" \
    SYSARMOR_RECORDER_TENANT_ID="$TENANT_ID" \
    SYSARMOR_AGENT_SOCK="$AGENT_SOCK" \
    bash "$ROOT/shared/recorder/recorder-vm.sh" "$@"
}

mark() {
  PHASE="$1" DETAIL="${2:-}" recorder mark
}

wait_agent_socket() {
  local deadline=$((SECONDS + 90))
  cd "$ENVDIR"
  until vagrant ssh node-a -c "sudo test -S '$AGENT_SOCK'" >/dev/null 2>&1; do
    if (( SECONDS >= deadline )); then
      echo "[bench-edr-lifecycle-vm][ERROR] timeout waiting for agent socket: $AGENT_SOCK" >&2
      vagrant ssh node-a -c "sudo systemctl status sysarmor-agent --no-pager -l || true" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 1
  done
}

resolve_agent_identity() {
  local health
  health="$(vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json agent health")"
  AGENT_ID="$(jq -r '.agentId // .agent_id // empty' <<<"$health")"
  TENANT_ID="$(jq -r '.tenantId // .tenant_id // empty' <<<"$health")"
  if [[ -z "$AGENT_ID" || -z "$TENANT_ID" ]]; then
    echo "[bench-edr-lifecycle-vm][ERROR] Agent health did not expose runtime identity: $health" >&2
    exit 1
  fi
}

run_workload() {
  cd "$ENVDIR"
  if [[ -f "$ROOT/data/workloads/vm/$WORKLOAD/run.sh" ]]; then
    vagrant upload "$ROOT/data/workloads/vm/$WORKLOAD/run.sh" /tmp/sysarmor-workload-run.sh node-a >/dev/null
    vagrant ssh node-a -c "sudo bash -c 'DURATION=$WORKLOAD_SECONDS REPEAT=$WORKLOAD_REPEAT C2=$WORKLOAD_C2 bash /tmp/sysarmor-workload-run.sh'" \
      > "$OUT_DIR/workload.out" 2>"$OUT_DIR/workload.err" || true
  elif [[ -f "$ROOT/data/scenarios/vm/$WORKLOAD/attack.sh" ]]; then
    vagrant ssh node-a -c "sudo bash -c 'GAP=1 C2=$WORKLOAD_C2 bash /vagrant/test/data/scenarios/vm/$WORKLOAD/attack.sh'" \
      > "$OUT_DIR/workload.out" 2>"$OUT_DIR/workload.err" || true
  else
    echo "[bench-edr-lifecycle-vm][ERROR] workload not found: $WORKLOAD" >&2
    exit 1
  fi
}

echo "[bench-edr-lifecycle-vm] output: $OUT_DIR"
wait_agent_socket
resolve_agent_identity
SYSARMOR_RECORDER_DURATION=3600 recorder start
mark baseline_start
sleep "$BASELINE_SECONDS"

mark agent_restart_start
cd "$ENVDIR"
vagrant ssh node-a -c "sudo systemctl restart sysarmor-agent" >/dev/null
wait_agent_socket
mark agent_restart_done

mark content_apply_start
vagrant upload "$REPO/$CONTENT_DIR" /tmp/sysarmor-life-content node-a >/dev/null
for content in "$REPO/$CONTENT_DIR"/*.json; do
  name="$(basename "$content")"
  vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json content apply --file '/tmp/sysarmor-life-content/$name' --allow-unsigned --agent-id '$AGENT_ID' --tenant-id '$TENANT_ID'" \
    > "$OUT_DIR/content.$name.apply.json" 2>"$OUT_DIR/content.$name.apply.err" || {
      echo "[bench-edr-lifecycle-vm][ERROR] content apply failed: $name" >&2
      exit 1
    }
done
mark content_apply_done

mark policy_apply_start "$POLICY"
vagrant upload "$REPO/$POLICY" /tmp/sysarmor-life-policy node-a >/dev/null
vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json policy apply collection --file /tmp/sysarmor-life-policy --agent-id '$AGENT_ID' --tenant-id '$TENANT_ID' --timeout 60s" \
  > "$OUT_DIR/collection-apply.json" 2>"$OUT_DIR/collection-apply.err" || {
    echo "[bench-edr-lifecycle-vm][ERROR] policy apply failed" >&2
    cat "$OUT_DIR/collection-apply.err" >&2 2>/dev/null || true
    exit 1
  }
mark policy_apply_done "$POLICY"

mark settle_start
sleep "$SETTLE_SECONDS"
mark steady_start
sleep "$STEADY_SECONDS"
mark workload_start "$WORKLOAD"
run_workload
mark workload_done "$WORKLOAD"

recorder stop
recorder report

cp "$REC_DIR/timeline.csv" "$OUT_DIR/timeline.csv"
cp "$REC_DIR/markers.ndjson" "$OUT_DIR/markers.ndjson"
cp "$REC_DIR/summary.json" "$OUT_DIR/summary.json"

echo "[bench-edr-lifecycle-vm] summary written to $OUT_DIR/summary.json"
