#!/usr/bin/env bash
set -euo pipefail

MODE="${1:?usage: assert-state.sh positive|absent CONTAINER MARKER [TIMEOUT]}"
CONTAINER="${2:?container required}"
MARKER="${3:?marker required}"
TIMEOUT="${4:-60}"
STATE_DIR="/var/lib/sysarmor/agent"
INSPECTOR="${SYSARMOR_STATE_INSPECTOR:?SYSARMOR_STATE_INSPECTOR is required}"
SNAPSHOT="$(mktemp -d)"
trap 'rm -rf "$SNAPSHOT"' EXIT

inspect_snapshot() {
  local mode="$1"
  rm -rf "$SNAPSHOT"
  mkdir -p "$SNAPSHOT"
  docker cp "$CONTAINER:$STATE_DIR/." "$SNAPSHOT"
  "$INSPECTOR" --mode "$mode" --state-dir "$SNAPSHOT" --marker "$MARKER" \
    --signal-rule web_runtime_spawns_shell
}

case "$MODE" in
  positive)
    deadline=$((SECONDS + TIMEOUT))
    until inspect_snapshot positive >/dev/null 2>&1; do
      if (( SECONDS >= deadline )); then
        echo "[release-assert][ERROR] 未观察到 marker Event 或 web_runtime_spawns_shell Signal: $MARKER" >&2
        inspect_snapshot positive >&2 || true
        exit 1
      fi
      sleep 1
    done
    ;;
  absent)
    inspect_snapshot absent
    ;;
  *)
    echo "[release-assert][ERROR] 不支持的模式: $MODE" >&2
    exit 2
    ;;
esac
