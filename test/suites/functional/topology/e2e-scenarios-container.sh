#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../../.." && pwd)"

bash "$ROOT/shared/harness/start-container.sh"
SYSARMOR_SKIP_START_CONTAINER=1 bash "$HERE/scenario-container.sh" apt
SYSARMOR_SKIP_START_CONTAINER=1 bash "$HERE/scenario-container.sh" staged
SYSARMOR_SKIP_START_CONTAINER=1 bash "$HERE/scenario-container.sh" benign
