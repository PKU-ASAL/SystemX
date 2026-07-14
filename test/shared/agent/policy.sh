#!/usr/bin/env bash

sa_agent_validate_policy() {
  local file="${1:?policy file required}"
  jq -e '
    type == "object" and
    (.policy_id | type == "string" and length > 0) and
    (.version | type == "number" and . >= 1) and
    (.collection | type == "object") and
    (.detection | type == "object") and
    (.telemetry | type == "object") and
    (.response | type == "object")
  ' "$file" >/dev/null
}

sa_agent_write_policy() {
  local output="${1:?output required}" id="${2:?id required}"
  local version="${3:?version required}" collection="${4:?collection required}"
  local detection="${5:?detection required}" response="${6:?response required}"
  jq -n --arg id "$id" --argjson version "$version" \
    --argjson collection "$collection" --argjson detection "$detection" \
    --argjson response "$response" '{
      policy_id: $id,
      version: $version,
      collection: $collection,
      detection: $detection,
      telemetry: {},
      response: $response
    }' >"$output"
}

sa_agent_apply_policy() {
  local ctl="${1:?ctl required}" socket="${2:?socket required}"
  local type="${3:?policy type required}" file="${4:?policy file required}"
  "$ctl" --socket "$socket" policy apply "$type" --file "$file"
}
