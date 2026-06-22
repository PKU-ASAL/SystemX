#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RESULTS="$ROOT/test/.results"
BIN="$ROOT/bin"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-agent-mtls.XXXXXX")"
PORT_BASE="${PORT_BASE:-$((33000 + RANDOM % 5000))}"
MANAGER_PORT="${MANAGER_PORT:-$PORT_BASE}"
GRPC_PORT="${GRPC_PORT:-$((PORT_BASE + 1))}"
MGR_URL="http://127.0.0.1:$MANAGER_PORT"
TENANT_ID="${TENANT_ID:-default}"
AGENT_ID="${AGENT_ID:-agent-mtls}"
HOST_ID="${HOST_ID:-host-mtls}"

mkdir -p "$RESULTS" "$BIN"

cleanup() {
  if [[ -n "${MGR_PID:-}" ]]; then kill "$MGR_PID" 2>/dev/null || true; fi
  rm -rf "$TMP"
}
trap cleanup EXIT

echo "[e2e-agent-mtls] building required binaries"
GOCACHE="${GOCACHE:-/tmp/sysarmor-go-cache}" CGO_ENABLED=0 go build -o "$BIN/sysarmor-manager" "$ROOT/cmd/sysarmor-manager"
GOCACHE="${GOCACHE:-/tmp/sysarmor-go-cache}" CGO_ENABLED=0 go build -o "$BIN/sysarmor-databatch-append" "$ROOT/cmd/sysarmor-databatch-append"

PKI_DIR="$TMP/pki"
"$ROOT/tools/pki/gen-mtls-dev.sh" "$PKI_DIR" "$TENANT_ID" "$AGENT_ID" localhost >/dev/null
UNTRUSTED_PKI_DIR="$TMP/untrusted-pki"
"$ROOT/tools/pki/gen-mtls-dev.sh" "$UNTRUSTED_PKI_DIR" "$TENANT_ID" "$AGENT_ID" localhost >/dev/null

"$BIN/sysarmor-manager" \
  --listen "127.0.0.1:$MANAGER_PORT" \
  --grpc-listen "127.0.0.1:$GRPC_PORT" \
  --store-backend memory \
  --grpc-tls-cert "$PKI_DIR/manager.pem" \
  --grpc-tls-key "$PKI_DIR/manager-key.pem" \
  --grpc-client-ca "$PKI_DIR/ca.pem" \
  --grpc-require-client-cert \
  >"$TMP/manager.log" 2>&1 &
MGR_PID=$!

wait_contains() {
  local name="$1"
  local needle="$2"
  local out="$3"
  shift 3
  local deadline=$((SECONDS + 10))
  until "$@" >"$out" && grep -Fq "$needle" "$out"; do
    if (( SECONDS >= deadline )); then
      echo "[e2e-agent-mtls][ERROR] timeout waiting for $needle via $name" >&2
      cat "$out" >&2 2>/dev/null || true
      cat "$TMP/manager.log" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 0.1
  done
}

wait_contains "manager healthz" '"ok":true' "$TMP/healthz.json" curl -sf "$MGR_URL/healthz"

cat > "$TMP/batch-good.json" <<JSON
{
  "header": {
    "batchId": "mtls-good-batch",
    "agentId": "$AGENT_ID",
    "hostId": "$HOST_ID",
    "tenantId": "$TENANT_ID",
    "eventCount": 1,
    "labels": {"agent_version": "e2e"}
  },
  "events": [
    {
      "sequence": 1,
      "event": {"id": "ev-mtls-good", "agentId": "$AGENT_ID", "hostId": "$HOST_ID", "tenantId": "$TENANT_ID", "scenario": "mtls", "behavior": "process.exec"}
    }
  ]
}
JSON

"$BIN/sysarmor-databatch-append" \
  --manager "127.0.0.1:$GRPC_PORT" \
  --input "$TMP/batch-good.json" \
  --tls-ca "$PKI_DIR/ca.pem" \
  --tls-cert "$PKI_DIR/agent.pem" \
  --tls-key "$PKI_DIR/agent-key.pem" \
  --tls-server-name localhost \
  > "$RESULTS/e2e-agent-mtls.good-ack.json"

cat > "$TMP/batch-wrong-agent.json" <<JSON
{
  "header": {
    "batchId": "mtls-wrong-agent-batch",
    "agentId": "agent-forged",
    "hostId": "$HOST_ID",
    "tenantId": "$TENANT_ID",
    "eventCount": 1,
    "labels": {"agent_version": "e2e"}
  },
  "events": [
    {
      "sequence": 1,
      "event": {"id": "ev-mtls-forged", "agentId": "agent-forged", "hostId": "$HOST_ID", "tenantId": "$TENANT_ID", "scenario": "mtls", "behavior": "process.exec"}
    }
  ]
}
JSON

set +e
"$BIN/sysarmor-databatch-append" \
  --manager "127.0.0.1:$GRPC_PORT" \
  --input "$TMP/batch-wrong-agent.json" \
  --tls-ca "$PKI_DIR/ca.pem" \
  --tls-cert "$PKI_DIR/agent.pem" \
  --tls-key "$PKI_DIR/agent-key.pem" \
  --tls-server-name localhost \
  > "$RESULTS/e2e-agent-mtls.wrong-agent.out" \
  2> "$RESULTS/e2e-agent-mtls.wrong-agent.err"
wrong_status=$?

"$BIN/sysarmor-databatch-append" \
  --manager "127.0.0.1:$GRPC_PORT" \
  --input "$TMP/batch-good.json" \
  --tls-ca "$PKI_DIR/ca.pem" \
  --tls-server-name localhost \
  --timeout 2s \
  > "$RESULTS/e2e-agent-mtls.no-client-cert.out" \
  2> "$RESULTS/e2e-agent-mtls.no-client-cert.err"
no_cert_status=$?

"$BIN/sysarmor-databatch-append" \
  --manager "127.0.0.1:$GRPC_PORT" \
  --input "$TMP/batch-good.json" \
  --tls-ca "$PKI_DIR/ca.pem" \
  --tls-cert "$UNTRUSTED_PKI_DIR/agent.pem" \
  --tls-key "$UNTRUSTED_PKI_DIR/agent-key.pem" \
  --tls-server-name localhost \
  --timeout 2s \
  > "$RESULTS/e2e-agent-mtls.untrusted-ca.out" \
  2> "$RESULTS/e2e-agent-mtls.untrusted-ca.err"
untrusted_ca_status=$?
set -e

if [[ "$wrong_status" -eq 0 ]]; then
  echo "[e2e-agent-mtls][ERROR] forged agent_id upload unexpectedly succeeded" >&2
  exit 1
fi
if ! grep -Fq "PermissionDenied" "$RESULTS/e2e-agent-mtls.wrong-agent.err"; then
  echo "[e2e-agent-mtls][ERROR] forged agent_id did not fail with PermissionDenied" >&2
  cat "$RESULTS/e2e-agent-mtls.wrong-agent.err" >&2
  exit 1
fi
if [[ "$no_cert_status" -eq 0 ]]; then
  echo "[e2e-agent-mtls][ERROR] upload without client certificate unexpectedly succeeded" >&2
  exit 1
fi
if [[ "$untrusted_ca_status" -eq 0 ]]; then
  echo "[e2e-agent-mtls][ERROR] upload with untrusted client CA unexpectedly succeeded" >&2
  exit 1
fi
if ! grep -Eiq "certificate|handshake|unknown authority|bad certificate|context deadline exceeded" "$RESULTS/e2e-agent-mtls.untrusted-ca.err"; then
  echo "[e2e-agent-mtls][ERROR] untrusted client CA did not fail with a TLS certificate error" >&2
  cat "$RESULTS/e2e-agent-mtls.untrusted-ca.err" >&2
  exit 1
fi

curl -sf "$MGR_URL/api/v1/agent-sessions?tenant_id=$TENANT_ID&agent_id=$AGENT_ID" > "$RESULTS/e2e-agent-mtls.sessions.json"
if ! grep -Fq '"last_ack_cursor":"mtls-good-batch"' "$RESULTS/e2e-agent-mtls.sessions.json"; then
  echo "[e2e-agent-mtls][ERROR] successful mTLS upload did not update session cursor" >&2
  cat "$RESULTS/e2e-agent-mtls.sessions.json" >&2
  exit 1
fi

cp "$PKI_DIR/README.txt" "$RESULTS/e2e-agent-mtls.pki-readme.txt"
cp "$TMP/manager.log" "$RESULTS/e2e-agent-mtls.manager.log"
echo "[e2e-agent-mtls] ok"
