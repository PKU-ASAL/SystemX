package managerapi

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/sysarmor/sysarmor-next-project/internal/store"
)

func (s *Server) renderAgentInstallScript(r *http.Request, enrollment store.Enrollment, token string) string {
	artifactDefault := enrollment.ArtifactURL
	artifactDownload := `curl -fsSL "$SYSARMOR_AGENT_BUNDLE_URL" -o "$tmp/sysarmor-agent.tar.gz"`
	if enrollment.ArtifactID != "" {
		artifactDefault = absoluteURL(r, "/api/v1/enrollment-artifact")
		artifactDownload = `curl -fsSL -H "Authorization: Enrollment $(cat "$ENROLLMENT_TOKEN_FILE")" "$SYSARMOR_AGENT_BUNDLE_URL" -o "$tmp/sysarmor-agent.tar.gz"`
	}
	artifactLine := fmt.Sprintf("SYSARMOR_AGENT_BUNDLE_URL=\"${SYSARMOR_AGENT_BUNDLE_URL:-}\"\nif [[ -z \"$SYSARMOR_AGENT_BUNDLE_URL\" ]]; then\n  SYSARMOR_AGENT_BUNDLE_URL=%s\nfi\n", shellQuote(artifactDefault))
	if artifactDefault == "" {
		artifactLine = ": \"${SYSARMOR_AGENT_BUNDLE_URL:?set SYSARMOR_AGENT_BUNDLE_URL to a sysarmor-agent tarball URL}\"\n"
	}
	artifactSHA := enrollment.ArtifactSHA256
	labels := renderAgentLabelConfig(enrollment.Labels)
	publicKey := string(s.artifactPub)
	profile := defaultString(enrollment.Profile, "linux-systemd")
	dependencies := renderInstallDependencies(profile)
	manifestCheck := renderManifestCheck(profile)
	manifestEnv := renderManifestEnv(profile)
	scopeConfig := renderInstallScopeConfig(profile)
	serviceInstall := renderInstallServiceStep(profile)
	lifecycle := renderInstallLifecycle(profile, enrollment.EnrollmentID)
	managerURL := strings.TrimSuffix(absoluteURL(r, "/api/v1/enrollment-certificate"), "/api/v1/enrollment-certificate")
	tokenFile := fmt.Sprintf("ENROLLMENT_TOKEN_FILE=\"$tmp/enrollment-token\"\ninstall -m 0600 /dev/null \"$ENROLLMENT_TOKEN_FILE\"\nprintf '%%s' %s > \"$ENROLLMENT_TOKEN_FILE\"", shellQuote(token))
	return fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail

%s
SYSARMOR_AGENT_BUNDLE_SHA256="${SYSARMOR_AGENT_BUNDLE_SHA256:-%s}"
SYSARMOR_INSTALL_PROFILE="${SYSARMOR_INSTALL_PROFILE:-%s}"
AGENT_HOME="${SYSARMOR_AGENT_HOME:-/opt/sysarmor/agent}"
CONFIG_DST="${SYSARMOR_CONFIG_DST:-/etc/sysarmor/agent/agent.yaml}"
POLICY_DST="${SYSARMOR_POLICY_DST:-/etc/sysarmor/agent/policy.json}"
SERVICE_DST="${SYSARMOR_SERVICE_DST:-/etc/systemd/system/sysarmor-agent.service}"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
%s
mkdir -p "$AGENT_HOME/bin" "$AGENT_HOME/runtime" "$AGENT_HOME/cache" "$(dirname "$CONFIG_DST")" /var/lib/sysarmor/agent
%s
%s
if [[ -n "$SYSARMOR_AGENT_BUNDLE_SHA256" ]]; then
  actual_sha="$(sha256sum "$tmp/sysarmor-agent.tar.gz" | awk '{print $1}')"
  if [[ "$actual_sha" != "$SYSARMOR_AGENT_BUNDLE_SHA256" ]]; then
    echo "[sysarmor-enroll][ERROR] artifact sha256 mismatch: got=$actual_sha want=$SYSARMOR_AGENT_BUNDLE_SHA256" >&2
    exit 1
  fi
fi
tar xzf "$tmp/sysarmor-agent.tar.gz" -C "$tmp"
test -f "$tmp/manifest.json" || { echo "[sysarmor-enroll][ERROR] distribution manifest.json missing" >&2; exit 1; }
test -f "$tmp/manifest.sig" || { echo "[sysarmor-enroll][ERROR] distribution manifest.sig missing" >&2; exit 1; }
cat > "$tmp/artifact-public.pem" <<'PEM'
%s
PEM
if grep -Fq "BEGIN PUBLIC KEY" "$tmp/artifact-public.pem"; then
  openssl dgst -sha256 -verify "$tmp/artifact-public.pem" -signature "$tmp/manifest.sig" "$tmp/manifest.json" >/dev/null
fi
%s
%s
if [[ -z "$DIST_ENTRYPOINT" || -z "$DIST_SYSTEMD_UNIT" ]]; then
  echo "[sysarmor-enroll][ERROR] manifest entrypoint/systemd_unit missing" >&2
  exit 1
fi
install -m 0755 "$tmp/$DIST_ENTRYPOINT" "$AGENT_HOME/bin/sysarmor-agent"
install -m 0755 "$tmp/bin/sysarmorctl" "$AGENT_HOME/bin/sysarmorctl"
%s
if [[ -n "$DIST_SENSOR_BUNDLE" ]]; then
  mkdir -p "$AGENT_HOME/bundles" "$AGENT_HOME/$DIST_SENSOR_INSTALL_DIR"
  rm -rf "$AGENT_HOME/bundles/$DIST_SENSOR_NAME"
  cp -a "$tmp/$DIST_SENSOR_BUNDLE" "$AGENT_HOME/bundles/$DIST_SENSOR_NAME"
fi
cat > "$CONFIG_DST" <<'YAML'
local:
  state_path: /var/lib/sysarmor/agent
  storage:
    max_bytes: 10GiB
    min_free_bytes: 2GiB
    segment_size: 64MiB
    signal_max_count: 100000
  export:
    retry_initial: 1s
    retry_max: 30s
    request_timeout: 10s
    max_inflight: 1
    wire_compression: none

agent:
%s
control:
  socket_path: /run/sysarmor/agent/control.sock

sensor:
  backend: tetragon
  mode: managed
  bundle_dir: /opt/sysarmor/agent/bundles/tetragon
  install_dir: /opt/sysarmor/agent/sensors
  observe_only: true
%s

telemetry:
  max_batch_items: 256
  max_batch_bytes: 256KiB
  flush_interval: 1s

policy:
  path: /etc/sysarmor/agent/policy.json
YAML
install -m 0640 "$tmp/policies/policy.json" "$POLICY_DST"

%s

for _ in $(seq 1 100); do
  [[ -S /run/sysarmor/agent/control.sock ]] && break
  sleep 0.1
done
"$AGENT_HOME/bin/sysarmorctl" --socket /run/sysarmor/agent/control.sock enroll \
  --manager-url %s --token-file "$ENROLLMENT_TOKEN_FILE"
`, artifactLine, artifactSHA, profile, tokenFile, dependencies, artifactDownload, publicKey, manifestCheck, manifestEnv, serviceInstall, labels, scopeConfig, lifecycle, shellQuote(managerURL))
}

func renderInstallDependencies(profile string) string {
	if profile != "linux-container" {
		return `if command -v apt-get >/dev/null 2>&1; then
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -y >/dev/null
  apt-get install -y ca-certificates curl openssl python3 >/dev/null
fi`
	}
	return `for bin in curl openssl tar sha256sum sed awk; do
  if ! command -v "$bin" >/dev/null 2>&1; then
    echo "[sysarmor-enroll][ERROR] missing required command for linux-container profile: $bin" >&2
    exit 1
  fi
done`
}

func renderManifestCheck(profile string) string {
	if profile != "linux-container" {
		return `python3 - "$tmp" <<'PY'
import hashlib, json, os, sys
root = sys.argv[1]
manifest = json.load(open(os.path.join(root, "manifest.json")))
if manifest.get("schema_version") != "sysarmor.agent.distribution/v1":
    raise SystemExit("unsupported distribution manifest schema")
for item in manifest.get("files", []):
    rel = item["path"].lstrip("./")
    if rel.startswith("/") or ".." in rel.split("/"):
        raise SystemExit(f"invalid manifest path: {rel}")
    path = os.path.join(root, rel)
    with open(path, "rb") as f:
        got = hashlib.sha256(f.read()).hexdigest()
    if got.lower() != item["sha256"].lower():
        raise SystemExit(f"sha256 mismatch: {rel}")
PY`
	}
	return `grep -Eq '"schema_version"[[:space:]]*:[[:space:]]*"sysarmor.agent.distribution/v1"' "$tmp/manifest.json" || {
  echo "[sysarmor-enroll][ERROR] unsupported distribution manifest schema" >&2
  exit 1
}
manifest_sums="$tmp/manifest-files.sha256"
: > "$manifest_sums"
tr -d '\n' < "$tmp/manifest.json" | sed 's/},[[:space:]]*{/}\
{/g' | while IFS= read -r item; do
  rel="$(printf '%s\n' "$item" | sed -n 's/.*"path"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"
  sum="$(printf '%s\n' "$item" | sed -n 's/.*"sha256"[[:space:]]*:[[:space:]]*"\([0-9a-fA-F]\{64\}\)".*/\1/p')"
  if [[ -z "$rel" || -z "$sum" ]]; then
    continue
  fi
  case "$rel" in
    /*|*../*|../*) echo "[sysarmor-enroll][ERROR] invalid manifest path: $rel" >&2; exit 1 ;;
  esac
  printf '%s  %s/%s\n' "$sum" "$tmp" "$rel" >> "$manifest_sums"
done
if [[ ! -s "$manifest_sums" ]]; then
  echo "[sysarmor-enroll][ERROR] distribution manifest files are missing" >&2
  exit 1
fi
sha256sum -c "$manifest_sums" >/dev/null`
}

func renderManifestEnv(profile string) string {
	if profile != "linux-container" {
		return `eval "$(python3 - "$tmp/manifest.json" <<'PY'
import json, shlex, sys
m = json.load(open(sys.argv[1]))
sensors = m.get("sensors") or []
sensor = sensors[0] if sensors else {}
values = {
    "DIST_ENTRYPOINT": m["entrypoint"],
    "DIST_SYSTEMD_UNIT": m["systemd_unit"],
    "DIST_SENSOR_NAME": sensor.get("name", "tetragon"),
    "DIST_SENSOR_BUNDLE": sensor.get("bundle_dir", ""),
    "DIST_SENSOR_INSTALL_DIR": sensor.get("install_dir", "sensors"),
}
for k, v in values.items():
    print(f"{k}={shlex.quote(v)}")
PY
)"`
	}
	return `manifest_flat="$(tr -d '\n' < "$tmp/manifest.json")"
DIST_ENTRYPOINT="$(printf '%s\n' "$manifest_flat" | sed -n 's/.*"entrypoint"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"
DIST_SYSTEMD_UNIT="$(printf '%s\n' "$manifest_flat" | sed -n 's/.*"systemd_unit"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"
DIST_SENSOR_BUNDLE="$(printf '%s\n' "$manifest_flat" | sed -n 's/.*"bundle_dir"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"
DIST_SENSOR_INSTALL_DIR="$(printf '%s\n' "$manifest_flat" | sed -n 's/.*"install_dir"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"
DIST_SENSOR_NAME="tetragon"
DIST_SENSOR_INSTALL_DIR="${DIST_SENSOR_INSTALL_DIR:-sensors}"`
}

func renderInstallScopeConfig(profile string) string {
	if profile != "linux-container" {
		return "  scope:\n    type: host"
	}
	return "  scope:\n    type: namespace\n    selector: self"
}

func renderInstallServiceStep(profile string) string {
	if profile != "linux-container" {
		return `install -m 0644 "$tmp/$DIST_SYSTEMD_UNIT" "$SERVICE_DST"`
	}
	return `echo "[sysarmor-enroll] linux-container profile: skipping systemd unit install"`
}

func renderInstallLifecycle(profile, enrollmentID string) string {
	if profile != "linux-container" {
		return fmt.Sprintf(`systemctl daemon-reload
systemctl enable --now sysarmor-agent
echo "[sysarmor-enroll] installed sysarmor-agent enrollment=%s"`, enrollmentID)
	}
	return fmt.Sprintf(`echo "[sysarmor-enroll] installed sysarmor-agent enrollment=%s"
"$AGENT_HOME/bin/sysarmor-agent" run --config "$CONFIG_DST" >"$AGENT_HOME/runtime/agent.log" 2>&1 &
echo $! > "$AGENT_HOME/runtime/agent.pid"`, enrollmentID)
}

func renderAgentLabelConfig(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString("  label.")
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(yamlQuote(labels[k]))
		b.WriteByte('\n')
	}
	return b.String()
}

func yamlQuote(v string) string {
	return strconv.Quote(v)
}

func shellQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", "'\"'\"'") + "'"
}

func defaultString(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
