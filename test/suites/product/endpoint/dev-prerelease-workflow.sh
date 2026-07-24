#!/usr/bin/env bash
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
workflow="$REPO/.github/workflows/dev-prerelease.yml"

test -f "$workflow"
grep -Fq 'workflow_dispatch:' "$workflow"
grep -Fq 'github.ref != '\''refs/heads/dev'\''' "$workflow"
grep -Fq 'go test ./...' "$workflow"
grep -Fq -- '--tetragon-mode download' "$workflow"
grep -Fq 'build-github-assets.sh' "$workflow"
grep -Fq 'actions/attest-build-provenance@' "$workflow"
grep -Fq 'actions/upload-artifact@' "$workflow"
grep -Fq 'actions/download-artifact@' "$workflow"
grep -Fq 'contents: read' "$workflow"
grep -Fq 'contents: write' "$workflow"
grep -Fq -- '--prerelease' "$workflow"
grep -Fq -- '--target "$GITHUB_SHA"' "$workflow"

echo "[dev-prerelease-workflow] ok"
