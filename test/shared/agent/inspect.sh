#!/usr/bin/env bash

sa_agent_health() {
  local ctl="${1:?ctl required}" socket="${2:?socket required}"
  "$ctl" --socket "$socket" --json agent health
}

sa_agent_events() {
  local ctl="${1:?ctl required}" socket="${2:?socket required}"
  shift 2
  "$ctl" --socket "$socket" --json event watch --include-recent --snapshot "$@"
}

sa_agent_signals() {
  local ctl="${1:?ctl required}" socket="${2:?socket required}"
  shift 2
  "$ctl" --socket "$socket" --json signal watch --include-recent --snapshot "$@"
}

sa_agent_tail_log() {
  local file="${1:?log file required}" lines="${2:-120}"
  tail -n "$lines" "$file" 2>/dev/null || true
}
