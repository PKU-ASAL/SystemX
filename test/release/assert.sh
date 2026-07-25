#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "$HERE/config.sh"

fail() {
  echo "[release-assert][ERROR] $*" >&2
  return 1
}

query_events() {
  docker exec "$1" sysarmorctl --json event watch --snapshot --include-recent \
    --behavior "$EXPECTED_BEHAVIOR" --limit 1000
}

query_signals_with_events() {
  docker exec "$1" sysarmorctl --json signal watch --snapshot --include-recent \
    --include-events --rule-id "$EXPECTED_SIGNAL_RULE" --limit 1000
}

assert_ready() {
  local container="$1" output="$2" deadline=$((SECONDS + HEALTH_TIMEOUT))
  until docker exec "$container" sysarmorctl --json agent health >"$output" 2>"$output.err" &&
    jq -e '
      .status == "ok" and
      .scope.type == "namespace" and .scope.selector == "self" and
      .capability.backend == "tetragon" and
      .sensor.running == true and .sensor.policyLoaded == true
    ' "$output" >/dev/null; do
    if (( SECONDS >= deadline )); then
      docker logs "$container" >&2 2>/dev/null || true
      fail "Agent 未在 ${HEALTH_TIMEOUT} 秒内就绪: $container"
      return
    fi
    sleep 1
  done
}

signal_matches_marker() {
  local input="$1" marker="$2"
  jq -s -e --arg marker "$marker" --arg rule "$EXPECTED_SIGNAL_RULE" \
    --arg severity "$EXPECTED_SIGNAL_SEVERITY" --arg behavior "$EXPECTED_BEHAVIOR" '
      any(.[];
        .signalFrame.signal.ruleId == $rule and
        .signalFrame.signal.severity == $severity and
        (.missingEventRefs | length) == 0 and
        any(.eventFrames[]?;
          .event.behavior == $behavior and
          ((.event.subjectProc.argv // []) | join(" ") | contains($marker))
        )
      )
    ' "$input" >/dev/null
}

assert_detected() {
  local container="$1" marker="$2" output="$3" deadline=$((SECONDS + DETECTION_TIMEOUT))
  until query_signals_with_events "$container" >"$output" 2>"$output.err" && signal_matches_marker "$output" "$marker"; do
    if (( SECONDS >= deadline )); then
      cat "$output" >&2 2>/dev/null || true
      fail "未观察到 marker Event 及其关联 Signal: $marker"
      return
    fi
    sleep 1
  done
}

assert_absent() {
  local container="$1" marker="$2" output="$3"
  sleep "$ISOLATION_SETTLE_SECONDS"
  query_events "$container" >"$output" 2>"$output.err"
  jq -s -e --arg marker "$marker" '
    all(.[]; ((.event.subjectProc.argv // []) | join(" ") | contains($marker)) | not)
  ' "$output" >/dev/null || fail "namespace/self 采集到了外部 marker: $marker"
}

assert_stopped_nonzero() {
  local container="$1" deadline=$((SECONDS + STOP_TIMEOUT)) running exit_code
  while :; do
    running="$(docker inspect "$container" --format '{{.State.Running}}')"
    [[ "$running" == "false" ]] && break
    if (( SECONDS >= deadline )); then
      fail "Agent 退出后容器仍在运行: $container"
      return
    fi
    sleep 0.2
  done
  exit_code="$(docker inspect "$container" --format '{{.State.ExitCode}}')"
  [[ "$exit_code" -ne 0 ]] || fail "Agent 异常退出后容器退出码为 0"
}

case "${1:-}" in
  ready) [[ $# -eq 3 ]] || fail "usage: assert.sh ready CONTAINER OUTPUT"; assert_ready "$2" "$3" ;;
  detected) [[ $# -eq 4 ]] || fail "usage: assert.sh detected CONTAINER MARKER OUTPUT"; assert_detected "$2" "$3" "$4" ;;
  absent) [[ $# -eq 4 ]] || fail "usage: assert.sh absent CONTAINER MARKER OUTPUT"; assert_absent "$2" "$3" "$4" ;;
  stopped-nonzero) [[ $# -eq 2 ]] || fail "usage: assert.sh stopped-nonzero CONTAINER"; assert_stopped_nonzero "$2" ;;
  *) fail "usage: assert.sh ready|detected|absent|stopped-nonzero ..." ;;
esac
