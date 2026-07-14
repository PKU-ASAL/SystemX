#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-jwt-test.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT
openssl genrsa -out "$TMP/private.pem" 2048 >/dev/null 2>&1
openssl rsa -in "$TMP/private.pem" -pubout -out "$TMP/public.pem" >/dev/null 2>&1

token="$($ROOT/tools/auth/issue-manager-jwt.sh "$TMP/private.pem" test-issuer test-audience)"
IFS=. read -r header payload signature <<<"$token"
[[ -n "$header" && -n "$payload" && -n "$signature" ]]

decode() {
  local value="$1" padding=$(( (4 - ${#1} % 4) % 4 ))
  value="${value//-/+}"
  value="${value//_/\/}"
  printf '%s' "$value$(printf '=%.0s' $(seq 1 "$padding"))" | base64 -d
}

decode "$payload" >"$TMP/payload.json"
jq -e '.sub == "test-admin" and .tenant_id == "default" and .roles == ["admin"] and .iss == "test-issuer" and .aud == "test-audience" and .exp > .iat' "$TMP/payload.json" >/dev/null
decode "$signature" >"$TMP/signature.bin"
printf '%s.%s' "$header" "$payload" >"$TMP/signed"
openssl dgst -sha256 -verify "$TMP/public.pem" -signature "$TMP/signature.bin" "$TMP/signed" >/dev/null

echo "manager JWT issuer test passed"
