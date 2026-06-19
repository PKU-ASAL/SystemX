#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
REPO="$(cd "$ROOT/.." && pwd)"
ENVDIR="$(cd "$ROOT/env/vm" && pwd)"
RESULTS="$ROOT/.results"
RUN_ID="${SYSARMOR_BENCH_RUN_ID:-$(date -u +%Y%m%dT%H%M%SZ)}"
OUT_DIR="$RESULTS/bench-collection-vm/$RUN_ID"
AGENT_SOCK="${SYSARMOR_AGENT_SOCK:-/var/run/sysarmor/agent.sock}"
AGENT_ID="${SYSARMOR_BENCH_AGENT_ID:-vm-owned-tetragon}"
TENANT_ID="${SYSARMOR_BENCH_TENANT_ID:-default}"
WORKLOAD="${DIAG_SCENARIO:-${SYSARMOR_BENCH_WORKLOAD:-mixed-edr-storm}}"
POLICIES_RAW="${POLICIES:-test/policies/collection-minimal-high-signal.json test/policies/collection-edr-balanced.json test/policies/collection-incident-deep.json test/policies/collection-debug-wide.json}"
CONTENT_DIR="${SYSARMOR_BENCH_CONTENT_DIR:-test/content}"
BASELINE_SECONDS="${SYSARMOR_BENCH_BASELINE_SECONDS:-5}"
SETTLE_SECONDS="${SYSARMOR_BENCH_SETTLE_SECONDS:-8}"
STEADY_SECONDS="${SYSARMOR_BENCH_STEADY_SECONDS:-8}"
WORKLOAD_SECONDS="${SYSARMOR_BENCH_WORKLOAD_SECONDS:-12}"
WORKLOAD_REPEAT="${SYSARMOR_BENCH_WORKLOAD_REPEAT:-1}"
WORKLOAD_C2="${SYSARMOR_DIAG_WORKLOAD_C2:-10.66.0.99}"

mkdir -p "$OUT_DIR"

cd "$ENVDIR"

wait_agent_socket() {
  local deadline=$((SECONDS + 60))
  until vagrant ssh node-a -c "sudo test -S '$AGENT_SOCK'" >/dev/null 2>&1; do
    if (( SECONDS >= deadline )); then
      echo "[bench-collection-vm][ERROR] timeout waiting for agent socket: $AGENT_SOCK" >&2
      vagrant ssh node-a -c "sudo systemctl status sysarmor-agent --no-pager -l || true" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 1
  done
}

set_agent_labels() {
  local bench_run="$1"
  local policy_name="$2"
  local workload="$3"
  local labels_json
  labels_json="$(python3 -c 'import json,sys; print(json.dumps({"benchmark_run":sys.argv[1],"workload":sys.argv[2],"policy_profile":sys.argv[3]}))' "$bench_run" "$workload" "$policy_name")"
  vagrant ssh node-a -c "sudo SYSARMOR_LABELS_JSON='$labels_json' python3 -c '
import json
import os
from pathlib import Path
p = Path(\"/etc/sysarmor/agent.yaml\")
lines = p.read_text().splitlines()
labels = json.loads(os.environ[\"SYSARMOR_LABELS_JSON\"])
out = []
for line in lines:
    stripped = line.strip()
    if stripped.startswith(\"label.benchmark_run:\") or stripped.startswith(\"label.workload:\") or stripped.startswith(\"label.policy_profile:\"):
        continue
    out.append(line)
next_section = next((i for i, line in enumerate(out) if line and not line.startswith(\" \") and line.strip().endswith(\":\") and line.strip() != \"agent:\"), len(out))
try:
    agent_idx = next(i for i, line in enumerate(out) if line.strip() == \"agent:\")
except StopIteration:
    out.insert(0, \"agent:\")
    agent_idx = 0
    next_section = 1
insert = [
    \"  label.benchmark_run: \" + labels[\"benchmark_run\"],
    \"  label.workload: \" + labels[\"workload\"],
    \"  label.policy_profile: \" + labels[\"policy_profile\"],
]
next_section = next((i for i in range(agent_idx + 1, len(out)) if out[i] and not out[i].startswith(\" \") and out[i].strip().endswith(\":\")), len(out))
out[next_section:next_section] = insert
p.write_text(\"\\n\".join(out) + \"\\n\")
'
sudo systemctl restart sysarmor-agent" >/dev/null
  wait_agent_socket
}

policy_name() {
  local file="$1"
  basename "$file" | sed -E 's/\.(json|yaml|yml)$//'
}

recorder() {
  local rec_run_id="$1"
  local labels="${2:-}"
  shift
  shift || true
  RUN_ID="$rec_run_id" \
    SYSARMOR_RECORDER_AGENT_ID="$AGENT_ID" \
    SYSARMOR_RECORDER_TENANT_ID="$TENANT_ID" \
    SYSARMOR_AGENT_SOCK="$AGENT_SOCK" \
    SYSARMOR_RECORDER_LABELS="$labels" \
    bash "$ROOT/tools/recorder/recorder-vm.sh" "$@"
}

mark() {
  local rec_run_id="$1"
  local phase="$2"
  local detail="${3:-}"
  PHASE="$phase" DETAIL="$detail" RUN_ID="$rec_run_id" bash "$ROOT/tools/recorder/recorder-vm.sh" mark
}

run_workload() {
  local policy_out="$1"
  cd "$ENVDIR"
  if [[ -f "$ROOT/workloads/vm/$WORKLOAD/run.sh" ]]; then
    vagrant upload "$ROOT/workloads/vm/$WORKLOAD/run.sh" /tmp/sysarmor-workload-run.sh node-a >/dev/null
    vagrant ssh node-a -c "sudo bash -c 'DURATION=$WORKLOAD_SECONDS REPEAT=$WORKLOAD_REPEAT C2=$WORKLOAD_C2 bash /tmp/sysarmor-workload-run.sh'" \
      > "$policy_out/workload.out" 2>"$policy_out/workload.err" || true
  elif [[ -f "$ROOT/scenarios/vm/$WORKLOAD/attack.sh" ]]; then
    vagrant ssh node-a -c "sudo bash -c 'GAP=1 C2=$WORKLOAD_C2 bash /vagrant/test/scenarios/vm/$WORKLOAD/attack.sh'" \
      > "$policy_out/workload.out" 2>"$policy_out/workload.err" || true
  else
    echo "[bench-collection-vm][ERROR] workload not found: $WORKLOAD" >&2
    exit 1
  fi
}

echo "[bench-collection-vm] output: $OUT_DIR"
wait_agent_socket

echo "[bench-collection-vm] uploading content packs and policies"
vagrant upload "$REPO/$CONTENT_DIR" /tmp/sysarmor-bench-content node-a >/dev/null

for content in "$REPO/$CONTENT_DIR"/*.json; do
  name="$(basename "$content")"
  vagrant ssh node-a -c "sudo sysarmorctl --agent-sock '$AGENT_SOCK' --json content apply --file '/tmp/sysarmor-bench-content/$name' --allow-unsigned --agent-id '$AGENT_ID' --tenant-id '$TENANT_ID'" \
    > "$OUT_DIR/content.$name.apply.json" \
    2>"$OUT_DIR/content.$name.apply.err" || {
      echo "[bench-collection-vm][ERROR] content apply failed: $name" >&2
      cat "$OUT_DIR/content.$name.apply.err" >&2 2>/dev/null || true
      exit 1
    }
done

for policy in $POLICIES_RAW; do
  if [[ ! -f "$REPO/$policy" ]]; then
    echo "[bench-collection-vm][ERROR] policy not found: $policy" >&2
    exit 1
  fi
  name="$(policy_name "$policy")"
  policy_out="$OUT_DIR/$name"
  rec_run_id="bench-collection-vm/$RUN_ID/$name"
  rec_dir="$RESULTS/recordings/$rec_run_id"
  rec_labels="benchmark_run=$RUN_ID,workload=$WORKLOAD,policy_profile=$name"
  mkdir -p "$policy_out"

  echo "[bench-collection-vm] recording policy=$name workload=$WORKLOAD"
  set_agent_labels "$RUN_ID" "$name" "$WORKLOAD"
  SYSARMOR_RECORDER_DURATION=3600 recorder "$rec_run_id" "$rec_labels" start
  mark "$rec_run_id" baseline_start "$name"
  sleep "$BASELINE_SECONDS"

  echo "[bench-collection-vm] applying policy: $policy"
  mark "$rec_run_id" policy_apply_start "$policy"
  vagrant upload "$REPO/$policy" "/tmp/sysarmor-bench-$name.policy" node-a >/dev/null
  vagrant ssh node-a -c "sudo sysarmorctl --agent-sock '$AGENT_SOCK' --json policy apply collection --file '/tmp/sysarmor-bench-$name.policy' --agent-id '$AGENT_ID' --tenant-id '$TENANT_ID' --timeout 60s" \
    > "$policy_out/collection-apply.json" \
    2>"$policy_out/collection-apply.err" || {
      echo "[bench-collection-vm][ERROR] policy apply failed: $policy" >&2
      cat "$policy_out/collection-apply.err" >&2 2>/dev/null || true
      exit 1
    }
  mark "$rec_run_id" policy_apply_done "$policy"
  if ! grep -Fq 'resolved_refs' "$policy_out/collection-apply.json"; then
    echo "[bench-collection-vm][ERROR] policy apply did not report resolved refs: $policy" >&2
    cat "$policy_out/collection-apply.json" >&2 2>/dev/null || true
    exit 1
  fi

  mark "$rec_run_id" settle_start "$name"
  sleep "$SETTLE_SECONDS"
  mark "$rec_run_id" steady_start "$name"
  sleep "$STEADY_SECONDS"
  mark "$rec_run_id" workload_start "$WORKLOAD"
  run_workload "$policy_out"
  sleep "$WORKLOAD_SECONDS"
  mark "$rec_run_id" workload_done "$WORKLOAD"

  recorder "$rec_run_id" "$rec_labels" stop
  recorder "$rec_run_id" "$rec_labels" report

  cp "$rec_dir/timeline.csv" "$policy_out/timeline.csv"
  cp "$rec_dir/markers.ndjson" "$policy_out/markers.ndjson"
  cp "$rec_dir/summary.json" "$policy_out/summary.json"
done

python3 "$HERE/bench_collection_report.py" "$OUT_DIR"

echo "[bench-collection-vm] matrix written to $OUT_DIR/matrix.csv and $OUT_DIR/matrix.json"
