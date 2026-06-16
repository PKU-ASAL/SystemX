package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/sysarmor/sysarmor-next-project/internal/store/migrations"
)

type MigrationResult struct {
	Version int `json:"version"`
}

func ApplyMigrations(ctx context.Context, db *sql.DB) (MigrationResult, error) {
	if db == nil {
		return MigrationResult{}, fmt.Errorf("postgres migration db is nil")
	}
	if _, err := db.ExecContext(ctx, migrations.PostgresSchema); err != nil {
		return MigrationResult{}, fmt.Errorf("apply postgres schema v%d: %w", migrations.PostgresVersion, err)
	}
	return MigrationResult{Version: migrations.PostgresVersion}, nil
}
