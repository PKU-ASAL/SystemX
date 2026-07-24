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
OUT="${SYSARMOR_AGENT_DIST_OUT:-$REPO/dist/sysarmor-agent-$OS_NAME-$ARCH-$VERSION.tar.gz}"
WORK=""

usage() {
  cat <<EOF
usage: package-agent.sh [--version VERSION] [--output FILE] [--agent-bin FILE] [--ctl-bin FILE]
                        [--tetragon-mode bundled|download] [--tetragon-archive FILE]
                        [--signing-key FILE]

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

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
ROOT="$WORK/root"
mkdir -p "$ROOT/bin" "$ROOT/systemd" "$ROOT/configs" "$ROOT/policies" "$ROOT/sensors/tetragon"
install -m 0755 "$AGENT_BIN" "$ROOT/bin/sysarmor-agent"
install -m 0755 "$CTL_BIN" "$ROOT/bin/sysarmorctl"
install -m 0644 "$SERVICE_FILE" "$ROOT/systemd/sysarmor-agent.service"
install -m 0755 "$INSTALLER_FILE" "$ROOT/install.sh"
install -m 0644 "$REPO/LICENSE" "$ROOT/LICENSE"
if [[ -f "$REPO/configs/agent.example.yaml" ]]; then
  install -m 0644 "$REPO/configs/agent.example.yaml" "$ROOT/configs/agent.example.yaml"
fi
install -m 0644 "$HERE/standalone.yaml" "$ROOT/configs/standalone.yaml"
install -m 0644 "$HERE/policy.json" "$ROOT/policies/policy.json"

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
$(file_json "systemd/sysarmor-agent.service" "0644"),
$(file_json "configs/standalone.yaml" "0644"),
$(file_json "policies/policy.json" "0644"),
$sensor_files
  ]
}
EOF

openssl dgst -sha256 -sign "$SIGNING_KEY" -out "$ROOT/manifest.sig" "$ROOT/manifest.json"
mkdir -p "$(dirname "$OUT")"
tar -C "$ROOT" -czf "$OUT" .
echo "$OUT"
