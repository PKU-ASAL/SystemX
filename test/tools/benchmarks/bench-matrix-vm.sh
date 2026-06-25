#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
RESULTS="$ROOT/.results"
RUN_ID="${SYSARMOR_BENCH_RUN_ID:-$(date -u +%Y%m%dT%H%M%SZ)}"
OUT_DIR="$RESULTS/bench-matrix-vm/$RUN_ID"

POLICIES="${POLICIES-test/policies/collection-minimal-high-signal.json test/policies/collection-edr-balanced.json test/policies/collection-incident-deep.json test/policies/collection-debug-wide.json}"
WORKLOADS="${WORKLOADS-benign-business exec-storm file-read-storm file-write-storm network-connect-storm mixed-edr-storm}"
SCENARIOS="${SCENARIOS-apt-fileless-c2 apt-staged-drop benign-ci-noise}"
INCLUDE_SCENARIOS="${INCLUDE_SCENARIOS:-1}"
STOP_ON_ERROR="${STOP_ON_ERROR:-0}"
EVALUATION_SCOPE="${EVALUATION_SCOPE:-local}"
SYNC_VM_AGENT="${SYSARMOR_BENCH_SYNC_VM_AGENT:-1}"

mkdir -p "$OUT_DIR"

cat >"$OUT_DIR/manifest.json" <<EOF
{
  "suite": "local-agent",
  "tool": "bench-matrix-vm",
  "evaluation_scope": "$EVALUATION_SCOPE",
  "topology": "vm",
  "run_id": "$RUN_ID",
  "policies": "$POLICIES",
  "workloads": "$WORKLOADS",
  "scenarios": "$SCENARIOS",
  "include_scenarios": "$INCLUDE_SCENARIOS"
}
EOF

run_case() {
  local kind="$1"
  local name="$2"
  local case_run_id="$RUN_ID/$kind/$name"
  local case_dir="$OUT_DIR/$kind/$name"
  mkdir -p "$case_dir"

  echo "[bench-matrix-vm] $kind=$name"
  if SYSARMOR_BENCH_RUN_ID="$case_run_id" \
      POLICIES="$POLICIES" \
      SYSARMOR_BENCH_CASE_TYPE="$kind" \
      DIAG_SCENARIO="$name" \
      bash "$HERE/bench-collection-vm.sh" >"$case_dir/run.out" 2>"$case_dir/run.err"; then
    printf '{"kind":"%s","name":"%s","status":"ok","bench_run_id":"%s"}\n' \
      "$kind" "$name" "$case_run_id" >"$case_dir/status.json"
  else
    local rc=$?
    printf '{"kind":"%s","name":"%s","status":"failed","exit_code":%s,"bench_run_id":"%s"}\n' \
      "$kind" "$name" "$rc" "$case_run_id" >"$case_dir/status.json"
    if [[ "$STOP_ON_ERROR" == "1" ]]; then
      echo "[bench-matrix-vm][ERROR] failed $kind=$name" >&2
      cat "$case_dir/run.err" >&2 2>/dev/null || true
      exit "$rc"
    fi
  fi
}

echo "[bench-matrix-vm] output: $OUT_DIR"
echo "[bench-matrix-vm] evaluation_scope: $EVALUATION_SCOPE"
echo "[bench-matrix-vm] policies: $POLICIES"
echo "[bench-matrix-vm] workloads: $WORKLOADS"
echo "[bench-matrix-vm] scenarios: $SCENARIOS"

if [[ "$SYNC_VM_AGENT" == "1" ]]; then
  bash "$ROOT/tools/vm/sync-agent.sh"
else
  echo "[bench-matrix-vm] VM agent sync disabled"
fi

for workload in $WORKLOADS; do
  run_case workload "$workload"
done

if [[ "$INCLUDE_SCENARIOS" == "1" ]]; then
  for scenario in $SCENARIOS; do
    run_case scenario "$scenario"
  done
fi

python3 "$HERE/bench_matrix_report.py" "$OUT_DIR"
python3 "$HERE/effectiveness_report.py" \
  --bench-matrix-dir "$OUT_DIR" \
  --output-dir "$RESULTS/effectiveness/$RUN_ID" \
  --topology vm \
  --scope "$EVALUATION_SCOPE" \
  --scenarios $SCENARIOS \
  --workloads $WORKLOADS

echo "[bench-matrix-vm] matrix written to $OUT_DIR/matrix.csv and $OUT_DIR/matrix.json"
echo "[bench-matrix-vm] effectiveness written to $RESULTS/effectiveness/$RUN_ID"
