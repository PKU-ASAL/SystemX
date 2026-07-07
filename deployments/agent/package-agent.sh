#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$HERE/../.." && pwd)"

VERSION="${SYSARMOR_AGENT_VERSION:-dev}"
OS_NAME="${SYSARMOR_AGENT_OS:-linux}"
ARCH="${SYSARMOR_AGENT_ARCH:-amd64}"
AGENT_BIN="${SYSARMOR_AGENT_BIN:-$REPO/dist/bin/sysarmor-agent}"
SERVICE_FILE="${SYSARMOR_AGENT_SERVICE:-$HERE/systemd/sysarmor-agent.service}"
TETRAGON_ARCHIVE="${SYSARMOR_TETRAGON_ARCHIVE:-}"
SIGNING_KEY="${SYSARMOR_ARTIFACT_SIGNING_KEY:-}"
OUT="${SYSARMOR_AGENT_DIST_OUT:-$REPO/dist/sysarmor-agent-$OS_NAME-$ARCH-$VERSION.tar.gz}"
WORK=""

usage() {
  cat <<EOF
usage: package-agent.sh [--version VERSION] [--output FILE] [--agent-bin FILE]
                        [--tetragon-archive FILE] [--signing-key FILE]

Build a signed SysArmor agent distribution tarball.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) VERSION="$2"; shift 2 ;;
    --output) OUT="$2"; shift 2 ;;
    --agent-bin) AGENT_BIN="$2"; shift 2 ;;
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
require_file "$SERVICE_FILE"
if [[ -z "$TETRAGON_ARCHIVE" && -f "$REPO/.cache/tetragon-v1.7.0-amd64.tar.gz" ]]; then
  TETRAGON_ARCHIVE="$REPO/.cache/tetragon-v1.7.0-amd64.tar.gz"
fi
if [[ -z "$TETRAGON_ARCHIVE" ]]; then
  echo "[package-agent][ERROR] --tetragon-archive or SYSARMOR_TETRAGON_ARCHIVE is required" >&2
  exit 1
fi
require_file "$TETRAGON_ARCHIVE"
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
install -m 0644 "$SERVICE_FILE" "$ROOT/systemd/sysarmor-agent.service"
if [[ -f "$REPO/configs/agent.example.yaml" ]]; then
  install -m 0644 "$REPO/configs/agent.example.yaml" "$ROOT/configs/agent.example.yaml"
fi

SYSARMOR_TETRAGON_ARCHIVE="$TETRAGON_ARCHIVE" \
  SYSARMOR_TETRAGON_BUNDLE_DIR="$ROOT/sensors/tetragon" \
  "$REPO/deployments/sensors/tetragon/install-bundle.sh" >/dev/null
rm -f "$ROOT"/sensors/tetragon/*.tar "$ROOT"/sensors/tetragon/*.tar.gz "$ROOT"/sensors/tetragon/*.tgz

file_json() {
  local rel="$1"
  local mode="$2"
  local sum
  sum="$(sha256sum "$ROOT/$rel" | awk '{print $1}')"
  printf '    {"path": "%s", "mode": "%s", "sha256": "%s"}' "$rel" "$mode" "$sum"
}

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
    "config_path": "/etc/sysarmor/agent.yaml",
    "policy_dir": "/etc/sysarmor/policies",
    "runtime_socket": "/run/sysarmor/agent.sock"
  },
  "sensors": [
    {
      "name": "tetragon",
      "backend": "tetragon",
      "bundle_dir": "sensors/tetragon",
      "install_dir": "sensors"
    }
  ],
  "files": [
$(file_json "bin/sysarmor-agent" "0755"),
$(file_json "systemd/sysarmor-agent.service" "0644"),
$(file_json "sensors/tetragon/bin/tetragon" "0755"),
$(file_json "sensors/tetragon/bin/tetra" "0755"),
$(file_json "sensors/tetragon/manifest.json" "0644")
  ]
}
EOF

openssl dgst -sha256 -sign "$SIGNING_KEY" -out "$ROOT/manifest.sig" "$ROOT/manifest.json"
mkdir -p "$(dirname "$OUT")"
tar -C "$ROOT" -czf "$OUT" .
echo "$OUT"
