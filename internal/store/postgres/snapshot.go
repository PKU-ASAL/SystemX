package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/sysarmor/sysarmor-next-project/internal/store"
	"github.com/sysarmor/sysarmor-next-project/internal/store/migrations"
)

const snapshotStateKey = "default"

func OpenSnapshotStore(ctx context.Context, db *sql.DB, migration MigrationResult) (*store.Store, error) {
	if db == nil {
		return nil, fmt.Errorf("postgres snapshot db is nil")
	}
	st, err := store.Open("")
	if err != nil {
		return nil, err
	}
	state, ok, err := loadSnapshot(ctx, db)
	if err != nil {
		return nil, err
	}
	if ok {
		if err := st.ImportState(state); err != nil {
			return nil, fmt.Errorf("import postgres snapshot state: %w", err)
		}
	}
	info := store.Info{
		Backend:          "postgres",
		StateVersion:     store.FileStoreStateVersion,
		MigrationVersion: migration.Version,
		PostgresSchema:   migrations.PostgresVersion,
	}
	st.ConfigureBackend(info, func(state store.State) error {
		return saveSnapshot(context.Background(), db, state)
	})
	return st, nil
}

func loadSnapshot(ctx context.Context, db *sql.DB) (store.State, bool, error) {
	var raw []byte
	err := db.QueryRowContext(ctx, "SELECT data FROM sysarmor_state WHERE state_key = $1", snapshotStateKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return store.State{}, false, nil
	}
	if err != nil {
		return store.State{}, false, fmt.Errorf("load postgres snapshot: %w", err)
	}
	var state store.State
	if err := json.Unmarshal(raw, &state); err != nil {
		return store.State{}, false, fmt.Errorf("decode postgres snapshot: %w", err)
	}
	return state, true, nil
}

func saveSnapshot(ctx context.Context, db *sql.DB, state store.State) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
INSERT INTO sysarmor_state (state_key, state_version, data, updated_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (state_key) DO UPDATE SET
  state_version = EXCLUDED.state_version,
  data = EXCLUDED.data,
  updated_at = now()
`, snapshotStateKey, store.FileStoreStateVersion, data)
	if err != nil {
		return fmt.Errorf("save postgres snapshot: %w", err)
	}
	return nil
}
