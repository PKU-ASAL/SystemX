#!/usr/bin/env bash
set -euo pipefail

PROFILE="${SYSARMOR_INSTALL_PROFILE:-linux-systemd}"
SOURCE_AGENT="${SYSARMOR_INSTALL_AGENT_SOURCE:?SYSARMOR_INSTALL_AGENT_SOURCE is required}"
SOURCE_CTL="${SYSARMOR_INSTALL_CTL_SOURCE:?SYSARMOR_INSTALL_CTL_SOURCE is required}"
SOURCE_SERVICE="${SYSARMOR_INSTALL_SERVICE_SOURCE:?SYSARMOR_INSTALL_SERVICE_SOURCE is required}"
SOURCE_CONFIG="${SYSARMOR_INSTALL_CONFIG_SOURCE:?SYSARMOR_INSTALL_CONFIG_SOURCE is required}"
SOURCE_POLICY="${SYSARMOR_INSTALL_POLICY_SOURCE:?SYSARMOR_INSTALL_POLICY_SOURCE is required}"
SOURCE_CONTENT="${SYSARMOR_INSTALL_CONTENT_SOURCE:?SYSARMOR_INSTALL_CONTENT_SOURCE is required}"
SOURCE_SENSOR="${SYSARMOR_INSTALL_SENSOR_SOURCE:?SYSARMOR_INSTALL_SENSOR_SOURCE is required}"
SOURCE_CONTAINER_ENTRYPOINT="${SYSARMOR_INSTALL_CONTAINER_ENTRYPOINT_SOURCE:-}"
AGENT_HOME="${SYSARMOR_AGENT_HOME:-/opt/sysarmor/agent}"
AGENT_DST="${SYSARMOR_AGENT_DST:-$AGENT_HOME/bin/sysarmor-agent}"
CTL_DST="${SYSARMOR_CTL_DST:-/usr/local/bin/sysarmorctl}"
CONTAINER_ENTRYPOINT_DST="${SYSARMOR_CONTAINER_ENTRYPOINT_DST:-/usr/local/bin/sysarmor-container-entrypoint}"
SERVICE_DST="${SYSARMOR_SERVICE_DST:-/etc/systemd/system/sysarmor-agent.service}"
CONFIG_DST="${SYSARMOR_CONFIG_DST:-/etc/sysarmor/agent/agent.yaml}"
POLICY_DST="${SYSARMOR_POLICY_DST:-/etc/sysarmor/agent/policy.json}"
STATE_DIR="${SYSARMOR_STATE_DIR:-/var/lib/sysarmor/agent}"
RUNTIME_DIR="${SYSARMOR_RUNTIME_DIR:-/run/sysarmor/agent}"
BUNDLE_DIR="${SYSARMOR_TETRAGON_BUNDLE_DIR:-$AGENT_HOME/bundles/tetragon}"
DEFAULT_CONTENT_DIR="${SYSARMOR_DEFAULT_CONTENT_DIR:-$AGENT_HOME/content/default}"
INSTALL_DIR="${SYSARMOR_TETRAGON_INSTALL_DIR:-$AGENT_HOME/sensors}"
SOCKET_PATH="${SYSARMOR_AGENT_SOCKET:-/run/sysarmor/agent/control.sock}"
ENABLE_SERVICE="${SYSARMOR_ENABLE_SERVICE:-1}"

declare -a TARGETS=() STAGES=() BACKUPS=() HAD_OLD=()
TRANSACTION_ACTIVE=0
WAS_ACTIVE=0
WAS_ENABLED=0

fail() {
  echo "[sysarmor-install][ERROR] $*" >&2
  exit 1
}

require_file() {
  [[ -f "$1" ]] || fail "missing file: $1"
}

validate_inputs() {
  case "$PROFILE" in
    linux-systemd) ;;
    linux-container) ENABLE_SERVICE=0 ;;
    *) fail "unsupported install profile: $PROFILE" ;;
  esac
  [[ "$(uname -s)" == "Linux" ]] || fail "Linux is required"
  if [[ "$ENABLE_SERVICE" == "1" ]]; then
    [[ "${EUID:-$(id -u)}" -eq 0 ]] || fail "root privileges are required"
    command -v systemctl >/dev/null 2>&1 || fail "systemctl is required"
  fi
  command -v jq >/dev/null 2>&1 || fail "jq is required"
  for file in "$SOURCE_AGENT" "$SOURCE_CTL" "$SOURCE_SERVICE" "$SOURCE_CONFIG" "$SOURCE_POLICY" \
    "$SOURCE_CONTENT/content-manifest.json" "$SOURCE_SENSOR/install-bundle.sh" "$SOURCE_SENSOR/bundle.env"; do
    require_file "$file"
  done
  if [[ "$PROFILE" == "linux-container" ]]; then
    require_file "$SOURCE_CONTAINER_ENTRYPOINT"
  fi
}

validate_default_content() {
  local entry file ref
  jq -e '.version != "" and (.entries | length > 0)' "$SOURCE_CONTENT/content-manifest.json" >/dev/null || \
    fail "invalid default content manifest"
  while IFS= read -r entry; do
    file="$(jq -r '.file' <<<"$entry")"
    [[ -n "$file" && "$file" == "$(basename "$file")" && -f "$SOURCE_CONTENT/$file" ]] || \
      fail "default content manifest references invalid file: $file"
    jq -e --argjson entry "$entry" \
      '.metadata.id == $entry.ref and .kind == $entry.kind and .metadata.version == $entry.version and .integrity.digest == $entry.digest' \
      "$SOURCE_CONTENT/$file" >/dev/null || fail "default content metadata mismatch: $file"
  done < <(jq -c '.entries[]' "$SOURCE_CONTENT/content-manifest.json")
  while IFS= read -r ref; do
    [[ -z "$ref" ]] && continue
    jq -s -e --arg ref "$ref" 'any(.[]; .kind == "rulepack" and any(.spec.rulesets[]?; .id == $ref))' \
      "$SOURCE_CONTENT"/*.json >/dev/null || fail "default policy references unavailable ruleset: $ref"
  done < <(jq -r '.detection.rulesets[]? | select(.enabled != false) | .ref' "$SOURCE_POLICY")
}

register_stage() {
  TARGETS+=("$1")
  STAGES+=("$2")
  BACKUPS+=("")
  HAD_OLD+=(0)
}

stage_file() {
  local source="$1" target="$2" mode="$3" parent stage
  parent="$(dirname "$target")"
  install -d -m 0755 "$parent"
  stage="$(mktemp "$parent/.$(basename "$target").stage.XXXXXX")"
  install -m "$mode" "$source" "$stage"
  register_stage "$target" "$stage"
}

stage_config() {
  local parent stage
  parent="$(dirname "$CONFIG_DST")"
  install -d -m 0750 "$parent"
  stage="$(mktemp "$parent/.agent.yaml.stage.XXXXXX")"
  if [[ -e "$CONFIG_DST" ]]; then
    "$SOURCE_AGENT" merge-release-config --existing "$CONFIG_DST" --release "$SOURCE_CONFIG" --output "$stage"
    chown --reference="$CONFIG_DST" "$stage"
    chmod --reference="$CONFIG_DST" "$stage"
  else
    install -m 0644 "$SOURCE_CONFIG" "$stage"
  fi
  register_stage "$CONFIG_DST" "$stage"
}

stage_directory() {
  local source="$1" target="$2" parent stage
  parent="$(dirname "$target")"
  install -d -m 0755 "$parent"
  stage="$(mktemp -d "$parent/.$(basename "$target").stage.XXXXXX")"
  cp -a "$source/." "$stage/"
  register_stage "$target" "$stage"
}

stage_sensor() {
  local parent stage
  parent="$(dirname "$BUNDLE_DIR")"
  install -d -m 0755 "$parent"
  stage="$(mktemp -d "$parent/.tetragon.stage.XXXXXX")"
  if [[ -x "$SOURCE_SENSOR/bin/tetragon" && -x "$SOURCE_SENSOR/bin/tetra" ]]; then
    cp -a "$SOURCE_SENSOR/." "$stage/"
  elif ! SYSARMOR_TETRAGON_BUNDLE_DIR="$stage" "$SOURCE_SENSOR/install-bundle.sh" >/dev/null; then
    rm -rf "$stage"
    fail "failed to stage Tetragon bundle"
  fi
  register_stage "$BUNDLE_DIR" "$stage"
}

stage_installation() {
  install -d -m 0700 "$STATE_DIR"
  install -d -m 0750 "$RUNTIME_DIR"
  install -d -m 0755 "$INSTALL_DIR" "$AGENT_HOME/runtime" "$AGENT_HOME/cache"
  stage_file "$SOURCE_AGENT" "$AGENT_DST" 0755
  stage_file "$SOURCE_CTL" "$CTL_DST" 0755
  if [[ "$PROFILE" == "linux-container" ]]; then
    stage_file "$SOURCE_CONTAINER_ENTRYPOINT" "$CONTAINER_ENTRYPOINT_DST" 0755
  else
    stage_file "$SOURCE_SERVICE" "$SERVICE_DST" 0644
  fi
  stage_config
  if [[ ! -e "$POLICY_DST" ]]; then
    stage_file "$SOURCE_POLICY" "$POLICY_DST" 0644
  fi
  stage_directory "$SOURCE_CONTENT" "$DEFAULT_CONTENT_DIR"
  stage_sensor
}

commit_installation() {
  local i target stage backup parent
  for i in "${!TARGETS[@]}"; do
    target="${TARGETS[$i]}"
    stage="${STAGES[$i]}"
    parent="$(dirname "$target")"
    backup="$parent/.$(basename "$target").previous.$$"
    BACKUPS[$i]="$backup"
    if [[ -e "$target" ]]; then
      mv "$target" "$backup"
      HAD_OLD[$i]=1
    fi
    mv "$stage" "$target"
    STAGES[$i]=""
  done
}

cleanup_transaction_files() {
  local path
  for path in "${STAGES[@]}" "${BACKUPS[@]}"; do
    [[ -z "$path" || ! -e "$path" ]] || rm -rf "$path"
  done
}

rollback_installation() {
  local i target backup rollback_failed=0
  trap - EXIT INT TERM
  if [[ "$ENABLE_SERVICE" == "1" ]]; then
    systemctl stop sysarmor-agent 2>/dev/null || true
  fi
  for ((i=${#TARGETS[@]}-1; i>=0; i--)); do
    target="${TARGETS[$i]}"
    backup="${BACKUPS[$i]}"
    [[ -n "$backup" ]] || continue
    if [[ -e "$target" ]] && ! rm -rf "$target"; then
      echo "[sysarmor-install][ERROR] rollback could not remove target: $target; backup preserved: $backup" >&2
      BACKUPS[$i]=""
      rollback_failed=1
      continue
    fi
    if [[ "${HAD_OLD[$i]}" == 1 && -e "$backup" ]]; then
      if ! mv "$backup" "$target"; then
        echo "[sysarmor-install][ERROR] rollback could not restore target: $target; backup preserved: $backup" >&2
        BACKUPS[$i]=""
        rollback_failed=1
      fi
    fi
  done
  cleanup_transaction_files
  if [[ "$ENABLE_SERVICE" == "1" && "$rollback_failed" == 0 ]]; then
    if ! systemctl daemon-reload 2>/dev/null; then
      echo "[sysarmor-install][ERROR] rollback could not reload systemd" >&2
      rollback_failed=1
    elif [[ "$WAS_ENABLED" != 1 ]] && ! systemctl disable sysarmor-agent 2>/dev/null; then
      echo "[sysarmor-install][ERROR] rollback could not restore disabled service state" >&2
      rollback_failed=1
    elif [[ "$WAS_ACTIVE" == 1 ]] && ! systemctl start sysarmor-agent 2>/dev/null; then
      echo "[sysarmor-install][ERROR] rollback could not restart the restored service" >&2
      rollback_failed=1
    fi
  fi
  return "$rollback_failed"
}

on_exit() {
  local status=$?
  if [[ "$TRANSACTION_ACTIVE" == 1 ]]; then
    if ! rollback_installation; then
      echo "[sysarmor-install][ERROR] installation rollback incomplete; preserved backups require manual recovery" >&2
      status=1
    fi
  else
    cleanup_transaction_files
  fi
  exit "$status"
}

wait_for_agent() {
  local attempt
  for attempt in $(seq 1 30); do
    if "$CTL_DST" --socket "$SOCKET_PATH" --json agent health >/dev/null 2>&1; then
      return
    fi
    sleep 1
  done
  systemctl status sysarmor-agent --no-pager -l >&2 || true
  fail "Agent did not become healthy within 30 seconds"
}

trap on_exit EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
validate_inputs
validate_default_content
stage_installation

if [[ "$ENABLE_SERVICE" == "1" ]]; then
  systemctl is-active --quiet sysarmor-agent && WAS_ACTIVE=1 || true
  systemctl is-enabled --quiet sysarmor-agent && WAS_ENABLED=1 || true
fi
TRANSACTION_ACTIVE=1
if [[ "$ENABLE_SERVICE" == "1" ]]; then
  systemctl stop sysarmor-agent 2>/dev/null || true
fi
commit_installation
if [[ "$ENABLE_SERVICE" == "1" ]]; then
  systemctl daemon-reload
  systemctl enable --now sysarmor-agent
  wait_for_agent
fi
TRANSACTION_ACTIVE=0
trap - EXIT INT TERM
cleanup_transaction_files
echo "[sysarmor-install] installed unified Agent transaction"
