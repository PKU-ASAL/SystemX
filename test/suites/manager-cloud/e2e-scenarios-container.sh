#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"

bash "$HERE/e2e-scenario-apt-container.sh"
bash "$HERE/e2e-scenario-staged-container.sh"
bash "$HERE/e2e-scenario-benign-container.sh"
