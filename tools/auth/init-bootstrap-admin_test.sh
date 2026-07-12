#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUTPUT="$(mktemp -d)"
trap 'rm -rf "$OUTPUT"' EXIT

first="$($ROOT/tools/auth/init-bootstrap-admin.sh "$OUTPUT")"
files=(bootstrap-admin-username bootstrap-admin-password auth-secret manager-jwt-private.pem manager-jwt-public.pem)

for file in "${files[@]}"; do
  [[ -s "$OUTPUT/$file" ]] || { echo "missing $file" >&2; exit 1; }
done
for file in bootstrap-admin-username bootstrap-admin-password auth-secret manager-jwt-private.pem; do
  [[ "$(stat -c %a "$OUTPUT/$file")" == "600" ]] || { echo "unsafe mode $file" >&2; exit 1; }
done
[[ "$first" == *"bootstrap admin password:"* ]] || { echo "first run did not print password" >&2; exit 1; }

before="$(sha256sum "$OUTPUT"/*)"
second="$($ROOT/tools/auth/init-bootstrap-admin.sh "$OUTPUT")"
after="$(sha256sum "$OUTPUT"/*)"
[[ "$before" == "$after" ]] || { echo "second run changed secrets" >&2; exit 1; }
[[ "$second" != *"bootstrap admin password:"* ]] || { echo "second run leaked password" >&2; exit 1; }

echo "bootstrap auth init test: PASS"
