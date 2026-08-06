#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$HERE/../../../.." && pwd)"

cd "$REPO"
go test ./apps/manager/internal/gateway -run 'Test(DataPlaneStreamBatches|ControlPlaneConnectSequenceRejectsReplayAndGap|ControlPlaneConnectReconnectReturnsResumeAndPendingCommands|ControlPlaneConnectRequiresRequestID|DataAckClassifiesRetryableBackendError|DataPlaneStreamBatchesRequiresAgentIdentity)'
go test ./apps/agent/internal/daemon -run 'Test(ControlChannelKeepsLongLivedContract|AgentRuntimeControlChannelProcessesPendingResponse)'
