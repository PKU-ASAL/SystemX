package postgres

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sysarmor/sysarmor-next-project/internal/store"
)

func TestPostgresRevokeAgentCertificateMatrix(t *testing.T) {
	certificate := store.AgentCertificate{TenantID: "tenant-a", AgentID: "agent-a", EnrollmentID: "enroll-a", SerialNumber: "42"}
	revokedAt := time.Date(2026, 8, 4, 1, 2, 3, 0, time.UTC)

	t.Run("first revoke", func(t *testing.T) {
		backend := revocationBackend(t, certificate, nil)
		got, found, err := backend.RevokeAgentCertificate(t.Context(), "tenant-a", "agent-a", "enroll-a", "42", revokedAt, "receipt-a")
		if err != nil || !found || got.RevocationReceipt != "receipt-a" || !got.RevokedAt.Equal(revokedAt) {
			t.Fatalf("certificate=%+v found=%t err=%v", got, found, err)
		}
		if queries := FakeAllQueries(); !strings.Contains(queries, "agent_unenrollments") {
			t.Fatalf("revocation did not persist legacy lifecycle: %s", queries)
		}
		if commits, rollbacks := FakeTransactionCounts(); commits != 1 || rollbacks != 0 {
			t.Fatalf("commits=%d rollbacks=%d", commits, rollbacks)
		}
	})

	t.Run("identical replay", func(t *testing.T) {
		certificate.RevokedAt = revokedAt
		certificate.RevocationReceipt = "receipt-a"
		backend := revocationBackend(t, certificate, nil)
		got, found, err := backend.RevokeAgentCertificate(t.Context(), "tenant-a", "agent-a", "enroll-a", "42", revokedAt.Add(time.Hour), "receipt-other")
		if err != nil || !found || got.RevocationReceipt != "receipt-a" || !got.RevokedAt.Equal(revokedAt) {
			t.Fatalf("certificate=%+v found=%t err=%v", got, found, err)
		}
	})

	t.Run("identity conflict", func(t *testing.T) {
		backend := revocationBackend(t, certificate, nil)
		_, _, err := backend.RevokeAgentCertificate(t.Context(), "tenant-a", "agent-other", "enroll-a", "42", revokedAt, "receipt-a")
		if !errors.Is(err, store.ErrConflict) {
			t.Fatalf("error=%v, want ErrConflict", err)
		}
		if commits, rollbacks := FakeTransactionCounts(); commits != 0 || rollbacks != 1 {
			t.Fatalf("commits=%d rollbacks=%d", commits, rollbacks)
		}
	})

	t.Run("read failure rolls back", func(t *testing.T) {
		backend := revocationBackend(t, certificate, errors.New("postgres read failed"))
		if _, _, err := backend.RevokeAgentCertificate(t.Context(), "tenant-a", "agent-a", "enroll-a", "42", revokedAt, "receipt-a"); err == nil {
			t.Fatal("error=nil, want query failure")
		}
		if commits, rollbacks := FakeTransactionCounts(); commits != 0 || rollbacks != 1 {
			t.Fatalf("commits=%d rollbacks=%d", commits, rollbacks)
		}
	})
}

func revocationBackend(t *testing.T, certificate store.AgentCertificate, queryErr error) *tableBackend {
	t.Helper()
	raw, err := json.Marshal(certificate)
	if err != nil {
		t.Fatal(err)
	}
	FakeSetQueryResult([][]byte{raw}, queryErr)
	db, err := sql.Open(FakeDriverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &tableBackend{db: db}
}
