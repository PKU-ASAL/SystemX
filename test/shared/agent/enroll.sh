#!/usr/bin/env bash

sa_agent_enroll() {
  local ctl="${1:?ctl required}" socket="${2:?socket required}"
  local manager="${3:?manager required}" token="${4:?token required}"
  "$ctl" --socket "$socket" enroll --manager-url "$manager" --token "$token"
}

sa_agent_unenroll() {
  local ctl="${1:?ctl required}" socket="${2:?socket required}"
  "$ctl" --socket "$socket" unenroll
}
