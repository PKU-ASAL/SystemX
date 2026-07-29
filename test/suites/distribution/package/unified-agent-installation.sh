#!/usr/bin/env bash
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
CORE="$REPO/deployments/agent/install-core.sh"
RELEASE="$REPO/deployments/agent/install-release.sh"
DEVELOPMENT="$REPO/deployments/agent/install-agent.sh"

fail() {
  echo "[unified-agent-installation][ERROR] $*" >&2
  exit 1
}

[[ -x "$CORE" ]] || fail "missing executable install-core.sh"
for wrapper in "$RELEASE" "$DEVELOPMENT"; do
  grep -Fq 'install-core.sh' "$wrapper" || fail "$(basename "$wrapper") does not delegate to install-core.sh"
  if grep -Eq 'systemctl[[:space:]]+(enable|start|restart)|commit_release_config_and_content|rollback_release_config_and_content' "$wrapper"; then
    fail "$(basename "$wrapper") contains installation transaction or service lifecycle logic"
  fi
done

echo "[unified-agent-installation] entry contract ok"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
mkdir -p "$WORK/deployments/agent/content" "$WORK/deployments/agent/systemd" "$WORK/deployments/sensors/tetragon" "$WORK/bin"
cp "$DEVELOPMENT" "$WORK/deployments/agent/install-agent.sh"
cp "$REPO/deployments/agent/content/"*.json "$WORK/deployments/agent/content/"
cp "$REPO/deployments/sensors/tetragon/bundle.env" "$WORK/deployments/sensors/tetragon/bundle.env"
for file in agent ctl service config policy; do
  : >"$WORK/$file"
done
cat >"$WORK/deployments/sensors/tetragon/install-bundle.sh" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
cat >"$WORK/bin/content-sign" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
while [[ $# -gt 0 ]]; do
  case "$1" in
    --input) input="$2"; shift 2 ;;
    --output) output="$2"; shift 2 ;;
    *) shift 2 ;;
  esac
done
jq '.integrity={digest:"test-digest",signature:"test-signature"}' "$input" >"$output"
EOF
cat >"$WORK/deployments/agent/install-core.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
jq -e '.entries | length > 0' "$SYSARMOR_INSTALL_CONTENT_SOURCE/content-manifest.json" >/dev/null
grep -Fq 'trust_keys: "dev-' "$SYSARMOR_INSTALL_CONFIG_SOURCE"
private_dir="$(dirname "$(dirname "$SYSARMOR_INSTALL_CONTENT_SOURCE")")"
test -f "$private_dir/content-signing-key.pem"
printf '%s\n' "$private_dir" >"$INSTALL_CAPTURE"
EOF
chmod +x "$WORK/deployments/agent/install-agent.sh" "$WORK/deployments/agent/install-core.sh" \
  "$WORK/deployments/sensors/tetragon/install-bundle.sh" "$WORK/bin/content-sign"

INSTALL_CAPTURE="$WORK/install-capture" \
SYSARMOR_AGENT_BIN="$WORK/agent" \
SYSARMOR_CTL_BIN="$WORK/ctl" \
SYSARMOR_CONTENT_SIGN_BIN="$WORK/bin/content-sign" \
SYSARMOR_AGENT_SERVICE="$WORK/service" \
SYSARMOR_AGENT_CONFIG="$WORK/config" \
SYSARMOR_COLLECTION_POLICY="$WORK/policy" \
  "$WORK/deployments/agent/install-agent.sh"
private_dir="$(cat "$WORK/install-capture")"
[[ ! -e "$private_dir" ]] || fail "development signing private directory was not removed"

echo "[unified-agent-installation] development adapter ok"

for benchmark in "$REPO/test/suites/performance/endpoint/run.sh" "$REPO/test/suites/performance/endpoint/lifecycle.sh"; do
  grep -Fq 'SYSARMOR_BENCH_CONTENT_DIR:-test/data/content' "$benchmark" || \
    fail "$(basename "$benchmark") does not keep product defaults separate from benchmark content"
done

echo "[unified-agent-installation] benchmark content boundary ok"

if jq -e '.detection.rule_overrides[]? | select(.rule_id == "credential_file_read" and .enabled == false)' \
  "$REPO/deployments/agent/policy.json" >/dev/null; then
  fail "default policy disables credential_file_read"
fi
if grep -Eq 'sudo sysarmorctl.*agent health' "$REPO/test/shared/recorder/recorder-vm.sh"; then
  fail "performance recorder creates sudo credential-read noise"
fi

echo "[unified-agent-installation] credential read defaults ok"
