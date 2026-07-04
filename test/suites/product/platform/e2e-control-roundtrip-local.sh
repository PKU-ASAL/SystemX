#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPO="$(cd "$ROOT/.." && pwd)"

echo "[e2e-control-roundtrip-local] running gateway and agent control roundtrip contracts"
cd "$REPO"
go test ./internal/gateway -run 'TestControlPlaneConnect(SendsPendingControlCommandAndPersistsAck|ReconnectReturnsResumeAndPendingCommands|AcceptsAgentAck)$' -count=1
go test ./internal/agent/daemon -run 'TestAgentRuntimeControlChannel(ProcessesPendingResponse|AppliesContentUpdate)$' -count=1
echo "[e2e-control-roundtrip-local] ok"
