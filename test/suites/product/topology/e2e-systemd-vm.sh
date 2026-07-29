#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../../.." && pwd)"
REPO="$(cd "$ROOT/.." && pwd)"
VM_ENV="${SYSARMOR_VM_ENV:-${ENV:-vm-topology}}"
ENVDIR="$(cd "$ROOT/environments/$VM_ENV" && pwd)"
RESULTS="$ROOT/.results"
PKI_DIR="${SYSARMOR_VM_MTLS_DIR:-$ROOT/.results/pki/$VM_ENV}"
AGENT_ID="vm-owned-tetragon"
CASE_LABEL="product-topology"

mkdir -p "$RESULTS"

cleanup() {
  (
    cd "$ENVDIR"
    vagrant ssh node-a -c "sudo systemctl stop sysarmor-agent 2>/dev/null || true" >/dev/null 2>&1 || true
  )
}
trap cleanup EXIT

echo "[e2e-agent-systemd-vm] starting VM topology"
bash "$ROOT/shared/harness/start-vm.sh" "$VM_ENV" >/dev/null

cd "$ENVDIR"
MANAGER_JWT="$("$REPO/tools/auth/issue-manager-jwt.sh" "$PKI_DIR/manager-jwt-private.pem" sysarmor-bff sysarmor-manager)"
MANAGER_CTL="SYSARMOR_MANAGER_JWT='$MANAGER_JWT' /tmp/sysarmorctl"

echo "[e2e-agent-systemd-vm] publishing agent artifact through manager"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"; cleanup' EXIT
TETRAGON_ARCHIVE="${SYSARMOR_TETRAGON_ARCHIVE:-}"
if [[ -z "$TETRAGON_ARCHIVE" && -f "$REPO/.cache/tetragon-v1.7.0-amd64.tar.gz" ]]; then
  TETRAGON_ARCHIVE="$REPO/.cache/tetragon-v1.7.0-amd64.tar.gz"
fi
if [[ -z "$TETRAGON_ARCHIVE" || ! -f "$TETRAGON_ARCHIVE" ]]; then
  echo "[e2e-agent-systemd-vm][ERROR] SYSARMOR_TETRAGON_ARCHIVE is required for product-topology agent artifact" >&2
  exit 1
fi
SIGNING_KEY="$PKI_DIR/artifact-signing-key.pem"
if [[ ! -f "$SIGNING_KEY" ]]; then
  echo "[e2e-agent-systemd-vm][ERROR] missing artifact signing key: $SIGNING_KEY" >&2
  exit 1
fi
"$REPO/deployments/agent/package-agent.sh" \
  --version topology-test \
  --output "$TMP/sysarmor-agent-linux-amd64.tar.gz" \
  --agent-bin "$REPO/dist/bin/sysarmor-agent" \
  --ctl-bin "$REPO/dist/bin/sysarmorctl" \
  --tetragon-archive "$TETRAGON_ARCHIVE" \
  --content-signing-key "$PKI_DIR/content-signing-key.pem" \
  --content-key-id topology-test \
  --signing-key "$SIGNING_KEY" >/dev/null
vagrant upload "$TMP/sysarmor-agent-linux-amd64.tar.gz" /tmp/sysarmor-agent-linux-amd64.tar.gz mgr >/dev/null

vagrant ssh mgr -c "curl -sf -H 'Authorization: Bearer $MANAGER_JWT' -X POST 'http://127.0.0.1:9443/api/v1/reset'" >/dev/null

ARTIFACT_JSON="$RESULTS/e2e-agent-systemd-vm.artifact.json"
vagrant ssh mgr -c "$MANAGER_CTL --manager-url http://127.0.0.1:9443 --json manager artifacts upload --file /tmp/sysarmor-agent-linux-amd64.tar.gz --name sysarmor-agent --kind agent --version topology-test --os linux --arch amd64 --status active" >"$ARTIFACT_JSON"
ARTIFACT_ID="$(python3 - "$ARTIFACT_JSON" <<'PY'
import json, sys
print(json.load(open(sys.argv[1]))["artifact"]["artifact_id"])
PY
)"
CHANNEL_JSON="$RESULTS/e2e-agent-systemd-vm.channel.json"
vagrant ssh mgr -c "$MANAGER_CTL --manager-url http://127.0.0.1:9443 --json manager channels upsert --channel topology-test --artifact-id $ARTIFACT_ID" >"$CHANNEL_JSON"

ENROLLMENT_JSON="$RESULTS/e2e-agent-systemd-vm.enrollment.json"
vagrant ssh mgr -c "$MANAGER_CTL --manager-url http://10.66.0.10:9443 --json manager enrollments create --agent-id $AGENT_ID --host-id vm-node-a --gateway-addr 10.66.0.10:9444 --gateway-sni sysarmor-gateway.local --channel topology-test --ttl 1h --label suite=$CASE_LABEL --label topology=vm" >"$ENROLLMENT_JSON"
INSTALL_URL="$(python3 - "$ENROLLMENT_JSON" <<'PY'
import json, sys
print(json.load(open(sys.argv[1]))["install_url"])
PY
)"

echo "[e2e-agent-systemd-vm] installing agent from manager enrollment"
vagrant ssh node-a -c "sudo systemctl stop sysarmor-agent 2>/dev/null || true; sudo rm -rf /opt/sysarmor/agent /etc/sysarmor/agent /var/lib/sysarmor/agent /etc/systemd/system/sysarmor-agent.service; curl -fsSL '$INSTALL_URL' | sudo bash" >/dev/null

wait_contains() {
  local name="$1"
  local needle="$2"
  local out="$3"
  shift 3
  local deadline=$((SECONDS + 60))
  until "$@" >"$out" 2>"$out.err" && grep -Fq "$needle" "$out"; do
    if (( SECONDS >= deadline )); then
      echo "[e2e-agent-systemd-vm][ERROR] timeout waiting for $needle via $name" >&2
      echo "--- last response ---" >&2
      cat "$out" >&2 2>/dev/null || true
      echo "--- last error ---" >&2
      cat "$out.err" >&2 2>/dev/null || true
      echo "--- systemd status ---" >&2
      vagrant ssh node-a -c "sudo systemctl status sysarmor-agent --no-pager -l || true" >&2 2>/dev/null || true
      echo "--- agent journal ---" >&2
      vagrant ssh node-a -c "sudo journalctl -u sysarmor-agent --no-pager -n 80 || true" >&2 2>/dev/null || true
      echo "--- manager log ---" >&2
      vagrant ssh mgr -c "sudo docker logs sysarmor-manager --tail 120 2>/dev/null || true" >&2 2>/dev/null || true
      echo "--- gateway log ---" >&2
      vagrant ssh mgr -c "sudo docker logs sysarmor-gateway --tail 120 2>/dev/null || true" >&2 2>/dev/null || true
      echo "--- worker log ---" >&2
      vagrant ssh mgr -c "sudo docker logs sysarmor-worker --tail 120 2>/dev/null || true" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 1
  done
}

wait_contains "agent-health" "\"agent_id\":\"$AGENT_ID\"" "$RESULTS/e2e-agent-systemd-vm.health.json" \
  vagrant ssh mgr -c "$MANAGER_CTL --manager-url 127.0.0.1:9443 --json manager health get --agent-id $AGENT_ID --tenant-id default"
wait_contains "agent-health artifact agent" '"backend":"tetragon"' "$RESULTS/e2e-agent-systemd-vm.health.json" \
  vagrant ssh mgr -c "$MANAGER_CTL --manager-url 127.0.0.1:9443 --json manager health get --agent-id $AGENT_ID --tenant-id default"
wait_contains "agent-session" "\"agent_id\":\"$AGENT_ID\"" "$RESULTS/e2e-agent-systemd-vm.sessions.json" \
  vagrant ssh mgr -c "$MANAGER_CTL --manager-url 127.0.0.1:9443 --json manager sessions list --agent-id $AGENT_ID --tenant-id default"
wait_contains "artifact list" "\"artifact_id\":\"$ARTIFACT_ID\"" "$RESULTS/e2e-agent-systemd-vm.artifacts.json" \
  vagrant ssh mgr -c "$MANAGER_CTL --manager-url 127.0.0.1:9443 --json manager artifacts list --kind agent --status active"
wait_contains "channel list" '"channel":"topology-test"' "$RESULTS/e2e-agent-systemd-vm.channels.json" \
  vagrant ssh mgr -c "$MANAGER_CTL --manager-url 127.0.0.1:9443 --json manager channels list --tenant-id default"
wait_contains "enrollment list" "\"artifact_id\":\"$ARTIFACT_ID\"" "$RESULTS/e2e-agent-systemd-vm.enrollments.json" \
  vagrant ssh mgr -c "$MANAGER_CTL --manager-url 127.0.0.1:9443 --json manager enrollments list --tenant-id default --status active"
wait_contains "manager events" "\"agentId\":\"$AGENT_ID\"" "$RESULTS/e2e-agent-systemd-vm.events.json" \
  vagrant ssh mgr -c "$MANAGER_CTL --manager-url 127.0.0.1:9443 --json manager events list --label suite=$CASE_LABEL --limit 50"
wait_contains "agent-session data plane" '"data_transport":"grpc_stream"' "$RESULTS/e2e-agent-systemd-vm.sessions.json" \
  vagrant ssh mgr -c "$MANAGER_CTL --manager-url 127.0.0.1:9443 --json manager sessions list --agent-id $AGENT_ID --tenant-id default"

read_agent_pid() {
  vagrant ssh node-a -c "systemctl show -p MainPID --value sysarmor-agent 2>/dev/null | awk '/^[0-9]+$/ { print \"PID=\" \$1; exit }'" 2>/dev/null \
    | tr -d '\r' \
    | awk -F= '/^PID=[0-9]+$/ { print $2; exit }' || true
}

wait_agent_pid() {
  local pid=""
  local deadline=$((SECONDS + 30))
  until [[ -n "$pid" && "$pid" != "0" ]]; do
    if (( SECONDS >= deadline )); then
      echo "[e2e-agent-systemd-vm][ERROR] sysarmor-agent MainPID is empty before restart" >&2
      vagrant ssh node-a -c "sudo systemctl status sysarmor-agent --no-pager -l || true" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 1
    pid="$(read_agent_pid)"
  done
  printf '%s\n' "$pid"
}

PID_BEFORE="$(wait_agent_pid)"

echo "[e2e-agent-systemd-vm] verifying systemd restarts agent"
if ! vagrant ssh node-a -c "sudo systemctl kill -s TERM sysarmor-agent" >/dev/null; then
  echo "[e2e-agent-systemd-vm][ERROR] failed to signal sysarmor-agent via systemctl; before=$PID_BEFORE" >&2
  vagrant ssh node-a -c "sudo systemctl status sysarmor-agent --no-pager -l || true" >&2 2>/dev/null || true
  vagrant ssh node-a -c "sudo journalctl -u sysarmor-agent --no-pager -n 80 || true" >&2 2>/dev/null || true
  exit 1
fi

deadline=$((SECONDS + 30))
PID_AFTER=""
until [[ -n "$PID_AFTER" && "$PID_AFTER" != "0" && "$PID_AFTER" != "$PID_BEFORE" ]]; do
  if (( SECONDS >= deadline )); then
    if [[ -z "$PID_AFTER" || "$PID_AFTER" == "0" ]]; then
      echo "[e2e-agent-systemd-vm][ERROR] sysarmor-agent MainPID is empty after restart signal; before=$PID_BEFORE after=${PID_AFTER:-empty}" >&2
    else
      echo "[e2e-agent-systemd-vm][ERROR] systemd did not restart agent; before=$PID_BEFORE after=$PID_AFTER" >&2
    fi
    vagrant ssh node-a -c "sudo systemctl status sysarmor-agent --no-pager -l || true" >&2 2>/dev/null || true
    vagrant ssh node-a -c "sudo journalctl -u sysarmor-agent --no-pager -n 80 || true" >&2 2>/dev/null || true
    exit 1
  fi
  sleep 1
  PID_AFTER="$(read_agent_pid)"
done

wait_contains "agent-health after systemd restart" "\"agent_id\":\"$AGENT_ID\"" "$RESULTS/e2e-agent-systemd-vm.health-after-restart.json" \
  vagrant ssh mgr -c "$MANAGER_CTL --manager-url 127.0.0.1:9443 --json manager health get --agent-id $AGENT_ID --tenant-id default"

vagrant ssh node-a -c "sudo systemctl status sysarmor-agent --no-pager -l" > "$RESULTS/e2e-agent-systemd-vm.systemd.txt" 2>&1 || true
vagrant ssh node-a -c "sudo journalctl -u sysarmor-agent --no-pager -n 120" > "$RESULTS/e2e-agent-systemd-vm.journal.txt" 2>&1 || true

python3 - "$RESULTS" "$ARTIFACT_ID" "$AGENT_ID" "$CASE_LABEL" <<'PY'
import json
import pathlib
import sys

root = pathlib.Path(sys.argv[1])
artifact_id, agent_id, case_label = sys.argv[2:5]

def load(name, default):
    path = root / name
    try:
        return json.loads(path.read_text())
    except Exception:
        return default

events = load("e2e-agent-systemd-vm.events.json", [])
sessions = load("e2e-agent-systemd-vm.sessions.json", {}).get("sessions", [])
health = load("e2e-agent-systemd-vm.health.json", {})
health_after = load("e2e-agent-systemd-vm.health-after-restart.json", {})
enrollment = load("e2e-agent-systemd-vm.enrollment.json", {}).get("enrollment", {})

summary = {
    "suite": "product-topology",
    "topology": "vm",
    "agent_id": agent_id,
    "artifact_id": artifact_id,
    "labels": {"suite": case_label, "topology": "vm"},
    "artifact_uploaded": bool(artifact_id),
    "channel_bound": True,
    "enrollment_created": bool(enrollment.get("enrollment_id")),
    "certificate_requested_during_install": True,
    "health_status": health.get("status"),
    "health_after_restart_status": health_after.get("status"),
    "session_count": len(sessions),
    "event_count": len(events),
    "sensor_backend": (health.get("sensor_capability") or {}).get("backend"),
    "sensor_running": (health.get("sensor_health") or {}).get("running"),
    "systemd_restart_verified": bool(health_after.get("agent_id") == agent_id),
}
(root / "e2e-agent-systemd-vm.summary.json").write_text(json.dumps(summary, indent=2, sort_keys=True) + "\n")
PY

echo "[e2e-agent-systemd-vm] ok"
