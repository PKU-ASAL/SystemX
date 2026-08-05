package gateway_test

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/sysarmor/sysarmor-next-project/internal/gateway"
	"github.com/sysarmor/sysarmor-next-project/internal/store"
	controlplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/controlplane/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

func TestRevokeModernEnrollmentRejectsLegacyDowngrade(t *testing.T) {
	caKey, caCert := newTestCA(t)
	identityURI := &url.URL{Scheme: "spiffe", Host: "sysarmor.local", Path: "/tenant/tenant-a/agent/agent-a"}
	clientCert, _ := newTestCert(t, caCert, caKey, "tenant-a/agent-a", []*url.URL{identityURI}, nil)
	var certificate store.AgentCertificate
	raw := `{"tenant_id":"tenant-a","agent_id":"agent-a","enrollment_id":"enroll-a","serial_number":"` +
		clientCert.SerialNumber.String() + `","unenrollment_protocol":"completion_v1"}`
	if err := json.Unmarshal([]byte(raw), &certificate); err != nil {
		t.Fatal(err)
	}
	st := &store.Store{}
	st.RecordAgentCertificate(certificate)
	server := gateway.NewControlServer(gateway.NewRuntime(gateway.RuntimeOptions{Store: st}))
	ctx := peer.NewContext(t.Context(), &peer.Peer{AuthInfo: credentials.TLSInfo{
		State: tls.ConnectionState{PeerCertificates: []*x509.Certificate{clientCert}},
	}})

	request := &controlplanev1.RevokeEnrollmentRequest{TenantId: "tenant-a", AgentId: "agent-a"}
	if _, err := server.RevokeEnrollment(ctx, request); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("RevokeEnrollment() error=%v, want PermissionDenied", err)
	}
}

func TestRevokeLegacyEnrollmentUsesMTLSPeerIdentity(t *testing.T) {
	caKey, caCert := newTestCA(t)
	identityURI := &url.URL{Scheme: "spiffe", Host: "sysarmor.local", Path: "/tenant/tenant-a/agent/agent-a"}
	clientCert, _ := newTestCert(t, caCert, caKey, "tenant-a/agent-a", []*url.URL{identityURI}, nil)
	serial := clientCert.SerialNumber.String()
	st := &store.Store{}
	st.RecordAgentCertificate(store.AgentCertificate{TenantID: "tenant-a", AgentID: "agent-a", EnrollmentID: "enroll-a", SerialNumber: serial})
	server := gateway.NewControlServer(gateway.NewRuntime(gateway.RuntimeOptions{Store: st}))
	ctx := peer.NewContext(t.Context(), &peer.Peer{AuthInfo: credentials.TLSInfo{
		State: tls.ConnectionState{PeerCertificates: []*x509.Certificate{clientCert}},
	}})
	for name, request := range map[string]*controlplanev1.RevokeEnrollmentRequest{
		"serial mismatch":     {TenantId: "tenant-a", AgentId: "agent-a", CertificateSerial: "999"},
		"enrollment mismatch": {TenantId: "tenant-a", AgentId: "agent-a", EnrollmentId: "enroll-other"},
		"agent mismatch":      {TenantId: "tenant-a", AgentId: "agent-other"},
		"completion upgrade":  {TenantId: "tenant-a", AgentId: "agent-a", CompletionTokenHash: strings.Repeat("a", 64)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := server.RevokeEnrollment(ctx, request); status.Code(err) != codes.PermissionDenied {
				t.Fatalf("RevokeEnrollment() error=%v, want PermissionDenied", err)
			}
		})
	}

	response, err := server.RevokeEnrollment(ctx, &controlplanev1.RevokeEnrollmentRequest{TenantId: "tenant-a", AgentId: "agent-a"})
	if err != nil {
		t.Fatal(err)
	}
	record, ok, readErr := st.GetUnenrollmentWithError("tenant-a", "enroll-a")
	if response.GetCompletionRequired() || response.GetReceiptId() == "" || readErr != nil || !ok || record.Status != store.UnenrollmentUnknownLegacy {
		t.Fatalf("response=%+v record=%+v ok=%t readErr=%v", response, record, ok, readErr)
	}
}
