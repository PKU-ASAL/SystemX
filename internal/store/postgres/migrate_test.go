package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
)

func TestApplyMigrationsExecutesPostgresSchema(t *testing.T) {
	db := openFakeDB(t, nil)
	got, err := ApplyMigrations(context.Background(), db)
	if err != nil {
		t.Fatalf("ApplyMigrations() error = %v", err)
	}
	if got.Version != 1 {
		t.Fatalf("migration version = %d, want 1", got.Version)
	}
	query := fakeSQLLastQuery()
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS agents",
		"CREATE TABLE IF NOT EXISTS incidents",
		"CREATE TABLE IF NOT EXISTS response_audit",
		"INSERT INTO schema_migrations (version) VALUES (1)",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("migration query missing %q: %s", want, query)
		}
	}
}

func TestApplyMigrationsRejectsNilDB(t *testing.T) {
	if _, err := ApplyMigrations(context.Background(), nil); err == nil {
		t.Fatal("ApplyMigrations(nil) error = nil")
	}
}

func TestApplyMigrationsWrapsExecError(t *testing.T) {
	db := openFakeDB(t, errors.New("boom"))
	if _, err := ApplyMigrations(context.Background(), db); err == nil || !strings.Contains(err.Error(), "apply postgres schema v1") {
		t.Fatalf("ApplyMigrations() error = %v", err)
	}
}

func openFakeDB(t *testing.T, execErr error) *sql.DB {
	t.Helper()
	fakeSQLSetExecError(execErr)
	db, err := sql.Open("sysarmor-postgres-migrate-test", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func init() {
	sql.Register("sysarmor-postgres-migrate-test", fakeSQLDriver{})
}

var fakeSQLState struct {
	sync.Mutex
	lastQuery string
	execErr   error
}

func fakeSQLSetExecError(err error) {
	fakeSQLState.Lock()
	defer fakeSQLState.Unlock()
	fakeSQLState.lastQuery = ""
	fakeSQLState.execErr = err
}

func fakeSQLLastQuery() string {
	fakeSQLState.Lock()
	defer fakeSQLState.Unlock()
	return fakeSQLState.lastQuery
}

type fakeSQLDriver struct{}

func (fakeSQLDriver) Open(string) (driver.Conn, error) {
	return fakeSQLConn{}, nil
}

type fakeSQLConn struct{}

func (fakeSQLConn) Prepare(query string) (driver.Stmt, error) {
	return fakeSQLStmt{query: query}, nil
}

func (fakeSQLConn) Close() error {
	return nil
}

func (fakeSQLConn) Begin() (driver.Tx, error) {
	return fakeSQLTx{}, nil
}

type fakeSQLStmt struct {
	query string
}

func (s fakeSQLStmt) Close() error {
	return nil
}

func (s fakeSQLStmt) NumInput() int {
	return -1
}

func (s fakeSQLStmt) Exec([]driver.Value) (driver.Result, error) {
	fakeSQLState.Lock()
	defer fakeSQLState.Unlock()
	fakeSQLState.lastQuery = s.query
	if fakeSQLState.execErr != nil {
		return nil, fakeSQLState.execErr
	}
	return driver.RowsAffected(1), nil
}

func (s fakeSQLStmt) Query([]driver.Value) (driver.Rows, error) {
	return fakeSQLRows{}, nil
}

type fakeSQLTx struct{}

func (fakeSQLTx) Commit() error {
	return nil
}

func (fakeSQLTx) Rollback() error {
	return nil
}

type fakeSQLRows struct{}

func (fakeSQLRows) Columns() []string {
	return nil
}

func (fakeSQLRows) Close() error {
	return nil
}

func (fakeSQLRows) Next([]driver.Value) error {
	return io.EOF
}
