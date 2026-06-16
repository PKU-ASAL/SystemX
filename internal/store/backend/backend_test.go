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

func TestOpenPostgresRunsMigrationBeforeAdapterError(t *testing.T) {
	fakeSetExecError(nil)
	result, err := Open(context.Background(), Options{
		Kind:           KindPostgres,
		PostgresDriver: fakeDriverName,
		PostgresDSN:    "test-dsn",
	})
	if !errors.Is(err, ErrPostgresAdapterNotImplemented) {
		t.Fatalf("Open(postgres) error = %v, want adapter sentinel", err)
	}
	if result.Migration.Version != 1 {
		t.Fatalf("migration version = %d, want 1", result.Migration.Version)
	}
	if !strings.Contains(fakeLastQuery(), "CREATE TABLE IF NOT EXISTS incidents") {
		t.Fatalf("postgres migration did not run: %s", fakeLastQuery())
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
}

func fakeSetExecError(err error) {
	fakeState.Lock()
	defer fakeState.Unlock()
	fakeState.lastQuery = ""
	fakeState.execErr = err
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
	fakeState.Lock()
	defer fakeState.Unlock()
	fakeState.lastQuery = s.query
	if fakeState.execErr != nil {
		return nil, fakeState.execErr
	}
	return driver.RowsAffected(1), nil
}

func (s fakeStmt) Query([]driver.Value) (driver.Rows, error) {
	return fakeRows{}, nil
}

type fakeTx struct{}

func (fakeTx) Commit() error {
	return nil
}

func (fakeTx) Rollback() error {
	return nil
}

type fakeRows struct{}

func (fakeRows) Columns() []string {
	return nil
}

func (fakeRows) Close() error {
	return nil
}

func (fakeRows) Next([]driver.Value) error {
	return io.EOF
}
