#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_ROOT="$(cd "$HERE/.." && pwd)"
# shellcheck source=/dev/null
source "$HERE/config.sh"
# shellcheck source=/dev/null
source "$HERE/scenarios.sh"

RESULT_ROOT=""
INSTALL_URL=""
TETRAGON_URL=""
ACTIVE_BUSINESS=""
ACTIVE_ATTACKER=""
ACTIVE_ATTACKER_HOST=""
ACTIVE_NETWORK=""
ACTIVE_RESULT_DIR=""

prepare_result_root() {
  local base="$1" run_id="$2"
  if [[ ! "$run_id" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]]; then
    echo "[release-test][ERROR] RUN_ID 非法: $run_id" >&2
    return 2
  fi
  mkdir -p "$base"
  RESULT_ROOT="$base/$run_id"
  rm -rf -- "$RESULT_ROOT"
  mkdir -p "$RESULT_ROOT"
}

capture_container() {
  local container="$1" prefix="$2"
  [[ -n "$container" && -n "$ACTIVE_RESULT_DIR" ]] || return 0
  docker inspect "$container" >"$ACTIVE_RESULT_DIR/$prefix-inspect.json" 2>/dev/null || true
  docker logs "$container" >"$ACTIVE_RESULT_DIR/$prefix.log" 2>&1 || true
}

cleanup() {
  capture_container "$ACTIVE_BUSINESS" business
  capture_container "$ACTIVE_ATTACKER" attacker
  [[ -z "$ACTIVE_BUSINESS" ]] || docker rm -f "$ACTIVE_BUSINESS" >/dev/null 2>&1 || true
  [[ -z "$ACTIVE_ATTACKER" ]] || docker rm -f "$ACTIVE_ATTACKER" >/dev/null 2>&1 || true
  [[ -z "$ACTIVE_NETWORK" ]] || docker network rm "$ACTIVE_NETWORK" >/dev/null 2>&1 || true
  ACTIVE_BUSINESS=""
  ACTIVE_ATTACKER=""
  ACTIVE_ATTACKER_HOST=""
  ACTIVE_NETWORK=""
  ACTIVE_RESULT_DIR=""
}
trap cleanup EXIT

build_image() {
  local args=(--network host -f "$HERE/images/$1/Dockerfile"
    --build-arg "SYSARMOR_INSTALL_URL=$INSTALL_URL"
    --build-arg "SYSARMOR_TETRAGON_URL=$TETRAGON_URL")
  [[ "$FRESH_DOWNLOAD" != "1" ]] || args+=(--no-cache)
  docker build "${args[@]}" -t "$2" "$HERE" >"$3/build.log" 2>&1
}

wait_http() {
  local container="$1" url="$2" deadline=$((SECONDS + SERVICE_TIMEOUT))
  until docker exec "$container" curl -fsS "$url" >/dev/null 2>&1; do
    if (( SECONDS >= deadline )); then
      docker logs "$container" >&2 2>/dev/null || true
      echo "[release-test][ERROR] 服务未在 ${SERVICE_TIMEOUT} 秒内就绪: $url" >&2
      return 1
    fi
    sleep 0.25
  done
}

start_attacker() {
  docker run -d --name "$ACTIVE_ATTACKER" --network "$ACTIVE_NETWORK" \
    --entrypoint node -e "CONTROL_HOST=$ACTIVE_ATTACKER" "$1" \
    /opt/sysarmor-release-test/payload-server/server.js >/dev/null
  wait_http "$ACTIVE_ATTACKER" "http://127.0.0.1:8080/healthz"
  ACTIVE_ATTACKER_HOST="$(docker inspect --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$ACTIVE_ATTACKER")"
  [[ -n "$ACTIVE_ATTACKER_HOST" ]]
}

start_business() {
  docker run -d --name "$ACTIVE_BUSINESS" --network "$ACTIVE_NETWORK" \
    --privileged --cgroupns=host -e "ATTACK_HOST=$ACTIVE_ATTACKER_HOST" \
    -v /sys/kernel/btf/vmlinux:/sys/kernel/btf/vmlinux:ro \
    -v /sys/fs/bpf:/sys/fs/bpf "$1" >/dev/null
}

run_scenarios() {
  local scenario marker attack
  while IFS= read -r scenario; do
    marker="sysarmor-$scenario-$(date +%s%N)"
    attack="$HERE/attacks/$(scenario_attack "$scenario")"
    "$attack" "$ACTIVE_BUSINESS" "$marker" >"$ACTIVE_RESULT_DIR/$scenario-response.json"
    "$HERE/assert.sh" detected "$ACTIVE_BUSINESS" "$scenario" "$marker" \
      "$ACTIVE_RESULT_DIR/$scenario.jsonl"
  done < <(release_scenarios)
}

run_isolation_checks() {
  local tag="$1" marker
  marker="sysarmor-sibling-$(date +%s%N)"
  docker run --rm --network "$ACTIVE_NETWORK" --entrypoint /bin/sh "$tag" \
    -c 'printf "%s\n" "$1" >/dev/null' sysarmor-sibling "$marker"
  "$HERE/assert.sh" absent "$ACTIVE_BUSINESS" "$marker" "$ACTIVE_RESULT_DIR/events-sibling.jsonl"

  marker="sysarmor-host-$(date +%s%N)"
  /bin/sh -c 'printf "%s\n" "$1" >/dev/null' sysarmor-host "$marker"
  "$HERE/assert.sh" absent "$ACTIVE_BUSINESS" "$marker" "$ACTIVE_RESULT_DIR/events-host.jsonl"
}

run_image() {
  local image="$1" tag="sysarmor-release-test:$1-$RUN_ID"
  ACTIVE_RESULT_DIR="$RESULT_ROOT/$image"
  ACTIVE_BUSINESS="sysarmor-release-$image-$RUN_ID"
  ACTIVE_ATTACKER="sysarmor-release-attacker-$image-$RUN_ID"
  ACTIVE_NETWORK="sysarmor-release-net-$image-$RUN_ID"
  mkdir -p "$ACTIVE_RESULT_DIR"
  docker rm -f "$ACTIVE_BUSINESS" "$ACTIVE_ATTACKER" >/dev/null 2>&1 || true
  docker network rm "$ACTIVE_NETWORK" >/dev/null 2>&1 || true

  echo "[release-test] building $image from $INSTALL_URL"
  build_image "$image" "$tag" "$ACTIVE_RESULT_DIR"
  docker network create "$ACTIVE_NETWORK" >/dev/null
  start_attacker "$tag"
  start_business "$tag"
  "$HERE/assert.sh" ready "$ACTIVE_BUSINESS" "$ACTIVE_RESULT_DIR/health.json"
  wait_http "$ACTIVE_BUSINESS" "http://127.0.0.1:3000/healthz"
  run_scenarios
  "$HERE/assert.sh" capture-signals "$ACTIVE_BUSINESS" "$ACTIVE_RESULT_DIR/signals-all.jsonl"
  run_isolation_checks "$tag"
  cleanup
  echo "[release-test] $image ok"
}

main() {
  prepare_result_root "$TEST_ROOT/.results/release" "$RUN_ID"
  INSTALL_URL="$(resolve_download_url "$(resolve_install_url)")"
  TETRAGON_URL="$(resolve_tetragon_url)"
  printf '%s\n' "$INSTALL_URL" >"$RESULT_ROOT/install-url.txt"
  printf '%s\n' "$TETRAGON_URL" >"$RESULT_ROOT/tetragon-url.txt"
  for image in $IMAGES; do
    case "$image" in
      ubuntu2204|ubuntu2404|debian12) run_image "$image" ;;
      *) echo "[release-test][ERROR] 不支持的镜像: $image" >&2; return 2 ;;
    esac
  done
  echo "[release-test] all images passed; results: $RESULT_ROOT"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
