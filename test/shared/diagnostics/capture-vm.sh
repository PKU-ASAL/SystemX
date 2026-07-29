#!/usr/bin/env bash
# 跑 VM endpoint 诊断场景。只验证本地 sysarmor-agent:
# agent owns Tetragon -> local control socket -> local event/signal streams.
# 用法: capture-vm.sh <scenario> [duration_s]
#   scenario: apt-fileless-c2 | apt-staged-drop | benign-ci-noise
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
REPO="$(cd "$ROOT/.." && pwd)"
VM_ENV="${SYSARMOR_VM_ENV:-${ENV:-vm-endpoint}}"
ENVDIR="$(cd "$ROOT/environments/$VM_ENV" && pwd)"
RESULTS="$ROOT/.results"
S="${1:?用法: capture-vm.sh <scenario> [duration_s]}"
DUR="${2:-30}"
: "${C2:=10.66.0.99}" "${GAP:=8}" "${CYCLES:=3}"
TETRAGON_ARCHIVE="${SYSARMOR_TETRAGON_ARCHIVE:-}"

mkdir -p "$RESULTS"

if [[ -z "$TETRAGON_ARCHIVE" ]]; then
  echo "[capture-vm][ERROR] SYSARMOR_TETRAGON_ARCHIVE is required" >&2
  exit 1
fi

cd "$ENVDIR"
vagrant upload "$REPO/dist/bin/sysarmor-agent" /tmp/sysarmor-agent.upload node-a >/dev/null
vagrant upload "$REPO/dist/bin/sysarmorctl" /tmp/sysarmorctl.upload node-a >/dev/null
vagrant upload "$REPO/dist/bin/sysarmor-content-sign" /tmp/sysarmor-content-sign.upload node-a >/dev/null
vagrant upload "$REPO/deployments" /tmp/sysarmor-deployments.upload node-a >/dev/null
vagrant upload "$TETRAGON_ARCHIVE" /tmp/sysarmor-tetragon.upload node-a >/dev/null

TETRAGON_BUNDLE_DIR="${TETRAGON_BUNDLE_DIR:-/opt/sysarmor/agent/bundles/tetragon}"
TETRAGON_INSTALL_DIR="${TETRAGON_INSTALL_DIR:-/opt/sysarmor/agent/sensors}"
TETRA_PATH="$TETRAGON_INSTALL_DIR/tetragon/current/bin/tetra"
TETRAGON_PATH="$TETRAGON_INSTALL_DIR/tetragon/current/bin/tetragon"
AGENT_SOCK="/run/sysarmor/agent/control.sock"
WORK="/tmp/sysarmor-vm-capture-$S"
SIGNAL_RULE=""
case "$S" in
  apt-fileless-c2)
    SIGNAL_RULE="reverse_shell_pattern"
    ;;
  apt-staged-drop)
    SIGNAL_RULE="payload_dropped"
    ;;
esac

cleanup() {
  vagrant ssh node-a -c "sudo systemctl stop sysarmor-agent 2>/dev/null || true" >/dev/null 2>&1 || true
}
trap cleanup EXIT

wait_contains() {
  local name="$1"
  local needle="$2"
  local out="$3"
  shift 3
  local deadline=$((SECONDS + 90))
  until "$@" >"$out" 2>"$out.err" && grep -Fq "$needle" "$out"; do
    if (( SECONDS >= deadline )); then
      echo "[capture-vm][ERROR] timeout waiting for $needle via $name" >&2
      echo "--- last response ---" >&2
      cat "$out" >&2 2>/dev/null || true
      echo "--- last error ---" >&2
      cat "$out.err" >&2 2>/dev/null || true
      echo "--- agent journal ---" >&2
      vagrant ssh node-a -c "sudo journalctl -u sysarmor-agent --no-pager -n 160 || true" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 1
  done
}

echo "[capture-vm] installing local agent-owned Tetragon path: $S (${DUR}s)"
vagrant ssh node-a -c "sudo systemctl stop sysarmor-agent 2>/dev/null || true; sudo systemctl disable sysarmor-agent 2>/dev/null || true; sudo systemctl reset-failed sysarmor-agent 2>/dev/null || true; sudo systemctl stop tetragon 2>/dev/null || true; sudo systemctl disable tetragon 2>/dev/null || true; sudo pkill -x sysarmor-agent 2>/dev/null || true; sudo pkill -x tetragon 2>/dev/null || true; sudo pkill -x tetra 2>/dev/null || true" >/dev/null

vagrant ssh node-a -c "sudo bash -c '
  set -euo pipefail
  rm -rf \"$WORK\"
  cat > \"$WORK/agent.yaml\" <<EOF
agent:
  label.scenario: $S

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

telemetry:
  max_batch_items: 256
  flush_interval: 200ms

local:
  state_path: $WORK/state
  export:
    retry_initial: 100ms
    retry_max: 500ms
    request_timeout: 2s

policy:
  path: /etc/sysarmor/agent/policy.json

health:
  interval: 500ms
EOF
  if ! SYSARMOR_AGENT_BIN=/tmp/sysarmor-agent.upload SYSARMOR_CTL_BIN=/tmp/sysarmorctl.upload SYSARMOR_CONTENT_SIGN_BIN=/tmp/sysarmor-content-sign.upload SYSARMOR_AGENT_CONFIG=\"$WORK/agent.yaml\" SYSARMOR_COLLECTION_POLICY=/tmp/sysarmor-deployments.upload/agent/policy.json SYSARMOR_TETRAGON_BUNDLE_DIR=$TETRAGON_BUNDLE_DIR SYSARMOR_TETRAGON_INSTALL_DIR=$TETRAGON_INSTALL_DIR SYSARMOR_TETRAGON_ARCHIVE=/tmp/sysarmor-tetragon.upload bash /tmp/sysarmor-deployments.upload/agent/install-agent.sh >/tmp/sysarmor-install-agent.log 2>&1; then
    cat /tmp/sysarmor-install-agent.log >&2
    exit 1
  fi
  test -f \"$TETRAGON_BUNDLE_DIR/manifest.json\"
  systemctl daemon-reload
'" >/dev/null

vagrant ssh node-a -c "sudo systemctl restart sysarmor-agent" >/dev/null

health="$(vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json agent health")"
AGENT_ID="$(jq -r '.agentId // .agent_id // empty' <<<"$health")"
TENANT_ID="$(jq -r '.tenantId // .tenant_id // empty' <<<"$health")"
if [[ -z "$AGENT_ID" || -z "$TENANT_ID" ]]; then
  echo "[capture-vm][ERROR] Agent health did not expose runtime identity: $health" >&2
  exit 1
fi

wait_contains "agent health" '"status":"ok"' "$RESULTS/vm.$S.agent-health.json" \
  vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json agent health --agent-id '$AGENT_ID' --tenant-id '$TENANT_ID'"
wait_contains "agent health tetragon" '"backend":"tetragon"' "$RESULTS/vm.$S.agent-health.json" \
  vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json agent health --agent-id '$AGENT_ID' --tenant-id '$TENANT_ID'"
wait_contains "agent capability" 'process.exec' "$RESULTS/vm.$S.agent-capability.json" \
  vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json agent capability --agent-id '$AGENT_ID' --tenant-id '$TENANT_ID'"
wait_contains "tracing policy" 'sysarmor-runtime-collection' "$RESULTS/vm.$S.tracingpolicy.txt" \
  vagrant ssh node-a -c "sudo '$TETRA_PATH' tracingpolicy list"
wait_contains "owned tetragon process" "$TETRAGON_PATH" "$RESULTS/vm.$S.tetragon-process.txt" \
  vagrant ssh node-a -c "ps -ef | grep tetragon | grep -v grep"

echo "[capture-vm] running scenario: $S"
vagrant ssh node-a -c "sudo bash -c '
  if [ -f /vagrant/test/data/scenarios/vm/$S/attack.sh ]; then
    GAP=$GAP CYCLES=$CYCLES C2=$C2 bash /vagrant/test/data/scenarios/vm/$S/attack.sh
  else
    echo \"[capture-vm] $S 无 attack.sh，执行 smoke\"
    /usr/bin/env true
  fi
'" >/dev/null
sleep "$DUR"

if [[ -n "$SIGNAL_RULE" ]]; then
  vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json signal watch --include-recent --snapshot --limit 200 --agent-id '$AGENT_ID' --tenant-id '$TENANT_ID' --timeout 20s" \
    > "$RESULTS/vm.$S.signals.ndjson" 2>"$RESULTS/vm.$S.signals.ndjson.err"
  if ! grep -Fq "\"name\":\"$SIGNAL_RULE\"" "$RESULTS/vm.$S.signals.ndjson"; then
    echo "[capture-vm][ERROR] local attack signal not found: $SIGNAL_RULE" >&2
    cat "$RESULTS/vm.$S.signals.ndjson" >&2 2>/dev/null || true
    exit 1
  fi
else
  vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json signal watch --include-recent --snapshot --limit 200 --agent-id '$AGENT_ID' --tenant-id '$TENANT_ID' --timeout 5s" > "$RESULTS/vm.$S.signals.ndjson" 2>/dev/null || true
fi

vagrant ssh node-a -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json event watch --include-recent --snapshot --limit 8192 --agent-id '$AGENT_ID' --tenant-id '$TENANT_ID' --timeout 20s" \
  > "$RESULTS/vm.$S.events.ndjson" 2>"$RESULTS/vm.$S.events.ndjson.err"
if ! grep -Fq "\"labels\":{\"scenario\":\"$S\"" "$RESULTS/vm.$S.events.ndjson"; then
  echo "[capture-vm][ERROR] local events do not contain label scenario=$S" >&2
  cat "$RESULTS/vm.$S.events.ndjson" >&2 2>/dev/null || true
  exit 1
fi

python3 "$ROOT/shared/reports/local_signal_report.py" "$S" \
  "$RESULTS/vm.$S.events.ndjson" \
  "$RESULTS/vm.$S.signals.ndjson" \
  "$RESULTS/vm.$S.local.json" \
  "$RESULTS/vm.$S.linked.json"

vagrant ssh node-a -c "sudo journalctl -u sysarmor-agent --no-pager -n 200" > "$RESULTS/vm.$S.agent.log" 2>&1 || true
echo "[capture-vm] local capture ok: $RESULTS/vm.$S.local.json"
