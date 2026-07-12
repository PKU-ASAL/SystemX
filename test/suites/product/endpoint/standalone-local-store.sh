#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)
cd "$ROOT"

go test ./internal/agent/localstore -run 'Test(Open|DeviceIdentity|Segment|Recover|Capacity|Checkpoint|Enrollment)' -count=1
go test ./internal/agent/daemon -run 'Test(AgentRuntimeSwitchesBatchIdentity|NetworkSupervisor|SpoolUploader)' -count=1
