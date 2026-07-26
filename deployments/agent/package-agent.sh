#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$HERE/../.." && pwd)"

VERSION="${SYSARMOR_AGENT_VERSION:-dev}"
OS_NAME="${SYSARMOR_AGENT_OS:-linux}"
ARCH="${SYSARMOR_AGENT_ARCH:-amd64}"
AGENT_BIN="${SYSARMOR_AGENT_BIN:-$REPO/dist/bin/sysarmor-agent}"
CTL_BIN="${SYSARMOR_CTL_BIN:-$REPO/dist/bin/sysarmorctl}"
SERVICE_FILE="${SYSARMOR_AGENT_SERVICE:-$HERE/systemd/sysarmor-agent.service}"
INSTALLER_FILE="${SYSARMOR_RELEASE_INSTALLER:-$HERE/install-release.sh}"
TETRAGON_ARCHIVE="${SYSARMOR_TETRAGON_ARCHIVE:-}"
TETRAGON_MODE="${SYSARMOR_TETRAGON_MODE:-bundled}"
SIGNING_KEY="${SYSARMOR_ARTIFACT_SIGNING_KEY:-}"
CONTENT_SIGNING_KEY="${SYSARMOR_CONTENT_SIGNING_KEY:-}"
CONTENT_KEY_ID="${SYSARMOR_CONTENT_KEY_ID:-sysarmor-release}"
OUT="${SYSARMOR_AGENT_DIST_OUT:-$REPO/dist/sysarmor-agent-$OS_NAME-$ARCH-$VERSION.tar.gz}"
WORK=""

usage() {
  cat <<EOF
usage: package-agent.sh [--version VERSION] [--output FILE] [--agent-bin FILE] [--ctl-bin FILE]
                        [--tetragon-mode bundled|download] [--tetragon-archive FILE]
                        [--signing-key FILE] [--content-signing-key FILE] [--content-key-id ID]

Build a signed SysArmor agent distribution tarball.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) VERSION="$2"; shift 2 ;;
    --output) OUT="$2"; shift 2 ;;
    --agent-bin) AGENT_BIN="$2"; shift 2 ;;
    --ctl-bin) CTL_BIN="$2"; shift 2 ;;
    --tetragon-mode) TETRAGON_MODE="$2"; shift 2 ;;
    --tetragon-archive) TETRAGON_ARCHIVE="$2"; shift 2 ;;
    --signing-key) SIGNING_KEY="$2"; shift 2 ;;
    --content-signing-key) CONTENT_SIGNING_KEY="$2"; shift 2 ;;
    --content-key-id) CONTENT_KEY_ID="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "[package-agent][ERROR] unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

require_file() {
  if [[ ! -f "$1" ]]; then
    echo "[package-agent][ERROR] missing file: $1" >&2
    exit 1
  fi
}

require_file "$AGENT_BIN"
require_file "$CTL_BIN"
require_file "$SERVICE_FILE"
require_file "$INSTALLER_FILE"
require_file "$REPO/LICENSE"
if [[ "$TETRAGON_MODE" != "bundled" && "$TETRAGON_MODE" != "download" ]]; then
  echo "[package-agent][ERROR] --tetragon-mode must be bundled or download" >&2
  exit 2
fi
if [[ "$TETRAGON_MODE" == "bundled" && -z "$TETRAGON_ARCHIVE" && -f "$REPO/.cache/tetragon-v1.7.0-amd64.tar.gz" ]]; then
  TETRAGON_ARCHIVE="$REPO/.cache/tetragon-v1.7.0-amd64.tar.gz"
fi
if [[ "$TETRAGON_MODE" == "bundled" && -z "$TETRAGON_ARCHIVE" ]]; then
  echo "[package-agent][ERROR] --tetragon-archive or SYSARMOR_TETRAGON_ARCHIVE is required" >&2
  exit 1
fi
if [[ "$TETRAGON_MODE" == "bundled" ]]; then
  require_file "$TETRAGON_ARCHIVE"
fi
if [[ -z "$SIGNING_KEY" ]]; then
  echo "[package-agent][ERROR] --signing-key or SYSARMOR_ARTIFACT_SIGNING_KEY is required" >&2
  exit 1
fi
require_file "$SIGNING_KEY"
if [[ -z "$CONTENT_SIGNING_KEY" ]]; then
  echo "[package-agent][ERROR] --content-signing-key or SYSARMOR_CONTENT_SIGNING_KEY is required" >&2
  exit 1
fi
require_file "$CONTENT_SIGNING_KEY"
if [[ -z "$CONTENT_KEY_ID" ]]; then
  echo "[package-agent][ERROR] --content-key-id must not be empty" >&2
  exit 1
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
ROOT="$WORK/root"
mkdir -p "$ROOT/bin" "$ROOT/container" "$ROOT/systemd" "$ROOT/configs" "$ROOT/policies" "$ROOT/sensors/tetragon" "$ROOT/content/default"
install -m 0755 "$AGENT_BIN" "$ROOT/bin/sysarmor-agent"
install -m 0755 "$CTL_BIN" "$ROOT/bin/sysarmorctl"
install -m 0755 "$HERE/sysarmor-container-entrypoint" "$ROOT/container/sysarmor-container-entrypoint"
install -m 0644 "$SERVICE_FILE" "$ROOT/systemd/sysarmor-agent.service"
install -m 0755 "$INSTALLER_FILE" "$ROOT/install.sh"
install -m 0644 "$REPO/LICENSE" "$ROOT/LICENSE"
if [[ -f "$REPO/configs/agent.example.yaml" ]]; then
  install -m 0644 "$REPO/configs/agent.example.yaml" "$ROOT/configs/agent.example.yaml"
fi
install -m 0644 "$HERE/standalone.yaml" "$ROOT/configs/standalone.yaml"
install -m 0644 "$HERE/standalone-container.yaml" "$ROOT/configs/standalone-container.yaml"
install -m 0644 "$HERE/policy.json" "$ROOT/policies/policy.json"

go build -o "$WORK/sysarmor-content-sign" "$REPO/cmd/sysarmor-content-sign"
content_public_key="$(openssl pkey -in "$CONTENT_SIGNING_KEY" -pubout -outform DER | tail -c 32 | base64 -w0)"
for config_file in "$ROOT/configs/standalone.yaml" "$ROOT/configs/standalone-container.yaml"; do
  cat >>"$config_file" <<EOF

content:
  default_path: /opt/sysarmor/agent/content/default
  path: /var/lib/sysarmor/agent/content
  trust_keys: "$CONTENT_KEY_ID=$content_public_key"
EOF
done

manifest_entries=""
for source in "$HERE/content"/*.json; do
  name="$(basename "$source")"
  target="$ROOT/content/default/$name"
  "$WORK/sysarmor-content-sign" --key "$CONTENT_SIGNING_KEY" --key-id "$CONTENT_KEY_ID" --input "$source" --output "$target"
  entry="$(jq -c '{ref:.metadata.id,kind:.kind,version:.metadata.version,digest:.integrity.digest,file:$file}' --arg file "$name" "$target")"
  if [[ -n "$manifest_entries" ]]; then
    manifest_entries="$manifest_entries,"
  fi
  manifest_entries="$manifest_entries$entry"
done
cat >"$ROOT/content/default/content-manifest.json" <<EOF
{"version":"$VERSION","entries":[$manifest_entries]}
EOF

if [[ "$TETRAGON_MODE" == "bundled" ]]; then
  SYSARMOR_TETRAGON_ARCHIVE="$TETRAGON_ARCHIVE" \
    SYSARMOR_TETRAGON_BUNDLE_DIR="$ROOT/sensors/tetragon" \
    "$REPO/deployments/sensors/tetragon/install-bundle.sh" >/dev/null
fi
install -m 0755 "$REPO/deployments/sensors/tetragon/install-bundle.sh" "$ROOT/sensors/tetragon/install-bundle.sh"
install -m 0644 "$REPO/deployments/sensors/tetragon/bundle.env" "$ROOT/sensors/tetragon/bundle.env"

file_json() {
  local rel="$1"
  local mode="$2"
  local sum
  sum="$(sha256sum "$ROOT/$rel" | awk '{print $1}')"
  printf '    {"path": "%s", "mode": "%s", "sha256": "%s"}' "$rel" "$mode" "$sum"
}

sensor_files="$(file_json "sensors/tetragon/install-bundle.sh" "0755"),
$(file_json "sensors/tetragon/bundle.env" "0644")"
if [[ "$TETRAGON_MODE" == "bundled" ]]; then
  sensor_files="$sensor_files,
$(file_json "sensors/tetragon/bin/tetragon" "0755"),
$(file_json "sensors/tetragon/bin/tetra" "0755"),
$(file_json "sensors/tetragon/manifest.json" "0644")"
fi

content_files="$(file_json "content/default/content-manifest.json" "0644")"
for content_file in "$ROOT/content/default"/*.json; do
  content_rel="content/default/$(basename "$content_file")"
  if [[ "$content_rel" == "content/default/content-manifest.json" ]]; then
    continue
  fi
  content_files="$content_files,
$(file_json "$content_rel" "0644")"
done

cat > "$ROOT/manifest.json" <<EOF
{
  "schema_version": "sysarmor.agent.distribution/v1",
  "name": "sysarmor-agent",
  "version": "$VERSION",
  "os": "$OS_NAME",
  "arch": "$ARCH",
  "entrypoint": "bin/sysarmor-agent",
  "systemd_unit": "systemd/sysarmor-agent.service",
  "install": {
    "agent_home": "/opt/sysarmor/agent",
    "config_path": "/etc/sysarmor/agent/agent.yaml",
    "policy_path": "/etc/sysarmor/agent/policy.json",
    "runtime_socket": "/run/sysarmor/agent/control.sock"
  },
  "sensors": [
    {
      "name": "tetragon",
      "backend": "tetragon",
      "delivery": "$TETRAGON_MODE",
      "bundle_dir": "sensors/tetragon",
      "install_dir": "sensors"
    }
  ],
  "files": [
$(file_json "LICENSE" "0644"),
$(file_json "install.sh" "0755"),
$(file_json "bin/sysarmor-agent" "0755"),
$(file_json "bin/sysarmorctl" "0755"),
$(file_json "container/sysarmor-container-entrypoint" "0755"),
$(file_json "systemd/sysarmor-agent.service" "0644"),
$(file_json "configs/standalone.yaml" "0644"),
$(file_json "configs/standalone-container.yaml" "0644"),
$(file_json "policies/policy.json" "0644"),
$content_files,
$sensor_files
  ]
}
EOF

openssl dgst -sha256 -sign "$SIGNING_KEY" -out "$ROOT/manifest.sig" "$ROOT/manifest.json"
mkdir -p "$(dirname "$OUT")"
tar -C "$ROOT" -czf "$OUT" .
echo "$OUT"
