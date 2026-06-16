package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
	responsemodel "github.com/sysarmor/sysarmor-next-project/internal/response"
	"github.com/sysarmor/sysarmor-next-project/internal/store"
	"github.com/sysarmor/sysarmor-next-project/internal/store/migrations"
	"google.golang.org/protobuf/encoding/protojson"
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
	if err := projectAgents(ctx, db, state.Agents); err != nil {
		return err
	}
	if err := projectAgentHealth(ctx, db, state.Health); err != nil {
		return err
	}
	if err := projectResponseAudit(ctx, db, state.Responses, state.ResponseAcks); err != nil {
		return err
	}
	return nil
}

func projectAgents(ctx context.Context, db *sql.DB, agentRows []json.RawMessage) error {
	for _, raw := range agentRows {
		var agent analyticsv1.AgentHello
		if err := protojson.Unmarshal(raw, &agent); err != nil {
			return fmt.Errorf("decode agent projection: %w", err)
		}
		if agent.GetAgentId() == "" {
			continue
		}
		tenantID := agent.GetTenantId()
		if tenantID == "" {
			tenantID = "default"
		}
		_, err := db.ExecContext(ctx, `
INSERT INTO agents (tenant_id, agent_id, host_id, version, data)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (tenant_id, agent_id) DO UPDATE SET
  host_id = EXCLUDED.host_id,
  version = EXCLUDED.version,
  observed_at = now(),
  data = EXCLUDED.data
`, tenantID, agent.GetAgentId(), agent.GetHostId(), agent.GetVersion(), []byte(raw))
		if err != nil {
			return fmt.Errorf("project agent: %w", err)
		}
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

func projectResponseAudit(ctx context.Context, db *sql.DB, commands []responsemodel.Command, acks []responsemodel.Ack) error {
	ackByResponseID := map[string]responsemodel.Ack{}
	for _, ack := range acks {
		if ack.ResponseID != "" {
			ackByResponseID[ack.ResponseID] = ack
		}
	}
	for _, cmd := range commands {
		if cmd.ResponseID == "" {
			continue
		}
		tenantID := cmd.TenantID
		if tenantID == "" {
			tenantID = "default"
		}
		commandData, err := json.Marshal(cmd)
		if err != nil {
			return fmt.Errorf("encode response command projection: %w", err)
		}
		var ackData any
		if ack, ok := ackByResponseID[cmd.ResponseID]; ok {
			data, err := json.Marshal(ack)
			if err != nil {
				return fmt.Errorf("encode response ack projection: %w", err)
			}
			ackData = data
		}
		_, err = db.ExecContext(ctx, `
INSERT INTO response_audit (tenant_id, response_id, agent_id, status, action, created_at, updated_at, command, ack)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (tenant_id, response_id) DO UPDATE SET
  agent_id = EXCLUDED.agent_id,
  status = EXCLUDED.status,
  action = EXCLUDED.action,
  updated_at = EXCLUDED.updated_at,
  command = EXCLUDED.command,
  ack = EXCLUDED.ack
`, tenantID, cmd.ResponseID, cmd.AgentID, cmd.Status, cmd.Action, cmd.CreatedAt, cmd.UpdatedAt, commandData, ackData)
		if err != nil {
			return fmt.Errorf("project response audit: %w", err)
		}
	}
	return nil
}
