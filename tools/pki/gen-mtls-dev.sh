#!/usr/bin/env bash
set -euo pipefail

OUT_DIR="${1:-test/.results/pki}"
TENANT_ID="${2:-default}"
AGENT_ID="${3:-agent-mtls}"
MANAGER_DNS="${4:-localhost}"
TRUST_DOMAIN="${SYSARMOR_TRUST_DOMAIN:-sysarmor.local}"
DAYS="${SYSARMOR_CERT_DAYS:-365}"

mkdir -p "$OUT_DIR"
chmod 700 "$OUT_DIR"

CA_KEY="$OUT_DIR/ca-key.pem"
CA_CERT="$OUT_DIR/ca.pem"
SERVER_KEY="$OUT_DIR/manager-key.pem"
SERVER_CSR="$OUT_DIR/manager.csr"
SERVER_CERT="$OUT_DIR/manager.pem"
CLIENT_KEY="$OUT_DIR/agent-key.pem"
CLIENT_CSR="$OUT_DIR/agent.csr"
CLIENT_CERT="$OUT_DIR/agent.pem"
SERVER_EXT="$OUT_DIR/manager.ext"
CLIENT_EXT="$OUT_DIR/agent.ext"

openssl genrsa -out "$CA_KEY" 4096 >/dev/null 2>&1
openssl req -x509 -new -nodes -key "$CA_KEY" -sha256 -days "$DAYS" \
  -subj "/CN=SysArmor Dev CA" \
  -out "$CA_CERT" >/dev/null 2>&1

openssl genrsa -out "$SERVER_KEY" 2048 >/dev/null 2>&1
openssl req -new -key "$SERVER_KEY" \
  -subj "/CN=$MANAGER_DNS" \
  -out "$SERVER_CSR" >/dev/null 2>&1
cat > "$SERVER_EXT" <<EOF
basicConstraints=CA:FALSE
keyUsage=digitalSignature,keyEncipherment
extendedKeyUsage=serverAuth
subjectAltName=DNS:$MANAGER_DNS,DNS:localhost,IP:127.0.0.1
EOF
openssl x509 -req -in "$SERVER_CSR" -CA "$CA_CERT" -CAkey "$CA_KEY" -CAcreateserial \
  -out "$SERVER_CERT" -days "$DAYS" -sha256 -extfile "$SERVER_EXT" >/dev/null 2>&1

openssl genrsa -out "$CLIENT_KEY" 2048 >/dev/null 2>&1
openssl req -new -key "$CLIENT_KEY" \
  -subj "/CN=tenant_id:$TENANT_ID,agent_id:$AGENT_ID" \
  -out "$CLIENT_CSR" >/dev/null 2>&1
cat > "$CLIENT_EXT" <<EOF
basicConstraints=CA:FALSE
keyUsage=digitalSignature,keyEncipherment
extendedKeyUsage=clientAuth
subjectAltName=URI:spiffe://$TRUST_DOMAIN/tenant/$TENANT_ID/agent/$AGENT_ID
EOF
openssl x509 -req -in "$CLIENT_CSR" -CA "$CA_CERT" -CAkey "$CA_KEY" -CAcreateserial \
  -out "$CLIENT_CERT" -days "$DAYS" -sha256 -extfile "$CLIENT_EXT" >/dev/null 2>&1

chmod 600 "$CA_KEY" "$SERVER_KEY" "$CLIENT_KEY"
chmod 644 "$CA_CERT" "$SERVER_CERT" "$CLIENT_CERT"
rm -f "$SERVER_CSR" "$CLIENT_CSR" "$SERVER_EXT" "$CLIENT_EXT"

cat > "$OUT_DIR/README.txt" <<EOF
SysArmor development mTLS material

Identity:
  tenant_id: $TENANT_ID
  agent_id:  $AGENT_ID
  uri_san:   spiffe://$TRUST_DOMAIN/tenant/$TENANT_ID/agent/$AGENT_ID

Files:
  ca.pem
  manager.pem
  manager-key.pem
  agent.pem
  agent-key.pem
EOF

cat <<EOF
$OUT_DIR
EOF
