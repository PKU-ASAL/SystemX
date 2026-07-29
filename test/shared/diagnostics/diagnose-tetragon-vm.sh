#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
VM_ENV="${SYSARMOR_VM_ENV:-${ENV:-vm-endpoint}}"
ENVDIR="$(cd "$ROOT/environments/$VM_ENV" && pwd)"
RESULTS="$ROOT/.results"
OUT_PREFIX="${SYSARMOR_DIAG_PREFIX:-tetragon-diagnostics-vm}"
PPROF_ADDRESS="${SYSARMOR_TETRAGON_PPROF_ADDRESS:-127.0.0.1:6060}"
MODE="${SYSARMOR_DIAG_MODE:-idle}"
WORKLOAD="${SCENARIO:-${SYSARMOR_DIAG_WORKLOAD:-edr-activity-heavy}}"
SAMPLE_SECONDS="${SYSARMOR_DIAG_SECONDS:-10}"
PERF_RECORD_SECONDS="${SYSARMOR_DIAG_PERF_RECORD_SECONDS:-30}"
PERF_RECORD_EVENT="${SYSARMOR_DIAG_PERF_RECORD_EVENT:-cpu-clock}"
PERF_RECORD_FREQ="${SYSARMOR_DIAG_PERF_RECORD_FREQ:-99}"
PERF_STAT_EVENTS="${SYSARMOR_DIAG_PERF_STAT_EVENTS:-task-clock,context-switches,cpu-migrations,page-faults,cpu-clock}"
WORKLOAD_WARMUP_SECONDS="${SYSARMOR_DIAG_WORKLOAD_WARMUP_SECONDS:-3}"
WORKLOAD_REPEAT="${SYSARMOR_DIAG_WORKLOAD_REPEAT:-3}"
WORKLOAD_C2="${SYSARMOR_DIAG_WORKLOAD_C2:-10.66.0.99}"
AGENT_ID=""
TENANT_ID=""
AGENT_SOCK="/run/sysarmor/agent/control.sock"
ENABLE_PPROF="${SYSARMOR_DIAG_ENABLE_PPROF:-1}"
RESTORE_CONFIG="${SYSARMOR_DIAG_RESTORE_CONFIG:-1}"
CONFIG_CHANGED=0
RESTORED=0

restore_config() {
  if [[ "$CONFIG_CHANGED" == "1" && "$RESTORE_CONFIG" == "1" && "$RESTORED" == "0" ]]; then
    vagrant ssh node-a -c "sudo test -f /tmp/sysarmor-agent.yaml.before-diag && sudo cp /tmp/sysarmor-agent.yaml.before-diag /etc/sysarmor/agent.yaml && sudo systemctl restart sysarmor-agent" >/dev/null || true
    RESTORED=1
  fi
}

trap restore_config EXIT

mkdir -p "$RESULTS"
mkdir -p "$(dirname "$RESULTS/$OUT_PREFIX")"

cd "$ENVDIR"

wait_agent_socket() {
  AGENT_SOCK="$(vagrant ssh node-a -c "sudo awk '/socket_path:/ {print \$2}' /etc/sysarmor/agent.yaml 2>/dev/null | tail -1" 2>/dev/null | tr -d '\r')"
  if [[ -z "$AGENT_SOCK" ]]; then
    AGENT_SOCK="/run/sysarmor/agent/control.sock"
  fi
  local deadline=$((SECONDS + 60))
  until vagrant ssh node-a -c "sudo test -S '$AGENT_SOCK'" >/dev/null 2>&1; do
    if (( SECONDS >= deadline )); then
      echo "[diagnose-tetragon-vm][ERROR] timeout waiting for agent socket: $AGENT_SOCK" >&2
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
    echo "[diagnose-tetragon-vm][ERROR] Agent health did not expose runtime identity: $health" >&2
    exit 1
  fi
}

if [[ "$MODE" != "idle" && "$MODE" != "workload" ]]; then
  echo "[diagnose-tetragon-vm][ERROR] unsupported SYSARMOR_DIAG_MODE: $MODE" >&2
  exit 1
fi
if [[ "$MODE" == "workload" && ! -f "$ROOT/data/workloads/vm/$WORKLOAD/run.sh" && ! -f "$ROOT/data/scenarios/vm/$WORKLOAD/attack.sh" ]]; then
  echo "[diagnose-tetragon-vm][ERROR] workload/scenario not found: $WORKLOAD" >&2
  exit 1
fi

echo "[diagnose-tetragon-vm] preparing diagnostic mode=$MODE workload=$WORKLOAD"
if [[ "$ENABLE_PPROF" == "1" ]]; then
  vagrant ssh node-a -c "sudo cp /etc/sysarmor/agent.yaml /tmp/sysarmor-agent.yaml.before-diag && sudo sed -i '/^  pprof_address:/d' /etc/sysarmor/agent.yaml && sudo sed -i '/^sensor:/a\  pprof_address: $PPROF_ADDRESS' /etc/sysarmor/agent.yaml && sudo systemctl restart sysarmor-agent" >/dev/null
  CONFIG_CHANGED=1
  wait_agent_socket
fi

if [[ "$MODE" == "workload" ]]; then
  if [[ -f "$ROOT/data/workloads/vm/$WORKLOAD/run.sh" ]]; then
    vagrant upload "$ROOT/data/workloads/vm/$WORKLOAD/run.sh" /tmp/sysarmor-diag-workload.sh node-a >/dev/null
  else
    vagrant ssh node-a -c "sudo cp '/vagrant/test/data/scenarios/vm/$WORKLOAD/attack.sh' /tmp/sysarmor-diag-workload.sh && sudo chmod +x /tmp/sysarmor-diag-workload.sh" >/dev/null
  fi
fi

wait_agent_socket
resolve_agent_identity

echo "[diagnose-tetragon-vm] collecting perf/pprof/strace artifacts"
vagrant ssh node-a -c "sudo bash -c '
set +e
pid=\$(pidof tetragon | awk \"{print \\\$1}\")
if [ -z \"\$pid\" ]; then
  echo \"tetragon pid not found\" >&2
  exit 0
fi
rm -f /tmp/sysarmor-tetragon-strace-c.txt \
  /tmp/sysarmor-tetragon-perf-stat.txt \
  /tmp/sysarmor-tetragon-perf-report.txt \
  /tmp/sysarmor-tetragon-perf-meta.txt \
  /tmp/sysarmor-tetragon-perf.data \
  /tmp/sysarmor-tetragon-health-before.json \
  /tmp/sysarmor-tetragon-health-after.json \
  /tmp/sysarmor-tetragon-signals.ndjson \
  /tmp/sysarmor-tetragon-workload.out \
  /tmp/sysarmor-tetragon-workload.err \
  /tmp/sysarmor-tetragon-summary.json \
  /tmp/sysarmor-tetragon-pprof-goroutine.txt \
  /tmp/sysarmor-tetragon-pprof-cpu.pb.gz \
  /tmp/sysarmor-tetragon-*.err \
  /tmp/sysarmor-tetragon-*.out
{
  echo \"pid=\$pid\"
  echo \"mode=${MODE}\"
  echo \"workload=${WORKLOAD}\"
  echo \"sample_seconds=${SAMPLE_SECONDS}\"
  echo \"perf_record_seconds=${PERF_RECORD_SECONDS}\"
  echo \"perf_record_event=${PERF_RECORD_EVENT}\"
  echo \"perf_record_freq=${PERF_RECORD_FREQ}\"
  echo \"perf_stat_events=${PERF_STAT_EVENTS}\"
  echo \"workload_warmup_seconds=${WORKLOAD_WARMUP_SECONDS}\"
  echo \"workload_repeat=${WORKLOAD_REPEAT}\"
} >/tmp/sysarmor-tetragon-perf-meta.txt

agent_sock=\$(awk \"/socket_path:/ {print \\\$2}\" /etc/sysarmor/agent.yaml 2>/dev/null | tail -1)
if [ -z \"\$agent_sock\" ]; then
  agent_sock=/run/sysarmor/agent/control.sock
fi
sysarmorctl --socket \"\$agent_sock\" --json agent health --agent-id ${AGENT_ID} --tenant-id ${TENANT_ID} >/tmp/sysarmor-tetragon-health-before.json 2>/tmp/sysarmor-tetragon-health-before.err || true

perf stat -e ${PERF_STAT_EVENTS} -p \"\$pid\" -- sleep ${PERF_RECORD_SECONDS} >/tmp/sysarmor-tetragon-perf-stat.out 2>/tmp/sysarmor-tetragon-perf-stat.txt &
stat_pid=\$!
perf record -e ${PERF_RECORD_EVENT} -F ${PERF_RECORD_FREQ} -p \"\$pid\" -g -o /tmp/sysarmor-tetragon-perf.data -- sleep ${PERF_RECORD_SECONDS} >/tmp/sysarmor-tetragon-perf-record.out 2>/tmp/sysarmor-tetragon-perf-record.err &
record_pid=\$!
curl -fsS \"http://${PPROF_ADDRESS}/debug/pprof/profile?seconds=${SAMPLE_SECONDS}\" -o /tmp/sysarmor-tetragon-pprof-cpu.pb.gz >/tmp/sysarmor-tetragon-pprof-cpu.out 2>/tmp/sysarmor-tetragon-pprof-cpu.err &
pprof_pid=\$!

if [ \"${MODE}\" = \"workload\" ]; then
  sleep ${WORKLOAD_WARMUP_SECONDS}
  for i in \$(seq 1 ${WORKLOAD_REPEAT}); do
    echo \"[workload] workload=${WORKLOAD} iteration=\$i\" >>/tmp/sysarmor-tetragon-workload.out
    REPEAT=1 C2=${WORKLOAD_C2} GAP=1 bash /tmp/sysarmor-diag-workload.sh >>/tmp/sysarmor-tetragon-workload.out 2>>/tmp/sysarmor-tetragon-workload.err || echo \"[workload] iteration=\$i exit=\$?\" >>/tmp/sysarmor-tetragon-workload.err
  done
else
  timeout ${SAMPLE_SECONDS}s strace -f -c -p \"\$pid\" -o /tmp/sysarmor-tetragon-strace-c.txt >/tmp/sysarmor-tetragon-strace-c.out 2>/tmp/sysarmor-tetragon-strace-c.err || true
fi

wait \$record_pid || true
wait \$stat_pid || true
wait \$pprof_pid || true
if [ \"${MODE}\" = \"workload\" ]; then
  echo \"strace skipped in workload mode to avoid perturbing perf samples\" >/tmp/sysarmor-tetragon-strace-c.txt
fi

perf report --stdio -i /tmp/sysarmor-tetragon-perf.data >/tmp/sysarmor-tetragon-perf-report.txt 2>/tmp/sysarmor-tetragon-perf-report.err || true
curl -fsS \"http://${PPROF_ADDRESS}/debug/pprof/goroutine?debug=1\" -o /tmp/sysarmor-tetragon-pprof-goroutine.txt >/tmp/sysarmor-tetragon-pprof-goroutine.out 2>/tmp/sysarmor-tetragon-pprof-goroutine.err || true
sysarmorctl --socket \"\$agent_sock\" --json agent health --agent-id ${AGENT_ID} --tenant-id ${TENANT_ID} >/tmp/sysarmor-tetragon-health-after.json 2>/tmp/sysarmor-tetragon-health-after.err || true
sysarmorctl --socket \"\$agent_sock\" --json signal watch --include-recent --snapshot --limit 2000 --agent-id ${AGENT_ID} --tenant-id ${TENANT_ID} --timeout 5s >/tmp/sysarmor-tetragon-signals.ndjson 2>/tmp/sysarmor-tetragon-signals.err || true

if grep -Fq \"data has no samples\" /tmp/sysarmor-tetragon-perf-report.err 2>/dev/null; then
  perf_samples=false
else
  perf_samples=true
fi
cat >/tmp/sysarmor-tetragon-summary.json <<EOS
{
  \"mode\": \"${MODE}\",
  \"workload\": \"${WORKLOAD}\",
  \"pid\": \"\$pid\",
  \"sample_seconds\": ${SAMPLE_SECONDS},
  \"perf_record_seconds\": ${PERF_RECORD_SECONDS},
  \"perf_record_event\": \"${PERF_RECORD_EVENT}\",
  \"perf_record_freq\": ${PERF_RECORD_FREQ},
  \"workload_repeat\": ${WORKLOAD_REPEAT},
  \"perf_samples\": \$perf_samples
}
EOS
'" > "$RESULTS/$OUT_PREFIX.remote.out" 2>"$RESULTS/$OUT_PREFIX.remote.err" || true

vagrant ssh node-a -c "sudo cat /tmp/sysarmor-tetragon-strace-c.txt 2>/dev/null || true" \
  > "$RESULTS/$OUT_PREFIX.strace-c.txt" 2>"$RESULTS/$OUT_PREFIX.strace-c.err"
vagrant ssh node-a -c "sudo cat /tmp/sysarmor-tetragon-perf-stat.txt 2>/dev/null || true" \
  > "$RESULTS/$OUT_PREFIX.perf-stat.txt" 2>/dev/null || true
vagrant ssh node-a -c "sudo cat /tmp/sysarmor-tetragon-perf-report.txt 2>/dev/null || true" \
  > "$RESULTS/$OUT_PREFIX.perf-report.txt" 2>"$RESULTS/$OUT_PREFIX.perf-report.err"
vagrant ssh node-a -c "sudo cat /tmp/sysarmor-tetragon-perf-meta.txt 2>/dev/null || true" \
  > "$RESULTS/$OUT_PREFIX.perf-meta.txt" 2>/dev/null || true
vagrant ssh node-a -c "sudo cat /tmp/sysarmor-tetragon-summary.json 2>/dev/null || true" \
  > "$RESULTS/$OUT_PREFIX.summary.json" 2>/dev/null || true
vagrant ssh node-a -c "sudo cat /tmp/sysarmor-tetragon-health-before.json 2>/dev/null || true" \
  > "$RESULTS/$OUT_PREFIX.health-before.json" 2>/dev/null || true
vagrant ssh node-a -c "sudo cat /tmp/sysarmor-tetragon-health-after.json 2>/dev/null || true" \
  > "$RESULTS/$OUT_PREFIX.health-after.json" 2>/dev/null || true
vagrant ssh node-a -c "sudo cat /tmp/sysarmor-tetragon-signals.ndjson 2>/dev/null || true" \
  > "$RESULTS/$OUT_PREFIX.signals.ndjson" 2>/dev/null || true
vagrant ssh node-a -c "sudo cat /tmp/sysarmor-tetragon-workload.out 2>/dev/null || true" \
  > "$RESULTS/$OUT_PREFIX.workload.out" 2>/dev/null || true
vagrant ssh node-a -c "sudo cat /tmp/sysarmor-tetragon-workload.err 2>/dev/null || true" \
  > "$RESULTS/$OUT_PREFIX.workload.err" 2>/dev/null || true
vagrant ssh node-a -c "sudo cat /tmp/sysarmor-tetragon-perf-record.err 2>/dev/null || true; sudo cat /tmp/sysarmor-tetragon-perf-report.err 2>/dev/null || true" \
  > "$RESULTS/$OUT_PREFIX.perf.vm.err" 2>/dev/null || true
vagrant ssh node-a -c "sudo cat /tmp/sysarmor-tetragon-pprof-goroutine.txt 2>/dev/null || true" \
  > "$RESULTS/$OUT_PREFIX.pprof-goroutine.txt" 2>/dev/null || true
vagrant ssh node-a -c "sudo cat /tmp/sysarmor-tetragon-pprof-cpu.pb.gz 2>/dev/null || true" \
  > "$RESULTS/$OUT_PREFIX.pprof-cpu.pb.gz" 2>/dev/null || true
vagrant ssh node-a -c "sudo cat /tmp/sysarmor-tetragon-pprof-goroutine.err 2>/dev/null || true; sudo cat /tmp/sysarmor-tetragon-pprof-cpu.err 2>/dev/null || true" \
  > "$RESULTS/$OUT_PREFIX.pprof.err" 2>/dev/null || true
vagrant ssh node-a -c "ps -ef | grep tetragon | grep -v grep || true" \
  > "$RESULTS/$OUT_PREFIX.ps.txt" 2>/dev/null || true

if [[ "$CONFIG_CHANGED" == "1" && "$RESTORE_CONFIG" == "1" ]]; then
  echo "[diagnose-tetragon-vm] restoring agent config"
  restore_config
  wait_agent_socket
fi

echo "[diagnose-tetragon-vm] artifacts written to $RESULTS/$OUT_PREFIX.*"
