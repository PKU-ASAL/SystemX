package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sysarmor/sysarmor-next-project/packages/contracts/controlmodel"
	responsemodel "github.com/sysarmor/sysarmor-next-project/packages/response"
)

func (b *tableBackend) ListResponses(ctx context.Context, tenantID, agentID string) ([]responsemodel.AuditRecord, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return queryResponses(ctx, b.db, tenantID, agentID)
}

func (b *tableBackend) ListEvidencePullbacks(ctx context.Context, tenantID, agentID string) ([]controlmodel.EvidencePullbackRequest, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return queryEvidencePullbacks(ctx, b.db, tenantID, agentID)
}

func (b *tableBackend) ListControlCommands(ctx context.Context, tenantID, agentID, commandType string) ([]controlmodel.ControlCommand, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return queryControlCommands(ctx, b.db, tenantID, agentID, commandType)
}

func (b *tableBackend) WriteResponse(ctx context.Context, cmd responsemodel.Command, ack *responsemodel.Ack) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return upsertResponseAudit(ctx, b.db, cmd, ack)
}

func (b *tableBackend) CreateResponse(ctx context.Context, cmd responsemodel.Command) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return insertResponseAudit(ctx, b.db, cmd)
}

func (b *tableBackend) CreateControlCommand(ctx context.Context, cmd controlmodel.ControlCommand) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return insertControlCommand(ctx, b.db, cmd)
}

func (b *tableBackend) WriteControlCommand(ctx context.Context, cmd controlmodel.ControlCommand) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return projectControlCommands(ctx, b.db, []controlmodel.ControlCommand{cmd})
}

func findControlCommand(ctx context.Context, db sqlExecutor, tenantID, commandID string) (*controlmodel.ControlCommand, error) {
	commands, err := queryControlCommands(ctx, db, tenantID, "", "")
	if err != nil {
		return nil, err
	}
	for _, command := range commands {
		if command.CommandID == commandID {
			return &command, nil
		}
	}
	return nil, nil
}

func sameControlCommandRequest(a, b controlmodel.ControlCommand) bool {
	return a.CommandID == b.CommandID && a.TenantID == b.TenantID && a.AgentID == b.AgentID &&
		a.Type == b.Type && a.PolicyID == b.PolicyID && a.PolicyVersion == b.PolicyVersion &&
		a.ContentRef == b.ContentRef && a.ContentKind == b.ContentKind && a.ContentVersion == b.ContentVersion &&
		a.Actor == b.Actor && a.Reason == b.Reason && bytes.Equal(a.PayloadJSON, b.PayloadJSON)
}

func queryResponses(ctx context.Context, db sqlExecutor, tenantID, agentID string) ([]responsemodel.AuditRecord, error) {
	rows, err := db.QueryContext(ctx, `
SELECT command, ack FROM response_audit
WHERE ($1 = '' OR tenant_id = $1)
  AND ($2 = '' OR agent_id = $2)
ORDER BY updated_at ASC, response_id ASC
`, tenantID, agentID)
	if err != nil {
		return nil, fmt.Errorf("query postgres response audit: %w", err)
	}
	defer rows.Close()
	out := []responsemodel.AuditRecord{}
	for rows.Next() {
		var commandRaw []byte
		var ackRaw []byte
		if err := rows.Scan(&commandRaw, &ackRaw); err != nil {
			return nil, fmt.Errorf("scan postgres response audit: %w", err)
		}
		var command responsemodel.Command
		if err := json.Unmarshal(commandRaw, &command); err != nil {
			return nil, fmt.Errorf("decode postgres response command: %w", err)
		}
		record := responsemodel.AuditRecord{Command: command}
		if len(ackRaw) > 0 {
			var ack responsemodel.Ack
			if err := json.Unmarshal(ackRaw, &ack); err != nil {
				return nil, fmt.Errorf("decode postgres response ack: %w", err)
			}
			record.Ack = &ack
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate postgres response audit: %w", err)
	}
	return out, nil
}

func queryControlCommands(ctx context.Context, db sqlExecutor, tenantID, agentID, commandType string) ([]controlmodel.ControlCommand, error) {
	rows, err := db.QueryContext(ctx, `
SELECT data FROM control_commands
WHERE ($1 = '' OR tenant_id = $1)
  AND ($2 = '' OR agent_id = $2)
  AND ($3 = '' OR command_type = $3)
ORDER BY created_at ASC, command_id ASC
`, tenantID, agentID, commandType)
	if err != nil {
		return nil, fmt.Errorf("query postgres control commands: %w", err)
	}
	defer rows.Close()
	out := []controlmodel.ControlCommand{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan postgres control command: %w", err)
		}
		var cmd controlmodel.ControlCommand
		if err := json.Unmarshal(raw, &cmd); err != nil {
			return nil, fmt.Errorf("decode postgres control command: %w", err)
		}
		out = append(out, cmd)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate postgres control commands: %w", err)
	}
	return out, nil
}

func queryEvidencePullbacks(ctx context.Context, db sqlExecutor, tenantID, agentID string) ([]controlmodel.EvidencePullbackRequest, error) {
	rows, err := db.QueryContext(ctx, `
SELECT data FROM evidence_pullbacks
WHERE ($1 = '' OR tenant_id = $1)
  AND ($2 = '' OR agent_id = $2)
ORDER BY created_at ASC, request_id ASC
`, tenantID, agentID)
	if err != nil {
		return nil, fmt.Errorf("query postgres evidence pullbacks: %w", err)
	}
	defer rows.Close()
	out := []controlmodel.EvidencePullbackRequest{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan postgres evidence pullback: %w", err)
		}
		var request controlmodel.EvidencePullbackRequest
		if err := json.Unmarshal(raw, &request); err != nil {
			return nil, fmt.Errorf("decode postgres evidence pullback: %w", err)
		}
		out = append(out, request)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate postgres evidence pullbacks: %w", err)
	}
	return out, nil
}

func upsertResponseAudit(ctx context.Context, db sqlExecutor, cmd responsemodel.Command, ack *responsemodel.Ack) error {
	if cmd.ResponseID == "" {
		return nil
	}
	tenantID := cmd.TenantID
	if tenantID == "" {
		tenantID = "default"
	}
	commandData, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("encode response command projection: %w", err)
	}
	if ack == nil {
		_, err = insertResponseAudit(ctx, db, cmd)
		return err
	}
	ackData, err := json.Marshal(*ack)
	if err != nil {
		return fmt.Errorf("encode response ack projection: %w", err)
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
	return nil
}

func insertResponseAudit(ctx context.Context, db sqlExecutor, cmd responsemodel.Command) (bool, error) {
	if cmd.ResponseID == "" {
		return false, nil
	}
	tenantID := cmd.TenantID
	if tenantID == "" {
		tenantID = "default"
	}
	commandData, err := json.Marshal(cmd)
	if err != nil {
		return false, fmt.Errorf("encode response command projection: %w", err)
	}
	result, err := db.ExecContext(ctx, `
INSERT INTO response_audit (tenant_id, response_id, agent_id, status, action, created_at, updated_at, command, ack)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULL)
ON CONFLICT (tenant_id, response_id) DO NOTHING
`, tenantID, cmd.ResponseID, cmd.AgentID, cmd.Status, cmd.Action, cmd.CreatedAt, cmd.UpdatedAt, commandData)
	if err != nil {
		return false, fmt.Errorf("create response audit: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read response create result: %w", err)
	}
	return rows > 0, nil
}

func insertControlCommand(ctx context.Context, db sqlExecutor, cmd controlmodel.ControlCommand) (bool, error) {
	if cmd.CommandID == "" {
		return false, nil
	}
	tenantID := cmd.TenantID
	if tenantID == "" {
		tenantID = "default"
	}
	data, err := json.Marshal(cmd)
	if err != nil {
		return false, fmt.Errorf("encode control command: %w", err)
	}
	result, err := db.ExecContext(ctx, `
INSERT INTO control_commands (tenant_id, command_id, agent_id, command_type, status, policy_id, policy_version, content_ref, content_kind, content_version, actor, reason, created_at, updated_at, sent_at, last_sent_at, acked_at, canceled_at, expired_at, attempt_count, data)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
ON CONFLICT (tenant_id, command_id) DO NOTHING
`, tenantID, cmd.CommandID, cmd.AgentID, cmd.Type, cmd.Status, cmd.PolicyID, cmd.PolicyVersion, cmd.ContentRef, cmd.ContentKind, cmd.ContentVersion, cmd.Actor, cmd.Reason, cmd.CreatedAt, cmd.UpdatedAt, nullableTime(cmd.SentAt), nullableTime(cmd.LastSentAt), nullableTime(cmd.AckedAt), nullableTime(cmd.CanceledAt), nullableTime(cmd.ExpiredAt), cmd.AttemptCount, data)
	if err != nil {
		return false, fmt.Errorf("create control command: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read control command create result: %w", err)
	}
	return rows > 0, nil
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func projectEvidencePullbacks(ctx context.Context, db sqlExecutor, pullbacks []controlmodel.EvidencePullbackRequest) error {
	for _, req := range pullbacks {
		if req.RequestID == "" {
			continue
		}
		tenantID := req.TenantID
		if tenantID == "" {
			tenantID = "default"
		}
		status := req.Status
		if status == "" {
			status = controlmodel.EvidencePullbackStatusPending
		}
		data, err := json.Marshal(req)
		if err != nil {
			return fmt.Errorf("encode evidence pullback projection: %w", err)
		}
		createdAt := req.CreatedAt
		updatedAt := req.UpdatedAt
		if createdAt.IsZero() || updatedAt.IsZero() {
			_, err = db.ExecContext(ctx, `
INSERT INTO evidence_pullbacks (tenant_id, request_id, agent_id, incident_id, status, data)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (tenant_id, request_id) DO UPDATE SET
  agent_id = EXCLUDED.agent_id,
  incident_id = EXCLUDED.incident_id,
  status = EXCLUDED.status,
  updated_at = now(),
  data = EXCLUDED.data
`, tenantID, req.RequestID, req.AgentID, req.IncidentID, status, data)
		} else {
			_, err = db.ExecContext(ctx, `
INSERT INTO evidence_pullbacks (tenant_id, request_id, agent_id, incident_id, status, created_at, updated_at, data)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (tenant_id, request_id) DO UPDATE SET
  agent_id = EXCLUDED.agent_id,
  incident_id = EXCLUDED.incident_id,
  status = EXCLUDED.status,
  updated_at = EXCLUDED.updated_at,
  data = EXCLUDED.data
`, tenantID, req.RequestID, req.AgentID, req.IncidentID, status, createdAt, updatedAt, data)
		}
		if err != nil {
			return fmt.Errorf("project evidence pullback: %w", err)
		}
	}
	return nil
}

func projectControlCommands(ctx context.Context, db sqlExecutor, commands []controlmodel.ControlCommand) error {
	for _, cmd := range commands {
		if cmd.CommandID == "" {
			continue
		}
		tenantID := cmd.TenantID
		if tenantID == "" {
			tenantID = "default"
		}
		status := cmd.Status
		if status == "" {
			status = controlmodel.ControlCommandStatusPending
		}
		data, err := json.Marshal(cmd)
		if err != nil {
			return fmt.Errorf("encode control command projection: %w", err)
		}
		var sentAt any
		if !cmd.SentAt.IsZero() {
			sentAt = cmd.SentAt
		}
		var lastSentAt any
		if !cmd.LastSentAt.IsZero() {
			lastSentAt = cmd.LastSentAt
		}
		var ackedAt any
		if !cmd.AckedAt.IsZero() {
			ackedAt = cmd.AckedAt
		}
		var canceledAt any
		if !cmd.CanceledAt.IsZero() {
			canceledAt = cmd.CanceledAt
		}
		var expiredAt any
		if !cmd.ExpiredAt.IsZero() {
			expiredAt = cmd.ExpiredAt
		}
		createdAt := cmd.CreatedAt
		updatedAt := cmd.UpdatedAt
		if createdAt.IsZero() || updatedAt.IsZero() {
			_, err = db.ExecContext(ctx, `
INSERT INTO control_commands (tenant_id, command_id, agent_id, command_type, status, policy_id, policy_version, content_ref, content_kind, content_version, actor, reason, sent_at, last_sent_at, acked_at, canceled_at, expired_at, attempt_count, data)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
ON CONFLICT (tenant_id, command_id) DO NOTHING
`, tenantID, cmd.CommandID, cmd.AgentID, cmd.Type, status, cmd.PolicyID, cmd.PolicyVersion, cmd.ContentRef, cmd.ContentKind, cmd.ContentVersion, cmd.Actor, cmd.Reason, sentAt, lastSentAt, ackedAt, canceledAt, expiredAt, cmd.AttemptCount, data)
		} else {
			_, err = db.ExecContext(ctx, `
INSERT INTO control_commands (tenant_id, command_id, agent_id, command_type, status, policy_id, policy_version, content_ref, content_kind, content_version, actor, reason, created_at, updated_at, sent_at, last_sent_at, acked_at, canceled_at, expired_at, attempt_count, data)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
ON CONFLICT (tenant_id, command_id) DO UPDATE SET
  agent_id = EXCLUDED.agent_id,
  command_type = EXCLUDED.command_type,
  status = EXCLUDED.status,
  policy_id = EXCLUDED.policy_id,
  policy_version = EXCLUDED.policy_version,
  content_ref = EXCLUDED.content_ref,
  content_kind = EXCLUDED.content_kind,
  content_version = EXCLUDED.content_version,
  actor = EXCLUDED.actor,
  reason = EXCLUDED.reason,
  updated_at = EXCLUDED.updated_at,
  sent_at = EXCLUDED.sent_at,
  last_sent_at = EXCLUDED.last_sent_at,
  acked_at = EXCLUDED.acked_at,
  canceled_at = EXCLUDED.canceled_at,
  expired_at = EXCLUDED.expired_at,
  attempt_count = EXCLUDED.attempt_count,
  data = EXCLUDED.data
WHERE control_commands.updated_at <= EXCLUDED.updated_at
`, tenantID, cmd.CommandID, cmd.AgentID, cmd.Type, status, cmd.PolicyID, cmd.PolicyVersion, cmd.ContentRef, cmd.ContentKind, cmd.ContentVersion, cmd.Actor, cmd.Reason, createdAt, updatedAt, sentAt, lastSentAt, ackedAt, canceledAt, expiredAt, cmd.AttemptCount, data)
		}
		if err != nil {
			return fmt.Errorf("project control command: %w", err)
		}
	}
	return nil
}

func severityRank(severity string) int {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func joinTextArray(values []string) string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, strings.ReplaceAll(value, "\x1f", ""))
	}
	return strings.Join(out, "\x1f")
}
