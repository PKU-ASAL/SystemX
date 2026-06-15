#!/usr/bin/env bash
# 跑 VM 拓扑场景。默认使用 v2 agent-managed sensor 主路径。
# 前提: vagrant up（env/vm/）已就绪，tetragon 在 node-a 上运行。
# 用法: capture-vm.sh <scenario> [duration_s]
#   scenario: apt-fileless-c2 | apt-staged-drop | benign-ci-noise
#   CAPTURE_MODE=managed | replay
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"
REPO="$(cd "$ROOT/.." && pwd)"
ENVDIR="$(cd "$ROOT/env/vm" && pwd)"
RESULTS="$ROOT/.results"
S="${1:?用法: capture-vm.sh <scenario> [duration_s]}"
DUR="${2:-30}"
CAPTURE_MODE="${CAPTURE_MODE:-managed}"
: "${C2:=10.66.0.99}" "${GAP:=8}" "${CYCLES:=3}"
mkdir -p "$RESULTS"

echo "[capture-vm] 确保 node-a 上 tetragon 在跑"
cd "$ENVDIR"
vagrant upload "$REPO/bin/sysarmor-agent" /tmp/sysarmor-agent node-a >/dev/null
vagrant ssh node-a -c "chmod +x /tmp/sysarmor-agent" >/dev/null
vagrant ssh node-a -c "sudo systemctl start tetragon 2>/dev/null || true; systemctl is-active tetragon || echo 'NOT RUNNING'" 2>/dev/null
vagrant ssh node-a -c "sudo tetra tracingpolicy add /vagrant/test/env/resources/syscall-capture.yaml 2>/dev/null | tail -1 || true" 2>/dev/null

if [[ "$CAPTURE_MODE" == "managed" ]]; then
  echo "[capture-vm] v2 managed daemon 主路径: $S（窗口 ${DUR}s）"
  TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
  vagrant ssh mgr -c "curl -sf -X POST 'http://127.0.0.1:9443/api/v1/reset?scenario=$S' >/dev/null" >/dev/null
  vagrant ssh node-a -c "sudo bash -c '
    set -euo pipefail
    WORK=/tmp/sysarmor-vm-capture-$S
    rm -rf \"\$WORK\"
    mkdir -p \"\$WORK/spool\"
    TETRA_PATH=\$(command -v tetra)
    cat > \"\$WORK/policy.yaml\" <<\"EOF\"
kinds: [EXEC, CONNECT, OPEN, WRITE, CHMOD]
EOF
    cat > \"\$WORK/agent.yaml\" <<EOF
agent:
  id: vm-node-a
  host_id: vm-node-a
  tenant_id: default
  token: $TOKEN
  scenario: $S

manager:
  address: http://10.66.0.10:9443
  transport: http

sensor:
  backend: tetragon
  mode: managed
  tetra_path: \$TETRA_PATH
  policy_path: \$WORK/policy.yaml
  observe_only: true
  restart: never
  max_restarts: 1
  restart_window: 1h

spool:
  path: \$WORK/spool
  max_bytes: 268435456
  batch_size: 256
  flush_interval: 200ms

upload:
  retry_initial: 100ms
  retry_max: 500ms
  request_timeout: 2s

health:
  interval: 500ms
EOF
    rm -f \"\$WORK/agent.log\"
    /tmp/sysarmor-agent run --config \"\$WORK/agent.yaml\" > \"\$WORK/agent.log\" 2>&1 &
    echo \$! > \"\$WORK/agent.pid\"
  '" >/dev/null
  sleep 1
  vagrant ssh node-a -c "sudo bash -c '
    if [ -f /vagrant/test/scenarios/vm/$S/attack.sh ]; then
      GAP=$GAP CYCLES=$CYCLES C2=$C2 bash /vagrant/test/scenarios/vm/$S/attack.sh
    else
      echo \"[capture-vm] $S 无 attack.sh，执行 replay-only smoke\"
      /usr/bin/env true
    fi
  '"
  sleep "$DUR"
  vagrant ssh node-a -c "sudo bash -c '
    WORK=/tmp/sysarmor-vm-capture-$S
    if [ -f \"\$WORK/agent.pid\" ]; then kill \"\$(cat \"\$WORK/agent.pid\")\" 2>/dev/null || true; fi
  '" >/dev/null
  vagrant ssh node-a -c "sudo cat /tmp/sysarmor-vm-capture-$S/agent.log 2>/dev/null || true" > "$RESULTS/$S.vm.agent.log"

  EVENTS="$(cd "$ENVDIR" && vagrant ssh mgr -c "/tmp/sysarmorctl --mgr 127.0.0.1:9443 events --scenario '$S' --json" | python3 -c 'import json,sys; print(len(json.load(sys.stdin)))')"
  ENDPOINT_SIGNALS="$(cd "$ENVDIR" && vagrant ssh mgr -c "/tmp/sysarmorctl --mgr 127.0.0.1:9443 signals --scenario '$S' --layer endpoint --json" | python3 -c 'import json,sys; print(len(json.load(sys.stdin)))')"
  CLOUD_SIGNALS="$(cd "$ENVDIR" && vagrant ssh mgr -c "/tmp/sysarmorctl --mgr 127.0.0.1:9443 signals --scenario '$S' --layer cloud --json" | python3 -c 'import json,sys; print(len(json.load(sys.stdin)))')"
  INCIDENTS="$(cd "$ENVDIR" && vagrant ssh mgr -c "/tmp/sysarmorctl --mgr 127.0.0.1:9443 incidents --scenario '$S' --json" | python3 -c 'import json,sys; print(len(json.load(sys.stdin).get("incidents", [])))')"
  python3 - "$RESULTS/vm.$S.managed.json" "$S" "$EVENTS" "$ENDPOINT_SIGNALS" "$CLOUD_SIGNALS" "$INCIDENTS" <<'PY'
import json, sys
path, scenario = sys.argv[1], sys.argv[2]
events, endpoint, cloud, incidents = map(int, sys.argv[3:7])
json.dump({"topology":"vm","scenario":scenario,"managed_events":events,"managed_endpoint_signals":endpoint,"managed_cloud_signals":cloud,"managed_incidents":incidents,"pass":1 if events > 0 else 0,"fail":0 if events > 0 else 1,"skip":0}, open(path, "w"))
PY
  if [[ "$EVENTS" -le 0 ]]; then
    echo "[capture-vm][ERROR] managed daemon uploaded 0 events"
    cat "$RESULTS/$S.vm.agent.log" >&2 2>/dev/null || true
    exit 1
  fi
  echo "[capture-vm] managed upload: events=$EVENTS endpoint=$ENDPOINT_SIGNALS cloud=$CLOUD_SIGNALS incidents=$INCIDENTS"
  exit 0
fi

if [[ "$CAPTURE_MODE" != "replay" ]]; then
  echo "[capture-vm][ERROR] unsupported CAPTURE_MODE=$CAPTURE_MODE"
  exit 1
fi

vagrant ssh mgr -c "curl -sf -X POST 'http://127.0.0.1:9443/api/v1/reset?scenario=$S-stream' >/dev/null" >/dev/null

echo "[capture-vm] v1 replay/stream 调试路径: $S（窗口 ${DUR}s）"
vagrant ssh node-a -c "sudo bash -c '
  MANAGER=http://10.66.0.10:9443
  rm -f /tmp/cap-$S.json
  touch /tmp/cap-$S.json
  tail -n +1 -F /tmp/cap-$S.json 2>/dev/null \
    | /tmp/sysarmor-agent --manager \$MANAGER --agent-id vm-node-a-stream --host-id vm-node-a --scenario $S-stream --stream-jsonl - --batch-size 256 --flush-interval 1s &
  AGENT=\$!
  timeout $DUR tetra getevents -o json > /tmp/cap-$S.json 2>/dev/null &
  CAP=\$!
  sleep 3
  if [ -f /vagrant/test/scenarios/vm/$S/attack.sh ]; then
    GAP=$GAP CYCLES=$CYCLES C2=$C2 bash /vagrant/test/scenarios/vm/$S/attack.sh
  else
    echo \"[capture-vm] $S 无 attack.sh，执行 replay-only smoke\"
    /usr/bin/env true
  fi
  sleep 4
  wait \$CAP 2>/dev/null || true
  kill \$AGENT 2>/dev/null; wait \$AGENT 2>/dev/null || true
  echo captured=\$(wc -l < /tmp/cap-$S.json)
'"

cd "$ENVDIR"
vagrant ssh node-a -c "cat /tmp/cap-$S.json" > "$RESULTS/$S.vm.tetragon.jsonl"
echo "[capture-vm] 落盘: $RESULTS/$S.vm.tetragon.jsonl ($(wc -l < "$RESULTS/$S.vm.tetragon.jsonl") 行)"

STREAM_EVENTS="$(cd "$ENVDIR" && vagrant ssh mgr -c "/tmp/sysarmorctl --mgr 127.0.0.1:9443 events --scenario '$S-stream' --json" | python3 -c 'import json,sys; print(len(json.load(sys.stdin)))')"
STREAM_ENDPOINT_SIGNALS="$(cd "$ENVDIR" && vagrant ssh mgr -c "/tmp/sysarmorctl --mgr 127.0.0.1:9443 signals --scenario '$S-stream' --layer endpoint --json" | python3 -c 'import json,sys; print(len(json.load(sys.stdin)))')"
STREAM_CLOUD_SIGNALS="$(cd "$ENVDIR" && vagrant ssh mgr -c "/tmp/sysarmorctl --mgr 127.0.0.1:9443 signals --scenario '$S-stream' --layer cloud --json" | python3 -c 'import json,sys; print(len(json.load(sys.stdin)))')"
STREAM_INCIDENTS="$(cd "$ENVDIR" && vagrant ssh mgr -c "/tmp/sysarmorctl --mgr 127.0.0.1:9443 incidents --scenario '$S-stream' --json" | python3 -c 'import json,sys; print(len(json.load(sys.stdin).get("incidents", [])))')"
python3 - "$RESULTS/vm.$S.stream.json" "$S" "$STREAM_EVENTS" "$STREAM_ENDPOINT_SIGNALS" "$STREAM_CLOUD_SIGNALS" "$STREAM_INCIDENTS" <<'PY'
import json, sys
path, scenario = sys.argv[1], sys.argv[2]
events, endpoint, cloud, incidents = map(int, sys.argv[3:7])
json.dump({"topology":"vm","scenario":scenario,"stream_events":events,"stream_endpoint_signals":endpoint,"stream_cloud_signals":cloud,"stream_incidents":incidents,"pass":1 if events > 0 else 0,"fail":0 if events > 0 else 1,"skip":0}, open(path, "w"))
PY
if [[ "$STREAM_EVENTS" -le 0 ]]; then
  echo "[capture-vm][ERROR] stream smoke uploaded 0 events"
  exit 1
fi

echo "[capture-vm] 生成 Phase1 replay events 并上传 manager"
python3 "$ROOT/harness/replay_scenario.py" --scenario "$S" --out "$RESULTS/$S.sensor.jsonl"
cd "$ENVDIR"
vagrant upload "$RESULTS/$S.sensor.jsonl" /tmp/$S.sensor.jsonl node-a >/dev/null
vagrant ssh mgr -c "curl -sf -X POST 'http://127.0.0.1:9443/api/v1/reset?scenario=$S' >/dev/null" >/dev/null
vagrant ssh node-a -c "/tmp/sysarmor-agent --manager http://10.66.0.10:9443 --agent-id vm-node-a --host-id vm-node-a --scenario '$S' --input-jsonl /tmp/$S.sensor.jsonl"
