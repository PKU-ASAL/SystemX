#!/usr/bin/env bash
# Capture resource usage samples for SysArmor EDR components.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
TOPO="${1:-container}"
DUR="${2:-30}"
SCENARIO="${3:-idle}"
INTERVAL="${INTERVAL:-2}"
RESULTS="$ROOT/.results"
OUT="$RESULTS/perf-resource.$TOPO.$SCENARIO.csv"
mkdir -p "$RESULTS"

write_header() {
  if [[ ! -f "$OUT" ]]; then
    echo "topology,scenario,sample_ts,elapsed_s,edr_cpu_pct,edr_rss_mb,agent_cpu_pct,agent_rss_mb,tetragon_cpu_pct,tetragon_rss_mb,tetra_cpu_pct,tetra_rss_mb,workload_cpu_pct,workload_rss_mb,business_latency_p95_ms,business_throughput_rps,dropped_events,parse_errors,notes" > "$OUT"
  fi
}

num() {
  local v="${1:-0}"
  v="${v//%/}"
  v="${v//MiB/}"
  v="${v//GiB/}"
  v="${v// /}"
  [[ -n "$v" ]] && printf '%s' "$v" || printf '0'
}

mb_from_docker_mem() {
  local raw="${1:-0MiB}"
  local used
  used="$(printf '%s' "$raw" | awk -F/ '{print $1}' | tr -d ' ')"
  case "$used" in
    *GiB) awk -v v="${used%GiB}" 'BEGIN { printf "%.1f", v * 1024 }' ;;
    *MiB) printf '%s' "${used%MiB}" ;;
    *KiB) awk -v v="${used%KiB}" 'BEGIN { printf "%.1f", v / 1024 }' ;;
    *B) awk -v v="${used%B}" 'BEGIN { printf "%.1f", v / 1024 / 1024 }' ;;
    *) num "$used" ;;
  esac
}

docker_stats_pair() {
  local name="$1"
  local stats cpu mem
  stats="$(docker stats "$name" --no-stream --format '{{.CPUPerc}},{{.MemUsage}}' 2>/dev/null || true)"
  cpu="$(num "$(printf '%s' "$stats" | cut -d, -f1)")"
  mem="$(mb_from_docker_mem "$(printf '%s' "$stats" | cut -d, -f2)")"
  printf '%s,%s' "${cpu:-0}" "${mem:-0}"
}

docker_proc_pair() {
  local container="$1"
  local pattern="$2"
  docker exec "$container" sh -c "ps -eo comm,pcpu,rss 2>/dev/null | awk -v p='$pattern' '\$1 ~ p { cpu += \$2; rss += \$3 } END { printf \"%.2f,%.1f\", cpu, rss / 1024 }'" 2>/dev/null || printf '0,0'
}

vm_proc_pair() {
  local pattern="$1"
  local envdir="$ROOT/environments/vm"
  (cd "$envdir" && vagrant ssh node-a -c "ps -eo comm,pcpu,rss 2>/dev/null | awk -v p='$pattern' '\$1 ~ p { cpu += \$2; rss += \$3 } END { printf \"%.2f,%.1f\", cpu, rss / 1024 }'" 2>/dev/null) || printf '0,0'
}

sum_pairs() {
  awk -F, -v a="$1" -v b="$2" -v c="$3" '
    BEGIN {
      split(a, aa, ","); split(b, bb, ","); split(c, cc, ",");
      printf "%.2f,%.1f", aa[1] + bb[1] + cc[1], aa[2] + bb[2] + cc[2]
    }'
}

sample_container() {
  local elapsed="$1" ts edr agent tetragon tetra workload total
  ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  edr="$(docker_stats_pair tetragon)"
  workload="$(docker_stats_pair node-a)"
  agent="$(docker_proc_pair tetragon '^sysarmor-agent$')"
  tetragon="$(docker_proc_pair tetragon '^tetragon$')"
  tetra="$(docker_proc_pair tetragon '^tetra$')"
  total="$(sum_pairs "$agent" "$tetragon" "$tetra")"
  echo "$TOPO,$SCENARIO,$ts,$elapsed,$total,$agent,$tetragon,$tetra,$workload,0,0,0,0,edr_container_stats=${edr/,/|}" >> "$OUT"
}

sample_vm() {
  local elapsed="$1" ts agent tetragon tetra workload total
  ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  agent="$(vm_proc_pair '^sysarmor-agent$')"
  tetragon="$(vm_proc_pair '^tetragon$')"
  tetra="$(vm_proc_pair '^tetra$')"
  total="$(sum_pairs "$agent" "$tetragon" "$tetra")"
  workload="$(vm_proc_pair '^(java|bash|curl|python|node|nginx|apache2)$')"
  echo "$TOPO,$SCENARIO,$ts,$elapsed,$total,$agent,$tetragon,$tetra,$workload,0,0,0,0,vm_process_stats" >> "$OUT"
}

write_header

elapsed=0
while (( elapsed <= DUR )); do
  case "$TOPO" in
    container) sample_container "$elapsed" ;;
    vm) sample_vm "$elapsed" ;;
    *) echo "unknown topology: $TOPO" >&2; exit 2 ;;
  esac
  (( elapsed >= DUR )) && break
  sleep "$INTERVAL"
  elapsed=$(( elapsed + INTERVAL ))
done

cp "$OUT" /tmp/perf-resource.csv 2>/dev/null || true
tail -1 "$OUT"
