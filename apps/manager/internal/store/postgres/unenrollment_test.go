package postgres

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sysarmor/sysarmor-next-project/apps/manager/internal/store"
)

func TestPostgresAuthorizeAgentUnenrollmentCommitsCertificateAndRecord(t *testing.T) {
	backend := unenrollmentBackend(t, nil)
	record, found, err := authorizePostgresUnenrollment(t, backend)
	if err != nil || !found || record.Status != store.UnenrollmentRevokedEndpointPending || record.RevocationReceipt != "receipt-a" {
		t.Fatalf("record=%+v found=%t err=%v", record, found, err)
	}
	if commits, rollbacks := FakeTransactionCounts(); commits != 1 || rollbacks != 0 {
		t.Fatalf("commits=%d rollbacks=%d", commits, rollbacks)
	}
	if queries := FakeAllQueries(); !strings.Contains(queries, "agent_certificates") || !strings.Contains(queries, "agent_unenrollments") {
		t.Fatalf("transaction queries=%s", queries)
	}
}

func TestPostgresAuthorizeAgentUnenrollmentRollsBackSecondWriteFailure(t *testing.T) {
	backend := unenrollmentBackend(t, nil)
	FakeSetExecErrorAt(2, errors.New("unenrollment insert failed"))
	if _, _, err := authorizePostgresUnenrollment(t, backend); err == nil {
		t.Fatal("authorization error=nil, want second write failure")
	}
	if commits, rollbacks := FakeTransactionCounts(); commits != 0 || rollbacks != 1 {
		t.Fatalf("commits=%d rollbacks=%d", commits, rollbacks)
	}
}

func TestPostgresCompleteAgentUnenrollmentValidatesTokenAndCommits(t *testing.T) {
	record := store.UnenrollmentRecord{
		TenantID: "tenant-a", AgentID: "agent-a", EnrollmentID: "enroll-a", CertificateSerial: "42",
		RevocationReceipt: "receipt-a", CompletionTokenHash: strings.Repeat("a", 64),
		Status: store.UnenrollmentRevokedEndpointPending, RevokedAt: time.Unix(100, 0).UTC(),
		CreatedAt: time.Unix(100, 0).UTC(), UpdatedAt: time.Unix(100, 0).UTC(),
	}
	backend := completionBackend(t, record)
	if _, _, err := backend.CompleteAgentUnenrollment(t.Context(), "tenant-a", "agent-a", "enroll-a", "42",
		"receipt-a", strings.Repeat("b", 64), time.Unix(200, 0).UTC()); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("conflict error=%v, want ErrConflict", err)
	}
	if commits, rollbacks := FakeTransactionCounts(); commits != 0 || rollbacks != 1 {
		t.Fatalf("conflict commits=%d rollbacks=%d", commits, rollbacks)
	}

	backend = completionBackend(t, record)
	completed, found, err := backend.CompleteAgentUnenrollment(t.Context(), "tenant-a", "agent-a", "enroll-a", "42",
		"receipt-a", strings.Repeat("a", 64), time.Unix(200, 0).UTC())
	if err != nil || !found || completed.Status != store.UnenrollmentEndpointCompleted || !completed.EndpointCompletedAt.Equal(time.Unix(200, 0).UTC()) {
		t.Fatalf("completed=%+v found=%t err=%v", completed, found, err)
	}
	if commits, rollbacks := FakeTransactionCounts(); commits != 1 || rollbacks != 0 {
		t.Fatalf("completion commits=%d rollbacks=%d", commits, rollbacks)
	}
}

func unenrollmentBackend(t *testing.T, queryErr error) *tableBackend {
	t.Helper()
	setUnenrollmentCertificateQuery(t)
	if queryErr != nil {
		FakeSetQueryValues(nil, queryErr)
	}
	db, err := sql.Open(FakeDriverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &tableBackend{db: db}
}

func setUnenrollmentCertificateQuery(t *testing.T) {
	t.Helper()
	raw, err := json.Marshal(store.AgentCertificate{
		TenantID: "tenant-a", AgentID: "agent-a", EnrollmentID: "enroll-a", SerialNumber: "42",
	})
	if err != nil {
		t.Fatal(err)
	}
	FakeSetQueryValues([][]driver.Value{{raw, nil}}, nil)
}

func completionBackend(t *testing.T, record store.UnenrollmentRecord) *tableBackend {
	t.Helper()
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	FakeSetQueryValues([][]driver.Value{{raw}}, nil)
	db, err := sql.Open(FakeDriverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &tableBackend{db: db}
}

func authorizePostgresUnenrollment(t *testing.T, backend *tableBackend) (store.UnenrollmentRecord, bool, error) {
	t.Helper()
	_, record, found, err := backend.AuthorizeAgentUnenrollment(t.Context(), "tenant-a", "agent-a", "enroll-a", "42",
		strings.Repeat("a", 64), time.Unix(100, 0).UTC(), "receipt-a")
	return record, found, err
}
