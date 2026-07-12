#!/usr/bin/env bash
set -euo pipefail

OUTPUT_DIR="${1:-deployments/pki/agent-plane-mtls/runtime}"
PRIVATE_KEY="$OUTPUT_DIR/manager-jwt-private.pem"
PUBLIC_KEY="$OUTPUT_DIR/manager-jwt-public.pem"

mkdir -p "$OUTPUT_DIR"
if [[ ! -f "$PRIVATE_KEY" ]]; then
  openssl genrsa -out "$PRIVATE_KEY" 3072 >/dev/null 2>&1
  chmod 600 "$PRIVATE_KEY"
fi
if [[ ! -f "$PUBLIC_KEY" ]]; then
  openssl rsa -in "$PRIVATE_KEY" -pubout -out "$PUBLIC_KEY" >/dev/null 2>&1
  chmod 644 "$PUBLIC_KEY"
fi

printf 'manager JWT keys ready: %s\n' "$OUTPUT_DIR"
