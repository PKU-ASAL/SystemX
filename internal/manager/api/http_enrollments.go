package managerapi

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sysarmor/sysarmor-next-project/internal/store"
)

func (s *Server) enrollments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		items := s.store.ListEnrollments(q.Get("tenant_id"), q.Get("status"))
		for i := range items {
			items[i] = publicEnrollment(items[i])
		}
		writeJSON(w, map[string]any{"enrollments": items})
	case http.MethodPost:
		if !s.requireOperator(w, r, "admin") {
			return
		}
		var req enrollmentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode enrollment: %v", err), http.StatusBadRequest)
			return
		}
		enrollment, token, err := newEnrollment(req, s.actorFromRequest(r, req.Actor))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		artifactID := strings.TrimSpace(req.ArtifactID)
		if strings.TrimSpace(req.Channel) != "" {
			channel, ok := s.store.GetChannel(enrollment.TenantID, req.Channel)
			if !ok {
				http.Error(w, "channel not found", http.StatusBadRequest)
				return
			}
			artifactID = channel.ArtifactID
			enrollment.Channel = channel.Channel
		}
		if artifactID != "" {
			artifact, ok := s.store.GetArtifact(enrollment.TenantID, artifactID)
			if !ok || artifact.Status != "active" {
				http.Error(w, "active artifact not found", http.StatusBadRequest)
				return
			}
			enrollment.ArtifactID = artifact.ArtifactID
			enrollment.ArtifactSHA256 = artifact.SHA256
			enrollment.ArtifactURL = artifactInstallURLForProfile(r, artifact, enrollment.Profile)
		}
		enrollment = s.store.CreateEnrollment(enrollment)
		if enrollment.EnrollmentID == "" {
			http.Error(w, "create enrollment failed", http.StatusBadRequest)
			return
		}
		if err := s.store.Save(); err != nil {
			http.Error(w, fmt.Sprintf("save store: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{
			"enrollment":  publicEnrollment(enrollment),
			"token":       token,
			"install_url": installURL(r, token),
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) agentInstallScript(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		http.Error(w, "token is required", http.StatusBadRequest)
		return
	}
	tokenHash := enrollmentTokenHash(token)
	enrollment, ok := s.store.GetEnrollmentByTokenHash(tokenHash)
	if !ok || enrollment.Status != "active" {
		http.Error(w, "enrollment not found", http.StatusNotFound)
		return
	}
	if !enrollment.ExpiresAt.IsZero() && time.Now().UTC().After(enrollment.ExpiresAt) {
		http.Error(w, "enrollment expired", http.StatusGone)
		return
	}
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	_, _ = w.Write([]byte(s.renderAgentInstallScript(r, enrollment, token)))
}

func (s *Server) enrollmentArtifact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	enrollment, ok := s.store.GetEnrollmentByTokenHash(enrollmentTokenHash(token))
	if token == "" || !ok || enrollment.Status != "active" || enrollment.ArtifactID == "" {
		http.NotFound(w, r)
		return
	}
	if !enrollment.ExpiresAt.IsZero() && time.Now().UTC().After(enrollment.ExpiresAt) {
		http.Error(w, "enrollment expired", http.StatusGone)
		return
	}
	artifact, ok := s.store.GetArtifact(enrollment.TenantID, enrollment.ArtifactID)
	if !ok || artifact.Status != "active" {
		http.NotFound(w, r)
		return
	}
	s.downloadArtifact(w, r, enrollment.TenantID, enrollment.ArtifactID)
}

func (s *Server) enrollmentCertificate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.caCert == nil || s.caKey == nil || len(s.caCertPEM) == 0 {
		http.Error(w, "agent certificate authority is not configured", http.StatusServiceUnavailable)
		return
	}
	var req certificateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode certificate request: %v", err), http.StatusBadRequest)
		return
	}
	token := strings.TrimSpace(req.Token)
	if token == "" {
		token = strings.TrimSpace(r.URL.Query().Get("token"))
	}
	tokenHash := enrollmentTokenHash(token)
	enrollment, ok := s.store.GetEnrollmentByTokenHash(tokenHash)
	if !ok || enrollment.Status != "active" {
		http.Error(w, "enrollment not found", http.StatusNotFound)
		return
	}
	if !enrollment.ExpiresAt.IsZero() && time.Now().UTC().After(enrollment.ExpiresAt) {
		http.Error(w, "enrollment expired", http.StatusGone)
		return
	}
	certPEM, cert, err := s.signAgentCSR(enrollment, []byte(req.CSR))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	usedAt := time.Now().UTC()
	enrollment, ok = s.store.MarkEnrollmentUsed(tokenHash, usedAt)
	if !ok {
		http.Error(w, "enrollment already used", http.StatusConflict)
		return
	}
	s.store.RecordAgentCertificate(store.AgentCertificate{
		TenantID:       defaultString(enrollment.TenantID, "default"),
		AgentID:        enrollment.AgentID,
		EnrollmentID:   enrollment.EnrollmentID,
		SerialNumber:   cert.SerialNumber.String(),
		Subject:        cert.Subject.String(),
		NotBefore:      cert.NotBefore,
		NotAfter:       cert.NotAfter,
		CreatedAt:      usedAt,
		CertificatePEM: string(certPEM),
	})
	if err := s.store.Save(); err != nil {
		http.Error(w, fmt.Sprintf("save store: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"tenant_id":       defaultString(enrollment.TenantID, "default"),
		"agent_id":        enrollment.AgentID,
		"enrollment_id":   enrollment.EnrollmentID,
		"certificate_pem": string(certPEM),
		"ca_pem":          string(s.caCertPEM),
		"serial_number":   cert.SerialNumber.String(),
		"not_after":       cert.NotAfter,
	})
}

func (s *Server) signAgentCSR(enrollment store.Enrollment, csrPEM []byte) ([]byte, *x509.Certificate, error) {
	block, _ := pem.Decode(csrPEM)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return nil, nil, fmt.Errorf("csr must be PEM encoded CERTIFICATE REQUEST")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parse csr: %w", err)
	}
	if err := csr.CheckSignature(); err != nil {
		return nil, nil, fmt.Errorf("verify csr signature: %w", err)
	}
	wantCN := fmt.Sprintf("tenant_id:%s,agent_id:%s", defaultString(enrollment.TenantID, "default"), enrollment.AgentID)
	if csr.Subject.CommonName != wantCN {
		return nil, nil, fmt.Errorf("csr common name %q does not match enrollment %q", csr.Subject.CommonName, wantCN)
	}
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, nil, fmt.Errorf("create serial: %w", err)
	}
	tenantID := defaultString(enrollment.TenantID, "default")
	trustDomain := defaultString(os.Getenv("SYSARMOR_TRUST_DOMAIN"), "sysarmor.local")
	uri, err := url.Parse(fmt.Sprintf("spiffe://%s/tenant/%s/agent/%s", trustDomain, tenantID, enrollment.AgentID))
	if err != nil {
		return nil, nil, fmt.Errorf("build agent uri san: %w", err)
	}
	now := time.Now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: fmt.Sprintf("tenant_id:%s,agent_id:%s", tenantID, enrollment.AgentID),
		},
		NotBefore:             now.Add(-1 * time.Minute),
		NotAfter:              now.Add(90 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		URIs:                  []*url.URL{uri},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, s.caCert, csr.PublicKey, s.caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("sign agent certificate: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, fmt.Errorf("parse signed certificate: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), cert, nil
}

func newEnrollment(req enrollmentRequest, actor string) (store.Enrollment, string, error) {
	if strings.TrimSpace(req.AgentID) == "" {
		return store.Enrollment{}, "", fmt.Errorf("agent_id is required")
	}
	if strings.TrimSpace(req.GatewayAddr) == "" {
		return store.Enrollment{}, "", fmt.Errorf("gateway_addr is required")
	}
	profile, err := normalizeInstallProfile(req.Profile)
	if err != nil {
		return store.Enrollment{}, "", err
	}
	ttl := 24 * time.Hour
	if strings.TrimSpace(req.TTL) != "" {
		parsed, err := time.ParseDuration(req.TTL)
		if err != nil {
			return store.Enrollment{}, "", fmt.Errorf("ttl: %w", err)
		}
		if parsed <= 0 {
			return store.Enrollment{}, "", fmt.Errorf("ttl must be positive")
		}
		ttl = parsed
	}
	token, err := newEnrollmentToken()
	if err != nil {
		return store.Enrollment{}, "", err
	}
	now := time.Now().UTC()
	id := "enr-" + now.Format("20060102T150405Z") + "-" + token[len(token)-8:]
	hostID := req.HostID
	if strings.TrimSpace(hostID) == "" {
		hostID = req.AgentID
	}
	enrollment := store.Enrollment{
		EnrollmentID: id,
		TenantID:     req.TenantID,
		AgentID:      req.AgentID,
		HostID:       hostID,
		TokenHash:    enrollmentTokenHash(token),
		TokenPreview: tokenPreview(token),
		GatewayAddr:  req.GatewayAddr,
		GatewaySNI:   req.GatewaySNI,
		Profile:      profile,
		Channel:      req.Channel,
		ArtifactID:   req.ArtifactID,
		ArtifactURL:  req.ArtifactURL,
		Labels:       cloneStringMap(req.Labels),
		Status:       "active",
		CreatedAt:    now,
		ExpiresAt:    now.Add(ttl),
		CreatedBy:    actor,
	}
	return enrollment, token, nil
}

func normalizeInstallProfile(profile string) (string, error) {
	switch strings.TrimSpace(profile) {
	case "", "linux-systemd":
		return "linux-systemd", nil
	case "linux-container":
		return "linux-container", nil
	default:
		return "", fmt.Errorf("unsupported install profile %q", profile)
	}
}

func newEnrollmentToken() (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate enrollment token: %w", err)
	}
	return "enr_" + base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func enrollmentTokenHash(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

func tokenPreview(token string) string {
	if len(token) <= 12 {
		return token
	}
	return token[:8] + "..." + token[len(token)-4:]
}

func publicEnrollment(enrollment store.Enrollment) store.Enrollment {
	enrollment.TokenHash = ""
	enrollment.Labels = cloneStringMap(enrollment.Labels)
	return enrollment
}

func installURL(r *http.Request, token string) string {
	return absoluteURL(r, "/api/v1/agent-install.sh?token="+token)
}

func artifactDownloadURL(r *http.Request, artifactID string) string {
	return absoluteURL(r, "/api/v1/artifacts/"+artifactID+"/download")
}

func absoluteURL(r *http.Request, path string) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwarded != "" {
		scheme = forwarded
	}
	host := r.Host
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); forwarded != "" {
		host = forwarded
	}
	return fmt.Sprintf("%s://%s%s", scheme, host, path)
}

func (s *Server) renderAgentInstallScript(r *http.Request, enrollment store.Enrollment, token string) string {
	artifactDefault := enrollment.ArtifactURL
	if enrollment.ArtifactID != "" {
		artifactDefault = absoluteURL(r, "/api/v1/enrollment-artifact?token="+url.QueryEscape(token))
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
mkdir -p "$AGENT_HOME/bin" "$AGENT_HOME/runtime" "$AGENT_HOME/cache" "$(dirname "$CONFIG_DST")" /var/lib/sysarmor/agent
%s
curl -fsSL "$SYSARMOR_AGENT_BUNDLE_URL" -o "$tmp/sysarmor-agent.tar.gz"
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
"$AGENT_HOME/bin/sysarmorctl" --socket /run/sysarmor/agent/control.sock --manager-url %s enroll \
  --token %s --tenant %s --agent-id %s --gateway %s --gateway-server-name %s
`, artifactLine, artifactSHA, profile, dependencies, publicKey, manifestCheck, manifestEnv, serviceInstall, labels, scopeConfig, lifecycle, shellQuote(managerURL), shellQuote(token), shellQuote(defaultString(enrollment.TenantID, "default")), shellQuote(enrollment.AgentID), shellQuote(enrollment.GatewayAddr), shellQuote(enrollment.GatewaySNI))
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
