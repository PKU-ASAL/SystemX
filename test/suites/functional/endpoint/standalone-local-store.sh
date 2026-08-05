#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)
cd "$ROOT"

go test ./apps/agent/internal/localstore -run 'Test(Open|DeviceIdentity|Segment|Recover|Capacity|Checkpoint|Enrollment)' -count=1
go test ./apps/agent/internal/daemon -run 'Test(AgentRuntimeSwitchesBatchIdentity|NetworkSupervisor|ExportPipeline)' -count=1
