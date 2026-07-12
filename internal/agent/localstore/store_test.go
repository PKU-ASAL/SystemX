package localstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenCreatesSecureBaselineAndStandaloneIdentity(t *testing.T) {
	root := filepath.Join(t.TempDir(), "agent")
	store, err := Open(t.Context(), Options{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	assertMode(t, root, 0o700)
	assertMode(t, filepath.Join(root, "spool"), 0o700)
	assertMode(t, filepath.Join(root, "agent.db"), 0o600)

	identity, err := store.DeviceIdentity(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if identity.DeviceID == "" || identity.HostID == "" || identity.CreatedAt.IsZero() {
		t.Fatalf("identity=%+v", identity)
	}
	if got := pragma(t, store, "journal_mode"); got != "wal" {
		t.Fatalf("journal_mode=%q", got)
	}
	if got := pragma(t, store, "foreign_keys"); got != "1" {
		t.Fatalf("foreign_keys=%q", got)
	}
	if got := schemaVersion(t, store); got != currentSchemaVersion {
		t.Fatalf("schema version=%d", got)
	}
	var state string
	if err := store.db.QueryRow("SELECT state FROM enrollment WHERE singleton = 1").Scan(&state); err != nil || state != "standalone" {
		t.Fatalf("enrollment state=%q error=%v", state, err)
	}
}

func TestDeviceIdentitySurvivesReopen(t *testing.T) {
	root := filepath.Join(t.TempDir(), "agent")
	first := openStore(t, root)
	want, err := first.DeviceIdentity(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second := openStore(t, root)
	defer second.Close()
	got, err := second.DeviceIdentity(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.DeviceID != want.DeviceID || got.CreatedAt != want.CreatedAt {
		t.Fatalf("identity changed: got=%+v want=%+v", got, want)
	}
}

func openStore(t *testing.T, root string) *Store {
	t.Helper()
	store, err := Open(t.Context(), Options{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func pragma(t *testing.T, store *Store, name string) string {
	t.Helper()
	var value string
	if err := store.db.QueryRow("PRAGMA " + name).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func schemaVersion(t *testing.T, store *Store) int {
	t.Helper()
	var version int
	if err := store.db.QueryRow("SELECT version FROM schema_meta").Scan(&version); err != nil {
		t.Fatal(err)
	}
	return version
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode=%o want=%o", path, got, want)
	}
}
