#!/usr/bin/env bash

sa_agent_enroll() {
  local ctl="${1:?ctl required}" socket="${2:?socket required}"
  local manager="${3:?manager required}" token="${4:?token required}"
  local tenant="${5:?tenant required}" agent="${6:?agent required}"
  local gateway="${7:?gateway required}" sni="${8:?sni required}"
  "$ctl" --socket "$socket" --manager-url "$manager" enroll \
    --token "$token" --tenant "$tenant" --agent-id "$agent" \
    --gateway "$gateway" --gateway-server-name "$sni"
}

sa_agent_unenroll() {
  local ctl="${1:?ctl required}" socket="${2:?socket required}"
  "$ctl" --socket "$socket" unenroll
}
