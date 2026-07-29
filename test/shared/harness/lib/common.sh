#!/usr/bin/env bash

sa_init_repo_paths() {
  TEST_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
  REPO_ROOT="$(cd "$TEST_ROOT/.." && pwd)"
  ROOT="$TEST_ROOT"
  RESULTS="$TEST_ROOT/.results"
  BIN="$REPO_ROOT/bin"
  mkdir -p "$RESULTS" "$BIN"
}

sa_make_tmp() {
  local prefix="${1:?prefix required}"
  mktemp -d "${TMPDIR:-/tmp}/${prefix}.XXXXXX"
}

sa_pick_ports() {
  local start="${1:-24000}"
  local span="${2:-20000}"
  PORT_BASE="${PORT_BASE:-$((start + RANDOM % span))}"
  MANAGER_PORT="${MANAGER_PORT:-$PORT_BASE}"
  GRPC_PORT="${GRPC_PORT:-$((PORT_BASE + 1))}"
  GATEWAY_HEALTH_PORT="${GATEWAY_HEALTH_PORT:-$((PORT_BASE + 2))}"
  MGR_URL="${MGR_URL:-http://127.0.0.1:$MANAGER_PORT}"
  GATEWAY_URL="${GATEWAY_URL:-http://127.0.0.1:$GATEWAY_HEALTH_PORT}"
}

sa_kill_pid_ref() {
  local ref="${1:?pid variable required}"
  local pid="${!ref:-}"
  if [[ -n "$pid" ]]; then
    kill "$pid" 2>/dev/null || true
  fi
}

sa_cleanup_tmp() {
  local path="${1:-}"
  if [[ -n "$path" ]]; then
    rm -rf "$path"
  fi
}

sa_print_wait_debug() {
  local out="${1:?output path required}"
  shift || true
  echo "--- last response ---" >&2
  cat "$out" >&2 2>/dev/null || true
  for log_path in "$@"; do
    if [[ -n "$log_path" ]]; then
      echo "--- $log_path ---" >&2
      cat "$log_path" >&2 2>/dev/null || true
    fi
  done
}

sa_wait_contains() {
  local name="${1:?name required}"
  local needle="${2:?needle required}"
  local out="${3:?output path required}"
  shift 3
  local timeout="${SA_WAIT_TIMEOUT:-10}"
  local interval="${SA_WAIT_INTERVAL:-0.1}"
  local deadline=$((SECONDS + timeout))
  until "$@" >"$out" 2>"$out.err" && grep -Fq "$needle" "$out"; do
    if (( SECONDS >= deadline )); then
      echo "[${SA_TEST_NAME:-harness}][ERROR] timeout waiting for $needle via $name" >&2
      sa_print_wait_debug "$out" "${SA_WAIT_LOGS[@]:-}"
      echo "--- last error ---" >&2
      cat "$out.err" >&2 2>/dev/null || true
      return 1
    fi
    sleep "$interval"
  done
}

sa_wait_url_contains() {
  local url="${1:?url required}"
  local needle="${2:?needle required}"
  local out="${3:?output path required}"
  sa_wait_contains "$url" "$needle" "$out" curl -sf "$url"
}

sa_wait_glob() {
  local pattern="${1:?glob pattern required}"
  local name="${2:-$pattern}"
  local timeout="${SA_WAIT_TIMEOUT:-10}"
  local interval="${SA_WAIT_INTERVAL:-0.1}"
  local deadline=$((SECONDS + timeout))
  until compgen -G "$pattern" >/dev/null; do
    if (( SECONDS >= deadline )); then
      echo "[${SA_TEST_NAME:-harness}][ERROR] timeout waiting for $name" >&2
      for log_path in "${SA_WAIT_LOGS[@]:-}"; do
        echo "--- $log_path ---" >&2
        cat "$log_path" >&2 2>/dev/null || true
      done
      return 1
    fi
    sleep "$interval"
  done
}

sa_wait_no_glob() {
  local pattern="${1:?glob pattern required}"
  local name="${2:-$pattern}"
  local timeout="${SA_WAIT_TIMEOUT:-10}"
  local interval="${SA_WAIT_INTERVAL:-0.1}"
  local deadline=$((SECONDS + timeout))
  while compgen -G "$pattern" >/dev/null; do
    if (( SECONDS >= deadline )); then
      echo "[${SA_TEST_NAME:-harness}][ERROR] timeout waiting for $name to disappear" >&2
      return 1
    fi
    sleep "$interval"
  done
}

sa_build_go_bins() {
  local pkg
  for pkg in "$@"; do
    GOCACHE="${GOCACHE:-/tmp/sysarmor-go-cache}" CGO_ENABLED=0 go build -o "$BIN/$pkg" "$REPO_ROOT/cmd/$pkg"
  done
}

sa_start_memory_gateway() {
  local extra_args=("$@")
  "$BIN/sysarmor-gateway" \
    --listen "127.0.0.1:$GRPC_PORT" \
    --health-listen "127.0.0.1:$GATEWAY_HEALTH_PORT" \
    --store-backend memory \
    "${extra_args[@]}" \
    >"$TMP/gateway.log" 2>&1 &
  GATEWAY_PID=$!
}

sa_start_memory_manager() {
  local extra_args=("$@")
  "$BIN/sysarmor-manager" \
    --listen "127.0.0.1:$MANAGER_PORT" \
    --store-backend memory \
    "${extra_args[@]}" \
    >"$TMP/manager.log" 2>&1 &
  MGR_PID=$!
}

sa_start_memory_agent_stack() {
  local token="${1:-}"
  sa_start_memory_manager
  sa_start_memory_gateway --local-ingest --dev-token "$token"
}

sa_manager_ctl() {
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager "$@"
}
