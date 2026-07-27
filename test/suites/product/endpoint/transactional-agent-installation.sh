#!/usr/bin/env bash
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

fail() {
  echo "[transactional-agent-installation][ERROR] $*" >&2
  exit 1
}

make_sources() {
  local src="$WORK/source"
  mkdir -p "$src/content" "$src/sensor/bin"
  cat >"$src/agent" <<'EOF'
#!/usr/bin/env bash
if [[ "${1:-}" == merge-release-config ]]; then
  while [[ $# -gt 0 ]]; do
    [[ "$1" != --release ]] || release="$2"
    [[ "$1" != --output ]] || output="$2"
    shift
  done
  cp "$release" "$output"
fi
EOF
  cat >"$src/ctl" <<'EOF'
#!/usr/bin/env bash
[[ "${HEALTH_FAIL:-0}" != 1 ]]
EOF
  chmod 0755 "$src/agent" "$src/ctl"
  printf 'new-service\n' >"$src/service"
  printf 'new-config\n' >"$src/config"
  cat >"$src/policy" <<'EOF'
{"detection":{"rulesets":[{"ref":"ruleset:test","enabled":true}]}}
EOF
  cat >"$src/content/rulepack.json" <<'EOF'
{"kind":"rulepack","metadata":{"id":"pack:test","version":"1"},"integrity":{"digest":"digest:test"},"spec":{"rulesets":[{"id":"ruleset:test"}]}}
EOF
  cat >"$src/content/content-manifest.json" <<'EOF'
{"version":"1","entries":[{"ref":"pack:test","kind":"rulepack","version":"1","digest":"digest:test","file":"rulepack.json"}]}
EOF
  printf '#!/usr/bin/env bash\nexit 0\n' >"$src/sensor/install-bundle.sh"
  chmod 0755 "$src/sensor/install-bundle.sh"
  printf 'VERSION=test\n' >"$src/sensor/bundle.env"
  printf '#!/usr/bin/env sh\nexit 0\n' >"$src/sensor/bin/tetragon"
  cp "$src/sensor/bin/tetragon" "$src/sensor/bin/tetra"
  chmod 0755 "$src/sensor/bin/tetragon" "$src/sensor/bin/tetra"
}

make_commands() {
  mkdir -p "$WORK/commands"
  cat >"$WORK/commands/systemctl" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$SYSTEMCTL_LOG"
case "$1" in
  is-active) [[ -e "$SERVICE_STATE/active" ]] ;;
  is-enabled) [[ -e "$SERVICE_STATE/enabled" ]] ;;
  stop) rm -f "$SERVICE_STATE/active" ;;
  start) : >"$SERVICE_STATE/active" ;;
  disable) rm -f "$SERVICE_STATE/enabled" ;;
  enable) : >"$SERVICE_STATE/enabled"; [[ "$*" != *--now* ]] || : >"$SERVICE_STATE/active" ;;
  daemon-reload|status) ;;
esac
EOF
  cat >"$WORK/commands/mv" <<'EOF'
#!/usr/bin/env bash
if [[ "$1" == *'.stage.'* && "$2" == "${FAIL_COMMIT_TARGET:-}" && ! -e "${FAIL_ONCE_STATE:-/nonexistent}" ]]; then
  : >"$FAIL_ONCE_STATE"
  exit 1
fi
if [[ "$1" == *'.stage.'* && "$2" == "${SIGNAL_COMMIT_TARGET:-}" && ! -e "${SIGNAL_ONCE_STATE:-/nonexistent}" ]]; then
  : >"$SIGNAL_ONCE_STATE"
  kill -s "${INSTALL_SIGNAL:-TERM}" "$PPID"
fi
if [[ "$1" == *'.previous.'* && "$2" == "${FAIL_RESTORE_TARGET:-}" ]]; then
  exit 1
fi
exec /usr/bin/mv "$@"
EOF
  printf '#!/usr/bin/env sh\nprintf "1\\n"\n' >"$WORK/commands/seq"
  printf '#!/usr/bin/env sh\nexit 0\n' >"$WORK/commands/sleep"
  chmod 0755 "$WORK/commands/"*
}

set_paths() {
  ROOT="$WORK/root-$1"
  AGENT="$ROOT/opt/agent/bin/sysarmor-agent"
  CTL="$ROOT/usr/bin/sysarmorctl"
  SERVICE="$ROOT/etc/systemd/system/sysarmor-agent.service"
  CONFIG="$ROOT/etc/sysarmor/agent.yaml"
  POLICY="$ROOT/etc/sysarmor/policy.json"
  CONTENT="$ROOT/opt/agent/content/default"
  SENSOR="$ROOT/opt/agent/bundles/tetragon"
  SERVICE_STATE="$ROOT/service-state"
  SYSTEMCTL_LOG="$ROOT/systemctl.log"
  export SERVICE_STATE SYSTEMCTL_LOG
}

seed_old_installation() {
  mkdir -p "$(dirname "$AGENT")" "$(dirname "$CTL")" "$(dirname "$SERVICE")" \
    "$(dirname "$CONFIG")" "$(dirname "$POLICY")" "$CONTENT" "$SENSOR" "$SERVICE_STATE"
  printf 'old-agent\n' >"$AGENT"
  printf 'old-ctl\n' >"$CTL"
  printf 'old-service\n' >"$SERVICE"
  printf 'old-config\n' >"$CONFIG"
  printf 'old-policy\n' >"$POLICY"
  printf 'old-content\n' >"$CONTENT/marker"
  printf 'old-sensor\n' >"$SENSOR/marker"
  : >"$SERVICE_STATE/active"
  : >"$SERVICE_STATE/enabled"
  : >"$SYSTEMCTL_LOG"
}

run_core() {
  env PATH="$WORK/commands:$PATH" \
    SYSARMOR_INSTALL_PROFILE=linux-systemd SYSARMOR_ENABLE_SERVICE=1 \
    SYSARMOR_INSTALL_AGENT_SOURCE="$WORK/source/agent" SYSARMOR_INSTALL_CTL_SOURCE="$WORK/source/ctl" \
    SYSARMOR_INSTALL_SERVICE_SOURCE="$WORK/source/service" SYSARMOR_INSTALL_CONFIG_SOURCE="$WORK/source/config" \
    SYSARMOR_INSTALL_POLICY_SOURCE="$WORK/source/policy" SYSARMOR_INSTALL_CONTENT_SOURCE="$WORK/source/content" \
    SYSARMOR_INSTALL_SENSOR_SOURCE="$WORK/source/sensor" SYSARMOR_AGENT_HOME="$ROOT/opt/agent" \
    SYSARMOR_AGENT_DST="$AGENT" SYSARMOR_CTL_DST="$CTL" SYSARMOR_SERVICE_DST="$SERVICE" \
    SYSARMOR_CONFIG_DST="$CONFIG" SYSARMOR_POLICY_DST="$POLICY" SYSARMOR_STATE_DIR="$ROOT/state" \
    SYSARMOR_RUNTIME_DIR="$ROOT/run" SYSARMOR_TETRAGON_BUNDLE_DIR="$SENSOR" \
    SYSARMOR_TETRAGON_INSTALL_DIR="$ROOT/opt/agent/sensors" SYSARMOR_DEFAULT_CONTENT_DIR="$CONTENT" \
    SYSARMOR_AGENT_SOCKET="$ROOT/run/control.sock" "$REPO/deployments/agent/install-core.sh"
}

assert_old_installation() {
  grep -Fxq old-agent "$AGENT" || fail "Agent was not rolled back"
  grep -Fxq old-ctl "$CTL" || fail "CLI was not rolled back"
  grep -Fxq old-service "$SERVICE" || fail "service was not rolled back"
  grep -Fxq old-config "$CONFIG" || fail "config was not rolled back"
  grep -Fxq old-policy "$POLICY" || fail "policy was not preserved"
  grep -Fxq old-content "$CONTENT/marker" || fail "content was not rolled back"
  grep -Fxq old-sensor "$SENSOR/marker" || fail "sensor was not rolled back"
  [[ -e "$SERVICE_STATE/active" && -e "$SERVICE_STATE/enabled" ]] || fail "service state was not restored"
}

make_sources
make_commands

for target_name in agent ctl service config content sensor; do
  set_paths "commit-$target_name"
  seed_old_installation
  case "$target_name" in
    agent) target="$AGENT" ;; ctl) target="$CTL" ;; service) target="$SERVICE" ;;
    config) target="$CONFIG" ;; content) target="$CONTENT" ;; sensor) target="$SENSOR" ;;
  esac
  if FAIL_COMMIT_TARGET="$target" FAIL_ONCE_STATE="$ROOT/fail-once" run_core >/dev/null 2>&1; then
    fail "$target_name commit failure was accepted"
  fi
  assert_old_installation
done

set_paths health
seed_old_installation
if HEALTH_FAIL=1 run_core >/dev/null 2>&1; then
  fail "health failure was accepted"
fi
assert_old_installation

set_paths rollback-failure
seed_old_installation
rollback_error="$ROOT/rollback-error.log"
if HEALTH_FAIL=1 FAIL_RESTORE_TARGET="$AGENT" run_core >/dev/null 2>"$rollback_error"; then
  fail "rollback restoration failure was accepted"
fi
shopt -s nullglob
rollback_backups=("$(dirname "$AGENT")/.$(basename "$AGENT").previous."*)
shopt -u nullglob
[[ ${#rollback_backups[@]} -eq 1 && -e "${rollback_backups[0]}" ]] || \
  { cat "$rollback_error" >&2; fail "failed rollback did not preserve the Agent backup"; }
grep -Fq "$AGENT" "$rollback_error" || fail "rollback failure did not identify the target"
grep -Fq "${rollback_backups[0]}" "$rollback_error" || fail "rollback failure did not identify the backup"

for install_signal in TERM INT; do
  set_paths "signal-${install_signal,,}"
  seed_old_installation
  if SIGNAL_COMMIT_TARGET="$CONTENT" SIGNAL_ONCE_STATE="$ROOT/signal-once" \
    INSTALL_SIGNAL="$install_signal" run_core >/dev/null 2>&1; then
    fail "$install_signal interruption was accepted"
  fi
  assert_old_installation
done

set_paths missing-ruleset
seed_old_installation
cp "$WORK/source/policy" "$WORK/policy.valid"
sed 's/ruleset:test/ruleset:missing/' "$WORK/policy.valid" >"$WORK/source/policy"
if run_core >/dev/null 2>&1; then
  fail "missing ruleset was accepted"
fi
if grep -Fxq 'stop sysarmor-agent' "$SYSTEMCTL_LOG"; then
  fail "service was stopped before policy/content validation"
fi
assert_old_installation
mv "$WORK/policy.valid" "$WORK/source/policy"

set_paths policy
mkdir -p "$SERVICE_STATE"
: >"$SYSTEMCTL_LOG"
if FAIL_COMMIT_TARGET="$POLICY" FAIL_ONCE_STATE="$ROOT/fail-once" run_core >/dev/null 2>&1; then
  fail "policy commit failure was accepted"
fi
[[ ! -e "$AGENT" && ! -e "$POLICY" && ! -e "$CONTENT" ]] || fail "first-install rollback left committed files"

echo "[transactional-agent-installation] ok"
