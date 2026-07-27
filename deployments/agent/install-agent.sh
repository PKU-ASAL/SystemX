#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CORE="$HERE/install-core.sh"
AGENT_BIN="${SYSARMOR_AGENT_BIN:-./bin/sysarmor-agent}"
CTL_BIN="${SYSARMOR_CTL_BIN:-./bin/sysarmorctl}"
CONTENT_SIGN_BIN="${SYSARMOR_CONTENT_SIGN_BIN:-./bin/sysarmor-content-sign}"
SERVICE_FILE="${SYSARMOR_AGENT_SERVICE:-$HERE/systemd/sysarmor-agent.service}"
CONFIG_FILE="${SYSARMOR_AGENT_CONFIG:-$HERE/standalone.yaml}"
POLICY_FILE="${SYSARMOR_COLLECTION_POLICY:-$HERE/policy.json}"
CONTENT_SOURCE="${SYSARMOR_CONTENT_SOURCE:-$HERE/content}"
SENSOR_SOURCE="${SYSARMOR_SENSOR_SOURCE:-$HERE/../sensors/tetragon}"
AGENT_HOME="${SYSARMOR_AGENT_HOME:-/opt/sysarmor/agent}"
STATE_DIR="${SYSARMOR_STATE_DIR:-/var/lib/sysarmor/agent}"
DEFAULT_CONTENT_DIR="${SYSARMOR_DEFAULT_CONTENT_DIR:-$AGENT_HOME/content/default}"
USER_CONTENT_DIR="${SYSARMOR_CONTENT_DIR:-$STATE_DIR/content}"
WORK="$(mktemp -d)"

cleanup() {
  rm -rf "$WORK"
}
trap cleanup EXIT INT TERM

fail() {
  echo "[install-agent][ERROR] $*" >&2
  exit 1
}

install_prerequisites() {
  if command -v jq >/dev/null 2>&1 && command -v openssl >/dev/null 2>&1; then
    return
  fi
  command -v apt-get >/dev/null 2>&1 || fail "jq and openssl are required"
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -y >/dev/null
  apt-get install -y jq openssl ca-certificates >/dev/null
}

for file in "$CORE" "$AGENT_BIN" "$CTL_BIN" "$CONTENT_SIGN_BIN" "$SERVICE_FILE" "$CONFIG_FILE" "$POLICY_FILE" "$SENSOR_SOURCE/install-bundle.sh" "$SENSOR_SOURCE/bundle.env"; do
  [[ -f "$file" ]] || fail "missing file: $file"
done
install_prerequisites

mkdir -p "$WORK/content/default"
key="$WORK/content-signing-key.pem"
entries="$WORK/content-entries.jsonl"
openssl genpkey -algorithm ED25519 -out "$key" >/dev/null 2>&1
public_key="$(openssl pkey -in "$key" -pubout -outform DER | tail -c 32 | base64 -w0)"
key_id="dev-$(openssl rand -hex 8)"

for source in "$CONTENT_SOURCE"/*.json; do
  name="$(basename "$source")"
  target="$WORK/content/default/$name"
  "$CONTENT_SIGN_BIN" --key "$key" --key-id "$key_id" --input "$source" --output "$target"
  jq -c --arg file "$name" \
    '{ref:.metadata.id,kind:.kind,version:.metadata.version,digest:.integrity.digest,file:$file}' \
    "$target" >>"$entries"
done
jq -s '{version:"dev",entries:.}' "$entries" >"$WORK/content/default/content-manifest.json"

cp "$CONFIG_FILE" "$WORK/agent.yaml"
cat >>"$WORK/agent.yaml" <<EOF

content:
  default_path: "$DEFAULT_CONTENT_DIR"
  path: "$USER_CONTENT_DIR"
  trust_keys: "$key_id=$public_key"
EOF

SYSARMOR_INSTALL_PROFILE=linux-systemd \
SYSARMOR_INSTALL_AGENT_SOURCE="$AGENT_BIN" \
SYSARMOR_INSTALL_CTL_SOURCE="$CTL_BIN" \
SYSARMOR_INSTALL_SERVICE_SOURCE="$SERVICE_FILE" \
SYSARMOR_INSTALL_CONFIG_SOURCE="$WORK/agent.yaml" \
SYSARMOR_INSTALL_POLICY_SOURCE="$POLICY_FILE" \
SYSARMOR_INSTALL_CONTENT_SOURCE="$WORK/content/default" \
SYSARMOR_INSTALL_SENSOR_SOURCE="$SENSOR_SOURCE" \
  "$CORE"
