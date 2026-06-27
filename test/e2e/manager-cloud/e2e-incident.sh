#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"

bash "$ROOT/e2e/manager-cloud/e2e-graph-evidence.sh"
bash "$ROOT/e2e/manager-cloud/e2e-incident-lifecycle.sh"
bash "$ROOT/e2e/manager-cloud/e2e-incident-attach-evidence.sh"
bash "$ROOT/e2e/manager-cloud/e2e-incident-merge.sh"
