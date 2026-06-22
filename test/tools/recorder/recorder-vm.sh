#!/usr/bin/env bash
set -euo pipefail

CMD="${1:-}"
if [[ -z "$CMD" ]]; then
  echo "usage: recorder-vm.sh <start|mark|stop|report>" >&2
  exit 2
fi

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
ENVDIR="$(cd "$ROOT/env/vm" && pwd)"
RESULTS="$ROOT/.results"
RUN_ID="${RUN_ID:-${SYSARMOR_RECORDER_RUN_ID:-manual}}"
OUT_DIR="$RESULTS/recordings/$RUN_ID"
AGENT_SOCK="${SYSARMOR_AGENT_SOCK:-/var/run/sysarmor/agent.sock}"
AGENT_ID="${SYSARMOR_RECORDER_AGENT_ID:-${SYSARMOR_BENCH_AGENT_ID:-vm-owned-tetragon}}"
TENANT_ID="${SYSARMOR_RECORDER_TENANT_ID:-${SYSARMOR_BENCH_TENANT_ID:-default}}"
DURATION="${DURATION:-${SYSARMOR_RECORDER_DURATION:-3600}}"
LABELS="${SYSARMOR_RECORDER_LABELS:-}"
PHASE="${PHASE:-}"
DETAIL="${DETAIL:-}"

mkdir -p "$OUT_DIR"

json_escape() {
  python3 -c 'import json,sys; print(json.dumps(sys.argv[1])[1:-1])' "$1"
}

mark_local() {
  local phase="$1"
  local detail="${2:-}"
  local ts
  ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  printf '{"ts":"%s","phase":"%s","detail":"%s"}\n' \
    "$ts" "$(json_escape "$phase")" "$(json_escape "$detail")" >> "$OUT_DIR/markers.ndjson"
}

start_remote_sampler() {
  cd "$ENVDIR"
  vagrant ssh node-a -c "sudo mkdir -p /run/sysarmor/recorder && sudo tee /run/sysarmor/recorder/recorder-vm.sh >/dev/null <<'EOS'
#!/usr/bin/env bash
set -euo pipefail
DUR=\"\${1:-3600}\"
AGENT_SOCK=\"\${2:-/var/run/sysarmor/agent.sock}\"
AGENT_ID=\"\${3:-vm-owned-tetragon}\"
TENANT_ID=\"\${4:-default}\"
LABELS=\"\${5:-}\"
STATE_DIR=/run/sysarmor/recorder
mkdir -p \"\$STATE_DIR\"
OUT=\"\$STATE_DIR/timeline.csv\"
DONE=\"\$STATE_DIR/done\"
STOP=\"\$STATE_DIR/stop\"
HEALTH_JSON=\"\$STATE_DIR/health.json\"
EVENTS_NDJSON=\"\$STATE_DIR/events.ndjson\"
SIGNALS_NDJSON=\"\$STATE_DIR/signals.ndjson\"
CURSOR_EVENTS_NDJSON=\"\$STATE_DIR/cursor-events.ndjson\"
CURSOR_SIGNALS_NDJSON=\"\$STATE_DIR/cursor-signals.ndjson\"
rm -f \"\$OUT\" \"\$DONE\" \"\$STOP\"
WATCH_LIMIT=\"\${SYSARMOR_RECORDER_WATCH_LIMIT:-20000}\"
echo 'ts,elapsed_s,agent_cpu_pct,agent_rss_mb,sensor_cpu_pct,sensor_rss_mb,edr_cpu_pct,edr_rss_mb,events_seen,events_scoped,signals_scoped,dropped_events,parse_errors,agent_active,sensor_running,policy_id,policy_version,event_cursor,signal_cursor' > \"\$OUT\"
CLK_TCK=\"\$(getconf CLK_TCK 2>/dev/null || echo 100)\"
num_json() {
  local key=\"\$1\"
  local file=\"\$2\"
  python3 - \"\$key\" \"\$file\" <<'PY' 2>/dev/null || true
import json, sys
key, path = sys.argv[1], sys.argv[2]
try:
    data = json.load(open(path))
except Exception:
    print(0)
    raise SystemExit
cur = data
for part in key.split('.'):
    cur = cur.get(part, {}) if isinstance(cur, dict) else {}
try:
    print(int(cur))
except Exception:
    print(0)
PY
}
str_json() {
  local key=\"\$1\"
  local file=\"\$2\"
  python3 - \"\$key\" \"\$file\" <<'PY' 2>/dev/null || true
import json, sys
key, path = sys.argv[1], sys.argv[2]
try:
    data = json.load(open(path))
except Exception:
    print('')
    raise SystemExit
cur = data
for part in key.split('.'):
    cur = cur.get(part, '') if isinstance(cur, dict) else ''
print(cur if isinstance(cur, str) else '')
PY
}
max_sequence() {
  local file=\"\$1\"
  python3 - \"\$file\" <<'PY' 2>/dev/null || true
import json, sys
path = sys.argv[1]
max_seq = 0
try:
    lines = open(path, errors='replace').read().splitlines()
except Exception:
    lines = []
for line in lines:
    if not line.strip():
        continue
    try:
        data = json.loads(line)
    except Exception:
        continue
    seq = data.get('sequence') or data.get('seq') or 0
    try:
        max_seq = max(max_seq, int(seq))
    except Exception:
        pass
print(max_seq)
PY
}
label_args() {
  local raw=\"\$1\"
  local old_ifs=\"\$IFS\"
  LABEL_ARGS=()
  IFS=','
  for item in \$raw; do
    case \"\$item\" in
      *=*) LABEL_ARGS+=(--label \"\$item\") ;;
    esac
  done
  IFS=\"\$old_ifs\"
}
LABEL_ARGS=()
label_args \"\$LABELS\"
sudo sysarmorctl --socket \"\$AGENT_SOCK\" --json event watch --include-recent --snapshot --limit \"\$WATCH_LIMIT\" --agent-id \"\$AGENT_ID\" --tenant-id \"\$TENANT_ID\" --timeout 1s >\"\$CURSOR_EVENTS_NDJSON\" 2>/dev/null || true
sudo sysarmorctl --socket \"\$AGENT_SOCK\" --json signal watch --include-recent --snapshot --limit \"\$WATCH_LIMIT\" --agent-id \"\$AGENT_ID\" --tenant-id \"\$TENANT_ID\" --timeout 1s >\"\$CURSOR_SIGNALS_NDJSON\" 2>/dev/null || true
EVENT_CURSOR=\"\$(max_sequence \"\$CURSOR_EVENTS_NDJSON\")\"
SIGNAL_CURSOR=\"\$(max_sequence \"\$CURSOR_SIGNALS_NDJSON\")\"
pid_list() {
  local names=\"\$1\"
  for name in \$names; do
    pidof \"\$name\" 2>/dev/null || true
  done | tr ' ' '\\n' | awk 'NF && !seen[\$1]++'
}
proc_cpu_jiffies() {
  local pids=\"\$1\"
  local total=0
  local pid stat rest utime stime
  for pid in \$pids; do
    [ -r \"/proc/\$pid/stat\" ] || continue
    stat=\"\$(cat \"/proc/\$pid/stat\" 2>/dev/null || true)\"
    rest=\"\${stat##*) }\"
    utime=\"\$(echo \"\$rest\" | awk '{print \$12}')\"
    stime=\"\$(echo \"\$rest\" | awk '{print \$13}')\"
    total=\$((total + \${utime:-0} + \${stime:-0}))
  done
  echo \"\$total\"
}
proc_rss_mb() {
  local pids=\"\$1\"
  local total=0
  local pid rss
  for pid in \$pids; do
    [ -r \"/proc/\$pid/status\" ] || continue
    rss=\"\$(awk '/VmRSS:/ {print \$2}' \"/proc/\$pid/status\" 2>/dev/null || echo 0)\"
    total=\$((total + \${rss:-0}))
  done
  awk -v kb=\"\$total\" 'BEGIN { printf \"%.2f\", kb / 1024 }'
}
cpu_pct() {
  local prev=\"\$1\"
  local curr=\"\$2\"
  local interval=\"\$3\"
  if [ \"\$prev\" -le 0 ] || [ \"\$curr\" -lt \"\$prev\" ] || [ \"\$interval\" -le 0 ]; then
    echo \"0.00\"
    return
  fi
  awk -v delta=\$((curr - prev)) -v hz=\"\$CLK_TCK\" -v sec=\"\$interval\" 'BEGIN { printf \"%.2f\", (delta / hz) / sec * 100 }'
}
elapsed=0
prev_agent_jiffies=0
prev_sensor_jiffies=0
prev_sample_epoch=0
while [ \"\$elapsed\" -le \"\$DUR\" ]; do
  [ -f \"\$STOP\" ] && break
  ts=\"\$(date -u +%Y-%m-%dT%H:%M:%SZ)\"
  sample_epoch=\"\$(date +%s)\"
  agent_pids=\"\$(pid_list 'sysarmor-agent')\"
  sensor_pids=\"\$(pid_list 'tetragon sysarmor-sensor')\"
  agent_jiffies=\"\$(proc_cpu_jiffies \"\$agent_pids\")\"
  sensor_jiffies=\"\$(proc_cpu_jiffies \"\$sensor_pids\")\"
  interval=\$((sample_epoch - prev_sample_epoch))
  agent_cpu=\"\$(cpu_pct \"\$prev_agent_jiffies\" \"\$agent_jiffies\" \"\$interval\")\"
  sensor_cpu=\"\$(cpu_pct \"\$prev_sensor_jiffies\" \"\$sensor_jiffies\" \"\$interval\")\"
  edr_cpu=\"\$(awk -v a=\"\$agent_cpu\" -v s=\"\$sensor_cpu\" 'BEGIN { printf \"%.2f\", a + s }')\"
  agent_rss=\"\$(proc_rss_mb \"\$agent_pids\")\"
  sensor_rss=\"\$(proc_rss_mb \"\$sensor_pids\")\"
  edr_rss=\"\$(awk -v a=\"\$agent_rss\" -v s=\"\$sensor_rss\" 'BEGIN { printf \"%.2f\", a + s }')\"
  prev_agent_jiffies=\"\$agent_jiffies\"
  prev_sensor_jiffies=\"\$sensor_jiffies\"
  prev_sample_epoch=\"\$sample_epoch\"
  agent_active=\"\$(systemctl is-active sysarmor-agent 2>/dev/null || true)\"
  if pidof tetragon >/dev/null 2>&1 || pidof sysarmor-sensor >/dev/null 2>&1; then sensor_running=1; else sensor_running=0; fi
  sudo sysarmorctl --socket \"\$AGENT_SOCK\" --json agent health --agent-id \"\$AGENT_ID\" --tenant-id \"\$TENANT_ID\" >\"\$HEALTH_JSON\" 2>/dev/null || true
  sudo sysarmorctl --socket \"\$AGENT_SOCK\" --json event watch --include-recent --snapshot --limit \"\$WATCH_LIMIT\" --agent-id \"\$AGENT_ID\" --tenant-id \"\$TENANT_ID\" --timeout 1s --after-seq \"\$EVENT_CURSOR\" \"\${LABEL_ARGS[@]}\" >\"\$EVENTS_NDJSON\" 2>/dev/null || true
  sudo sysarmorctl --socket \"\$AGENT_SOCK\" --json signal watch --include-recent --snapshot --limit \"\$WATCH_LIMIT\" --agent-id \"\$AGENT_ID\" --tenant-id \"\$TENANT_ID\" --timeout 1s --after-seq \"\$SIGNAL_CURSOR\" \"\${LABEL_ARGS[@]}\" >\"\$SIGNALS_NDJSON\" 2>/dev/null || true
  events=\"\$(num_json sensor.eventsSeen \"\$HEALTH_JSON\")\"
  events_scoped=\"\$(wc -l <\"\$EVENTS_NDJSON\" 2>/dev/null || echo 0)\"
  signals_scoped=\"\$(wc -l <\"\$SIGNALS_NDJSON\" 2>/dev/null || echo 0)\"
  dropped=\"\$(num_json sensor.eventsDropped \"\$HEALTH_JSON\")\"
  parse_errors=\"\$(num_json sensor.parseErrors \"\$HEALTH_JSON\")\"
  policy_id=\"\$(str_json policyId \"\$HEALTH_JSON\")\"
  policy_version=\"\$(str_json policyVersion \"\$HEALTH_JSON\")\"
  echo \"\$ts,\$elapsed,\$agent_cpu,\$agent_rss,\$sensor_cpu,\$sensor_rss,\$edr_cpu,\$edr_rss,\$events,\$events_scoped,\$signals_scoped,\$dropped,\$parse_errors,\$agent_active,\$sensor_running,\$policy_id,\$policy_version,\$EVENT_CURSOR,\$SIGNAL_CURSOR\" >> \"\$OUT\"
  [ \"\$elapsed\" -ge \"\$DUR\" ] && break
  sleep 1
  elapsed=\$((elapsed + 1))
done
touch \"\$DONE\"
EOS
sudo chmod +x /run/sysarmor/recorder/recorder-vm.sh
sudo sh -c 'nohup bash /run/sysarmor/recorder/recorder-vm.sh "\$1" "\$2" "\$3" "\$4" "\$5" >/run/sysarmor/recorder/recorder.log 2>&1 &' sh '$DURATION' '$AGENT_SOCK' '$AGENT_ID' '$TENANT_ID' '$LABELS'
" >/dev/null
}

stop_remote_sampler() {
  cd "$ENVDIR"
  vagrant ssh node-a -c "sudo mkdir -p /run/sysarmor/recorder && sudo touch /run/sysarmor/recorder/stop" >/dev/null || true
  vagrant ssh node-a -c "deadline=\$((SECONDS + 90)); until sudo test -f /run/sysarmor/recorder/done; do if (( SECONDS >= deadline )); then sudo cat /run/sysarmor/recorder/recorder.log 2>/dev/null || true; exit 1; fi; sleep 1; done; sudo cat /run/sysarmor/recorder/timeline.csv" \
    > "$OUT_DIR/timeline.csv" 2>"$OUT_DIR/timeline.err"
  vagrant ssh node-a -c "sudo cat /run/sysarmor/recorder/recorder.log 2>/dev/null || true" \
    > "$OUT_DIR/recorder.log" 2>/dev/null || true
  vagrant ssh node-a -c "sudo cat /run/sysarmor/recorder/events.ndjson 2>/dev/null || true" \
    > "$OUT_DIR/events.ndjson" 2>/dev/null || true
  vagrant ssh node-a -c "sudo cat /run/sysarmor/recorder/signals.ndjson 2>/dev/null || true" \
    > "$OUT_DIR/signals.ndjson" 2>/dev/null || true
}

case "$CMD" in
  start)
    : > "$OUT_DIR/markers.ndjson"
    mark_local "recorder_start" ""
    start_remote_sampler
    echo "[recorder-vm] started RUN_ID=$RUN_ID OUT_DIR=$OUT_DIR"
    ;;
  mark)
    if [[ -z "$PHASE" ]]; then
      echo "[recorder-vm][ERROR] PHASE is required for mark" >&2
      exit 2
    fi
    mark_local "$PHASE" "$DETAIL"
    echo "[recorder-vm] mark RUN_ID=$RUN_ID PHASE=$PHASE"
    ;;
  stop)
    mark_local "recorder_stop" ""
    stop_remote_sampler
    echo "[recorder-vm] stopped RUN_ID=$RUN_ID OUT_DIR=$OUT_DIR"
    ;;
  report)
    python3 "$HERE/../benchmarks/bench_lifecycle_report.py" "$OUT_DIR"
    echo "[recorder-vm] report written to $OUT_DIR/summary.json"
    ;;
  *)
    echo "usage: recorder-vm.sh <start|mark|stop|report>" >&2
    exit 2
    ;;
esac
