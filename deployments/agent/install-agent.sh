#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

AGENT_BIN="${SYSARMOR_AGENT_BIN:-./bin/sysarmor-agent}"
CTL_BIN="${SYSARMOR_CTL_BIN:-./bin/sysarmorctl}"
SERVICE_FILE="${SYSARMOR_AGENT_SERVICE:-$HERE/systemd/sysarmor-agent.service}"
CONFIG_FILE="${SYSARMOR_AGENT_CONFIG:-}"
POLICY_FILE="${SYSARMOR_COLLECTION_POLICY:-}"
DEFAULT_CONFIG="$HERE/standalone.yaml"
DEFAULT_POLICY="$HERE/policy.json"
AGENT_HOME="${SYSARMOR_AGENT_HOME:-/opt/sysarmor/agent}"
AGENT_DST="${SYSARMOR_AGENT_DST:-$AGENT_HOME/bin/sysarmor-agent}"
CTL_DST="${SYSARMOR_CTL_DST:-/usr/local/bin/sysarmorctl}"
SERVICE_DST="${SYSARMOR_SERVICE_DST:-/etc/systemd/system/sysarmor-agent.service}"
CONFIG_DST="${SYSARMOR_CONFIG_DST:-/etc/sysarmor/agent/agent.yaml}"
POLICY_DST="${SYSARMOR_POLICY_DST:-/etc/sysarmor/agent/policy.json}"
BUNDLE_DIR="${SYSARMOR_TETRAGON_BUNDLE_DIR:-$AGENT_HOME/bundles/tetragon}"
INSTALL_DIR="${SYSARMOR_TETRAGON_INSTALL_DIR:-$AGENT_HOME/sensors}"
SENSOR_INSTALLER="${SYSARMOR_TETRAGON_INSTALLER:-$HERE/../sensors/tetragon/install-bundle.sh}"
ENABLE_SERVICE="${SYSARMOR_ENABLE_SERVICE:-1}"

require_file() {
  if [[ ! -f "$1" ]]; then
    echo "[install-agent][ERROR] missing file: $1" >&2
    exit 1
  fi
}

require_file "$AGENT_BIN"
require_file "$CTL_BIN"
require_file "$SERVICE_FILE"
require_file "$SENSOR_INSTALLER"

export DEBIAN_FRONTEND=noninteractive
systemctl stop sysarmor-agent 2>/dev/null || true
systemctl disable sysarmor-agent 2>/dev/null || true
systemctl reset-failed sysarmor-agent 2>/dev/null || true

if command -v apt-get >/dev/null 2>&1; then
  apt-get update -y >/dev/null
  apt-get install -y ca-certificates curl >/dev/null
  apt-get install -y linux-tools-common "linux-tools-$(uname -r)" >/dev/null 2>&1 || \
    apt-get install -y linux-tools-common linux-tools-generic >/dev/null 2>&1 || true
fi

systemctl stop tetragon 2>/dev/null || true
systemctl disable tetragon 2>/dev/null || true
pkill -x tetragon 2>/dev/null || true
pkill -x tetra 2>/dev/null || true

install -d -m 0750 "$(dirname "$CONFIG_DST")"
install -d -m 0700 /var/lib/sysarmor/agent
mkdir -p "$(dirname "$AGENT_DST")" "$(dirname "$CTL_DST")" "$(dirname "$SERVICE_DST")" "$BUNDLE_DIR" "$INSTALL_DIR" "$AGENT_HOME/runtime" "$AGENT_HOME/cache"
install -m 0755 "$AGENT_BIN" "$AGENT_DST"
install -m 0755 "$CTL_BIN" "$CTL_DST"
install -m 0644 "$SERVICE_FILE" "$SERVICE_DST"

if [[ -z "$CONFIG_FILE" ]]; then
  CONFIG_FILE="$DEFAULT_CONFIG"
fi
if [[ -z "$POLICY_FILE" ]]; then
  POLICY_FILE="$DEFAULT_POLICY"
fi
if [[ -n "$CONFIG_FILE" ]]; then
  require_file "$CONFIG_FILE"
  install -m 0644 "$CONFIG_FILE" "$CONFIG_DST"
fi
if [[ -n "$POLICY_FILE" ]]; then
  require_file "$POLICY_FILE"
  install -m 0644 "$POLICY_FILE" "$POLICY_DST"
fi

SYSARMOR_TETRAGON_BUNDLE_DIR="$BUNDLE_DIR" \
SYSARMOR_TETRAGON_ARCHIVE="${SYSARMOR_TETRAGON_ARCHIVE:-}" \
"$SENSOR_INSTALLER"

systemctl daemon-reload 2>/dev/null || true
if [[ "$ENABLE_SERVICE" == "1" ]]; then
  systemctl enable --now sysarmor-agent
  systemctl is-active --quiet sysarmor-agent
fi

echo "[install-agent] installed sysarmor-agent with Tetragon bundle"
