#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../../.." && pwd)"

SYSARMOR_SKIP_START_CONTAINER=1 bash "$HERE/e2e-scenario-apt-container.sh"
SYSARMOR_SKIP_START_CONTAINER=1 bash "$HERE/e2e-scenario-staged-container.sh"
SYSARMOR_SKIP_START_CONTAINER=1 bash "$HERE/e2e-scenario-benign-container.sh"
