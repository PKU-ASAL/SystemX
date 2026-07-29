#!/usr/bin/env bash
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
build="$REPO/.github/workflows/release-build.yml"
candidate="$REPO/.github/workflows/release-candidate.yml"
stable="$REPO/.github/workflows/release-stable.yml"
dependabot="$REPO/.github/dependabot.yml"

require_file() {
  if [[ ! -f "$1" ]]; then
    echo "[release-workflow-contract][ERROR] missing file: $1" >&2
    exit 1
  fi
}

require_file "$build"
require_file "$candidate"
require_file "$stable"
test ! -e "$REPO/.github/workflows/dev-prerelease.yml"

grep -Fq 'workflow_call:' "$build"
grep -Fq 'release_type:' "$build"
grep -Fq 'content_key_id:' "$build"
grep -Fq 'release_environment:' "$build"
grep -Fq 'SYSARMOR_ARTIFACT_SIGNING_KEY_PEM' "$build"
grep -Fq 'SYSARMOR_CONTENT_SIGNING_KEY_PEM' "$build"
grep -Fq 'SYSARMOR_CONTENT_KEY_ID' "$build"
grep -Fq 'contents: read' "$build"
grep -Fq 'id-token: write' "$build"
grep -Fq 'attestations: write' "$build"
grep -Fq 'go test ./...' "$build"
grep -Fq 'make test-distribution SOURCE=local' "$build"
grep -Fq 'container-entrypoint.sh' "$build"
grep -Fq -- '--tetragon-mode download' "$build"
grep -Fq -- '--signing-key "$manifest_key"' "$build"
grep -Fq -- '--content-signing-key "$content_key"' "$build"
grep -Fq -- '--content-key-id "$CONTENT_KEY_ID"' "$build"
grep -Fq 'openssl genrsa' "$build"
grep -Fq 'openssl genpkey -algorithm ED25519' "$build"
grep -Fq 'sha256sum -c SHA256SUMS' "$build"
grep -Fq 'actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1' "$build"
grep -Fq 'actions/setup-go@b7ad1dad31e06c5925f5d2fc7ad053ef454303e # v7.0.0' "$build"
grep -Fq 'actions/attest-build-provenance@0f67c3f4856b2e3261c31976d6725780e5e4c373 # v4.1.1' "$build"
grep -Fq 'actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a # v7.0.1' "$build"

grep -Fq 'workflow_dispatch:' "$candidate"
grep -Fq 'rc_number:' "$candidate"
grep -Fq 'release/v' "$candidate"
grep -Fq '^[1-9][0-9]*$' "$candidate"
grep -Fq 'uses: ./.github/workflows/release-build.yml' "$candidate"
grep -Fq 'release_type: rc' "$candidate"
grep -Fq 'contents: write' "$candidate"
grep -Fq -- '--target "$SOURCE_SHA"' "$candidate"
grep -Fq -- '--prerelease' "$candidate"
grep -Fq 'actions/download-artifact@3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c # v8.0.1' "$candidate"

grep -Fq 'workflow_dispatch:' "$stable"
grep -Fq 'accepted_rc_tag:' "$stable"
grep -Fq '"$GITHUB_REF" != '\''refs/heads/main'\''' "$stable"
grep -Fq 'production-release' "$stable"
grep -Fq '[[ "$RC_TAG" =~ ^v([0-9]+\.[0-9]+\.[0-9]+)-rc\.[1-9][0-9]*$ ]]' "$stable"
grep -Fq '[[ "${BASH_REMATCH[1]}" == "$VERSION_INPUT" ]]' "$stable"
grep -Fq 'git rev-parse "HEAD^{tree}"' "$stable"
grep -Fq 'git rev-parse "$RC_TAG^{tree}"' "$stable"
grep -Fq 'uses: ./.github/workflows/release-build.yml' "$stable"
grep -Fq 'release_type: ga' "$stable"
grep -Fq 'contents: write' "$stable"
grep -Fq -- '--target "$SOURCE_SHA"' "$stable"
if grep -Fq -- '--prerelease' "$stable"; then
  echo "[release-workflow-contract][ERROR] stable release must not be a prerelease" >&2
  exit 1
fi

test -f "$dependabot"
grep -Fq 'package-ecosystem: github-actions' "$dependabot"
grep -Fq 'target-branch: dev' "$dependabot"
grep -Fq 'interval: weekly' "$dependabot"

echo "[release-workflow-contract] ok"
