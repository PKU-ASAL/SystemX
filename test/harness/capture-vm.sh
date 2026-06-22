#!/usr/bin/env bash
# 跑 VM 拓扑场景。当前阶段只验证本地 sysarmor-agent:
# agent owns Tetragon -> local control socket -> local event/signal streams.
# 用法: capture-vm.sh <scenario> [duration_s]
#   scenario: apt-fileless-c2 | apt-staged-drop | benign-ci-noise
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"
REPO="$(cd "$ROOT/.." && pwd)"
ENVDIR="$(cd "$ROOT/env/vm" && pwd)"
RESULTS="$ROOT/.results"
S="${1:?用法: capture-vm.sh <scenario> [duration_s]}"
DUR="${2:-30}"
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
: "${C2:=10.66.0.99}" "${GAP:=8}" "${CYCLES:=3}"
TETRAGON_ARCHIVE="${SYSARMOR_TETRAGON_ARCHIVE:-}"

mkdir -p "$RESULTS"

if [[ -z "$TETRAGON_ARCHIVE" ]]; then
  echo "[capture-vm][ERROR] SYSARMOR_TETRAGON_ARCHIVE is required" >&2
  exit 1
fi

cd "$ENVDIR"
vagrant upload "$REPO/bin/sysarmor-agent" /tmp/sysarmor-agent.upload node-a >/dev/null
vagrant upload "$REPO/bin/sysarmorctl" /tmp/sysarmorctl.upload node-a >/dev/null
vagrant upload "$REPO/deployments" /tmp/sysarmor-deployments.upload node-a >/dev/null
vagrant upload "$TETRAGON_ARCHIVE" /tmp/sysarmor-tetragon.upload node-a >/dev/null

TETRAGON_BUNDLE_DIR="${TETRAGON_BUNDLE_DIR:-/opt/sysarmor/bundles/tetragon}"
TETRAGON_INSTALL_DIR="${TETRAGON_INSTALL_DIR:-/opt/sysarmor/sensors}"
TETRA_PATH="$TETRAGON_INSTALL_DIR/tetragon/current/bin/tetra"
TETRAGON_PATH="$TETRAGON_INSTALL_DIR/tetragon/current/bin/tetragon"
AGENT_SOCK="/var/run/sysarmor/agent.sock"
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
  mkdir -p \"$WORK/spool\"
  cat > \"$WORK/policy.yaml\" <<\"EOF\"
{"behaviors":["process.exec","network.connect","file.open","file.write","file.chmod"],"observe_only":true}
EOF
  cat > \"$WORK/agent.yaml\" <<EOF
agent:
  id: vm-node-a
  host_id: vm-node-a
  tenant_id: default
  token: $TOKEN
  scenario: $S

manager:
  transport: local

control:
  socket_path: $AGENT_SOCK

sensor:
  backend: tetragon
  mode: managed
  bundle_dir: $TETRAGON_BUNDLE_DIR
  install_dir: $TETRAGON_INSTALL_DIR
  policy_path: $WORK/policy.yaml
  scope:
    type: host
  observe_only: true
  restart: always
  max_restarts: 3
  restart_window: 500ms

spool:
  path: $WORK/spool
  max_bytes: 268435456
  batch_size: 256
  flush_interval: 200ms

data_plane:
  retry_initial: 100ms
  retry_max: 500ms
  request_timeout: 2s

health:
  interval: 500ms
EOF
  SYSARMOR_AGENT_BIN=/tmp/sysarmor-agent.upload SYSARMOR_AGENT_CONFIG=\"$WORK/agent.yaml\" SYSARMOR_COLLECTION_POLICY=\"$WORK/policy.yaml\" SYSARMOR_TETRAGON_BUNDLE_DIR=$TETRAGON_BUNDLE_DIR SYSARMOR_TETRAGON_INSTALL_DIR=$TETRAGON_INSTALL_DIR SYSARMOR_TETRAGON_ARCHIVE=/tmp/sysarmor-tetragon.upload bash /tmp/sysarmor-deployments.upload/install-agent.sh
  test -f \"$TETRAGON_BUNDLE_DIR/manifest.json\"
  systemctl daemon-reload
  install -m 0755 /tmp/sysarmorctl.upload /usr/local/bin/sysarmorctl
'" >/dev/null

vagrant ssh node-a -c "sudo systemctl restart sysarmor-agent" >/dev/null

wait_contains "agent health" '"status":"ok"' "$RESULTS/vm.$S.agent-health.json" \
  vagrant ssh node-a -c "sudo sysarmorctl --agent-sock '$AGENT_SOCK' --json agent health --agent-id vm-node-a --tenant-id default"
wait_contains "agent health tetragon" '"backend":"tetragon"' "$RESULTS/vm.$S.agent-health.json" \
  vagrant ssh node-a -c "sudo sysarmorctl --agent-sock '$AGENT_SOCK' --json agent health --agent-id vm-node-a --tenant-id default"
wait_contains "agent capability" 'process.exec' "$RESULTS/vm.$S.agent-capability.json" \
  vagrant ssh node-a -c "sudo sysarmorctl --agent-sock '$AGENT_SOCK' --json agent capability --agent-id vm-node-a --tenant-id default"
wait_contains "tracing policy" 'sysarmor-runtime-collection' "$RESULTS/vm.$S.tracingpolicy.txt" \
  vagrant ssh node-a -c "sudo '$TETRA_PATH' tracingpolicy list"
wait_contains "owned tetragon process" "$TETRAGON_PATH" "$RESULTS/vm.$S.tetragon-process.txt" \
  vagrant ssh node-a -c "ps -ef | grep tetragon | grep -v grep"

echo "[capture-vm] running scenario: $S"
vagrant ssh node-a -c "sudo bash -c '
  if [ -f /vagrant/test/scenarios/vm/$S/attack.sh ]; then
    GAP=$GAP CYCLES=$CYCLES C2=$C2 bash /vagrant/test/scenarios/vm/$S/attack.sh
  else
    echo \"[capture-vm] $S 无 attack.sh，执行 smoke\"
    /usr/bin/env true
  fi
'" >/dev/null
sleep "$DUR"

if [[ -n "$SIGNAL_RULE" ]]; then
  vagrant ssh node-a -c "sudo sysarmorctl --agent-sock '$AGENT_SOCK' --json signal watch --include-recent --snapshot --limit 200 --agent-id vm-node-a --tenant-id default --timeout 20s" \
    > "$RESULTS/vm.$S.signals.ndjson" 2>"$RESULTS/vm.$S.signals.ndjson.err"
  if ! grep -Fq "\"name\":\"$SIGNAL_RULE\"" "$RESULTS/vm.$S.signals.ndjson"; then
    echo "[capture-vm][ERROR] local attack signal not found: $SIGNAL_RULE" >&2
    cat "$RESULTS/vm.$S.signals.ndjson" >&2 2>/dev/null || true
    exit 1
  fi
else
  vagrant ssh node-a -c "sudo sysarmorctl --agent-sock '$AGENT_SOCK' --json signal watch --include-recent --snapshot --limit 200 --agent-id vm-node-a --tenant-id default --timeout 5s" > "$RESULTS/vm.$S.signals.ndjson" 2>/dev/null || true
fi

vagrant ssh node-a -c "sudo sysarmorctl --agent-sock '$AGENT_SOCK' --json event watch --include-recent --snapshot --limit 8192 --agent-id vm-node-a --tenant-id default --timeout 20s" \
  > "$RESULTS/vm.$S.events.ndjson" 2>"$RESULTS/vm.$S.events.ndjson.err"
if ! grep -Fq "\"scenario\":\"$S\"" "$RESULTS/vm.$S.events.ndjson"; then
  echo "[capture-vm][ERROR] local events do not contain scenario=$S" >&2
  cat "$RESULTS/vm.$S.events.ndjson" >&2 2>/dev/null || true
  exit 1
fi

python3 "$HERE/local_signal_report.py" "$S" \
  "$RESULTS/vm.$S.events.ndjson" \
  "$RESULTS/vm.$S.signals.ndjson" \
  "$RESULTS/vm.$S.local.json" \
  "$RESULTS/vm.$S.linked.json"

vagrant ssh node-a -c "sudo journalctl -u sysarmor-agent --no-pager -n 200" > "$RESULTS/vm.$S.agent.log" 2>&1 || true
echo "[capture-vm] local capture ok: $RESULTS/vm.$S.local.json"
