#!/usr/bin/env bash
# Assert the current VM phase: local sysarmor-agent facts captured from ctl.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCENARIO="${1:?用法: assert-vm-local.sh <scenario>}"
SUMMARY="$ROOT/.results/vm.$SCENARIO.local.json"
EVENTS="$ROOT/.results/vm.$SCENARIO.events.ndjson"
SIGNALS="$ROOT/.results/vm.$SCENARIO.signals.ndjson"
LINKED="$ROOT/.results/vm.$SCENARIO.linked.json"
SIGNAL_RULE=""
case "$SCENARIO" in
  apt-fileless-c2)
    SIGNAL_RULE="reverse_shell_pattern"
    ;;
  apt-staged-drop)
    SIGNAL_RULE="payload_dropped"
    ;;
esac

if [[ ! -s "$SUMMARY" ]]; then
  echo "[assert-vm-local][ERROR] missing capture summary: $SUMMARY" >&2
  exit 1
fi
if [[ ! -s "$EVENTS" ]]; then
  echo "[assert-vm-local][ERROR] missing local event stream: $EVENTS" >&2
  exit 1
fi

python3 - "$SUMMARY" "$SCENARIO" <<'PY'
import json, sys
summary_path, scenario = sys.argv[1], sys.argv[2]
data = json.load(open(summary_path))
if data.get("scenario") != scenario:
    raise SystemExit(f"scenario mismatch: got {data.get('scenario')!r} want {scenario!r}")
if data.get("agent_mode") != "local":
    raise SystemExit(f"agent_mode mismatch: {data.get('agent_mode')!r}")
if int(data.get("events", 0)) <= 0:
    raise SystemExit("expected at least one local event")
if scenario != "benign-ci-noise" and int(data.get("attack_signals", 0)) <= 0:
    raise SystemExit("expected at least one attack signal")
if scenario != "benign-ci-noise" and int(data.get("attack_signals_with_event_refs", 0)) <= 0:
    raise SystemExit("expected attack signal eventRefs")
if scenario != "benign-ci-noise" and int(data.get("attack_signals_with_resolved_events", 0)) <= 0:
    raise SystemExit("expected attack signal eventRefs to resolve to local events")
if data.get("missing_event_refs"):
    raise SystemExit(f"missing event refs: {data['missing_event_refs']}")
PY

if ! grep -Fq "\"labels\":{\"scenario\":\"$SCENARIO\"" "$EVENTS"; then
  echo "[assert-vm-local][ERROR] local events do not contain label scenario=$SCENARIO" >&2
  exit 1
fi
if [[ -n "$SIGNAL_RULE" ]] && ! grep -Fq "\"name\":\"$SIGNAL_RULE\"" "$SIGNALS"; then
  echo "[assert-vm-local][ERROR] local attack signal not found: $SIGNAL_RULE" >&2
  exit 1
fi
if [[ -n "$SIGNAL_RULE" ]] && ! grep -Fq '"where":"SIGNAL_WHERE_ENDPOINT"' "$SIGNALS"; then
  echo "[assert-vm-local][ERROR] local attack signal is not endpoint-scoped" >&2
  exit 1
fi
if [[ -n "$SIGNAL_RULE" ]] && ! grep -Fq "\"$SIGNAL_RULE\"" "$LINKED"; then
  echo "[assert-vm-local][ERROR] linked signal report missing rule: $SIGNAL_RULE" >&2
  exit 1
fi

echo "[assert-vm-local] ok"
