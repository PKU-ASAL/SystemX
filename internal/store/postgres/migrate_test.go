package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"
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
	query := FakeLastQuery()
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
	FakeSetExecError(execErr)
	db, err := sql.Open(FakeDriverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
