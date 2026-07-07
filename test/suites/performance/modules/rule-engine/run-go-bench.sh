#!/usr/bin/env bash
# Run local Go microbenchmarks for the endpoint rule engine and matcher package.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_ROOT="$(cd "$HERE/../../../.." && pwd)"
ROOT="$(cd "$TEST_ROOT/.." && pwd)"
RUN_ID="${RUN_ID:-$(date -u +%Y%m%dT%H%M%SZ)}"
BENCHTIME="${BENCHTIME:-200ms}"
COUNT="${COUNT:-1}"
OUT_DIR="$TEST_ROOT/.results/rule-engine/$RUN_ID"
mkdir -p "$OUT_DIR"

run_bench() {
  local name="$1"
  local pkg="$2"
  local pattern="$3"
  local out="$OUT_DIR/$name.txt"
  (
    cd "$ROOT"
    go test "$pkg" -run '^$' -bench "$pattern" -benchtime="$BENCHTIME" -count="$COUNT" -benchmem
  ) | tee "$out"
}

run_bench detection ./internal/endpoint/detection 'BenchmarkEngineProcess'
run_bench matcher ./internal/endpoint/matcher .

cat > "$OUT_DIR/summary.txt" <<EOF
run_id=$RUN_ID
benchtime=$BENCHTIME
count=$COUNT
detection=$OUT_DIR/detection.txt
matcher=$OUT_DIR/matcher.txt
EOF

printf 'rule engine benchmark results: %s\n' "$OUT_DIR"
