#!/usr/bin/env bash

resolve_install_url() {
  if [[ -n "${SYSARMOR_INSTALL_URL:-}" ]]; then
    printf '%s\n' "$SYSARMOR_INSTALL_URL"
    return
  fi

  local response
  response="$(mktemp)"
  if ! curl -fsSL "https://api.github.com/repos/PKU-ASAL/sysarmor/releases?per_page=20" -o "$response"; then
    rm -f "$response"
    echo "[release][ERROR] 无法查询 GitHub pre-release；请设置 SYSARMOR_INSTALL_URL=<install.sh URL>" >&2
    return 1
  fi
  python3 - "$response" <<'PY'
import json
import sys

for release in json.load(open(sys.argv[1])):
    if not release.get("prerelease"):
        continue
    for asset in release.get("assets", []):
        if asset.get("name") == "install.sh":
            print(asset["browser_download_url"])
            raise SystemExit(0)
raise SystemExit("GitHub 上未找到包含 install.sh 的 pre-release")
PY
  local status=$?
  rm -f "$response"
  return "$status"
}
