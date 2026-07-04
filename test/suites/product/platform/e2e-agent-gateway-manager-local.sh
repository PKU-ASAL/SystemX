#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPO="$(cd "$ROOT/.." && pwd)"

echo "[e2e-agent-gateway-manager-local] running in-process gateway/manager data path"
cd "$REPO"
go test ./test/e2e/platform -run '^TestAgentGatewayManagerLocalDataPath$' -count=1
echo "[e2e-agent-gateway-manager-local] ok"
