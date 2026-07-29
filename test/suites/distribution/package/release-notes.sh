#!/usr/bin/env bash
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
renderer="$REPO/deployments/packages/render-github-release-notes.sh"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

require_text() {
  local file="$1"
  local text="$2"
  grep -Fq -- "$text" "$file" || {
    echo "[release-notes][ERROR] missing text: $text" >&2
    exit 1
  }
}

expect_invalid() {
  local name="$1"
  shift
  set +e
  "$renderer" "$@" >"$work/$name.out" 2>"$work/$name.err"
  local status=$?
  set -e
  [[ "$status" -eq 2 ]] || {
    echo "[release-notes][ERROR] $name exited $status, expected 2" >&2
    exit 1
  }
}

GITHUB_SHA=0123456789abcdef "$renderer" v0.1.0-rc.2 PKU-ASAL/sysarmor rc >"$work/rc.md"
require_text "$work/rc.md" 'SysArmor `v0.1.0-rc.2` release candidate from commit `0123456789abcdef`.'
require_text "$work/rc.md" 'releases/download/v0.1.0-rc.2/install.sh'
require_text "$work/rc.md" '--profile linux-container'
require_text "$work/rc.md" '--privileged --cgroupns=host --restart unless-stopped'
require_text "$work/rc.md" 'gh attestation verify sysarmor-agent-linux-amd64-v0.1.0-rc.2.tar.gz'
require_text "$work/rc.md" 'https://github.com/PKU-ASAL/sysarmor/commits/v0.1.0-rc.2'

GITHUB_SHA=fedcba9876543210 "$renderer" v0.1.0 PKU-ASAL/sysarmor ga >"$work/ga.md"
require_text "$work/ga.md" 'SysArmor `v0.1.0` release from commit `fedcba9876543210`.'
require_text "$work/ga.md" 'releases/download/v0.1.0/install.sh'
require_text "$work/ga.md" 'https://github.com/PKU-ASAL/sysarmor/commits/v0.1.0'

expect_invalid bad-version v0.1 PKU-ASAL/sysarmor ga
expect_invalid bad-repository v0.1.0 'owner only' ga
expect_invalid bad-type v0.1.0 PKU-ASAL/sysarmor beta
expect_invalid rc-as-ga v0.1.0-rc.2 PKU-ASAL/sysarmor ga
expect_invalid ga-as-rc v0.1.0 PKU-ASAL/sysarmor rc
expect_invalid missing-argument v0.1.0 PKU-ASAL/sysarmor

echo "[release-notes] ok"
