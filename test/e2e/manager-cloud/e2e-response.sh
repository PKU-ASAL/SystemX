#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"

bash "$ROOT/e2e/manager-cloud/e2e-response-observe-only.sh"
bash "$ROOT/e2e/manager-cloud/e2e-response-policy-deny.sh"
bash "$ROOT/e2e/manager-cloud/e2e-response-scope-deny.sh"
bash "$ROOT/e2e/manager-cloud/e2e-response-audit.sh"
bash "$ROOT/e2e/manager-cloud/e2e-response-approval.sh"
