#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENABLE_SERVICE="${SYSARMOR_ENABLE_SERVICE:-1}"
AGENT_HOME="${SYSARMOR_AGENT_HOME:-/opt/sysarmor/agent}"
AGENT_DST="${SYSARMOR_AGENT_DST:-$AGENT_HOME/bin/sysarmor-agent}"
CTL_DST="${SYSARMOR_CTL_DST:-/usr/local/bin/sysarmorctl}"
SERVICE_DST="${SYSARMOR_SERVICE_DST:-/etc/systemd/system/sysarmor-agent.service}"
CONFIG_DST="${SYSARMOR_CONFIG_DST:-/etc/sysarmor/agent/agent.yaml}"
POLICY_DST="${SYSARMOR_POLICY_DST:-/etc/sysarmor/agent/policy.json}"
STATE_DIR="${SYSARMOR_STATE_DIR:-/var/lib/sysarmor/agent}"
RUNTIME_DIR="${SYSARMOR_RUNTIME_DIR:-/run/sysarmor/agent}"
BUNDLE_DIR="${SYSARMOR_TETRAGON_BUNDLE_DIR:-$AGENT_HOME/bundles/tetragon}"
INSTALL_DIR="${SYSARMOR_TETRAGON_INSTALL_DIR:-$AGENT_HOME/sensors}"
SOCKET_PATH="${SYSARMOR_AGENT_SOCKET:-/run/sysarmor/agent/control.sock}"

fail() {
  echo "[sysarmor-install][ERROR] $*" >&2
  exit 1
}

require_file() {
  [[ -f "$1" ]] || fail "发行包缺少文件: $1；请重新下载并校验 SHA256SUMS"
}

install_if_absent() {
  local source="$1"
  local target="$2"
  if [[ ! -e "$target" ]]; then
    install -D -m 0644 "$source" "$target"
  fi
}

verify_platform() {
  [[ "$(uname -s)" == "Linux" ]] || fail "当前仅支持 Linux"
  case "$(uname -m)" in
    x86_64|amd64) ;;
    *) fail "当前仅支持 x86_64，检测到 $(uname -m)" ;;
  esac
  if [[ "$ENABLE_SERVICE" == "1" ]]; then
    [[ "${EUID:-$(id -u)}" -eq 0 ]] || fail "需要 root 权限，请使用 sudo ./install.sh"
    command -v systemctl >/dev/null 2>&1 || fail "未找到 systemctl，当前版本仅支持 systemd"
  fi
}

wait_for_agent() {
  local attempt
  for attempt in $(seq 1 30); do
    if "$CTL_DST" --socket "$SOCKET_PATH" --json agent health >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  systemctl status sysarmor-agent --no-pager -l >&2 || true
  fail "Agent 未在 30 秒内就绪；请运行 journalctl -u sysarmor-agent 查看日志"
}

install_sensor_bundle() {
  local parent stage backup
  parent="$(dirname "$BUNDLE_DIR")"
  install -d -m 0755 "$parent"
  stage="$(mktemp -d "$parent/.tetragon.XXXXXX")"
  if [[ -x "$HERE/sensors/tetragon/bin/tetragon" && -x "$HERE/sensors/tetragon/bin/tetra" ]]; then
    cp -a "$HERE/sensors/tetragon/." "$stage/"
  elif ! SYSARMOR_TETRAGON_BUNDLE_DIR="$stage" "$HERE/sensors/tetragon/install-bundle.sh" >/dev/null; then
    rm -rf "$stage"
    return 1
  fi

  backup="$parent/.tetragon.previous.$$"
  if [[ -e "$BUNDLE_DIR" ]]; then
    mv "$BUNDLE_DIR" "$backup"
  fi
  if ! mv "$stage" "$BUNDLE_DIR"; then
    [[ ! -e "$backup" ]] || mv "$backup" "$BUNDLE_DIR"
    return 1
  fi
  rm -rf "$backup"
}

verify_platform
for file in bin/sysarmor-agent bin/sysarmorctl systemd/sysarmor-agent.service configs/standalone.yaml policies/policy.json sensors/tetragon/install-bundle.sh sensors/tetragon/bundle.env; do
  require_file "$HERE/$file"
done

if [[ "$ENABLE_SERVICE" == "1" ]]; then
  systemctl stop sysarmor-agent 2>/dev/null || true
fi

install -d -m 0755 "$(dirname "$AGENT_DST")" "$(dirname "$CTL_DST")" "$(dirname "$SERVICE_DST")" "$INSTALL_DIR"
install -d -m 0750 "$(dirname "$CONFIG_DST")" "$RUNTIME_DIR"
install -d -m 0700 "$STATE_DIR"
install -m 0755 "$HERE/bin/sysarmor-agent" "$AGENT_DST"
install -m 0755 "$HERE/bin/sysarmorctl" "$CTL_DST"
install -m 0644 "$HERE/systemd/sysarmor-agent.service" "$SERVICE_DST"
install_if_absent "$HERE/configs/standalone.yaml" "$CONFIG_DST"
install_if_absent "$HERE/policies/policy.json" "$POLICY_DST"
install_sensor_bundle

if [[ "$ENABLE_SERVICE" == "1" ]]; then
  systemctl daemon-reload
  systemctl enable --now sysarmor-agent
  wait_for_agent
fi

echo "[sysarmor-install] standalone Agent 安装完成"
echo "[sysarmor-install] 健康检查: sudo sysarmorctl agent health"
