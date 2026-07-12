package localstore

import (
	"context"
	"fmt"
	"os"
)

const currentSchemaVersion = 1

const baselineSchema = `
CREATE TABLE IF NOT EXISTS schema_meta (
  version INTEGER PRIMARY KEY
);
CREATE TABLE IF NOT EXISTS device_identity (
  singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
  device_id TEXT NOT NULL UNIQUE,
  host_id TEXT NOT NULL,
  created_at_ns INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS enrollment (
  singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
  state TEXT NOT NULL CHECK (state IN ('standalone', 'managed')),
  tenant_id TEXT,
  agent_id TEXT,
  gateway_address TEXT,
  tls_ca_path TEXT,
  tls_cert_path TEXT,
  tls_key_path TEXT,
  tls_server_name TEXT,
  upload_history INTEGER NOT NULL DEFAULT 0,
  managed_from_seq INTEGER,
  updated_at_ns INTEGER NOT NULL
);`

func (s *Store) initialize(ctx context.Context, dbPath string) error {
	for _, statement := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("configure local state: %w", err)
		}
	}
	if err := os.Chmod(dbPath, 0o600); err != nil {
		return fmt.Errorf("secure local state: %w", err)
	}
	if err := s.applyBaseline(ctx); err != nil {
		return err
	}
	return s.ensureIdentity(ctx)
}

func (s *Store) applyBaseline(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin local schema: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, baselineSchema); err != nil {
		return fmt.Errorf("create local schema: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO schema_meta(version) VALUES (?)", currentSchemaVersion); err != nil {
		return fmt.Errorf("record local schema: %w", err)
	}
	var version int
	if err := tx.QueryRowContext(ctx, "SELECT version FROM schema_meta").Scan(&version); err != nil {
		return fmt.Errorf("read local schema version: %w", err)
	}
	if version != currentSchemaVersion {
		return fmt.Errorf("unsupported local schema version %d", version)
	}
	return tx.Commit()
}
