#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-agent-harness.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

source "$HERE/install.sh"
source "$HERE/enroll.sh"
source "$HERE/policy.sh"
source "$HERE/inspect.sh"

CONFIG="$TMP/agent.yaml"
POLICY="$TMP/policy.json"
SOCKET="$TMP/control.sock"
STATE="$TMP/state"

sa_agent_write_policy "$POLICY" test-policy 1 '{"behaviors":["process.exec"]}' '{}' '{}'
sa_agent_validate_policy "$POLICY"
sa_agent_write_config "$CONFIG" "$STATE" "$SOCKET" "$POLICY" $'  backend: fake\n  mode: managed'

grep -Fq "state_path: $STATE" "$CONFIG"
grep -Fq "socket_path: $SOCKET" "$CONFIG"
grep -Fq "path: $STATE/content" "$CONFIG"
grep -Fq "path: $POLICY" "$CONFIG"
if grep -Eq '^(manager|data_plane):|^  (id|token|batch_size|policy_path):' "$CONFIG"; then
  echo "legacy field written to Agent config" >&2
  exit 1
fi

printf '%s\n' '{"behaviors":["process.exec"]}' >"$TMP/collection-only.json"
if sa_agent_validate_policy "$TMP/collection-only.json" 2>/dev/null; then
  echo "collection-only policy accepted" >&2
  exit 1
fi

cat >"$TMP/sysarmorctl" <<'SH'
#!/usr/bin/env bash
printf '%s\n' "$@" >"${FAKE_CTL_ARGS:?}"
SH
chmod +x "$TMP/sysarmorctl"
FAKE_CTL_ARGS="$TMP/ctl.args" sa_agent_enroll "$TMP/sysarmorctl" "$SOCKET" \
  "https://manager.test" secret-token
grep -Fxq -- "--socket" "$TMP/ctl.args"
grep -Fxq -- "enroll" "$TMP/ctl.args"
grep -Fxq -- "secret-token" "$TMP/ctl.args"
if grep -Eq -- '--tenant|--agent-id|--gateway' "$TMP/ctl.args"; then
  echo "legacy enrollment identity argument used" >&2
  exit 1
fi

touch "$SOCKET"
sa_agent_wait_ready socket test -e "$SOCKET"

echo "agent harness tests passed"
