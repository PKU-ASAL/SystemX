#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../../.." && pwd)"
REPO="$(cd "$ROOT/.." && pwd)"
VM_ENV="${SYSARMOR_VM_ENV:-${ENV:-vm-endpoint}}"
ENVDIR="$(cd "$ROOT/environments/$VM_ENV" && pwd)"
RESULTS="$ROOT/.results"
SCENARIO="${SCENARIO:-apt-staged-drop-owned-vm}"
DUR="${DUR:-8}"
C2="${C2:-10.66.0.99}"
GAP="${GAP:-3}"
TETRAGON_ARCHIVE="${SYSARMOR_TETRAGON_ARCHIVE:-}"
TETRAGON_CGROUP_RATE="${SYSARMOR_TETRAGON_CGROUP_RATE:-}"
TETRAGON_PROCESS_CACHE_SIZE="${SYSARMOR_TETRAGON_PROCESS_CACHE_SIZE:-4096}"
TETRAGON_DATA_CACHE_SIZE="${SYSARMOR_TETRAGON_DATA_CACHE_SIZE:-128}"
TETRAGON_EVENT_QUEUE_SIZE="${SYSARMOR_TETRAGON_EVENT_QUEUE_SIZE:-1024}"
TETRAGON_RB_QUEUE_SIZE="${SYSARMOR_TETRAGON_RB_QUEUE_SIZE:-8192}"
CAPTURE_PERF="${SYSARMOR_CAPTURE_PERF:-1}"

mkdir -p "$RESULTS"

cleanup() {
  (
    cd "$ENVDIR"
    vagrant ssh node-a -c "sudo systemctl stop sysarmor-agent 2>/dev/null || true; sudo pkill -x tetragon 2>/dev/null || true; sudo pkill -x tetra 2>/dev/null || true" >/dev/null 2>&1 || true
  )
}
trap cleanup EXIT

echo "[e2e-agent-real-tetragon-owned-vm] starting VM topology"
bash "$ROOT/shared/harness/start-vm.sh" "$VM_ENV" >/dev/null

cd "$ENVDIR"

echo "[e2e-agent-real-tetragon-owned-vm] installing agent distribution with owned real tetragon"
if [[ -z "$TETRAGON_ARCHIVE" ]]; then
  echo "[e2e-agent-real-tetragon-owned-vm][ERROR] SYSARMOR_TETRAGON_ARCHIVE is required" >&2
  exit 1
fi
vagrant upload "$REPO/dist/bin/sysarmor-agent" /tmp/sysarmor-agent.upload node-a >/dev/null
vagrant upload "$REPO/dist/bin/sysarmorctl" /tmp/sysarmorctl.upload node-a >/dev/null
vagrant upload "$REPO/dist/bin/sysarmor-content-sign" /tmp/sysarmor-content-sign.upload node-a >/dev/null
vagrant upload "$REPO/deployments" /tmp/sysarmor-deployments.upload node-a >/dev/null
vagrant upload "$REPO/deployments/agent/content" /tmp/sysarmor-content.upload node-a >/dev/null
vagrant upload "$REPO/test/data/policies/collection-balanced.json" /tmp/sysarmor-collection-balanced.json node-a >/dev/null
vagrant upload "$TETRAGON_ARCHIVE" /tmp/sysarmor-tetragon.upload node-a >/dev/null

TETRAGON_BUNDLE_DIR="${TETRAGON_BUNDLE_DIR:-/opt/sysarmor/agent/bundles/tetragon}"
TETRAGON_INSTALL_DIR="${TETRAGON_INSTALL_DIR:-/opt/sysarmor/agent/sensors}"
TETRA_PATH="$TETRAGON_INSTALL_DIR/tetragon/current/bin/tetra"
TETRAGON_PATH="$TETRAGON_INSTALL_DIR/tetragon/current/bin/tetragon"
AGENT_SOCK="/run/sysarmor/agent/control.sock"

vagrant ssh node-a -c "sudo systemctl stop sysarmor-agent 2>/dev/null || true; sudo systemctl disable sysarmor-agent 2>/dev/null || true; sudo systemctl reset-failed sysarmor-agent 2>/dev/null || true; sudo systemctl stop tetragon 2>/dev/null || true; sudo systemctl disable tetragon 2>/dev/null || true; sudo pkill -x sysarmor-agent 2>/dev/null || true; sudo pkill -x tetragon 2>/dev/null || true; sudo pkill -x tetra 2>/dev/null || true" >/dev/null

vagrant ssh node-a -c "sudo tee /tmp/sysarmor-agent.yaml >/dev/null <<EOF
agent:
  label.scenario: $SCENARIO

control:
  socket_path: $AGENT_SOCK

sensor:
  backend: tetragon
  mode: managed
  bundle_dir: $TETRAGON_BUNDLE_DIR
  install_dir: $TETRAGON_INSTALL_DIR
  scope:
    type: host
  observe_only: true
  restart: always
  max_restarts: 3
  restart_window: 500ms
  cgroup_rate: $TETRAGON_CGROUP_RATE
  process_cache_size: $TETRAGON_PROCESS_CACHE_SIZE
  data_cache_size: $TETRAGON_DATA_CACHE_SIZE
  event_queue_size: $TETRAGON_EVENT_QUEUE_SIZE
  rb_queue_size: $TETRAGON_RB_QUEUE_SIZE

telemetry:
  max_batch_items: 256
  flush_interval: 200ms

local:
  state_path: /var/lib/sysarmor/agent/telemetry-owned-tetragon
  export:
    retry_initial: 100ms
    retry_max: 500ms
    request_timeout: 2s

policy:
  path: /etc/sysarmor/agent/policy.json

health:
  interval: 500ms
EOF
sudo systemctl stop sysarmor-agent 2>/dev/null || true
sudo systemctl disable sysarmor-agent 2>/dev/null || true
sudo systemctl reset-failed sysarmor-agent 2>/dev/null || true
sudo rm -rf /var/lib/sysarmor/agent/telemetry-owned-tetragon '$TETRAGON_BUNDLE_DIR' '$TETRAGON_INSTALL_DIR/tetragon'
sudo systemctl daemon-reload" >/dev/null

vagrant ssh node-a -c "if ! sudo SYSARMOR_AGENT_BIN=/tmp/sysarmor-agent.upload SYSARMOR_CTL_BIN=/tmp/sysarmorctl.upload SYSARMOR_CONTENT_SIGN_BIN=/tmp/sysarmor-content-sign.upload SYSARMOR_AGENT_CONFIG=/tmp/sysarmor-agent.yaml SYSARMOR_COLLECTION_POLICY=/tmp/sysarmor-deployments.upload/agent/policy.json SYSARMOR_TETRAGON_BUNDLE_DIR='$TETRAGON_BUNDLE_DIR' SYSARMOR_TETRAGON_INSTALL_DIR='$TETRAGON_INSTALL_DIR' SYSARMOR_TETRAGON_ARCHIVE=/tmp/sysarmor-tetragon.upload bash /tmp/sysarmor-deployments.upload/agent/install-agent.sh >/tmp/sysarmor-install-agent.log 2>&1; then sudo cat /tmp/sysarmor-install-agent.log >&2; exit 1; fi" >/dev/null
vagrant ssh node-a -c "sudo test -f '$TETRAGON_BUNDLE_DIR/manifest.json'" >/dev/null
vagrant ssh node-a -c "sudo systemctl restart sysarmor-agent" >/dev/null

wait_contains() {
  local name="$1"
  local needle="$2"
  local out="$3"
  shift 3
  local deadline=$((SECONDS + 90))
  until "$@" >"$out" 2>"$out.err" && grep -Fq "$needle" "$out"; do
    if (( SECONDS >= deadline )); then
      echo "[e2e-agent-real-tetragon-owned-vm][ERROR] timeout waiting for $needle via $name" >&2
      echo "--- last response ---" >&2
      cat "$out" >&2 2>/dev/null || true
      echo "--- last error ---" >&2
      cat "$out.err" >&2 2>/dev/null || true
      echo "--- systemd status ---" >&2
      vagrant ssh node-a -c "sudo systemctl status sysarmor-agent --no-pager -l || true" >&2 2>/dev/null || true
      echo "--- agent journal ---" >&2
      vagrant ssh node-a -c "sudo journalctl -u sysarmor-agent --no-pager -n 160 || true" >&2 2>/dev/null || true
      echo "--- tetragon process ---" >&2
      vagrant ssh node-a -c "ps -ef | grep tetragon | grep -v grep || true" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 1
  done
}

wait_socket() {
  local path="$1"
  local deadline=$((SECONDS + 90))
  until vagrant ssh node-a -c "sudo test -S '$path'" >/dev/null 2>&1; do
    if (( SECONDS >= deadline )); then
      echo "[e2e-agent-real-tetragon-owned-vm][ERROR] timeout waiting for agent socket: $path" >&2
      echo "--- systemd status ---" >&2
      vagrant ssh node-a -c "sudo systemctl status sysarmor-agent --no-pager -l || true" >&2 2>/dev/null || true
      echo "--- agent journal ---" >&2
      vagrant ssh node-a -c "sudo journalctl -u sysarmor-agent --no-pager -n 200 || true" >&2 2>/dev/null || true
      echo "--- install log ---" >&2
      vagrant ssh node-a -c "sudo cat /tmp/sysarmor-install-agent.log 2>/dev/null || true" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 1
  done
}

wait_absent() {
  local name="$1"
  local pattern="$2"
  local out="$3"
  shift 3
  local deadline=$((SECONDS + 60))
  while "$@" >"$out" 2>"$out.err"; do
    if (( SECONDS >= deadline )); then
      echo "[e2e-agent-real-tetragon-owned-vm][ERROR] $name still present after service stop" >&2
      echo "--- matching process ---" >&2
      cat "$out" >&2 2>/dev/null || true
      echo "--- agent journal ---" >&2
      vagrant ssh node-a -c "sudo journalctl -u sysarmor-agent --no-pager -n 160 || true" >&2 2>/dev/null || true
      exit 1
    fi
    if ! grep -Fq "$pattern" "$out"; then
      return 0
    fi
    sleep 1
  done
  : >"$out"
}

wait_socket "$AGENT_SOCK"

echo "[e2e-agent-real-tetragon-owned-vm] applying EDR-balanced content and collection policy via local control"
for content_name in \
  context-credential-path-prefixes.json \
  context-payload-path-prefixes.json \
  context-persistence-path-prefixes.json \
  context-secret-volume-prefixes.json \
  ioc-c2-ip-feed.json \
  ioc-c2-port-feed.json; do
  vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json content apply --file '/tmp/sysarmor-content.upload/$content_name' --allow-unsigned --agent-id vm-owned-tetragon --tenant-id default" \
    > "$RESULTS/e2e-agent-real-tetragon-owned-vm.content.$content_name.json" \
    2>"$RESULTS/e2e-agent-real-tetragon-owned-vm.content.$content_name.json.err"
done
vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json policy apply collection --file /tmp/sysarmor-collection-balanced.json --agent-id vm-owned-tetragon --tenant-id default --timeout 60s" \
  > "$RESULTS/e2e-agent-real-tetragon-owned-vm.collection-apply.json" \
  2>"$RESULTS/e2e-agent-real-tetragon-owned-vm.collection-apply.json.err"
if ! grep -Fq 'resolved_refs' "$RESULTS/e2e-agent-real-tetragon-owned-vm.collection-apply.json"; then
  echo "[e2e-agent-real-tetragon-owned-vm][ERROR] collection apply did not report resolvedRefs" >&2
  cat "$RESULTS/e2e-agent-real-tetragon-owned-vm.collection-apply.json" >&2 2>/dev/null || true
  cat "$RESULTS/e2e-agent-real-tetragon-owned-vm.collection-apply.json.err" >&2 2>/dev/null || true
  exit 1
fi
if ! grep -Fq 'process.binary_prefix' "$RESULTS/e2e-agent-real-tetragon-owned-vm.collection-apply.json"; then
  echo "[e2e-agent-real-tetragon-owned-vm][ERROR] collection apply did not report process.binary_prefix pushdown" >&2
  cat "$RESULTS/e2e-agent-real-tetragon-owned-vm.collection-apply.json" >&2 2>/dev/null || true
  exit 1
fi

wait_contains "agent-health backend" '"backend":"tetragon"' "$RESULTS/e2e-agent-real-tetragon-owned-vm.health.json" \
  vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json agent health --agent-id vm-owned-tetragon --tenant-id default"
wait_contains "agent-health ok" '"status":"ok"' "$RESULTS/e2e-agent-real-tetragon-owned-vm.health.json" \
  vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json agent health --agent-id vm-owned-tetragon --tenant-id default"
wait_contains "agent-health capability kernel" '"kernelRelease":' "$RESULTS/e2e-agent-real-tetragon-owned-vm.health.json" \
  vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json agent health --agent-id vm-owned-tetragon --tenant-id default"
wait_contains "agent-health capability btf" '"btfAvailable":true' "$RESULTS/e2e-agent-real-tetragon-owned-vm.health.json" \
  vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json agent health --agent-id vm-owned-tetragon --tenant-id default"
wait_contains "agent-health capability bpffs" '"bpffsAvailable":true' "$RESULTS/e2e-agent-real-tetragon-owned-vm.health.json" \
  vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json agent health --agent-id vm-owned-tetragon --tenant-id default"
wait_contains "agent-health policy" '"policyLoaded":true' "$RESULTS/e2e-agent-real-tetragon-owned-vm.health.json" \
  vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json agent health --agent-id vm-owned-tetragon --tenant-id default"
wait_contains "agent capability collection" 'process.exec' "$RESULTS/e2e-agent-real-tetragon-owned-vm.capability.json" \
  vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json agent capability --agent-id vm-owned-tetragon --tenant-id default"
wait_contains "agent current policy" '"policyId":"default-edr-policy"' "$RESULTS/e2e-agent-real-tetragon-owned-vm.policy.json" \
  vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json policy current --agent-id vm-owned-tetragon --tenant-id default"
wait_contains "agent-owned tracing policy" 'sysarmor-runtime-collection' "$RESULTS/e2e-agent-real-tetragon-owned-vm.tracingpolicy.txt" \
  vagrant ssh node-a -c "sudo '$TETRA_PATH' tracingpolicy list"
wait_contains "agent-owned tetragon process" "$TETRAGON_PATH" "$RESULTS/e2e-agent-real-tetragon-owned-vm.ps.txt" \
  vagrant ssh node-a -c "ps -ef | grep tetragon | grep -v grep"

echo "[e2e-agent-real-tetragon-owned-vm] running apt-staged-drop attack"
vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json agent health --agent-id vm-owned-tetragon --tenant-id default" \
  > "$RESULTS/e2e-agent-real-tetragon-owned-vm.health-before-attack.json" 2>"$RESULTS/e2e-agent-real-tetragon-owned-vm.health-before-attack.json.err"
if [[ "$CAPTURE_PERF" == "1" ]]; then
  rm -f "$RESULTS/perf-resource.vm.$SCENARIO.csv"
  vagrant ssh node-a -c "sudo tee /tmp/sysarmor-vm-perf-sampler.sh >/dev/null <<'EOS'
#!/usr/bin/env bash
set -euo pipefail
DUR=\"\${1:-30}\"
INTERVAL=\"\${2:-2}\"
SCENARIO=\"\${3:-idle}\"
OUT=/tmp/sysarmor-perf-resource.csv
DONE=/tmp/sysarmor-perf-resource.done
rm -f \"\$OUT\" \"\$DONE\"
echo 'topology,scenario,sample_ts,elapsed_s,edr_cpu_pct,edr_rss_mb,agent_cpu_pct,agent_rss_mb,tetragon_cpu_pct,tetragon_rss_mb,tetra_cpu_pct,tetra_rss_mb,workload_cpu_pct,workload_rss_mb,business_latency_p95_ms,business_throughput_rps,dropped_events,parse_errors,notes' > \"\$OUT\"
pair() {
  local pattern=\"\$1\"
  ps -eo comm,pcpu,rss 2>/dev/null | awk -v p=\"\$pattern\" '\$1 ~ p { cpu += \$2; rss += \$3 } END { printf \"%.2f,%.1f\", cpu, rss / 1024 }'
}
sum3() {
  awk -F, -v a=\"\$1\" -v b=\"\$2\" -v c=\"\$3\" 'BEGIN { split(a, aa, \",\"); split(b, bb, \",\"); split(c, cc, \",\"); printf \"%.2f,%.1f\", aa[1] + bb[1] + cc[1], aa[2] + bb[2] + cc[2] }'
}
elapsed=0
while (( elapsed <= DUR )); do
  ts=\"\$(date -u +%Y-%m-%dT%H:%M:%SZ)\"
  agent=\"\$(pair '^sysarmor-agent$')\"
  tetragon=\"\$(pair '^tetragon$')\"
  tetra=\"\$(pair '^tetra$')\"
  total=\"\$(sum3 \"\$agent\" \"\$tetragon\" \"\$tetra\")\"
  workload=\"\$(pair '^(java|bash|curl|python|node|nginx|apache2)$')\"
  echo \"vm,\$SCENARIO,\$ts,\$elapsed,\$total,\$agent,\$tetragon,\$tetra,\$workload,0,0,0,0,vm_in_guest_sampler\" >> \"\$OUT\"
  (( elapsed >= DUR )) && break
  sleep \"\$INTERVAL\"
  elapsed=\$((elapsed + INTERVAL))
done
touch \"\$DONE\"
EOS
sudo chmod +x /tmp/sysarmor-vm-perf-sampler.sh
sudo nohup /tmp/sysarmor-vm-perf-sampler.sh '$((DUR + GAP + 6))' '${PERF_INTERVAL:-2}' '$SCENARIO' >/tmp/sysarmor-vm-perf-sampler.log 2>&1 &" \
    > "$RESULTS/e2e-agent-real-tetragon-owned-vm.perf-resource.out" 2>"$RESULTS/e2e-agent-real-tetragon-owned-vm.perf-resource.err"
fi
vagrant ssh node-a -c "sudo bash -c 'GAP=$GAP C2=$C2 bash /vagrant/test/data/scenarios/vm/apt-staged-drop/attack.sh'" >/dev/null
sleep "$DUR"
vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json agent health --agent-id vm-owned-tetragon --tenant-id default" \
  > "$RESULTS/e2e-agent-real-tetragon-owned-vm.health-after-attack.json" 2>"$RESULTS/e2e-agent-real-tetragon-owned-vm.health-after-attack.json.err"
if [[ "$CAPTURE_PERF" == "1" ]]; then
  vagrant ssh node-a -c "deadline=\$((SECONDS + 60)); until sudo test -f /tmp/sysarmor-perf-resource.done; do if (( SECONDS >= deadline )); then sudo cat /tmp/sysarmor-vm-perf-sampler.log 2>/dev/null || true; exit 1; fi; sleep 1; done; sudo cat /tmp/sysarmor-perf-resource.csv" \
    > "$RESULTS/perf-resource.vm.$SCENARIO.csv" 2>>"$RESULTS/e2e-agent-real-tetragon-owned-vm.perf-resource.err"
  python3 "$ROOT/shared/reports/local_perf_report.py" "$SCENARIO" \
    "$RESULTS/e2e-agent-real-tetragon-owned-vm.health-before-attack.json" \
    "$RESULTS/e2e-agent-real-tetragon-owned-vm.health-after-attack.json" \
    "$RESULTS/perf-resource.vm.$SCENARIO.csv" \
    "$RESULTS/e2e-agent-real-tetragon-owned-vm.perf-summary.json"
fi

vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json signal watch --include-recent --snapshot --limit 2000 --agent-id vm-owned-tetragon --tenant-id default --timeout 20s" \
  > "$RESULTS/e2e-agent-real-tetragon-owned-vm.local-signals.ndjson" 2>"$RESULTS/e2e-agent-real-tetragon-owned-vm.local-signals.ndjson.err"
if ! grep -Fq '"name":"payload_dropped"' "$RESULTS/e2e-agent-real-tetragon-owned-vm.local-signals.ndjson"; then
  echo "[e2e-agent-real-tetragon-owned-vm][ERROR] local attack signal not found: payload_dropped" >&2
  cat "$RESULTS/e2e-agent-real-tetragon-owned-vm.local-signals.ndjson" >&2 2>/dev/null || true
  exit 1
fi
if ! grep -Fq '"name":"payload_lifecycle"' "$RESULTS/e2e-agent-real-tetragon-owned-vm.local-signals.ndjson"; then
  echo "[e2e-agent-real-tetragon-owned-vm][ERROR] local multi-event attack signal not found: payload_lifecycle" >&2
  cat "$RESULTS/e2e-agent-real-tetragon-owned-vm.local-signals.ndjson" >&2 2>/dev/null || true
  exit 1
fi
if ! grep -Fq '"where":"SIGNAL_WHERE_ENDPOINT"' "$RESULTS/e2e-agent-real-tetragon-owned-vm.local-signals.ndjson"; then
  echo "[e2e-agent-real-tetragon-owned-vm][ERROR] local attack signal is not endpoint-scoped" >&2
  cat "$RESULTS/e2e-agent-real-tetragon-owned-vm.local-signals.ndjson" >&2 2>/dev/null || true
  exit 1
fi
vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json event watch --include-recent --snapshot --limit 8192 --agent-id vm-owned-tetragon --tenant-id default --timeout 20s" \
  > "$RESULTS/e2e-agent-real-tetragon-owned-vm.local-events.ndjson" 2>"$RESULTS/e2e-agent-real-tetragon-owned-vm.local-events.ndjson.err"
if ! grep -Fq "\"labels\":{\"scenario\":\"$SCENARIO\"" "$RESULTS/e2e-agent-real-tetragon-owned-vm.local-events.ndjson"; then
  echo "[e2e-agent-real-tetragon-owned-vm][ERROR] local events do not contain label scenario=$SCENARIO" >&2
  cat "$RESULTS/e2e-agent-real-tetragon-owned-vm.local-events.ndjson" >&2 2>/dev/null || true
  exit 1
fi
python3 "$ROOT/shared/reports/local_signal_report.py" "$SCENARIO" \
  "$RESULTS/e2e-agent-real-tetragon-owned-vm.local-events.ndjson" \
  "$RESULTS/e2e-agent-real-tetragon-owned-vm.local-signals.ndjson" \
  "$RESULTS/e2e-agent-real-tetragon-owned-vm.local-summary.json" \
  "$RESULTS/e2e-agent-real-tetragon-owned-vm.local-linked.json"
python3 - "$RESULTS/e2e-agent-real-tetragon-owned-vm.local-summary.json" <<'PY'
import json
import sys

summary = json.load(open(sys.argv[1]))
if int(summary.get("attack_signals", 0)) <= 0:
    raise SystemExit("expected at least one attack signal")
if int(summary.get("attack_signals_with_event_refs", 0)) <= 0:
    raise SystemExit("expected attack signal eventRefs")
if int(summary.get("attack_signals_with_resolved_events", 0)) <= 0:
    raise SystemExit("expected attack signal eventRefs to resolve to local events")
if int(summary.get("multi_event_attack_signals", 0)) <= 0:
    raise SystemExit("expected at least one attack signal with multiple eventRefs")
if summary.get("missing_event_refs"):
    raise SystemExit(f"missing event refs: {summary['missing_event_refs']}")
PY

PID_BEFORE="$(vagrant ssh node-a -c "systemctl show -p MainPID --value sysarmor-agent" 2>/dev/null | tr -d '\r' | tail -1)"
if [[ -z "$PID_BEFORE" || "$PID_BEFORE" == "0" ]]; then
  echo "[e2e-agent-real-tetragon-owned-vm][ERROR] sysarmor-agent MainPID is empty before restart" >&2
  exit 1
fi

echo "[e2e-agent-real-tetragon-owned-vm] verifying systemd restarts owned real Tetragon agent"
vagrant ssh node-a -c "sudo kill -TERM $PID_BEFORE" >/dev/null
deadline=$((SECONDS + 30))
PID_AFTER=""
until [[ -n "$PID_AFTER" && "$PID_AFTER" != "0" && "$PID_AFTER" != "$PID_BEFORE" ]]; do
  if (( SECONDS >= deadline )); then
    echo "[e2e-agent-real-tetragon-owned-vm][ERROR] systemd did not restart agent; before=$PID_BEFORE after=${PID_AFTER:-empty}" >&2
    vagrant ssh node-a -c "sudo systemctl status sysarmor-agent --no-pager -l || true" >&2 2>/dev/null || true
    exit 1
  fi
  sleep 1
  PID_AFTER="$(vagrant ssh node-a -c "systemctl show -p MainPID --value sysarmor-agent" 2>/dev/null | tr -d '\r' | tail -1)"
done

wait_contains "agent-health recovered after restart" '"running":true' "$RESULTS/e2e-agent-real-tetragon-owned-vm.health-after-restart.json" \
  vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json agent health --agent-id vm-owned-tetragon --tenant-id default"
wait_contains "agent-health capability after restart" '"kernelRelease":' "$RESULTS/e2e-agent-real-tetragon-owned-vm.health-after-restart.json" \
  vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json agent health --agent-id vm-owned-tetragon --tenant-id default"
wait_contains "agent-health running after restart" '"running":true' "$RESULTS/e2e-agent-real-tetragon-owned-vm.health-after-restart.json" \
  vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json agent health --agent-id vm-owned-tetragon --tenant-id default"
wait_contains "agent-health policy after restart" '"policyLoaded":true' "$RESULTS/e2e-agent-real-tetragon-owned-vm.health-after-restart.json" \
  vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json agent health --agent-id vm-owned-tetragon --tenant-id default"
wait_contains "owned tetragon process after restart" "$TETRAGON_PATH" "$RESULTS/e2e-agent-real-tetragon-owned-vm.ps-after-restart.txt" \
  vagrant ssh node-a -c "ps -ef | grep tetragon | grep -v grep"

echo "[e2e-agent-real-tetragon-owned-vm] verifying service stop cleans owned Tetragon process"
vagrant ssh node-a -c "sudo systemctl stop sysarmor-agent" >/dev/null
wait_absent "owned tetragon process" "$TETRAGON_PATH" "$RESULTS/e2e-agent-real-tetragon-owned-vm.ps-after-stop.txt" \
  vagrant ssh node-a -c "ps -ef | grep tetragon | grep -v grep"
wait_absent "owned tetra getevents process" "$TETRA_PATH" "$RESULTS/e2e-agent-real-tetragon-owned-vm.tetra-after-stop.txt" \
  vagrant ssh node-a -c "ps -ef | grep tetra | grep getevents | grep -v grep"
vagrant ssh node-a -c "sudo systemctl start sysarmor-agent" >/dev/null
wait_contains "agent-health recovered after service start" '"running":true' "$RESULTS/e2e-agent-real-tetragon-owned-vm.health-after-service-start.json" \
  vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json agent health --agent-id vm-owned-tetragon --tenant-id default"
wait_contains "agent-health capability after service start" '"bpffsAvailable":true' "$RESULTS/e2e-agent-real-tetragon-owned-vm.health-after-service-start.json" \
  vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json agent health --agent-id vm-owned-tetragon --tenant-id default"
wait_contains "agent-health policy after service start" '"policyLoaded":true' "$RESULTS/e2e-agent-real-tetragon-owned-vm.health-after-service-start.json" \
  vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json agent health --agent-id vm-owned-tetragon --tenant-id default"
wait_contains "owned tetragon process after service start" "$TETRAGON_PATH" "$RESULTS/e2e-agent-real-tetragon-owned-vm.ps-after-service-start.txt" \
  vagrant ssh node-a -c "ps -ef | grep tetragon | grep -v grep"
wait_absent "owned tetra getevents process after service start" "$TETRA_PATH" "$RESULTS/e2e-agent-real-tetragon-owned-vm.tetra-after-service-start.txt" \
  vagrant ssh node-a -c "ps -ef | grep tetra | grep getevents | grep -v grep"

vagrant ssh node-a -c "sudo systemctl status sysarmor-agent --no-pager -l" > "$RESULTS/e2e-agent-real-tetragon-owned-vm.systemd.txt" 2>&1 || true
vagrant ssh node-a -c "sudo journalctl -u sysarmor-agent --no-pager -n 200" > "$RESULTS/e2e-agent-real-tetragon-owned-vm.journal.txt" 2>&1 || true

echo "[e2e-agent-real-tetragon-owned-vm] ok"
