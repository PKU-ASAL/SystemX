#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROFILE="${SYSARMOR_INSTALL_PROFILE:-linux-systemd}"
AGENT_HOME="${SYSARMOR_AGENT_HOME:-/opt/sysarmor/agent}"
AGENT_DST="${SYSARMOR_AGENT_DST:-$AGENT_HOME/bin/sysarmor-agent}"
CTL_DST="${SYSARMOR_CTL_DST:-/usr/local/bin/sysarmorctl}"
CONTAINER_ENTRYPOINT_DST="${SYSARMOR_CONTAINER_ENTRYPOINT_DST:-/usr/local/bin/sysarmor-container-entrypoint}"
SERVICE_DST="${SYSARMOR_SERVICE_DST:-/etc/systemd/system/sysarmor-agent.service}"
CONFIG_DST="${SYSARMOR_CONFIG_DST:-/etc/sysarmor/agent/agent.yaml}"
POLICY_DST="${SYSARMOR_POLICY_DST:-/etc/sysarmor/agent/policy.json}"
STATE_DIR="${SYSARMOR_STATE_DIR:-/var/lib/sysarmor/agent}"
RUNTIME_DIR="${SYSARMOR_RUNTIME_DIR:-/run/sysarmor/agent}"
BUNDLE_DIR="${SYSARMOR_TETRAGON_BUNDLE_DIR:-$AGENT_HOME/bundles/tetragon}"
DEFAULT_CONTENT_DIR="${SYSARMOR_DEFAULT_CONTENT_DIR:-$AGENT_HOME/content/default}"
INSTALL_DIR="${SYSARMOR_TETRAGON_INSTALL_DIR:-$AGENT_HOME/sensors}"
SOCKET_PATH="${SYSARMOR_AGENT_SOCKET:-/run/sysarmor/agent/control.sock}"
TX_CONTENT_BACKUP=""
TX_CONFIG_BACKUP=""
TX_CONTENT_STAGE=""
TX_CONFIG_STAGE=""
TX_OLD_CONTENT=0
TX_OLD_CONFIG=0
TX_NEW_CONTENT=0
TX_NEW_CONFIG=0

usage() {
  echo "usage: install.sh [--profile linux-systemd|linux-container]"
}

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

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --profile)
        [[ $# -ge 2 ]] || fail "--profile 缺少参数"
        PROFILE="$2"
        shift 2
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *) fail "不支持的参数: $1" ;;
    esac
  done
  case "$PROFILE" in
    linux-systemd|linux-container) ;;
    *) fail "不支持的安装 profile: $PROFILE；可选值为 linux-systemd、linux-container" ;;
  esac
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

install_release_config_and_content() {
  local source parent stage config_parent config_stage
  source="$HERE/content/default"
  parent="$(dirname "$DEFAULT_CONTENT_DIR")"
  config_parent="$(dirname "$CONFIG_DST")"
  install -d -m 0755 "$parent"
  install -d -m 0750 "$config_parent"
  stage="$(mktemp -d "$parent/.default.XXXXXX")"
  config_stage="$(mktemp "$config_parent/.agent.yaml.XXXXXX")"
  cp -a "$source/." "$stage/"
  if [[ -e "$CONFIG_DST" ]]; then
    if ! "$AGENT_DST" merge-release-config --existing "$CONFIG_DST" --release "$CONFIG_SOURCE" --output "$config_stage"; then
      rm -rf "$stage"
      rm -f "$config_stage"
      return 1
    fi
    chmod 0644 "$config_stage"
  else
    install -m 0644 "$CONFIG_SOURCE" "$config_stage"
  fi
  if ! validate_default_content "$stage"; then
    rm -rf "$stage"
    rm -f "$config_stage"
    return 1
  fi
  commit_release_config_and_content "$stage" "$config_stage"
}

commit_release_config_and_content() {
  local stage="$1" config_stage="$2" parent config_parent
  parent="$(dirname "$DEFAULT_CONTENT_DIR")"
  config_parent="$(dirname "$CONFIG_DST")"
  TX_CONTENT_BACKUP="$parent/.default.previous.$$"
  TX_CONFIG_BACKUP="$config_parent/.agent.yaml.previous.$$"
  TX_CONTENT_STAGE="$stage"
  TX_CONFIG_STAGE="$config_stage"
  trap 'rollback_release_config_and_content' EXIT
  trap 'abort_release_transaction 130' INT
  trap 'abort_release_transaction 143' TERM
  if [[ -e "$DEFAULT_CONTENT_DIR" ]]; then
    TX_OLD_CONTENT=1
    if ! mv "$DEFAULT_CONTENT_DIR" "$TX_CONTENT_BACKUP"; then
      rollback_release_config_and_content
      return 1
    fi
  fi
  if [[ -e "$CONFIG_DST" ]]; then
    TX_OLD_CONFIG=1
    if ! mv "$CONFIG_DST" "$TX_CONFIG_BACKUP"; then
      rollback_release_config_and_content
      return 1
    fi
  fi
  TX_NEW_CONFIG=1
  if ! mv "$TX_CONFIG_STAGE" "$CONFIG_DST"; then
    rollback_release_config_and_content
    return 1
  fi
  TX_NEW_CONTENT=1
  if ! mv "$TX_CONTENT_STAGE" "$DEFAULT_CONTENT_DIR"; then
    rollback_release_config_and_content
    return 1
  fi
  trap - INT TERM EXIT
  rm -f "$TX_CONFIG_BACKUP"
  rm -rf "$TX_CONTENT_BACKUP"
  reset_release_transaction
}

rollback_release_config_and_content() {
  trap - INT TERM EXIT
  [[ "$TX_NEW_CONTENT" != 1 ]] || rm -rf "$DEFAULT_CONTENT_DIR"
  [[ "$TX_NEW_CONFIG" != 1 ]] || rm -f "$CONFIG_DST"
  [[ "$TX_OLD_CONFIG" != 1 || ! -e "$TX_CONFIG_BACKUP" ]] || mv "$TX_CONFIG_BACKUP" "$CONFIG_DST"
  [[ "$TX_OLD_CONTENT" != 1 || ! -e "$TX_CONTENT_BACKUP" ]] || mv "$TX_CONTENT_BACKUP" "$DEFAULT_CONTENT_DIR"
  [[ -z "$TX_CONFIG_STAGE" || ! -e "$TX_CONFIG_STAGE" ]] || rm -f "$TX_CONFIG_STAGE"
  [[ -z "$TX_CONTENT_STAGE" || ! -e "$TX_CONTENT_STAGE" ]] || rm -rf "$TX_CONTENT_STAGE"
  reset_release_transaction
}

abort_release_transaction() {
  local status="$1"
  rollback_release_config_and_content
  exit "$status"
}

reset_release_transaction() {
  TX_CONTENT_BACKUP="" TX_CONFIG_BACKUP="" TX_CONTENT_STAGE="" TX_CONFIG_STAGE=""
  TX_OLD_CONTENT=0 TX_OLD_CONFIG=0 TX_NEW_CONTENT=0 TX_NEW_CONFIG=0
}

validate_default_content() {
  local dir entry file
  dir="$1"
  command -v jq >/dev/null 2>&1 || fail "未找到 jq，无法校验默认内容清单"
  jq -e '.version != "" and (.entries | length > 0)' "$dir/content-manifest.json" >/dev/null || return 1
  while IFS= read -r entry; do
    file="$(jq -r '.file' <<<"$entry")"
    [[ -n "$file" && "$file" == "$(basename "$file")" && -f "$dir/$file" ]] || return 1
    jq -e --argjson entry "$entry" \
      '.metadata.id == $entry.ref and .kind == $entry.kind and .metadata.version == $entry.version and .integrity.digest == $entry.digest' \
      "$dir/$file" >/dev/null || return 1
  done < <(jq -c '.entries[]' "$dir/content-manifest.json")
}

parse_args "$@"
if [[ "$PROFILE" == "linux-container" ]]; then
  ENABLE_SERVICE=0
  CONFIG_SOURCE="$HERE/configs/standalone-container.yaml"
else
  ENABLE_SERVICE="${SYSARMOR_ENABLE_SERVICE:-1}"
  CONFIG_SOURCE="$HERE/configs/standalone.yaml"
fi

verify_platform
for file in bin/sysarmor-agent bin/sysarmorctl systemd/sysarmor-agent.service configs/standalone.yaml configs/standalone-container.yaml container/sysarmor-container-entrypoint policies/policy.json content/default/content-manifest.json sensors/tetragon/install-bundle.sh sensors/tetragon/bundle.env; do
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
if [[ "$PROFILE" == "linux-container" ]]; then
  install -D -m 0755 "$HERE/container/sysarmor-container-entrypoint" "$CONTAINER_ENTRYPOINT_DST"
else
  install -m 0644 "$HERE/systemd/sysarmor-agent.service" "$SERVICE_DST"
fi
install_if_absent "$HERE/policies/policy.json" "$POLICY_DST"
install_release_config_and_content
install_sensor_bundle

if [[ "$ENABLE_SERVICE" == "1" ]]; then
  systemctl daemon-reload
  systemctl enable --now sysarmor-agent
  wait_for_agent
fi

echo "[sysarmor-install] standalone Agent 安装完成"
if [[ "$PROFILE" == "linux-container" ]]; then
  echo "[sysarmor-install] 容器入口: $CONTAINER_ENTRYPOINT_DST"
else
  echo "[sysarmor-install] 健康检查: sudo sysarmorctl agent health"
fi
