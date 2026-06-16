#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RESULTS="$ROOT/test/.results"
BIN="$ROOT/bin"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-store-status.XXXXXX")"
PORT_BASE="${PORT_BASE:-$((58000 + RANDOM % 2000))}"
MANAGER_PORT="${MANAGER_PORT:-$PORT_BASE}"
GRPC_PORT="${GRPC_PORT:-$((PORT_BASE + 1))}"
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
MGR_URL="http://127.0.0.1:$MANAGER_PORT"

mkdir -p "$RESULTS" "$BIN"

cleanup() {
  if [[ -n "${MGR_PID:-}" ]]; then kill "$MGR_PID" 2>/dev/null || true; fi
  rm -rf "$TMP"
}
trap cleanup EXIT

echo "[e2e-store-status] building binaries"
make -C "$ROOT" build >/dev/null

"$BIN/sysarmor-manager" \
  --listen "127.0.0.1:$MANAGER_PORT" \
  --grpc-listen "127.0.0.1:$GRPC_PORT" \
  --store-backend file \
  --store "$TMP/store.json" \
  --dev-token "$TOKEN" \
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
      echo "[e2e-store-status][ERROR] $name missing $needle" >&2
      cat "$out" >&2 2>/dev/null || true
      cat "$TMP/manager.log" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 0.2
  done
}

wait_contains "healthz" '"ok":true' "$RESULTS/e2e-store-status.health.json" curl -sf "$MGR_URL/healthz"

"$BIN/sysarmorctl" --mgr "$MGR_URL" --json store-status > "$RESULTS/e2e-store-status.store.json"
for want in '"backend":"file"' '"state_version":1' '"migration_version":1' '"postgres_schema_version":1'; do
  if ! grep -Fq "$want" "$RESULTS/e2e-store-status.store.json"; then
    echo "[e2e-store-status][ERROR] store status missing $want" >&2
    cat "$RESULTS/e2e-store-status.store.json" >&2
    exit 1
  fi
done

cp "$TMP/manager.log" "$RESULTS/e2e-store-status.manager.log"
echo "[e2e-store-status] ok"
