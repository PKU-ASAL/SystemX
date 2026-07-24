#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_ROOT="$(cd "$HERE/.." && pwd)"
REPO="$(cd "$TEST_ROOT/.." && pwd)"
# shellcheck source=/dev/null
source "$HERE/common.sh"

IMAGES="${IMAGES:-ubuntu2204 ubuntu2404 debian12}"
RUN_ID="${RUN_ID:-$(date -u +%Y%m%dT%H%M%SZ)}"
RESULT_ROOT="$TEST_ROOT/.results/release/$RUN_ID"
INSTALL_URL="$(resolve_install_url)"
ACTIVE_CONTAINER=""
ACTIVE_RESULT_DIR=""
INSPECTOR="$RESULT_ROOT/inspect-state"

cleanup() {
  if [[ -n "$ACTIVE_CONTAINER" ]]; then
    mkdir -p "$ACTIVE_RESULT_DIR"
    docker inspect "$ACTIVE_CONTAINER" >"$ACTIVE_RESULT_DIR/container-inspect.json" 2>/dev/null || true
    docker logs "$ACTIVE_CONTAINER" >"$ACTIVE_RESULT_DIR/container.log" 2>&1 || true
    docker rm -f "$ACTIVE_CONTAINER" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

wait_for_health() {
  local container="$1"
  local output="$2"
  local deadline=$((SECONDS + 90))
  until docker exec "$container" sysarmorctl --json agent health >"$output" 2>"$output.err"; do
    if (( SECONDS >= deadline )); then
      echo "[release-test][ERROR] Agent 未在 90 秒内就绪: $container" >&2
      docker logs "$container" >&2 2>/dev/null || true
      return 1
    fi
    sleep 1
  done
}

wait_for_stopped() {
  local container="$1"
  local deadline=$((SECONDS + 20))
  while [[ "$(docker inspect "$container" --format '{{.State.Running}}')" == "true" ]]; do
    if (( SECONDS >= deadline )); then
      echo "[release-test][ERROR] Agent 退出后容器仍在运行: $container" >&2
      return 1
    fi
    sleep 0.2
  done
}

build_image() {
  local image="$1"
  local tag="$2"
  local result_dir="$3"
  docker build --network host \
    --build-arg "SYSARMOR_INSTALL_URL=$INSTALL_URL" \
    -t "$tag" "$HERE/images/$image" >"$result_dir/build.log" 2>&1
}

start_container() {
  local name="$1"
  local tag="$2"
  docker run -d \
    --name "$name" \
    --privileged \
    --cgroupns=host \
    -v /sys/kernel/btf/vmlinux:/sys/kernel/btf/vmlinux:ro \
    -v /sys/fs/bpf:/sys/fs/bpf \
    "$tag" >/dev/null
}

assert_scope() {
  local container="$1"
  docker exec "$container" grep -Fq 'type: namespace' /etc/sysarmor/agent/agent.yaml
  docker exec "$container" grep -Fq 'selector: self' /etc/sysarmor/agent/agent.yaml
}

record_state() {
  local container="$1"
  local result_dir="$2"
  docker exec "$container" sqlite3 -header /var/lib/sysarmor/agent/agent.db \
    "SELECT rule_id,severity,COUNT(*) AS count FROM signals GROUP BY rule_id,severity;" >"$result_dir/signals.txt"
  docker exec "$container" sqlite3 -header /var/lib/sysarmor/agent/agent.db \
    "SELECT state,COUNT(*) AS count,SUM(record_count) AS records FROM segments GROUP BY state;" >"$result_dir/segments.txt"
  docker logs "$container" >"$result_dir/container.log" 2>&1
}

assert_collection_and_scope() {
  local image="$1"
  local tag="$2"
  local container="$3"
  local positive="sysarmor-positive-$image-$(date +%s%N)"
  docker exec "$container" /bin/sh -c "/bin/sh -c ': # node $positive'"
  "$HERE/assert-state.sh" positive "$container" "$positive"

  local sibling="sysarmor-sibling-$image-$(date +%s%N)"
  docker run --rm --entrypoint /bin/sh "$tag" -c "/bin/sh -c ': # $sibling'" >/dev/null
  local host="sysarmor-host-$image-$(date +%s%N)"
  /bin/sh -c ": # $host"
  sleep 3
  "$HERE/assert-state.sh" absent "$container" "$sibling"
  "$HERE/assert-state.sh" absent "$container" "$host"
}

assert_restart_recovery() {
  local image="$1"
  local container="$2"
  local result_dir="$3"
  local agent_pid exit_code
  agent_pid="$(docker exec "$container" pgrep -o -f '^/opt/sysarmor/agent/bin/sysarmor-agent run')"
  docker exec "$container" kill "$agent_pid"
  wait_for_stopped "$container"
  exit_code="$(docker inspect "$container" --format '{{.State.ExitCode}}')"
  [[ "$exit_code" -ne 0 ]] || {
    echo "[release-test][ERROR] Agent 异常退出后容器退出码为 0" >&2
    return 1
  }

  docker start "$container" >/dev/null
  wait_for_health "$container" "$result_dir/health-after-restart.json"
  local recovered="sysarmor-recovered-$image-$(date +%s%N)"
  docker exec "$container" /bin/sh -c "/bin/sh -c ': # node $recovered'"
  "$HERE/assert-state.sh" positive "$container" "$recovered"
}

run_image() {
  local image="$1"
  local result_dir="$RESULT_ROOT/$image"
  local tag="sysarmor-release-test:$image-$RUN_ID"
  local container="sysarmor-release-$image-$RUN_ID"
  mkdir -p "$result_dir"
  ACTIVE_CONTAINER="$container"
  ACTIVE_RESULT_DIR="$result_dir"
  docker rm -f "$container" >/dev/null 2>&1 || true

  echo "[release-test] building $image from $INSTALL_URL"
  build_image "$image" "$tag" "$result_dir"
  start_container "$container" "$tag"
  wait_for_health "$container" "$result_dir/health.json"
  assert_scope "$container"
  assert_collection_and_scope "$image" "$tag" "$container"
  assert_restart_recovery "$image" "$container" "$result_dir"
  record_state "$container" "$result_dir"

  docker rm -f "$container" >/dev/null
  ACTIVE_CONTAINER=""
  ACTIVE_RESULT_DIR=""
  echo "[release-test] $image ok"
}

mkdir -p "$RESULT_ROOT"
(cd "$REPO" && GOCACHE="$RESULT_ROOT/go-build-cache" go build -o "$INSPECTOR" ./test/release)
export SYSARMOR_STATE_INSPECTOR="$INSPECTOR"
printf '%s\n' "$INSTALL_URL" >"$RESULT_ROOT/install-url.txt"
for image in $IMAGES; do
  case "$image" in
    ubuntu2204|ubuntu2404|debian12) run_image "$image" ;;
    *) echo "[release-test][ERROR] 不支持的镜像: $image" >&2; exit 2 ;;
  esac
done

echo "[release-test] all images passed; results: $RESULT_ROOT"
