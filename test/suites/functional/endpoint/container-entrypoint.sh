#!/usr/bin/env bash
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
ENTRYPOINT="$REPO/deployments/agent/sysarmor-container-entrypoint"
WORK="$(mktemp -d)"
WRAPPER_PID=""

cleanup() {
  for pid_file in "$WORK/child.pid" "$WORK/workload.pid" "$WORK/agent.pid"; do
    if [[ -f "$pid_file" ]]; then
      kill -KILL "$(cat "$pid_file")" 2>/dev/null || true
    fi
  done
  if [[ -n "$WRAPPER_PID" ]]; then
    kill -KILL "$WRAPPER_PID" 2>/dev/null || true
    wait "$WRAPPER_PID" 2>/dev/null || true
  fi
  rm -rf "$WORK"
}
trap cleanup EXIT

wait_for_file() {
  local path="$1"
  local deadline=$((SECONDS + 5))
  until [[ -f "$path" ]]; do
    if (( SECONDS >= deadline )); then
      echo "[container-entrypoint][ERROR] timeout waiting for $path" >&2
      exit 1
    fi
    sleep 0.05
  done
}

cat >"$WORK/agent" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
echo "$$" >"$SYSARMOR_TEST_AGENT_PID"
trap 'touch "$SYSARMOR_TEST_AGENT_STOPPED"; exit 0' TERM INT
while true; do sleep 1; done
SH
cat >"$WORK/ctl" <<'SH'
#!/usr/bin/env sh
exit 0
SH
cat >"$WORK/workload" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
echo "$$" >"$SYSARMOR_TEST_WORKLOAD_PID"
touch "$SYSARMOR_TEST_WORKLOAD_STARTED"
trap 'touch "$SYSARMOR_TEST_WORKLOAD_STOPPED"; exit 0' TERM INT
while true; do sleep 1; done
SH
cat >"$WORK/stubborn-workload" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
echo "$$" >"$SYSARMOR_TEST_WORKLOAD_PID"
trap '' TERM INT
(
  trap '' TERM INT
  echo "$BASHPID" >"$SYSARMOR_TEST_CHILD_PID"
  while true; do sleep 1; done
) &
touch "$SYSARMOR_TEST_WORKLOAD_STARTED"
while true; do sleep 1; done
SH
cat >"$WORK/orphaning-workload" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
echo "$$" >"$SYSARMOR_TEST_WORKLOAD_PID"
trap 'exit 0' TERM INT
(
  trap '' TERM INT
  echo "$BASHPID" >"$SYSARMOR_TEST_CHILD_PID"
  while true; do sleep 1; done
) &
touch "$SYSARMOR_TEST_WORKLOAD_STARTED"
while true; do sleep 1; done
SH
chmod 0755 "$WORK/agent" "$WORK/ctl" "$WORK/workload" "$WORK/stubborn-workload" "$WORK/orphaning-workload"

run_entrypoint() {
  local workload="${1:-$WORK/workload}"
  SYSARMOR_AGENT_BIN="$WORK/agent" \
  SYSARMOR_CTL_BIN="$WORK/ctl" \
  SYSARMOR_AGENT_CONFIG="$WORK/agent.yaml" \
  SYSARMOR_AGENT_READY_TIMEOUT=2 \
  SYSARMOR_TEST_AGENT_PID="$WORK/agent.pid" \
  SYSARMOR_TEST_AGENT_STOPPED="$WORK/agent.stopped" \
  SYSARMOR_TEST_WORKLOAD_PID="$WORK/workload.pid" \
  SYSARMOR_TEST_CHILD_PID="$WORK/child.pid" \
  SYSARMOR_TEST_WORKLOAD_STARTED="$WORK/workload.started" \
  SYSARMOR_TEST_WORKLOAD_STOPPED="$WORK/workload.stopped" \
    SYSARMOR_STOP_TIMEOUT=1 \
    "$ENTRYPOINT" "$workload" >"$WORK/entrypoint.log" 2>&1 &
  WRAPPER_PID=$!
}

require_wrapper_exit() {
  local deadline=$((SECONDS + 4))
  while kill -0 "$WRAPPER_PID" 2>/dev/null; do
    if (( SECONDS >= deadline )); then
      echo "[container-entrypoint][ERROR] wrapper did not stop an unresponsive workload" >&2
      return 1
    fi
    sleep 0.05
  done
  wait "$WRAPPER_PID" || true
  WRAPPER_PID=""
}

require_file_quickly() {
  local path="$1"
  local attempt
  for attempt in $(seq 1 10); do
    [[ -f "$path" ]] && return 0
    sleep 0.05
  done
  echo "[container-entrypoint][ERROR] Agent did not receive TERM promptly" >&2
  return 1
}

run_entrypoint
wait_for_file "$WORK/workload.started"
kill "$(cat "$WORK/agent.pid")"
if wait "$WRAPPER_PID"; then
  echo "[container-entrypoint][ERROR] wrapper succeeded after Agent exited" >&2
  exit 1
fi
WRAPPER_PID=""
wait_for_file "$WORK/workload.stopped"

rm -f "$WORK/agent.pid" "$WORK/agent.stopped" "$WORK/workload.started" "$WORK/workload.stopped"
run_entrypoint
wait_for_file "$WORK/workload.started"
kill -TERM "$WRAPPER_PID"
wait "$WRAPPER_PID" || true
WRAPPER_PID=""
wait_for_file "$WORK/agent.stopped"
wait_for_file "$WORK/workload.stopped"

rm -f "$WORK/agent.pid" "$WORK/workload.pid" "$WORK/child.pid" "$WORK/workload.started"
run_entrypoint "$WORK/stubborn-workload"
wait_for_file "$WORK/workload.started"
wait_for_file "$WORK/child.pid"
kill "$(cat "$WORK/agent.pid")"
require_wrapper_exit
if kill -0 "$(cat "$WORK/child.pid")" 2>/dev/null; then
  echo "[container-entrypoint][ERROR] workload child survived wrapper shutdown" >&2
  exit 1
fi

rm -f "$WORK/agent.pid" "$WORK/agent.stopped" "$WORK/workload.pid" "$WORK/child.pid" "$WORK/workload.started"
run_entrypoint "$WORK/stubborn-workload"
wait_for_file "$WORK/workload.started"
wait_for_file "$WORK/child.pid"
kill -TERM "$WRAPPER_PID"
require_file_quickly "$WORK/agent.stopped"
require_wrapper_exit

rm -f "$WORK/agent.pid" "$WORK/agent.stopped" "$WORK/workload.pid" "$WORK/child.pid" "$WORK/workload.started"
run_entrypoint "$WORK/orphaning-workload"
wait_for_file "$WORK/workload.started"
wait_for_file "$WORK/child.pid"
kill "$(cat "$WORK/agent.pid")"
require_wrapper_exit
if kill -0 "$(cat "$WORK/child.pid")" 2>/dev/null; then
  echo "[container-entrypoint][ERROR] orphaned workload child survived wrapper shutdown" >&2
  exit 1
fi

echo "[container-entrypoint] ok"
