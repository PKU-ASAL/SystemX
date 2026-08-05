#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
REPO="$(cd "$ROOT/.." && pwd)"

echo "[e2e-control-roundtrip-local] running gateway and agent control roundtrip contracts"
cd "$REPO"
go test ./apps/manager/internal/gateway -run 'TestControlPlaneConnect(SendsPendingControlCommandAndPersistsAck|ReconnectReturnsResumeAndPendingCommands|AcceptsAgentAck)$' -count=1
go test ./apps/agent/internal/daemon -run 'TestAgentRuntimeControlChannel(ProcessesPendingResponse|AppliesContentUpdate)$' -count=1
echo "[e2e-control-roundtrip-local] ok"
