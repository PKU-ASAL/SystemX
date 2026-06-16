package backend

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	incidentv1 "github.com/sysarmor/sysarmor-next-project/api/proto/incident/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
	responsemodel "github.com/sysarmor/sysarmor-next-project/internal/response"
)

func TestOpenFileAndMemoryBackends(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "store.json")
	file, err := Open(context.Background(), Options{Kind: KindFile, Path: filePath})
	if err != nil {
		t.Fatalf("Open(file) error = %v", err)
	}
	if file.Store == nil || file.Store.Info().Backend != "file" || file.Store.Info().Path != filePath {
		t.Fatalf("file result = %+v", file.Store.Info())
	}

	memory, err := Open(context.Background(), Options{Kind: KindMemory})
	if err != nil {
		t.Fatalf("Open(memory) error = %v", err)
	}
	if memory.Store == nil || memory.Store.Info().Backend != "memory" {
		t.Fatalf("memory result = %+v", memory.Store.Info())
	}
}

func TestOpenPostgresRunsMigrationAndPersistsSnapshot(t *testing.T) {
	fakeSetExecError(nil)
	fakeSetSnapshot(nil)
	result, err := Open(context.Background(), Options{
		Kind:           KindPostgres,
		PostgresDriver: fakeDriverName,
		PostgresDSN:    "test-dsn",
	})
	if err != nil {
		t.Fatalf("Open(postgres) error = %v", err)
	}
	if result.Migration.Version != 1 {
		t.Fatalf("migration version = %d, want 1", result.Migration.Version)
	}
	if result.Store == nil || result.Store.Info().Backend != KindPostgres {
		t.Fatalf("store info = %+v", result.Store.Info())
	}
	if !strings.Contains(fakeLastQuery(), "CREATE TABLE IF NOT EXISTS incidents") {
		t.Fatalf("postgres migration did not run: %s", fakeLastQuery())
	}
	result.Store.CreateResponse(responsemodel.Command{
		ResponseID: "resp-pg",
		TenantID:   "default",
		AgentID:    "agent-pg",
		Action:     "collect",
		Mode:       "observe",
	})
	if err := result.Store.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	reopened, err := Open(context.Background(), Options{
		Kind:           KindPostgres,
		PostgresDriver: fakeDriverName,
		PostgresDSN:    "test-dsn",
	})
	if err != nil {
		t.Fatalf("reopen postgres error = %v", err)
	}
	audits := reopened.Store.ListResponses("default", "agent-pg")
	if len(audits) != 1 || audits[0].Command.ResponseID != "resp-pg" {
		t.Fatalf("reopened audits = %+v", audits)
	}
}

func TestOpenPostgresPreservesIdempotentIngestAcrossReopen(t *testing.T) {
	fakeSetExecError(nil)
	fakeSetSnapshot(nil)
	result, err := Open(context.Background(), Options{
		Kind:           KindPostgres,
		PostgresDriver: fakeDriverName,
		PostgresDSN:    "test-dsn",
	})
	if err != nil {
		t.Fatalf("Open(postgres) error = %v", err)
	}
	event := &eventv1.CanonicalEvent{Id: "ev-pg-idempotent", Scenario: "pg-idempotent", Kind: eventv1.EventKind_EVENT_KIND_EXEC}
	signal := &signalv1.Signal{Id: "sig-pg-idempotent", Scenario: "pg-idempotent", Name: "reverse_shell_pattern", Where: signalv1.SignalWhere_SIGNAL_WHERE_ENDPOINT}
	if !result.Store.AddEvent(event) || result.Store.AddEvent(event) {
		t.Fatal("event idempotency failed before save")
	}
	if !result.Store.AddSignal(signal) || result.Store.AddSignal(signal) {
		t.Fatal("signal idempotency failed before save")
	}
	if err := result.Store.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	reopened, err := Open(context.Background(), Options{
		Kind:           KindPostgres,
		PostgresDriver: fakeDriverName,
		PostgresDSN:    "test-dsn",
	})
	if err != nil {
		t.Fatalf("reopen postgres error = %v", err)
	}
	if reopened.Store.AddEvent(event) {
		t.Fatal("duplicate event inserted after postgres reopen")
	}
	if reopened.Store.AddSignal(signal) {
		t.Fatal("duplicate signal inserted after postgres reopen")
	}
	if got := reopened.Store.ListEvents("pg-idempotent", ""); len(got) != 1 || got[0].GetId() != event.GetId() {
		t.Fatalf("events after duplicate replay = %+v", got)
	}
	if got := reopened.Store.ListSignals("pg-idempotent", "endpoint", false); len(got) != 1 || got[0].GetId() != signal.GetId() {
		t.Fatalf("signals after duplicate replay = %+v", got)
	}
}

func TestOpenPostgresPersistsPolicyAndIncidentStateAcrossReopen(t *testing.T) {
	fakeSetExecError(nil)
	fakeSetSnapshot(nil)
	result, err := Open(context.Background(), Options{
		Kind:           KindPostgres,
		PostgresDriver: fakeDriverName,
		PostgresDSN:    "test-dsn",
	})
	if err != nil {
		t.Fatalf("Open(postgres) error = %v", err)
	}
	policy := policymodel.DefaultPolicy("default")
	policy.PolicyID = "postgres-policy"
	policy.Version = 7
	policy.Published = false
	result.Store.UpsertPolicy(policy)
	published, ok := result.Store.PublishPolicy("default", "postgres-policy", 7, true)
	if !ok || !published.Published {
		t.Fatalf("PublishPolicy() = %+v, %v", published, ok)
	}
	assignment, ok := result.Store.AssignPolicy(policymodel.Assignment{
		TenantID:      "default",
		AgentID:       "agent-pg-policy",
		PolicyID:      "postgres-policy",
		PolicyVersion: 7,
	})
	if !ok {
		t.Fatal("AssignPolicy() ok = false")
	}
	result.Store.RecordPolicyAudit(policymodel.AuditRecord{
		TenantID:      "default",
		Action:        "policy.assign",
		PolicyID:      "postgres-policy",
		PolicyVersion: 7,
		AssignmentID:  assignment.AssignmentID,
		Actor:         "tester",
	})
	result.Store.AddIncident(&incidentv1.Incident{Id: "inc-pg", Scenario: "pg-policy", Summary: "persisted incident"})
	if _, ok := result.Store.UpdateIncidentStatus("inc-pg", "", "suppressed", "known test", "tester"); !ok {
		t.Fatal("UpdateIncidentStatus() ok = false")
	}
	if err := result.Store.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	reopened, err := Open(context.Background(), Options{
		Kind:           KindPostgres,
		PostgresDriver: fakeDriverName,
		PostgresDSN:    "test-dsn",
	})
	if err != nil {
		t.Fatalf("reopen postgres error = %v", err)
	}
	effective, ok := reopened.Store.EffectivePolicy("default", "agent-pg-policy", "", "")
	if !ok || effective.PolicyID != "postgres-policy" || effective.Version != 7 || !effective.Published {
		t.Fatalf("effective policy after reopen = %+v, %v", effective, ok)
	}
	audits := reopened.Store.ListPolicyAudits("default", "postgres-policy")
	if len(audits) != 1 || audits[0].Actor != "tester" || audits[0].AssignmentID != assignment.AssignmentID {
		t.Fatalf("policy audits after reopen = %+v", audits)
	}
	incidents := reopened.Store.ListIncidents("pg-policy")
	if len(incidents) != 1 || incidents[0].GetStatus() != "suppressed" || incidents[0].GetStatusActor() != "tester" {
		t.Fatalf("incidents after reopen = %+v", incidents)
	}
}

func TestOpenPostgresValidatesConfigAndWrapsMigrationError(t *testing.T) {
	if _, err := Open(context.Background(), Options{Kind: KindPostgres}); err == nil || !strings.Contains(err.Error(), "postgres driver is required") {
		t.Fatalf("missing driver error = %v", err)
	}
	if _, err := Open(context.Background(), Options{Kind: KindPostgres, PostgresDriver: fakeDriverName}); err == nil || !strings.Contains(err.Error(), "postgres dsn is required") {
		t.Fatalf("missing dsn error = %v", err)
	}
	fakeSetExecError(errors.New("boom"))
	if _, err := Open(context.Background(), Options{Kind: KindPostgres, PostgresDriver: fakeDriverName, PostgresDSN: "test-dsn"}); err == nil || !strings.Contains(err.Error(), "apply postgres schema v1") {
		t.Fatalf("migration error = %v", err)
	}
}

func TestOpenRejectsUnknownBackend(t *testing.T) {
	if _, err := Open(context.Background(), Options{Kind: "other"}); err == nil || !strings.Contains(err.Error(), "unknown store backend") {
		t.Fatalf("unknown backend error = %v", err)
	}
}

const fakeDriverName = "sysarmor-backend-postgres-test"

func init() {
	sql.Register(fakeDriverName, fakeDriver{})
}

var fakeState struct {
	sync.Mutex
	lastQuery string
	execErr   error
	snapshot  []byte
}

func fakeSetExecError(err error) {
	fakeState.Lock()
	defer fakeState.Unlock()
	fakeState.lastQuery = ""
	fakeState.execErr = err
}

func fakeSetSnapshot(data []byte) {
	fakeState.Lock()
	defer fakeState.Unlock()
	fakeState.snapshot = data
}

func fakeLastQuery() string {
	fakeState.Lock()
	defer fakeState.Unlock()
	return fakeState.lastQuery
}

type fakeDriver struct{}

func (fakeDriver) Open(string) (driver.Conn, error) {
	return fakeConn{}, nil
}

type fakeConn struct{}

func (fakeConn) Prepare(query string) (driver.Stmt, error) {
	return fakeStmt{query: query}, nil
}

func (fakeConn) Close() error {
	return nil
}

func (fakeConn) Begin() (driver.Tx, error) {
	return fakeTx{}, nil
}

type fakeStmt struct {
	query string
}

func (s fakeStmt) Close() error {
	return nil
}

func (s fakeStmt) NumInput() int {
	return -1
}

func (s fakeStmt) Exec([]driver.Value) (driver.Result, error) {
	return s.ExecContext(context.Background(), nil)
}

func (s fakeStmt) ExecContext(_ context.Context, args []driver.NamedValue) (driver.Result, error) {
	fakeState.Lock()
	defer fakeState.Unlock()
	fakeState.lastQuery = s.query
	if fakeState.execErr != nil {
		return nil, fakeState.execErr
	}
	if strings.Contains(s.query, "INSERT INTO sysarmor_state") && len(args) >= 3 {
		switch data := args[2].Value.(type) {
		case []byte:
			fakeState.snapshot = append([]byte(nil), data...)
		case string:
			fakeState.snapshot = []byte(data)
		}
	}
	return driver.RowsAffected(1), nil
}

func (s fakeStmt) Query([]driver.Value) (driver.Rows, error) {
	return s.QueryContext(context.Background(), nil)
}

func (s fakeStmt) QueryContext(context.Context, []driver.NamedValue) (driver.Rows, error) {
	fakeState.Lock()
	defer fakeState.Unlock()
	if strings.Contains(s.query, "SELECT data FROM sysarmor_state") && len(fakeState.snapshot) > 0 {
		return &fakeRows{cols: []string{"data"}, values: []driver.Value{append([]byte(nil), fakeState.snapshot...)}}, nil
	}
	return &fakeRows{}, nil
}

type fakeTx struct{}

func (fakeTx) Commit() error {
	return nil
}

func (fakeTx) Rollback() error {
	return nil
}

type fakeRows struct {
	cols   []string
	values []driver.Value
	done   bool
}

func (r *fakeRows) Columns() []string {
	return r.cols
}

func (*fakeRows) Close() error {
	return nil
}

func (r *fakeRows) Next(dest []driver.Value) error {
	if r.done || len(r.values) == 0 {
		return io.EOF
	}
	r.done = true
	copy(dest, r.values)
	return nil
}
