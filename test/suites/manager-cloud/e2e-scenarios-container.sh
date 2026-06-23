#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"

bash "$ROOT/harness/e2e-agent-apt-container.sh"
bash "$ROOT/harness/e2e-agent-staged-container.sh"
bash "$ROOT/harness/e2e-agent-benign-container.sh"
