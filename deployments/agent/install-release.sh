#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROFILE="${SYSARMOR_INSTALL_PROFILE:-linux-systemd}"

usage() {
  echo "usage: install.sh [--profile linux-systemd|linux-container]"
}

fail() {
  echo "[sysarmor-install][ERROR] $*" >&2
  exit 1
}

validate_release_manifest() {
  local manifest="$HERE/manifest.json" rel expected actual
  declare -A seen=()
  [[ -f "$manifest" && -f "$HERE/manifest.sig" ]] || fail "发行包缺少 manifest.json 或 manifest.sig"
  command -v jq >/dev/null 2>&1 || fail "校验发行包需要 jq"
  command -v sha256sum >/dev/null 2>&1 || fail "校验发行包需要 sha256sum"
  jq -e '.schema_version == "sysarmor.agent.distribution/v1" and (.files | length > 0)' \
    "$manifest" >/dev/null || fail "发行包 manifest 格式无效"
  while IFS=$'\t' read -r rel expected; do
    [[ -n "$rel" && "$rel" != /* && "/$rel/" != *"/../"* && "$rel" != *$'\t'* ]] || \
      fail "发行包 manifest 路径无效: $rel"
    [[ "$expected" =~ ^[0-9a-fA-F]{64}$ ]] || fail "发行包 SHA256 无效: $rel"
    [[ -z "${seen[$rel]:-}" ]] || fail "发行包 manifest 路径重复: $rel"
    seen[$rel]=1
    [[ -f "$HERE/$rel" ]] || fail "发行包缺少文件: $rel"
    actual="$(sha256sum "$HERE/$rel" | awk '{print $1}')"
    [[ "${actual,,}" == "${expected,,}" ]] || fail "发行包 SHA256 不匹配: $rel"
  done < <(jq -r '.files[] | [.path, .sha256] | @tsv' "$manifest")
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --profile)
      [[ $# -ge 2 ]] || { echo "[sysarmor-install][ERROR] --profile 缺少参数" >&2; exit 1; }
      PROFILE="$2"
      shift 2
      ;;
    -h|--help) usage; exit 0 ;;
    *) echo "[sysarmor-install][ERROR] 不支持的参数: $1" >&2; exit 1 ;;
  esac
done

case "$PROFILE" in
  linux-systemd) default_config="$HERE/configs/standalone.yaml" ;;
  linux-container) default_config="$HERE/configs/standalone-container.yaml" ;;
  *) echo "[sysarmor-install][ERROR] 不支持的安装 profile: $PROFILE；可选值为 linux-systemd、linux-container" >&2; exit 1 ;;
esac
config="${SYSARMOR_RELEASE_CONFIG:-$default_config}"

validate_release_manifest

SYSARMOR_INSTALL_PROFILE="$PROFILE" \
SYSARMOR_INSTALL_AGENT_SOURCE="$HERE/bin/sysarmor-agent" \
SYSARMOR_INSTALL_CTL_SOURCE="$HERE/bin/sysarmorctl" \
SYSARMOR_INSTALL_SERVICE_SOURCE="$HERE/systemd/sysarmor-agent.service" \
SYSARMOR_INSTALL_CONFIG_SOURCE="$config" \
SYSARMOR_INSTALL_POLICY_SOURCE="$HERE/policies/policy.json" \
SYSARMOR_INSTALL_CONTENT_SOURCE="$HERE/content/default" \
SYSARMOR_INSTALL_SENSOR_SOURCE="$HERE/sensors/tetragon" \
SYSARMOR_INSTALL_CONTAINER_ENTRYPOINT_SOURCE="$HERE/container/sysarmor-container-entrypoint" \
  exec "$HERE/install-core.sh"
