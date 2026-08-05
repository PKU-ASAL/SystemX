package managerapi

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sysarmor/sysarmor-next-project/internal/store"
)

func TestUnenrollmentCompletionIsTokenAuthenticatedAndIdempotent(t *testing.T) {
	st := &store.Store{}
	st.RecordAgentCertificate(store.AgentCertificate{
		TenantID: "tenant-a", AgentID: "agent-a", EnrollmentID: "enroll-a", SerialNumber: "42",
	})
	token := "completion-token-a"
	tokenHash := sha256.Sum256([]byte(token))
	record, ok, err := st.AuthorizeAgentUnenrollment("tenant-a", "agent-a", "enroll-a", "42", hex.EncodeToString(tokenHash[:]), time.Unix(100, 0).UTC())
	if err != nil || !ok {
		t.Fatalf("authorize=%+v ok=%t err=%v", record, ok, err)
	}
	handler := newTestServer(st).Handler()

	for attempt := 0; attempt < 2; attempt++ {
		rec := postCompletion(t, handler, token, record.RevocationReceipt)
		if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), token) || strings.Contains(rec.Body.String(), hex.EncodeToString(tokenHash[:])) {
			t.Fatalf("attempt=%d status=%d body=%s", attempt, rec.Code, rec.Body.String())
		}
	}
	completed, ok, err := st.GetUnenrollmentWithError("tenant-a", "enroll-a")
	if err != nil || !ok || completed.Status != store.UnenrollmentEndpointCompleted || completed.EndpointCompletedAt.IsZero() {
		t.Fatalf("completed=%+v ok=%t err=%v", completed, ok, err)
	}
}

func TestUnenrollmentCompletionRejectsWrongTokenWithoutDetail(t *testing.T) {
	st := &store.Store{}
	st.RecordAgentCertificate(store.AgentCertificate{
		TenantID: "tenant-a", AgentID: "agent-a", EnrollmentID: "enroll-a", SerialNumber: "42",
	})
	tokenHash := sha256.Sum256([]byte("correct-token"))
	record, ok, err := st.AuthorizeAgentUnenrollment("tenant-a", "agent-a", "enroll-a", "42", hex.EncodeToString(tokenHash[:]), time.Unix(100, 0).UTC())
	if err != nil || !ok {
		t.Fatal(err)
	}
	rec := postCompletion(t, newTestServer(st).Handler(), "wrong-token", record.RevocationReceipt)
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "Unauthorized") || strings.Contains(rec.Body.String(), "token") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestUnenrollmentCompletionRejectsOversizedAnonymousBody(t *testing.T) {
	handler := newTestServer(&store.Store{}).Handler()
	body := strings.Repeat(" ", maxUnenrollmentCompletionBody+1)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/unenrollment-completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func postCompletion(t *testing.T, handler http.Handler, token, receipt string) *httptest.ResponseRecorder {
	t.Helper()
	body := fmt.Sprintf(`{"schema_version":"sysarmor.unenrollment-completion/v1","tenant_id":"tenant-a","agent_id":"agent-a","enrollment_id":"enroll-a","certificate_serial":"42","revocation_receipt":%q,"completion_token":%q}`, receipt, token)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/unenrollment-completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
