#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$HERE/../../../.." && pwd)"

cd "$REPO"
go test ./internal/gateway -run 'Test(DataPlaneStreamBatches|ControlPlaneConnectSequenceRejectsReplayAndGap|ControlPlaneConnectReconnectReturnsResumeAndPendingCommands|ControlPlaneConnectRequiresRequestID|DataAckClassifiesRetryableBackendError|DataPlaneStreamBatchesRequiresAgentIdentity)'
go test ./internal/agent/daemon -run 'Test(ControlChannelKeepsLongLivedContract|AgentRuntimeControlChannelProcessesPendingResponse)'
