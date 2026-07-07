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
			enrollment.ArtifactURL = artifactDownloadURL(r, artifact.ArtifactID)
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
		Profile:      req.Profile,
		Channel:      req.Channel,
		ArtifactID:   req.ArtifactID,
		ArtifactURL:  req.ArtifactURL,
		Labels:       cloneStringMap(req.Labels),
		Status:       "active",
		CreatedAt:    now,
		ExpiresAt:    now.Add(ttl),
		CreatedBy:    actor,
	}
	if enrollment.Profile == "" {
		enrollment.Profile = "linux-tetragon"
	}
	return enrollment, token, nil
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
	artifactLine := fmt.Sprintf("SYSARMOR_AGENT_BUNDLE_URL=\"${SYSARMOR_AGENT_BUNDLE_URL:-}\"\nif [[ -z \"$SYSARMOR_AGENT_BUNDLE_URL\" ]]; then\n  SYSARMOR_AGENT_BUNDLE_URL=%s\nfi\n", shellQuote(artifactDefault))
	if artifactDefault == "" {
		artifactLine = ": \"${SYSARMOR_AGENT_BUNDLE_URL:?set SYSARMOR_AGENT_BUNDLE_URL to a sysarmor-agent tarball URL}\"\n"
	}
	artifactSHA := enrollment.ArtifactSHA256
	labels := renderAgentLabelConfig(enrollment.Labels)
	publicKey := string(s.artifactPub)
	return fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail

%s
SYSARMOR_AGENT_BUNDLE_SHA256="${SYSARMOR_AGENT_BUNDLE_SHA256:-%s}"
SYSARMOR_ENROLLMENT_CERT_URL="${SYSARMOR_ENROLLMENT_CERT_URL:-%s}"
AGENT_HOME="${SYSARMOR_AGENT_HOME:-/opt/sysarmor/agent}"
CONFIG_DST="${SYSARMOR_CONFIG_DST:-/etc/sysarmor/agent.yaml}"
SERVICE_DST="${SYSARMOR_SERVICE_DST:-/etc/systemd/system/sysarmor-agent.service}"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$AGENT_HOME/bin" "$AGENT_HOME/runtime" "$AGENT_HOME/cache" /etc/sysarmor/policies /etc/sysarmor/pki /run/sysarmor "$(dirname "$CONFIG_DST")"
if command -v apt-get >/dev/null 2>&1; then
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -y >/dev/null
  apt-get install -y ca-certificates curl openssl python3 >/dev/null
fi
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
if [[ -s "$tmp/artifact-public.pem" ]]; then
  openssl dgst -sha256 -verify "$tmp/artifact-public.pem" -signature "$tmp/manifest.sig" "$tmp/manifest.json" >/dev/null
fi
python3 - "$tmp" <<'PY'
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
PY
eval "$(python3 - "$tmp/manifest.json" <<'PY'
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
)"
if [[ -z "$DIST_ENTRYPOINT" || -z "$DIST_SYSTEMD_UNIT" ]]; then
  echo "[sysarmor-enroll][ERROR] manifest entrypoint/systemd_unit missing" >&2
  exit 1
fi
install -m 0755 "$tmp/$DIST_ENTRYPOINT" "$AGENT_HOME/bin/sysarmor-agent"
install -m 0644 "$tmp/$DIST_SYSTEMD_UNIT" "$SERVICE_DST"
if [[ -n "$DIST_SENSOR_BUNDLE" ]]; then
  mkdir -p "$AGENT_HOME/bundles" "$AGENT_HOME/$DIST_SENSOR_INSTALL_DIR"
  rm -rf "$AGENT_HOME/bundles/$DIST_SENSOR_NAME"
  cp -a "$tmp/$DIST_SENSOR_BUNDLE" "$AGENT_HOME/bundles/$DIST_SENSOR_NAME"
fi
openssl genrsa -out /etc/sysarmor/pki/agent-key.pem 2048 >/dev/null 2>&1
chmod 0600 /etc/sysarmor/pki/agent-key.pem
openssl req -new -key /etc/sysarmor/pki/agent-key.pem \
  -subj "/CN=tenant_id:%s,agent_id:%s" \
  -out "$tmp/agent.csr" >/dev/null 2>&1
python3 - "$SYSARMOR_ENROLLMENT_CERT_URL" "$tmp/agent.csr" %s > "$tmp/cert-response.json" <<'PY'
import json, sys, urllib.request
url, csr_path, token = sys.argv[1], sys.argv[2], sys.argv[3]
payload = json.dumps({"token": token, "csr": open(csr_path).read()}).encode()
req = urllib.request.Request(url, data=payload, headers={"Content-Type": "application/json"})
with urllib.request.urlopen(req, timeout=30) as resp:
    sys.stdout.write(resp.read().decode())
PY
python3 - "$tmp/cert-response.json" <<'PY'
import json, sys
data = json.load(open(sys.argv[1]))
open("/etc/sysarmor/pki/agent.pem", "w").write(data["certificate_pem"])
open("/etc/sysarmor/pki/ca.pem", "w").write(data["ca_pem"])
PY
chmod 0644 /etc/sysarmor/pki/agent.pem /etc/sysarmor/pki/ca.pem

cat > "$CONFIG_DST" <<'YAML'
agent:
  id: %s
  host_id: %s
  tenant_id: %s
  token: %s
%s
manager:
  address: %s
  transport: grpc
  tls_ca: "/etc/sysarmor/pki/ca.pem"
  tls_cert: "/etc/sysarmor/pki/agent.pem"
  tls_key: "/etc/sysarmor/pki/agent-key.pem"
  tls_server_name: %s
  tls_insecure: false

control:
  socket_path: /run/sysarmor/agent.sock

sensor:
  backend: tetragon
  mode: managed
  bundle_dir: /opt/sysarmor/agent/bundles/tetragon
  install_dir: /opt/sysarmor/agent/sensors
  policy_path: /etc/sysarmor/policies/sysarmor-tetragon.yaml
  observe_only: true
YAML

systemctl daemon-reload
systemctl enable --now sysarmor-agent
echo "[sysarmor-enroll] installed sysarmor-agent enrollment=%s"
`, artifactLine, artifactSHA, absoluteURL(r, "/api/v1/enrollment-certificate"), publicKey, defaultString(enrollment.TenantID, "default"), enrollment.AgentID, shellQuote(token), yamlQuote(enrollment.AgentID), yamlQuote(enrollment.HostID), yamlQuote(defaultString(enrollment.TenantID, "default")), yamlQuote(token), labels, yamlQuote(enrollment.GatewayAddr), yamlQuote(enrollment.GatewaySNI), enrollment.EnrollmentID)
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
