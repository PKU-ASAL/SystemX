#!/usr/bin/env bash
set -euo pipefail

PRIVATE_KEY="${1:?usage: issue-manager-jwt.sh PRIVATE_KEY ISSUER AUDIENCE}"
ISSUER="${2:?issuer required}"
AUDIENCE="${3:?audience required}"
SUBJECT="${SYSARMOR_JWT_SUBJECT:-test-admin}"
TENANT="${SYSARMOR_JWT_TENANT:-default}"
TTL="${SYSARMOR_JWT_TTL_SECONDS:-300}"

base64url() {
  base64 -w0 | tr '+/' '-_' | tr -d '='
}

now="$(date +%s)"
expires=$((now + TTL))
header="$(printf '%s' '{"alg":"RS256","typ":"JWT"}' | base64url)"
payload="$(jq -cn \
  --arg sub "$SUBJECT" --arg tenant "$TENANT" --arg iss "$ISSUER" --arg aud "$AUDIENCE" \
  --argjson iat "$now" --argjson exp "$expires" \
  '{sub:$sub,tenant_id:$tenant,roles:["admin"],iss:$iss,aud:$aud,iat:$iat,exp:$exp}' | base64url)"
signed="$header.$payload"
signature="$(printf '%s' "$signed" | openssl dgst -sha256 -sign "$PRIVATE_KEY" | base64url)"
printf '%s.%s\n' "$signed" "$signature"
