#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_FILE="${PLATFORM_COMPOSE:-$ROOT/deployments/compose.platform.yaml}"
SECRET_DIR="${PKI_RUNTIME_DIR:-$ROOT/deployments/pki/agent-plane-mtls/runtime}"
MANAGER_URL="${SYSARMOR_MANAGER_URL:-http://127.0.0.1:19443}"
UI_URL="${SYSARMOR_MANAGER_UI_URL:-http://127.0.0.1:4173}"
COOKIE_JAR="$(mktemp)"
trap 'rm -f "$COOKIE_JAR"' EXIT
chmod 600 "$COOKIE_JAR"

pass() { printf '[PASS] %s\n' "$1"; }
fail() { printf '[FAIL] %s\n' "$1" >&2; exit 1; }

check_secrets() {
  local file
  for file in bootstrap-admin-username bootstrap-admin-password auth-secret manager-jwt-private.pem manager-jwt-public.pem; do
    [[ -s "$SECRET_DIR/$file" ]] || fail "auth secret $file"
  done
  for file in bootstrap-admin-username bootstrap-admin-password auth-secret manager-jwt-private.pem; do
    [[ "$(stat -c %a "$SECRET_DIR/$file")" == "600" ]] || fail "auth secret permissions $file"
  done
  pass "authentication secrets"
}

check_services() {
  local running required
  running="$(docker compose -f "$COMPOSE_FILE" ps --services --filter status=running)"
  for required in postgres kafka redis opensearch manager gateway worker manager-ui; do
    grep -qx "$required" <<<"$running" || fail "service $required is not running"
  done
  pass "platform services"
}

status() {
  curl --noproxy '*' -sS -o /dev/null -w '%{http_code}' "$1"
}

check_endpoints() {
  [[ "$(status "$MANAGER_URL/healthz")" == "200" ]] || fail "manager health"
  [[ "$(status "$UI_URL/login")" == "200" ]] || fail "manager UI login"
  [[ "$(status "$UI_URL/api/manager/agents")" == "401" ]] || fail "unauthenticated BFF rejection"
  pass "public and protected endpoints"
}

check_login() {
  local csrf username password login_status api_status
  csrf="$(curl --noproxy '*' -sS -c "$COOKIE_JAR" "$UI_URL/api/auth/csrf" | jq -er '.csrfToken')"
  username="$(<"$SECRET_DIR/bootstrap-admin-username")"
  password="$(<"$SECRET_DIR/bootstrap-admin-password")"
  login_status="$(printf '%s\n' \
    "data-urlencode = \"csrfToken=$csrf\"" \
    "data-urlencode = \"username=$username\"" \
    "data-urlencode = \"password=$password\"" \
    "data-urlencode = \"redirectTo=$UI_URL/\"" | \
    curl --noproxy '*' -sS -o /dev/null -w '%{http_code}' -b "$COOKIE_JAR" -c "$COOKIE_JAR" \
      -X POST "$UI_URL/api/auth/callback/credentials" --config -)"
  [[ "$login_status" == "302" || "$login_status" == "200" ]] || fail "bootstrap admin login"
  api_status="$(curl --noproxy '*' -sS -o /dev/null -w '%{http_code}' -b "$COOKIE_JAR" "$UI_URL/api/manager/agents")"
  [[ "$api_status" == "200" ]] || fail "authenticated BFF to Manager"
  pass "bootstrap admin authenticated request"
}

check_secrets
check_services
check_endpoints
check_login
echo "SysArmor doctor: PASS"
