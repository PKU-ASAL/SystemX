#!/usr/bin/env bash
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

make_fake_executable() {
  local path="$1"
  mkdir -p "$(dirname "$path")"
  printf '#!/usr/bin/env sh\nexit 0\n' >"$path"
  chmod 0755 "$path"
}

mkdir -p "$WORK/bin"
(cd "$REPO" && go build -o "$WORK/bin/sysarmor-agent" ./apps/agent/cmd/sysarmor-agent)
make_fake_executable "$WORK/bin/sysarmorctl"
make_fake_executable "$WORK/tetragon/tetragon-v1.7.0/bin/tetragon"
make_fake_executable "$WORK/tetragon/tetragon-v1.7.0/bin/tetra"
tar -C "$WORK/tetragon" -czf "$WORK/tetragon.tar.gz" tetragon-v1.7.0
openssl genrsa -out "$WORK/signing-key.pem" 2048 >/dev/null 2>&1
openssl genpkey -algorithm ED25519 -out "$WORK/content-signing-key.pem" >/dev/null 2>&1

if SYSARMOR_TETRAGON_SHA256="$(sha256sum "$WORK/tetragon.tar.gz" | awk '{print $1}')" \
  "$REPO/deployments/agent/package-agent.sh" \
  --version missing-content-key \
  --output "$WORK/missing-content-key.tar.gz" \
  --agent-bin "$WORK/bin/sysarmor-agent" \
  --ctl-bin "$WORK/bin/sysarmorctl" \
  --tetragon-archive "$WORK/tetragon.tar.gz" \
  --signing-key "$WORK/signing-key.pem" >/dev/null 2>&1; then
  echo "[standalone-release-package][ERROR] package accepted missing content signing key" >&2
  exit 1
fi

archive="$WORK/sysarmor-agent-linux-amd64-test.tar.gz"
SYSARMOR_TETRAGON_SHA256="$(sha256sum "$WORK/tetragon.tar.gz" | awk '{print $1}')" \
  "$REPO/deployments/agent/package-agent.sh" \
  --version test \
  --output "$archive" \
  --agent-bin "$WORK/bin/sysarmor-agent" \
  --ctl-bin "$WORK/bin/sysarmorctl" \
  --tetragon-archive "$WORK/tetragon.tar.gz" \
  --content-signing-key "$WORK/content-signing-key.pem" \
  --content-key-id release-test \
  --signing-key "$WORK/signing-key.pem" >/dev/null

mkdir -p "$WORK/release"
tar -xzf "$archive" -C "$WORK/release"
test -x "$WORK/release/install.sh"
test -x "$WORK/release/container/sysarmor-container-entrypoint"
test -f "$WORK/release/configs/standalone-container.yaml"
test -f "$WORK/release/content/default/content-manifest.json"
if [[ -f "$REPO/configs/agent.example.yaml" ]]; then
  test -f "$WORK/release/configs/agent.example.yaml"
  jq -e '.files[] | select(.path == "configs/agent.example.yaml" and .sha256 != "")' \
    "$WORK/release/manifest.json" >/dev/null
fi
grep -Fq '"signature_alg": "ed25519"' "$WORK/release/content/default/rulepack-cep-endpoint.json"
grep -Fq 'Mulan Permissive Software License' "$WORK/release/LICENSE"
while IFS= read -r packaged_file; do
  rel="${packaged_file#"$WORK/release/"}"
  [[ "$rel" == "manifest.json" || "$rel" == "manifest.sig" ]] && continue
  jq -e --arg rel "$rel" '.files[] | select(.path == $rel and .sha256 != "")' \
    "$WORK/release/manifest.json" >/dev/null || {
      echo "[standalone-release-package][ERROR] unsigned package file: $rel" >&2
      exit 1
    }
done < <(find "$WORK/release" -type f -print)

write_upgrade_config() {
  local path="$1" marker="$2"
  cat >"$path" <<EOF
agent:
  label.upgrade_marker: $marker
local:
  state_path: /custom/state
sensor:
  backend: tetragon
  mode: managed
  scope:
    type: container
    selector: 0123456789ab
telemetry:
  max_batch_items: 17
  max_batch_bytes: 64KiB
  flush_interval: 3s
policy:
  path: /custom/policy.json
content:
  default_path: /old/default
  path: /custom/content
  trust_keys: "old=key"
EOF
}

install_env=(
  SYSARMOR_ENABLE_SERVICE=0
  SYSARMOR_AGENT_HOME="$WORK/root/opt/sysarmor/agent"
  SYSARMOR_CTL_DST="$WORK/root/usr/local/bin/sysarmorctl"
  SYSARMOR_SERVICE_DST="$WORK/root/etc/systemd/system/sysarmor-agent.service"
  SYSARMOR_CONFIG_DST="$WORK/root/etc/sysarmor/agent/agent.yaml"
  SYSARMOR_POLICY_DST="$WORK/root/etc/sysarmor/agent/policy.json"
  SYSARMOR_STATE_DIR="$WORK/root/var/lib/sysarmor/agent"
  SYSARMOR_RUNTIME_DIR="$WORK/root/run/sysarmor/agent"
  SYSARMOR_TETRAGON_BUNDLE_DIR="$WORK/root/opt/sysarmor/agent/bundles/tetragon"
  SYSARMOR_TETRAGON_INSTALL_DIR="$WORK/root/opt/sysarmor/agent/sensors"
  SYSARMOR_DEFAULT_CONTENT_DIR="$WORK/root/opt/sysarmor/agent/content/default"
)

cp "$WORK/release/bin/sysarmorctl" "$WORK/sysarmorctl.original"
printf '\ntampered\n' >>"$WORK/release/bin/sysarmorctl"
if env "${install_env[@]}" "$WORK/release/install.sh" >/dev/null 2>&1; then
  echo "[standalone-release-package][ERROR] manifest checksum mismatch was accepted" >&2
  exit 1
fi
test ! -e "$WORK/root/opt/sysarmor/agent/bin/sysarmor-agent"
mv "$WORK/sysarmorctl.original" "$WORK/release/bin/sysarmorctl"
chmod 0755 "$WORK/release/bin/sysarmorctl"

cp "$WORK/release/configs/standalone.yaml" "$WORK/enrollment.yaml"
cat >>"$WORK/enrollment.yaml" <<'EOF'

agent:
  label.install_source: enrollment
EOF
SYSARMOR_RELEASE_CONFIG="$WORK/enrollment.yaml" env "${install_env[@]}" "$WORK/release/install.sh" >/dev/null

test -x "$WORK/root/opt/sysarmor/agent/bin/sysarmor-agent"
test -x "$WORK/root/usr/local/bin/sysarmorctl"
test -x "$WORK/root/opt/sysarmor/agent/bundles/tetragon/bin/tetragon"
test -f "$WORK/root/opt/sysarmor/agent/content/default/content-manifest.json"
grep -Fq 'state_path: /var/lib/sysarmor/agent' "$WORK/root/etc/sysarmor/agent/agent.yaml"
grep -Fq 'label.install_source: enrollment' "$WORK/root/etc/sysarmor/agent/agent.yaml"
if grep -Eq '^[[:space:]]*(tetra_path|tetragon_path):' "$WORK/root/etc/sysarmor/agent/agent.yaml"; then
  echo "[standalone-release-package][ERROR] host config points at the bundle before activation" >&2
  exit 1
fi

printf 'keep-old-default\n' >"$WORK/root/opt/sysarmor/agent/content/default/existing-marker"
printf 'keep-old-config\n' >"$WORK/root/etc/sysarmor/agent/agent.yaml"
cp "$WORK/release/content/default/context-shell-binaries.json" "$WORK/context-shell-binaries.original.json"
jq '.metadata.version = "tampered"' "$WORK/context-shell-binaries.original.json" >"$WORK/release/content/default/context-shell-binaries.json"
if env "${install_env[@]}" "$WORK/release/install.sh" >/dev/null 2>&1; then
  echo "[standalone-release-package][ERROR] invalid default content was installed" >&2
  exit 1
fi
grep -Fxq keep-old-default "$WORK/root/opt/sysarmor/agent/content/default/existing-marker"
grep -Fxq keep-old-config "$WORK/root/etc/sysarmor/agent/agent.yaml"
mv "$WORK/context-shell-binaries.original.json" "$WORK/release/content/default/context-shell-binaries.json"
write_upgrade_config "$WORK/root/etc/sysarmor/agent/agent.yaml" keep-user-config
chmod 0600 "$WORK/root/etc/sysarmor/agent/agent.yaml"
env "${install_env[@]}" "$WORK/release/install.sh" >/dev/null
test ! -e "$WORK/root/opt/sysarmor/agent/content/default/existing-marker"
grep -Fq 'trust_keys: "release-test=' "$WORK/root/etc/sysarmor/agent/agent.yaml"
grep -Fq 'label.upgrade_marker: keep-user-config' "$WORK/root/etc/sysarmor/agent/agent.yaml"
grep -Fq 'state_path: /custom/state' "$WORK/root/etc/sysarmor/agent/agent.yaml"
grep -Fq 'selector: 0123456789ab' "$WORK/root/etc/sysarmor/agent/agent.yaml"
grep -Fq 'max_batch_items: 17' "$WORK/root/etc/sysarmor/agent/agent.yaml"
grep -Fq 'path: "/custom/content"' "$WORK/root/etc/sysarmor/agent/agent.yaml"
[[ "$(stat -c '%a' "$WORK/root/etc/sysarmor/agent/agent.yaml")" == 600 ]]
if grep -Fq 'old=key' "$WORK/root/etc/sysarmor/agent/agent.yaml"; then
  echo "[standalone-release-package][ERROR] old content trust key was preserved" >&2
  exit 1
fi

printf 'transaction-old-content\n' >"$WORK/root/opt/sysarmor/agent/content/default/transaction-marker"
write_upgrade_config "$WORK/root/etc/sysarmor/agent/agent.yaml" transaction-old-config
mkdir -p "$WORK/fail-bin"
cat >"$WORK/fail-bin/mv" <<'EOF'
#!/usr/bin/env bash
if [[ "$1" == */.agent.yaml.* && "$2" == */agent.yaml && ! -e "$FAIL_MV_STATE" ]]; then
  : >"$FAIL_MV_STATE"
  exit 1
fi
exec /usr/bin/mv "$@"
EOF
chmod 0755 "$WORK/fail-bin/mv"
if FAIL_MV_STATE="$WORK/fail-mv.state" PATH="$WORK/fail-bin:$PATH" env "${install_env[@]}" "$WORK/release/install.sh" >/dev/null 2>&1; then
  echo "[standalone-release-package][ERROR] config commit failure was accepted" >&2
  exit 1
fi
grep -Fxq transaction-old-content "$WORK/root/opt/sysarmor/agent/content/default/transaction-marker"
grep -Fq 'label.upgrade_marker: transaction-old-config' "$WORK/root/etc/sysarmor/agent/agent.yaml"
grep -Fq 'trust_keys: "old=key"' "$WORK/root/etc/sysarmor/agent/agent.yaml"
rm -f "$WORK/root/opt/sysarmor/agent/content/default/transaction-marker"

printf 'preserved-policy\n' >"$WORK/root/etc/sysarmor/agent/policy.json"
env "${install_env[@]}" "$WORK/release/install.sh" >/dev/null
grep -Fq 'trust_keys: "release-test=' "$WORK/root/etc/sysarmor/agent/agent.yaml"
grep -Fxq 'preserved-policy' "$WORK/root/etc/sysarmor/agent/policy.json"

echo "[standalone-release-package] ok"

container_install_env=(
  SYSARMOR_AGENT_HOME="$WORK/container-root/opt/sysarmor/agent"
  SYSARMOR_CTL_DST="$WORK/container-root/usr/local/bin/sysarmorctl"
  SYSARMOR_SERVICE_DST="$WORK/container-root/etc/systemd/system/sysarmor-agent.service"
  SYSARMOR_CONFIG_DST="$WORK/container-root/etc/sysarmor/agent/agent.yaml"
  SYSARMOR_POLICY_DST="$WORK/container-root/etc/sysarmor/agent/policy.json"
  SYSARMOR_STATE_DIR="$WORK/container-root/var/lib/sysarmor/agent"
  SYSARMOR_RUNTIME_DIR="$WORK/container-root/run/sysarmor/agent"
  SYSARMOR_TETRAGON_BUNDLE_DIR="$WORK/container-root/opt/sysarmor/agent/bundles/tetragon"
  SYSARMOR_TETRAGON_INSTALL_DIR="$WORK/container-root/opt/sysarmor/agent/sensors"
  SYSARMOR_CONTAINER_ENTRYPOINT_DST="$WORK/container-root/usr/local/bin/sysarmor-container-entrypoint"
)
env "${container_install_env[@]}" "$WORK/release/install.sh" --profile linux-container >/dev/null
test -x "$WORK/container-root/usr/local/bin/sysarmor-container-entrypoint"
grep -Fq 'type: namespace' "$WORK/container-root/etc/sysarmor/agent/agent.yaml"
grep -Fq 'selector: self' "$WORK/container-root/etc/sysarmor/agent/agent.yaml"
if grep -Eq '^[[:space:]]*(tetra_path|tetragon_path):' "$WORK/container-root/etc/sysarmor/agent/agent.yaml"; then
  echo "[standalone-release-package][ERROR] container config points at the bundle before activation" >&2
  exit 1
fi
test ! -e "$WORK/container-root/etc/systemd/system/sysarmor-agent.service"
if env "${container_install_env[@]}" "$WORK/release/install.sh" --profile unsupported >/dev/null 2>&1; then
  echo "[standalone-release-package][ERROR] unsupported install profile was accepted" >&2
  exit 1
fi

echo "[standalone-release-package] linux-container profile ok"

thin_archive="$WORK/sysarmor-agent-linux-amd64-thin.tar.gz"
"$REPO/deployments/agent/package-agent.sh" \
  --version test-thin \
  --output "$thin_archive" \
  --agent-bin "$WORK/bin/sysarmor-agent" \
  --ctl-bin "$WORK/bin/sysarmorctl" \
  --tetragon-mode download \
  --content-signing-key "$WORK/content-signing-key.pem" \
  --content-key-id release-test \
  --signing-key "$WORK/signing-key.pem" >/dev/null

mkdir -p "$WORK/thin-release"
tar -xzf "$thin_archive" -C "$WORK/thin-release"
test ! -e "$WORK/thin-release/sensors/tetragon/bin/tetragon"
test -x "$WORK/thin-release/sensors/tetragon/install-bundle.sh"
rm -rf "$WORK/root/opt/sysarmor/agent/bundles/tetragon"
SYSARMOR_TETRAGON_URL="file://$WORK/tetragon.tar.gz" \
SYSARMOR_TETRAGON_SHA256="$(sha256sum "$WORK/tetragon.tar.gz" | awk '{print $1}')" \
SYSARMOR_TETRAGON_ARCHIVE= \
  env "${install_env[@]}" "$WORK/thin-release/install.sh" >/dev/null
test -x "$WORK/root/opt/sysarmor/agent/bundles/tetragon/bin/tetragon"
printf 'keep\n' >"$WORK/root/opt/sysarmor/agent/bundles/tetragon/existing-marker"
if SYSARMOR_TETRAGON_URL="file://$WORK/tetragon.tar.gz" \
  SYSARMOR_TETRAGON_SHA256="$(printf '0%.0s' {1..64})" \
  SYSARMOR_TETRAGON_ARCHIVE= \
  env "${install_env[@]}" "$WORK/thin-release/install.sh" >/dev/null 2>&1; then
  echo "[standalone-release-package][ERROR] invalid Tetragon checksum was accepted" >&2
  exit 1
fi
grep -Fxq keep "$WORK/root/opt/sysarmor/agent/bundles/tetragon/existing-marker"

thin_container_env=(
  SYSARMOR_AGENT_HOME="$WORK/thin-container-root/opt/sysarmor/agent"
  SYSARMOR_CTL_DST="$WORK/thin-container-root/usr/local/bin/sysarmorctl"
  SYSARMOR_SERVICE_DST="$WORK/thin-container-root/etc/systemd/system/sysarmor-agent.service"
  SYSARMOR_CONFIG_DST="$WORK/thin-container-root/etc/sysarmor/agent/agent.yaml"
  SYSARMOR_POLICY_DST="$WORK/thin-container-root/etc/sysarmor/agent/policy.json"
  SYSARMOR_STATE_DIR="$WORK/thin-container-root/var/lib/sysarmor/agent"
  SYSARMOR_RUNTIME_DIR="$WORK/thin-container-root/run/sysarmor/agent"
  SYSARMOR_TETRAGON_BUNDLE_DIR="$WORK/thin-container-root/opt/sysarmor/agent/bundles/tetragon"
  SYSARMOR_TETRAGON_INSTALL_DIR="$WORK/thin-container-root/opt/sysarmor/agent/sensors"
  SYSARMOR_CONTAINER_ENTRYPOINT_DST="$WORK/thin-container-root/usr/local/bin/sysarmor-container-entrypoint"
)
SYSARMOR_TETRAGON_URL="file://$WORK/tetragon.tar.gz" \
SYSARMOR_TETRAGON_SHA256="$(sha256sum "$WORK/tetragon.tar.gz" | awk '{print $1}')" \
SYSARMOR_TETRAGON_ARCHIVE= \
  env "${thin_container_env[@]}" "$WORK/thin-release/install.sh" --profile linux-container >/dev/null
test -x "$WORK/thin-container-root/opt/sysarmor/agent/bundles/tetragon/bin/tetragon"
grep -Fq 'type: namespace' "$WORK/thin-container-root/etc/sysarmor/agent/agent.yaml"
grep -Fq 'selector: self' "$WORK/thin-container-root/etc/sysarmor/agent/agent.yaml"

echo "[standalone-release-package] thin package ok"
