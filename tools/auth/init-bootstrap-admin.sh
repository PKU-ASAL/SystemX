#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUTPUT_DIR="${1:-$ROOT/deployments/pki/agent-plane-mtls/runtime}"
USERNAME_FILE="$OUTPUT_DIR/bootstrap-admin-username"
PASSWORD_FILE="$OUTPUT_DIR/bootstrap-admin-password"
AUTH_SECRET_FILE="$OUTPUT_DIR/auth-secret"

mkdir -p "$OUTPUT_DIR"
existing=0
for file in "$USERNAME_FILE" "$PASSWORD_FILE" "$AUTH_SECRET_FILE"; do
  [[ -e "$file" ]] && existing=$((existing + 1))
done
if ((existing > 0 && existing < 3)); then
  echo "bootstrap auth secret set is incomplete: $OUTPUT_DIR" >&2
  exit 1
fi

if ((existing == 0)); then
  password="$(openssl rand -base64 24 | tr -d '\n')"
  umask 077
  printf '%s\n' admin >"$USERNAME_FILE"
  printf '%s\n' "$password" >"$PASSWORD_FILE"
  openssl rand -base64 48 >"$AUTH_SECRET_FILE"
  chmod 600 "$USERNAME_FILE" "$PASSWORD_FILE" "$AUTH_SECRET_FILE"
  printf 'bootstrap admin password: %s\n' "$password"
fi

bash "$ROOT/tools/pki/gen-manager-jwt.sh" "$OUTPUT_DIR"
echo "bootstrap admin authentication ready: $OUTPUT_DIR"
