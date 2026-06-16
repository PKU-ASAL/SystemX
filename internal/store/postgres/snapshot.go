package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
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
	if err := projectAgentHealth(ctx, db, state.Health); err != nil {
		return err
	}
	return nil
}

func projectAgentHealth(ctx context.Context, db *sql.DB, healthRows []json.RawMessage) error {
	for _, raw := range healthRows {
		var health agenthealth.AgentHealth
		if err := json.Unmarshal(raw, &health); err != nil {
			return fmt.Errorf("decode agent health projection: %w", err)
		}
		if health.AgentID == "" {
			continue
		}
		tenantID := health.TenantID
		if tenantID == "" {
			tenantID = "default"
		}
		observedAt := health.ObservedAt
		if observedAt.IsZero() {
			_, err := db.ExecContext(ctx, `
INSERT INTO agent_health (tenant_id, agent_id, host_id, scope_type, scope_selector, data)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (tenant_id, agent_id) DO UPDATE SET
  host_id = EXCLUDED.host_id,
  scope_type = EXCLUDED.scope_type,
  scope_selector = EXCLUDED.scope_selector,
  observed_at = now(),
  data = EXCLUDED.data
`, tenantID, health.AgentID, health.HostID, health.Scope.Type, health.Scope.Selector, []byte(raw))
			if err != nil {
				return fmt.Errorf("project agent health: %w", err)
			}
			continue
		}
		_, err := db.ExecContext(ctx, `
INSERT INTO agent_health (tenant_id, agent_id, host_id, scope_type, scope_selector, observed_at, data)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (tenant_id, agent_id) DO UPDATE SET
  host_id = EXCLUDED.host_id,
  scope_type = EXCLUDED.scope_type,
  scope_selector = EXCLUDED.scope_selector,
  observed_at = EXCLUDED.observed_at,
  data = EXCLUDED.data
`, tenantID, health.AgentID, health.HostID, health.Scope.Type, health.Scope.Selector, observedAt, []byte(raw))
		if err != nil {
			return fmt.Errorf("project agent health: %w", err)
		}
	}
	return nil
}
