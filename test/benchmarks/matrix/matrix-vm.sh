#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
RESULTS="$ROOT/.results"
RUN_ID="${SYSARMOR_BENCH_RUN_ID:-$(date -u +%Y%m%dT%H%M%SZ)}"
OUT_DIR="$RESULTS/bench-matrix-vm/$RUN_ID"

POLICIES="${POLICIES-test/data/policies/collection-minimal-high-signal.json test/data/policies/collection-edr-balanced.json test/data/policies/collection-incident-deep.json}"
WORKLOADS="${WORKLOADS-business-normal}"
SCENARIOS="${SCENARIOS-apt-fileless-c2 apt-staged-drop benign-ci-noise}"
MATRIX_MODE="${MATRIX_MODE:-cross}"
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
  "matrix_mode": "$MATRIX_MODE"
}
EOF

run_case() {
  local workload="${1:-}"
  local scenario="${2:-}"
  local workload_label="${workload:-none}"
  local scenario_label="${scenario:-none}"
  local case_name="workload=${workload_label}__scenario=${scenario_label}"
  local case_run_id="$RUN_ID/cases/$case_name"
  local case_dir="$OUT_DIR/cases/$case_name"
  mkdir -p "$case_dir"

  echo "[bench-matrix-vm] workload=$workload_label scenario=$scenario_label"
  if SYSARMOR_BENCH_RUN_ID="$case_run_id" \
      POLICIES="$POLICIES" \
      SYSARMOR_BENCH_WORKLOAD="$workload" \
      SYSARMOR_BENCH_SCENARIO="$scenario" \
      bash "$HERE/bench-collection-vm.sh" >"$case_dir/run.out" 2>"$case_dir/run.err"; then
    printf '{"name":"%s","workload":"%s","scenario":"%s","status":"ok","bench_run_id":"%s"}\n' \
      "$case_name" "$workload" "$scenario" "$case_run_id" >"$case_dir/status.json"
  else
    local rc=$?
    printf '{"name":"%s","workload":"%s","scenario":"%s","status":"failed","exit_code":%s,"bench_run_id":"%s"}\n' \
      "$case_name" "$workload" "$scenario" "$rc" "$case_run_id" >"$case_dir/status.json"
    if [[ "$STOP_ON_ERROR" == "1" ]]; then
      echo "[bench-matrix-vm][ERROR] failed workload=$workload_label scenario=$scenario_label" >&2
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
echo "[bench-matrix-vm] matrix_mode: $MATRIX_MODE"

if [[ "$SYNC_VM_AGENT" == "1" ]]; then
  bash "$ROOT/shared/vm/sync-agent.sh"
  cd "$ROOT/environments/vm" && vagrant rsync node-a >/dev/null 2>&1 || true
  cd "$HERE"
else
  echo "[bench-matrix-vm] VM agent sync disabled"
fi

case "$MATRIX_MODE" in
  workload)
    for workload in $WORKLOADS; do
      run_case "$workload" ""
    done
    ;;
  scenario)
    for scenario in $SCENARIOS; do
      run_case "" "$scenario"
    done
    ;;
  cross)
    for workload in $WORKLOADS; do
      for scenario in $SCENARIOS; do
        run_case "$workload" "$scenario"
      done
    done
    ;;
  all)
    for workload in $WORKLOADS; do
      run_case "$workload" ""
    done
    for scenario in $SCENARIOS; do
      run_case "" "$scenario"
    done
    for workload in $WORKLOADS; do
      for scenario in $SCENARIOS; do
        run_case "$workload" "$scenario"
      done
    done
    ;;
  *)
    echo "[bench-matrix-vm][ERROR] unsupported MATRIX_MODE=$MATRIX_MODE (want workload|scenario|cross|all)" >&2
    exit 1
    ;;
esac

python3 "$HERE/bench_matrix_report.py" "$OUT_DIR"
python3 "$HERE/effectiveness_report.py" \
  --bench-matrix-dir "$OUT_DIR" \
  --output-dir "$RESULTS/effectiveness/$RUN_ID" \
  --topology vm \
  --scope "$EVALUATION_SCOPE" \
  --scenarios $SCENARIOS \
  --workloads $WORKLOADS

echo "[bench-matrix-vm] matrix written to $OUT_DIR/matrix.csv"
echo "[bench-matrix-vm] effectiveness written to $RESULTS/effectiveness/$RUN_ID"
