#!/usr/bin/env bash

sa_agent_write_config() {
  local output="${1:?output required}" state="${2:?state required}"
  local socket="${3:?socket required}" policy="${4:?policy required}"
  local sensor="${5:?sensor block required}"
  mkdir -p "$(dirname "$output")" "$state" "$(dirname "$socket")"
  cat >"$output" <<EOF
local:
  state_path: $state

control:
  socket_path: $socket

content:
  path: $state/content

sensor:
$sensor

policy:
  path: $policy
EOF
}

sa_agent_start_process() {
  local binary="${1:?binary required}" config="${2:?config required}"
  local log="${3:?log required}" pid_ref="${4:?pid ref required}"
  "$binary" run --config "$config" >"$log" 2>&1 &
  printf -v "$pid_ref" '%s' "$!"
}

sa_agent_wait_ready() {
  local name="${1:?name required}"
  shift
  local deadline=$((SECONDS + ${SA_AGENT_WAIT_TIMEOUT:-10}))
  until "$@"; do
    if (( SECONDS >= deadline )); then
      echo "[agent-harness][ERROR] timeout waiting for $name" >&2
      return 1
    fi
    sleep 0.1
  done
}
