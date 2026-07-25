#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_ROOT="$(cd "$HERE/.." && pwd)"
# shellcheck source=/dev/null
source "$HERE/config.sh"

RESULT_ROOT="$TEST_ROOT/.results/release/$RUN_ID"
INSTALL_URL="$(resolve_download_url "$(resolve_install_url)")"
ASSERT="$HERE/assert.sh"
ACTIVE_CONTAINER=""
ACTIVE_RESULT_DIR=""

cleanup() {
  if [[ -n "$ACTIVE_CONTAINER" ]]; then
    mkdir -p "$ACTIVE_RESULT_DIR"
    docker inspect "$ACTIVE_CONTAINER" >"$ACTIVE_RESULT_DIR/container-inspect.json" 2>/dev/null || true
    docker logs "$ACTIVE_CONTAINER" >"$ACTIVE_RESULT_DIR/container.log" 2>&1 || true
    docker rm -f "$ACTIVE_CONTAINER" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

build_image() {
  local args=(--network host --build-arg "SYSARMOR_INSTALL_URL=$INSTALL_URL")
  if [[ "$FRESH_DOWNLOAD" == "1" ]]; then
    args+=(--no-cache)
  fi
  docker build "${args[@]}" -t "$2" "$HERE/images/$1" >"$3/build.log" 2>&1
}

start_container() {
  docker run -d --name "$1" --privileged --cgroupns=host \
    -v /sys/kernel/btf/vmlinux:/sys/kernel/btf/vmlinux:ro \
    -v /sys/fs/bpf:/sys/fs/bpf "$2" >/dev/null
}

run_attack_in_container() {
  docker exec -i "$1" /bin/bash -s -- "$2" <"$ATTACK_SCRIPT"
}

run_attack_in_sibling() {
  docker run --rm -i --entrypoint /bin/bash "$1" -s -- "$2" <"$ATTACK_SCRIPT" >/dev/null
}

run_attack_on_host() {
  /bin/bash "$ATTACK_SCRIPT" "$1"
}

verify_collection_and_scope() {
  local image="$1" tag="$2" container="$3" result_dir="$4" marker
  marker="sysarmor-positive-$image-$(date +%s%N)"
  run_attack_in_container "$container" "$marker"
  "$ASSERT" detected "$container" "$marker" "$result_dir/detected-initial.jsonl"

  marker="sysarmor-sibling-$image-$(date +%s%N)"
  run_attack_in_sibling "$tag" "$marker"
  "$ASSERT" absent "$container" "$marker" "$result_dir/events-sibling.jsonl"

  marker="sysarmor-host-$image-$(date +%s%N)"
  run_attack_on_host "$marker"
  "$ASSERT" absent "$container" "$marker" "$result_dir/events-host.jsonl"
}

verify_restart_recovery() {
  local image="$1" container="$2" result_dir="$3" agent_pid marker
  agent_pid="$(docker exec "$container" pgrep -o -f '^/opt/sysarmor/agent/bin/sysarmor-agent run')"
  docker exec "$container" kill "$agent_pid"
  "$ASSERT" stopped-nonzero "$container"
  docker start "$container" >/dev/null
  "$ASSERT" ready "$container" "$result_dir/health-after-restart.json"
  marker="sysarmor-recovered-$image-$(date +%s%N)"
  run_attack_in_container "$container" "$marker"
  "$ASSERT" detected "$container" "$marker" "$result_dir/detected-after-restart.jsonl"
}

run_image() {
  local image="$1" result_dir="$RESULT_ROOT/$1"
  local tag="sysarmor-release-test:$image-$RUN_ID" container="sysarmor-release-$image-$RUN_ID"
  mkdir -p "$result_dir"
  ACTIVE_CONTAINER="$container"
  ACTIVE_RESULT_DIR="$result_dir"
  docker rm -f "$container" >/dev/null 2>&1 || true

  echo "[release-test] building $image from $INSTALL_URL"
  build_image "$image" "$tag" "$result_dir"
  start_container "$container" "$tag"
  "$ASSERT" ready "$container" "$result_dir/health.json"
  verify_collection_and_scope "$image" "$tag" "$container" "$result_dir"
  if [[ "$RESTART_TEST" == "1" ]]; then
    verify_restart_recovery "$image" "$container" "$result_dir"
  fi
  docker logs "$container" >"$result_dir/container.log" 2>&1

  docker rm -f "$container" >/dev/null
  ACTIVE_CONTAINER=""
  ACTIVE_RESULT_DIR=""
  echo "[release-test] $image ok"
}

mkdir -p "$RESULT_ROOT"
printf '%s\n' "$INSTALL_URL" >"$RESULT_ROOT/install-url.txt"
for image in $IMAGES; do
  case "$image" in
    ubuntu2204|ubuntu2404|debian12) run_image "$image" ;;
    *) echo "[release-test][ERROR] 不支持的镜像: $image" >&2; exit 2 ;;
  esac
done

echo "[release-test] all images passed; results: $RESULT_ROOT"
