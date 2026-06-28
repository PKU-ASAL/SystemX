#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
REPO="$(cd "$ROOT/.." && pwd)"
ENVDIR="$(cd "$ROOT/environments/vm" && pwd)"
RESULTS="$ROOT/.results"
RUN_ID="${SYSARMOR_BENCH_RUN_ID:-$(date -u +%Y%m%dT%H%M%SZ)}"
OUT_DIR="$RESULTS/bench-collection-vm/$RUN_ID"
AGENT_SOCK="${SYSARMOR_AGENT_SOCK:-/var/run/sysarmor/agent.sock}"
AGENT_ID="${SYSARMOR_BENCH_AGENT_ID:-vm-owned-tetragon}"
TENANT_ID="${SYSARMOR_BENCH_TENANT_ID:-default}"
if [[ -v SYSARMOR_BENCH_WORKLOAD ]]; then
  WORKLOAD="$SYSARMOR_BENCH_WORKLOAD"
else
  WORKLOAD="${DIAG_SCENARIO:-edr-activity-heavy}"
fi
SCENARIO="${SYSARMOR_BENCH_SCENARIO:-}"
VARIANT="${SYSARMOR_BENCH_VARIANT:-}"
MATCHER_STRATEGY="${SYSARMOR_BENCH_MATCHER_STRATEGY:-${SYSARMOR_TEST_MATCHER_STRATEGY:-}}"
POLICIES_RAW="${POLICIES:-test/data/policies/collection-minimal.json test/data/policies/collection-balanced.json test/data/policies/collection-deep.json}"
CONTENT_DIR="${SYSARMOR_BENCH_CONTENT_DIR:-test/data/content}"
DETECTION_POLICY="${SYSARMOR_BENCH_DETECTION_POLICY:-test/data/policies/detection-cep-endpoint.json}"
APPLY_DETECTION="${SYSARMOR_BENCH_APPLY_DETECTION:-1}"
BASELINE_SECONDS="${SYSARMOR_BENCH_BASELINE_SECONDS:-3}"
SETTLE_SECONDS="${SYSARMOR_BENCH_SETTLE_SECONDS:-8}"
STEADY_SECONDS="${SYSARMOR_BENCH_STEADY_SECONDS:-4}"
POLICY_SETTLE_SECONDS="${SYSARMOR_BENCH_POLICY_SETTLE_SECONDS:-10}"
WORKLOAD_SECONDS="${SYSARMOR_BENCH_WORKLOAD_SECONDS:-10}"
WORKLOAD_WARMUP_SECONDS="${SYSARMOR_BENCH_WORKLOAD_WARMUP_SECONDS:-2}"
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
  local workload_name="${3:-}"
  local scenario_name="${4:-}"
  local labels_json
  local labels_b64
labels_json="$(python3 -c '
import json,sys
bench_run, workload_name, policy_name, scenario_name, variant, matcher_strategy = sys.argv[1:7]
labels = {"benchmark_run": bench_run, "policy_profile": policy_name}
if workload_name:
    labels["workload"] = workload_name
if scenario_name:
    labels["scenario"] = scenario_name
if variant:
    labels["variant"] = variant
if matcher_strategy:
    labels["matcher_strategy"] = matcher_strategy
print(json.dumps(labels))
' "$bench_run" "$workload_name" "$policy_name" "$scenario_name" "$VARIANT" "$MATCHER_STRATEGY")"
  labels_b64="$(printf '%s' "$labels_json" | base64 -w0)"
  vagrant ssh node-a -c "sudo SYSARMOR_LABELS_B64='$labels_b64' python3 - <<'PY'
import base64
import json
import os
from pathlib import Path
p = Path('/etc/sysarmor/agent.yaml')
lines = p.read_text().splitlines()
labels = json.loads(base64.b64decode(os.environ['SYSARMOR_LABELS_B64']).decode())
out = []
managed_prefixes = (
    'scenario:',
    'label.benchmark_run:',
    'label.workload:',
    'label.scenario:',
    'label.policy_profile:',
    'label.variant:',
    'label.matcher_strategy:',
)
for line in lines:
    stripped = line.strip()
    if stripped.startswith(managed_prefixes):
        continue
    out.append(line)
next_section = next((i for i, line in enumerate(out) if line and not line.startswith(' ') and line.strip().endswith(':') and line.strip() != 'agent:'), len(out))
try:
    agent_idx = next(i for i, line in enumerate(out) if line.strip() == 'agent:')
except StopIteration:
    out.insert(0, 'agent:')
    agent_idx = 0
    next_section = 1
insert = [
    '  label.benchmark_run: ' + labels['benchmark_run'],
    '  label.policy_profile: ' + labels['policy_profile'],
]
if 'workload' in labels:
    insert.append('  label.workload: ' + labels['workload'])
if 'scenario' in labels:
    insert.append('  label.scenario: ' + labels['scenario'])
if 'variant' in labels:
    insert.append('  label.variant: ' + labels['variant'])
if 'matcher_strategy' in labels:
    insert.append('  label.matcher_strategy: ' + labels['matcher_strategy'])
next_section = next((i for i in range(agent_idx + 1, len(out)) if out[i] and not out[i].startswith(' ') and out[i].strip().endswith(':')), len(out))
out[next_section:next_section] = insert
p.write_text('\n'.join(out) + '\n')
PY
sudo systemctl restart sysarmor-agent" >/dev/null
  wait_agent_socket
}

set_runtime_feature_flags() {
  local matcher_strategy="${1:-}"
  if [[ -z "$matcher_strategy" ]]; then
    return 0
  fi
  case "$matcher_strategy" in
    linear|optimized) ;;
    *)
      echo "[bench-collection-vm][ERROR] unsupported matcher strategy: $matcher_strategy" >&2
      exit 1
      ;;
  esac
  vagrant ssh node-a -c "sudo SYSARMOR_MATCHER_STRATEGY='$matcher_strategy' python3 - <<'PY'
import os
from pathlib import Path

p = Path('/etc/sysarmor/agent.yaml')
strategy = os.environ['SYSARMOR_MATCHER_STRATEGY']
lines = p.read_text().splitlines()
out = []
skip_runtime = False
for line in lines:
    is_top = bool(line and not line.startswith(' ') and line.strip().endswith(':'))
    if is_top:
        skip_runtime = line.strip() == 'runtime:'
    if skip_runtime:
        continue
    out.append(line)
insert = [
    'runtime:',
    '  feature_flags:',
    '    matcher_strategy: ' + strategy,
]
insert_at = next((i for i, line in enumerate(out) if line.strip() == 'sensor:'), len(out))
out[insert_at:insert_at] = insert + ['']
p.write_text('\n'.join(out) + '\n')
PY
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
    bash "$ROOT/shared/recorder/recorder-vm.sh" "$@"
}

mark() {
  local rec_run_id="$1"
  local phase="$2"
  local detail="${3:-}"
  PHASE="$phase" DETAIL="$detail" RUN_ID="$rec_run_id" bash "$ROOT/shared/recorder/recorder-vm.sh" mark
}

run_workload() {
  local policy_out="$1"
  local workload_name="${2:-$WORKLOAD}"
  cd "$ENVDIR"
  if [[ -f "$ROOT/data/workloads/vm/$workload_name/run.sh" ]]; then
    vagrant upload "$ROOT/data/workloads/vm/$workload_name/run.sh" /tmp/sysarmor-workload-run.sh node-a >/dev/null
    vagrant ssh node-a -c "sudo bash -c 'DURATION=$WORKLOAD_SECONDS REPEAT=$WORKLOAD_REPEAT C2=$WORKLOAD_C2 bash /tmp/sysarmor-workload-run.sh'" \
      > "$policy_out/workload.out" 2>"$policy_out/workload.err" || true
  elif [[ -f "$ROOT/data/scenarios/vm/$workload_name/attack.sh" ]]; then
    vagrant upload "$ROOT/data/scenarios/vm/$workload_name/attack.sh" /tmp/sysarmor-scenario-attack.sh node-a >/dev/null
    vagrant ssh node-a -c "sudo bash -c 'GAP=1 C2=$WORKLOAD_C2 bash /tmp/sysarmor-scenario-attack.sh'" \
      > "$policy_out/workload.out" 2>"$policy_out/workload.err" || true
  else
    echo "[bench-collection-vm][ERROR] workload not found: $workload_name" >&2
    exit 1
  fi
}

start_workload_background() {
  local policy_out="$1"
  local workload_name="$2"
  WORKLOAD_PID=""
  cd "$ENVDIR"
  if [[ ! -f "$ROOT/data/workloads/vm/$workload_name/run.sh" ]]; then
    echo "[bench-collection-vm][ERROR] workload not found: $workload_name" >&2
    exit 1
  fi
  vagrant upload "$ROOT/data/workloads/vm/$workload_name/run.sh" /tmp/sysarmor-workload-run.sh node-a >/dev/null
  vagrant ssh node-a -c "sudo bash -c 'DURATION=$WORKLOAD_SECONDS REPEAT=0 C2=$WORKLOAD_C2 bash /tmp/sysarmor-workload-run.sh'" \
    > "$policy_out/workload.out" 2>"$policy_out/workload.err" &
  WORKLOAD_PID=$!
}

run_scenario() {
  local policy_out="$1"
  local scenario_name="$2"
  cd "$ENVDIR"
  if [[ ! -f "$ROOT/data/scenarios/vm/$scenario_name/attack.sh" ]]; then
    echo "[bench-collection-vm][ERROR] scenario not found: $scenario_name" >&2
    exit 1
  fi
  vagrant upload "$ROOT/data/scenarios/vm/$scenario_name/attack.sh" /tmp/sysarmor-scenario-attack.sh node-a >/dev/null
  vagrant ssh node-a -c "sudo bash -c 'GAP=1 C2=$WORKLOAD_C2 bash /tmp/sysarmor-scenario-attack.sh'" \
    > "$policy_out/scenario.out" 2>"$policy_out/scenario.err" || true
}

run_case_activity() {
  local policy_out="$1"
  local workload_name="${2:-}"
  local scenario_name="${3:-}"
  local workload_pid=""

  if [[ -n "$workload_name" ]]; then
    start_workload_background "$policy_out" "$workload_name"
    workload_pid="$WORKLOAD_PID"
    sleep "$WORKLOAD_WARMUP_SECONDS"
  fi

  if [[ -n "$scenario_name" ]]; then
    run_scenario "$policy_out" "$scenario_name"
  fi

  if [[ -n "$workload_pid" ]]; then
    wait "$workload_pid" || true
  fi
}

echo "[bench-collection-vm] output: $OUT_DIR"
echo "[bench-collection-vm] variant: ${VARIANT:-default}"
echo "[bench-collection-vm] matcher_strategy: ${MATCHER_STRATEGY:-config-default}"
wait_agent_socket
set_runtime_feature_flags "$MATCHER_STRATEGY"

echo "[bench-collection-vm] uploading content packs and policies"
vagrant upload "$REPO/$CONTENT_DIR" /tmp/sysarmor-bench-content node-a >/dev/null
if [[ "$APPLY_DETECTION" == "1" && ! -f "$REPO/$DETECTION_POLICY" ]]; then
  echo "[bench-collection-vm][ERROR] detection policy not found: $DETECTION_POLICY" >&2
  exit 1
fi
if [[ "$APPLY_DETECTION" == "1" ]]; then
  vagrant upload "$REPO/$DETECTION_POLICY" /tmp/sysarmor-bench-detection.policy node-a >/dev/null
fi

apply_content_and_detection() {
  local policy_out="$1"
  for content in "$REPO/$CONTENT_DIR"/*.json; do
    name="$(basename "$content")"
    vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json content apply --file '/tmp/sysarmor-bench-content/$name' --allow-unsigned --agent-id '$AGENT_ID' --tenant-id '$TENANT_ID'" \
      > "$policy_out/content.$name.apply.json" \
      2>"$policy_out/content.$name.apply.err" || {
        echo "[bench-collection-vm][ERROR] content apply failed: $name" >&2
        cat "$policy_out/content.$name.apply.err" >&2 2>/dev/null || true
        exit 1
      }
  done

  if [[ "$APPLY_DETECTION" == "1" ]]; then
    echo "[bench-collection-vm] applying detection policy: $DETECTION_POLICY"
    vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json policy apply --type detection --file /tmp/sysarmor-bench-detection.policy --agent-id '$AGENT_ID' --tenant-id '$TENANT_ID' --timeout 60s" \
      > "$policy_out/detection-apply.json" \
      2>"$policy_out/detection-apply.err" || {
        echo "[bench-collection-vm][ERROR] detection policy apply failed: $DETECTION_POLICY" >&2
        cat "$policy_out/detection-apply.err" >&2 2>/dev/null || true
        exit 1
      }
    if grep -Fq '"status":"rejected"' "$policy_out/detection-apply.json"; then
      echo "[bench-collection-vm][ERROR] detection policy rejected: $DETECTION_POLICY" >&2
      cat "$policy_out/detection-apply.json" >&2 2>/dev/null || true
      exit 1
    fi
  else
    echo "[bench-collection-vm] detection policy apply disabled"
  fi
}

for policy in $POLICIES_RAW; do
  if [[ ! -f "$REPO/$policy" ]]; then
    echo "[bench-collection-vm][ERROR] policy not found: $policy" >&2
    exit 1
  fi
  name="$(policy_name "$policy")"
  policy_out="$OUT_DIR/$name"
  rec_run_id="bench-collection-vm/$RUN_ID/$name"
  rec_dir="$RESULTS/recordings/$rec_run_id"
  case_workload="$WORKLOAD"
  case_scenario="$SCENARIO"
  if [[ -z "$case_workload" && -z "$case_scenario" ]]; then
    echo "[bench-collection-vm][ERROR] at least one of SYSARMOR_BENCH_WORKLOAD or SYSARMOR_BENCH_SCENARIO is required" >&2
    exit 1
  fi
  rec_labels="benchmark_run=$RUN_ID,policy_profile=$name"
  if [[ -n "$VARIANT" ]]; then
    rec_labels="$rec_labels,variant=$VARIANT"
  fi
  if [[ -n "$MATCHER_STRATEGY" ]]; then
    rec_labels="$rec_labels,matcher_strategy=$MATCHER_STRATEGY"
  fi
  if [[ -n "$case_workload" ]]; then
    rec_labels="$rec_labels,workload=$case_workload"
  fi
  if [[ -n "$case_scenario" ]]; then
    rec_labels="$rec_labels,scenario=$case_scenario"
  fi
  mkdir -p "$policy_out"
  cat >"$policy_out/runtime-feature-flags.json" <<EOF
{
  "variant": "$VARIANT",
  "matcher_strategy": "$MATCHER_STRATEGY"
}
EOF

  echo "[bench-collection-vm] recording policy=$name workload=${case_workload:-none} scenario=${case_scenario:-none}"
  set_agent_labels "$RUN_ID" "$name" "$case_workload" "$case_scenario"
  apply_content_and_detection "$policy_out"
  SYSARMOR_RECORDER_DURATION=3600 recorder "$rec_run_id" "$rec_labels" start
  mark "$rec_run_id" baseline_start "$name"
  sleep "$BASELINE_SECONDS"

  echo "[bench-collection-vm] applying policy: $policy"
  mark "$rec_run_id" policy_apply_start "$policy"
  vagrant upload "$REPO/$policy" "/tmp/sysarmor-bench-$name.policy" node-a >/dev/null
  vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json policy apply collection --file '/tmp/sysarmor-bench-$name.policy' --agent-id '$AGENT_ID' --tenant-id '$TENANT_ID' --timeout 60s" \
    > "$policy_out/collection-apply.json" \
    2>"$policy_out/collection-apply.err" || {
      echo "[bench-collection-vm][ERROR] policy apply failed: $policy" >&2
      cat "$policy_out/collection-apply.err" >&2 2>/dev/null || true
      exit 1
    }
  mark "$rec_run_id" policy_apply_done "$policy"
  if ! grep -Fq 'generated_policy_hash' "$policy_out/collection-apply.json" && ! grep -Fq 'resolved_refs' "$policy_out/collection-apply.json"; then
    echo "[bench-collection-vm][ERROR] policy apply did not report generated policy details: $policy" >&2
    cat "$policy_out/collection-apply.json" >&2 2>/dev/null || true
    exit 1
  fi

  echo "[bench-collection-vm] waiting ${POLICY_SETTLE_SECONDS}s for sensor BPF reload"
  sleep "$POLICY_SETTLE_SECONDS"

  mark "$rec_run_id" settle_start "$name"
  sleep "$SETTLE_SECONDS"
  mark "$rec_run_id" steady_start "$name"
  sleep "$STEADY_SECONDS"
  mark "$rec_run_id" workload_start "${case_workload:-none}"
  if [[ -n "$case_scenario" ]]; then
    mark "$rec_run_id" scenario_start "$case_scenario"
  fi
  run_case_activity "$policy_out" "$case_workload" "$case_scenario"
  if [[ -n "$case_scenario" ]]; then
    mark "$rec_run_id" scenario_done "$case_scenario"
  fi
  mark "$rec_run_id" workload_done "${case_workload:-none}"

  recorder "$rec_run_id" "$rec_labels" stop
  recorder "$rec_run_id" "$rec_labels" report

  cp "$rec_dir/timeline.csv" "$policy_out/timeline.csv"
  cp "$rec_dir/markers.ndjson" "$policy_out/markers.ndjson"
  cp "$rec_dir/summary.json" "$policy_out/summary.json"
  cp "$rec_dir/events.ndjson" "$policy_out/events.ndjson" 2>/dev/null || true
  cp "$rec_dir/events-all.ndjson" "$policy_out/events-all.ndjson" 2>/dev/null || true
  cp "$rec_dir/signals.ndjson" "$policy_out/signals.ndjson" 2>/dev/null || true
  cp "$rec_dir/signals-all.ndjson" "$policy_out/signals-all.ndjson" 2>/dev/null || true
  cp "$rec_dir/event-watch.err" "$policy_out/event-watch.err" 2>/dev/null || true
  cp "$rec_dir/event-all-watch.err" "$policy_out/event-all-watch.err" 2>/dev/null || true
  cp "$rec_dir/signal-watch.err" "$policy_out/signal-watch.err" 2>/dev/null || true
  cp "$rec_dir/signal-all-watch.err" "$policy_out/signal-all-watch.err" 2>/dev/null || true
done

python3 "$HERE/bench_collection_report.py" "$OUT_DIR"

echo "[bench-collection-vm] matrix written to $OUT_DIR/matrix.csv"
