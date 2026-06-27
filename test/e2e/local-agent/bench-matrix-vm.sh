#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"

EVALUATION_SCOPE="${EVALUATION_SCOPE:-local}" exec bash "$ROOT/benchmarks/matrix/matrix-vm.sh" "$@"
