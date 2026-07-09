#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WEB_DIR="${WEB_DIR:-$ROOT_DIR/web/manager}"
WEB_HOST="${WEB_HOST:-127.0.0.1}"
WEB_DEV_PORT="${WEB_DEV_PORT:-5173}"
WEB_PREVIEW_PORT="${WEB_PREVIEW_PORT:-4173}"
WEB_MODE="${WEB_MODE:-preview}"
WEB_RUN_DIR="${WEB_RUN_DIR:-$ROOT_DIR/.run}"
WEB_LOG="${WEB_LOG:-$WEB_RUN_DIR/manager-console.log}"
WEB_PID="${WEB_PID:-$WEB_RUN_DIR/manager-console.pid}"

port_pids() {
  ss -ltnp 2>/dev/null |
    sed -n "s/.*:\($WEB_DEV_PORT\|$WEB_PREVIEW_PORT\) .*pid=\([0-9][0-9]*\).*/\2/p" |
    sort -u
}

preview_pids() {
  ps -eo pid=,args= |
    awk -v port="$WEB_PREVIEW_PORT" '
      $0 ~ /next/ && $0 ~ /start/ && $0 ~ "--port " port { print $1 }
    ' |
    sort -u
}

pid_csv() {
  tr '\n' ',' | sed 's/,$//'
}

start() {
  mkdir -p "$WEB_RUN_DIR"
  local url="http://$WEB_HOST:$WEB_PREVIEW_PORT"
  local command=(pnpm start --hostname "$WEB_HOST" --port "$WEB_PREVIEW_PORT")
  if [[ "$WEB_MODE" == "dev" ]]; then
    url="http://$WEB_HOST:$WEB_DEV_PORT"
    command=(pnpm dev --hostname "$WEB_HOST" --port "$WEB_DEV_PORT")
  fi

  local pids
  pids="$(port_pids)"
  if [[ -n "$pids" ]]; then
    echo "Manager console is already running"
    status
    return 0
  fi

  (
    cd "$WEB_DIR"
    setsid "${command[@]}" >"$WEB_LOG" 2>&1 </dev/null &
    echo "$!" >"$WEB_PID"
  )

  sleep 1
  local pid
  pid="$(cat "$WEB_PID")"
  if kill -0 "$pid" 2>/dev/null; then
    echo "Manager console started: $url"
    echo "Log: ${WEB_LOG#$ROOT_DIR/}"
    return 0
  fi

  echo "Manager console failed to start. Log: ${WEB_LOG#$ROOT_DIR/}" >&2
  tail -80 "$WEB_LOG" >&2 || true
  return 1
}

status() {
  echo "SysArmor Manager Console:"
  echo "  dev:     http://$WEB_HOST:$WEB_DEV_PORT"
  echo "  preview: http://$WEB_HOST:$WEB_PREVIEW_PORT"
  local pids
  pids="$(port_pids)"
  if [[ -z "$pids" ]]; then
    pids="$(preview_pids)"
  fi
  if [[ -n "$pids" ]]; then
    ps -p "$(printf '%s\n' "$pids" | pid_csv)" -o pid=,cmd=
  elif curl --noproxy '*' -sf "http://$WEB_HOST:$WEB_PREVIEW_PORT/" >/dev/null 2>&1; then
    echo "  running: preview is reachable"
  else
    echo "  not running"
  fi
}

stop() {
  local pids
  pids="$(port_pids)"
  if [[ -z "$pids" ]]; then
    pids="$(preview_pids)"
  fi
  if [[ -f "$WEB_PID" ]]; then
    local pid
    pid="$(cat "$WEB_PID")"
    if kill -0 "$pid" 2>/dev/null; then
      pids="$(printf '%s\n%s\n' "$pids" "$pid" | sed '/^$/d' | sort -u)"
    fi
  fi

  if [[ -z "$pids" ]]; then
    echo "Web dev/preview server is not running"
    return 0
  fi

  kill $pids
  rm -f "$WEB_PID"
  echo "Stopped web process(es): $(printf '%s\n' "$pids" | pid_csv)"
}

case "${1:-}" in
  up) start ;;
  status) status ;;
  stop) stop ;;
  *)
    echo "usage: $0 {up|status|stop}" >&2
    exit 2
    ;;
esac
