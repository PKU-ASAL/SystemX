#!/usr/bin/env bash

IMAGES="${IMAGES:-ubuntu2204 ubuntu2404 debian12}"
RUN_ID="${RUN_ID:-$(date -u +%Y%m%dT%H%M%SZ)}"
FRESH_DOWNLOAD="${FRESH_DOWNLOAD:-1}"
RELEASE_PROXY_URL="${RELEASE_PROXY_URL-https://gh-proxy.org}"

HEALTH_TIMEOUT="${HEALTH_TIMEOUT:-90}"
DETECTION_TIMEOUT="${DETECTION_TIMEOUT:-60}"
ISOLATION_TIMEOUT="${ISOLATION_TIMEOUT:-3}"
SERVICE_TIMEOUT="${SERVICE_TIMEOUT:-30}"
DOWNLOAD_CONNECT_TIMEOUT="${DOWNLOAD_CONNECT_TIMEOUT:-10}"
DOWNLOAD_TIMEOUT="${DOWNLOAD_TIMEOUT:-60}"

resolve_install_url() {
  local configured="${URL:-${SYSARMOR_INSTALL_URL:-}}"
  if [[ -n "$configured" ]]; then
    printf '%s\n' "$configured"
    return
  fi

  local install_url
  install_url="$(curl -fsSL --connect-timeout "$DOWNLOAD_CONNECT_TIMEOUT" --max-time "$DOWNLOAD_TIMEOUT" \
    'https://api.github.com/repos/PKU-ASAL/sysarmor/releases?per_page=20' |
    jq -er '[.[] | select(.prerelease) | .assets[] | select(.name == "install.sh")][0].browser_download_url')" || {
    echo "[release][ERROR] GitHub 上未找到可用的 pre-release install.sh；请设置 URL=<install.sh URL>" >&2
    return 1
  }
  printf '%s\n' "$install_url"
}

probe_download_url() {
  curl -fsSL --connect-timeout "$DOWNLOAD_CONNECT_TIMEOUT" --max-time "$DOWNLOAD_TIMEOUT" \
    "$1" -o /dev/null 2>/dev/null
}

resolve_download_url() {
  local source_url="$1" proxied_url
  if probe_download_url "$source_url"; then
    printf '%s\n' "$source_url"
    return
  fi
  if [[ -z "$RELEASE_PROXY_URL" || "$source_url" != https://github.com/* ]]; then
    echo "[release][ERROR] 无法下载公开安装器: $source_url" >&2
    return 1
  fi
  proxied_url="${RELEASE_PROXY_URL%/}/$source_url"
  if probe_download_url "$proxied_url"; then
    echo "[release] GitHub 直连失败，使用代理: $RELEASE_PROXY_URL" >&2
    printf '%s\n' "$proxied_url"
    return
  fi
  echo "[release][ERROR] GitHub 直连和代理均无法下载安装器" >&2
  echo "  直连: $source_url" >&2
  echo "  代理: $proxied_url" >&2
  return 1
}
