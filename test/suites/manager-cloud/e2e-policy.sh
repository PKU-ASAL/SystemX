#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"

bash "$ROOT/suites/manager-cloud/e2e-policy-endpoint-disable.sh"
bash "$ROOT/suites/manager-cloud/e2e-policy-agent-refresh.sh"
bash "$ROOT/suites/manager-cloud/e2e-policy-cloud-disable.sh"
bash "$ROOT/suites/manager-cloud/e2e-policy-publish.sh"
