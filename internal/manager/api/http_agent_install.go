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
	lifecycle := renderEnrollmentLifecycle(profile, enrollment.EnrollmentID)
	managerURL := strings.TrimSuffix(absoluteURL(r, "/api/v1/enrollment-certificate"), "/api/v1/enrollment-certificate")
	tokenFile := fmt.Sprintf("ENROLLMENT_TOKEN_FILE=\"$tmp/enrollment-token\"\ninstall -m 0600 /dev/null \"$ENROLLMENT_TOKEN_FILE\"\nprintf '%%s' %s > \"$ENROLLMENT_TOKEN_FILE\"", shellQuote(token))
	return fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail

%s
SYSARMOR_AGENT_BUNDLE_SHA256="${SYSARMOR_AGENT_BUNDLE_SHA256:-%s}"
SYSARMOR_INSTALL_PROFILE="${SYSARMOR_INSTALL_PROFILE:-%s}"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
%s
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
test -x "$tmp/install.sh" || { echo "[sysarmor-enroll][ERROR] distribution install.sh missing" >&2; exit 1; }
case "$SYSARMOR_INSTALL_PROFILE" in
  linux-systemd) enrollment_config_source="$tmp/configs/standalone.yaml" ;;
  linux-container) enrollment_config_source="$tmp/configs/standalone-container.yaml" ;;
esac
cp "$enrollment_config_source" "$tmp/enrollment-agent.yaml"
cat >> "$tmp/enrollment-agent.yaml" <<'YAML'

agent:
%s
YAML
SYSARMOR_RELEASE_CONFIG="$tmp/enrollment-agent.yaml" \
  "$tmp/install.sh" --profile "$SYSARMOR_INSTALL_PROFILE"

%s

for _ in $(seq 1 100); do
  [[ -S /run/sysarmor/agent/control.sock ]] && break
  sleep 0.1
done
/usr/local/bin/sysarmorctl --socket /run/sysarmor/agent/control.sock enroll \
  --manager-url %s --token-file "$ENROLLMENT_TOKEN_FILE"
`, artifactLine, artifactSHA, profile, tokenFile, dependencies, artifactDownload, publicKey, manifestCheck, labels, lifecycle, shellQuote(managerURL))
}

func renderInstallDependencies(profile string) string {
	if profile != "linux-container" {
		return `if command -v apt-get >/dev/null 2>&1; then
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -y >/dev/null
  apt-get install -y ca-certificates curl jq openssl python3 >/dev/null
fi`
	}
	return `for bin in curl jq openssl tar sha256sum sed awk; do
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

func renderEnrollmentLifecycle(profile, enrollmentID string) string {
	if profile != "linux-container" {
		return fmt.Sprintf(`echo "[sysarmor-enroll] installed sysarmor-agent enrollment=%s"`, enrollmentID)
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
