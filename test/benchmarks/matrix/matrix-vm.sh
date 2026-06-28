#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
RESULTS="$ROOT/.results"
RUN_ID="${SYSARMOR_BENCH_RUN_ID:-$(date -u +%Y%m%dT%H%M%SZ)}"
OUT_DIR="$RESULTS/bench-matrix-vm/$RUN_ID"

POLICIES="${POLICIES-test/data/policies/collection-minimal.json test/data/policies/collection-balanced.json test/data/policies/collection-deep.json}"
WORKLOADS="${WORKLOADS-business-normal}"
SCENARIOS="${SCENARIOS-apt-fileless-c2 apt-staged-drop benign-ci-noise}"
MATCHER_VARIANTS="${MATCHER_VARIANTS:-}"
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
  "matcher_variants": "$MATCHER_VARIANTS",
  "matrix_mode": "$MATRIX_MODE"
}
EOF

run_case() {
  local variant="${1:-}"
  local matcher_strategy="${2:-}"
  local workload="${3:-}"
  local scenario="${4:-}"
  local workload_label="${workload:-none}"
  local scenario_label="${scenario:-none}"
  local variant_label="${variant:-default}"
  local case_name="workload=${workload_label}__scenario=${scenario_label}"
  if [[ -n "$variant" ]]; then
    case_name="variant=${variant_label}__$case_name"
  fi
  local case_run_id="$RUN_ID/cases/$case_name"
  local case_dir="$OUT_DIR/cases/$case_name"
  mkdir -p "$case_dir"

  echo "[bench-matrix-vm] variant=$variant_label matcher_strategy=${matcher_strategy:-config-default} workload=$workload_label scenario=$scenario_label"
  if SYSARMOR_BENCH_RUN_ID="$case_run_id" \
      POLICIES="$POLICIES" \
      SYSARMOR_BENCH_VARIANT="$variant" \
      SYSARMOR_BENCH_MATCHER_STRATEGY="$matcher_strategy" \
      SYSARMOR_BENCH_WORKLOAD="$workload" \
      SYSARMOR_BENCH_SCENARIO="$scenario" \
      bash "$HERE/collection-vm.sh" >"$case_dir/run.out" 2>"$case_dir/run.err"; then
    printf '{"name":"%s","variant":"%s","matcher_strategy":"%s","workload":"%s","scenario":"%s","status":"ok","bench_run_id":"%s"}\n' \
      "$case_name" "$variant" "$matcher_strategy" "$workload" "$scenario" "$case_run_id" >"$case_dir/status.json"
  else
    local rc=$?
    printf '{"name":"%s","variant":"%s","matcher_strategy":"%s","workload":"%s","scenario":"%s","status":"failed","exit_code":%s,"bench_run_id":"%s"}\n' \
      "$case_name" "$variant" "$matcher_strategy" "$workload" "$scenario" "$rc" "$case_run_id" >"$case_dir/status.json"
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
echo "[bench-matrix-vm] matcher_variants: ${MATCHER_VARIANTS:-default}"
echo "[bench-matrix-vm] matrix_mode: $MATRIX_MODE"

if [[ "$SYNC_VM_AGENT" == "1" ]]; then
  bash "$ROOT/shared/vm/sync-agent.sh"
  cd "$ROOT/environments/vm" && vagrant rsync node-a >/dev/null 2>&1 || true
  cd "$HERE"
else
  echo "[bench-matrix-vm] VM agent sync disabled"
fi

run_mode_for_variant() {
  local variant="${1:-}"
  local matcher_strategy="${2:-}"
  case "$MATRIX_MODE" in
    workload)
      for workload in $WORKLOADS; do
        run_case "$variant" "$matcher_strategy" "$workload" ""
      done
      ;;
    scenario)
      for scenario in $SCENARIOS; do
        run_case "$variant" "$matcher_strategy" "" "$scenario"
      done
      ;;
    cross)
      for workload in $WORKLOADS; do
        for scenario in $SCENARIOS; do
          run_case "$variant" "$matcher_strategy" "$workload" "$scenario"
        done
      done
      ;;
    all)
      for workload in $WORKLOADS; do
        run_case "$variant" "$matcher_strategy" "$workload" ""
      done
      for scenario in $SCENARIOS; do
        run_case "$variant" "$matcher_strategy" "" "$scenario"
      done
      for workload in $WORKLOADS; do
        for scenario in $SCENARIOS; do
          run_case "$variant" "$matcher_strategy" "$workload" "$scenario"
        done
      done
      ;;
    *)
      echo "[bench-matrix-vm][ERROR] unsupported MATRIX_MODE=$MATRIX_MODE (want workload|scenario|cross|all)" >&2
      exit 1
      ;;
  esac
}

if [[ -n "$MATCHER_VARIANTS" ]]; then
  for matcher_strategy in $MATCHER_VARIANTS; do
    case "$matcher_strategy" in
      linear|optimized) ;;
      *)
        echo "[bench-matrix-vm][ERROR] unsupported matcher variant: $matcher_strategy" >&2
        exit 1
        ;;
    esac
    run_mode_for_variant "matcher-$matcher_strategy" "$matcher_strategy"
  done
else
  run_mode_for_variant "" ""
fi

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
