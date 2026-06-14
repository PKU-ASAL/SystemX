#!/usr/bin/env bash
# Capture a lightweight getevents throughput baseline for the selected topology.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"
TOPO="${1:-container}"
DUR="${2:-10}"
RESULTS="$ROOT/.results"
OUT="$RESULTS/perf-getevents.$TOPO.csv"
mkdir -p "$RESULTS"

write_header() {
  if [[ ! -f "$OUT" ]]; then
    echo "topology,duration_s,events,eps,cpu_pct,rss_mb,dropped_events" > "$OUT"
  fi
}

container_perf() {
  docker start tetragon >/dev/null 2>&1 || true
  local tmp="/tmp/perf-getevents-container.jsonl"
  timeout "$DUR" docker exec tetragon tetra getevents -o json > "$tmp" 2>/dev/null || true
  local events
  events="$(wc -l < "$tmp" | tr -d ' ')"
  local stats cpu rss
  stats="$(docker stats tetragon --no-stream --format '{{.CPUPerc}},{{.MemUsage}}' 2>/dev/null || true)"
  cpu="$(printf '%s' "$stats" | cut -d, -f1 | tr -d '%')"
  rss="$(printf '%s' "$stats" | cut -d, -f2 | awk '{print $1}')"
  if [[ "$rss" == *GiB ]]; then
    rss="$(awk -v v="${rss%GiB}" 'BEGIN { printf "%.1f", v * 1024 }')"
  else
    rss="${rss%MiB}"
  fi
  echo "$TOPO,$DUR,$events,$(awk -v e="$events" -v d="$DUR" 'BEGIN { printf "%.2f", e / d }'),${cpu:-0},${rss:-0},0" >> "$OUT"
}

vm_perf() {
  local envdir="$ROOT/env/vm"
  local tmp="/tmp/perf-getevents-vm.jsonl"
  cd "$envdir"
  vagrant ssh node-a -c "sudo systemctl start tetragon 2>/dev/null || true; timeout $DUR tetra getevents -o json > $tmp 2>/dev/null || true"
  local events rss
  events="$(vagrant ssh node-a -c "wc -l < $tmp" 2>/dev/null | tr -dc '0-9')"
  rss="$(vagrant ssh node-a -c "ps -C tetragon -o rss= | awk '{sum+=\$1} END {printf \"%.1f\", sum/1024}'" 2>/dev/null | tr -dc '0-9.')"
  echo "$TOPO,$DUR,${events:-0},$(awk -v e="${events:-0}" -v d="$DUR" 'BEGIN { printf "%.2f", e / d }'),0,${rss:-0},0" >> "$OUT"
}

write_header
case "$TOPO" in
  container) container_perf ;;
  vm) vm_perf ;;
  *) echo "unknown topology: $TOPO" >&2; exit 2 ;;
esac

cp "$OUT" /tmp/perf-getevents.csv 2>/dev/null || true
tail -1 "$OUT"
