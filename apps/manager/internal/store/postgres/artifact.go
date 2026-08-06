package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sysarmor/sysarmor-next-project/apps/manager/internal/store"
)

func (b *tableBackend) ListArtifacts(ctx context.Context, tenantID, kind, status string) ([]store.Artifact, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return queryArtifacts(ctx, b.db, tenantID, kind, status)
}

func (b *tableBackend) GetArtifact(ctx context.Context, tenantID, artifactID string) (store.Artifact, bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return queryArtifact(ctx, b.db, tenantID, artifactID)
}

func (b *tableBackend) ListChannels(ctx context.Context, tenantID string) ([]store.ArtifactChannel, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return queryChannels(ctx, b.db, tenantID)
}

func (b *tableBackend) GetChannel(ctx context.Context, tenantID, channel string) (store.ArtifactChannel, bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return queryChannel(ctx, b.db, tenantID, channel)
}

func (b *tableBackend) WriteArtifact(ctx context.Context, artifact store.Artifact) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return upsertArtifact(ctx, b.db, artifact)
}

func (b *tableBackend) WriteChannel(ctx context.Context, channel store.ArtifactChannel) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return upsertChannel(ctx, b.db, channel)
}

func queryArtifacts(ctx context.Context, db sqlExecutor, tenantID, kind, status string) ([]store.Artifact, error) {
	if tenantID == "" {
		tenantID = "default"
	}
	rows, err := db.QueryContext(ctx, `
SELECT data FROM artifacts
WHERE tenant_id = $1
  AND ($2 = '' OR artifact_kind = $2)
  AND ($3 = '' OR status = $3)
ORDER BY created_at ASC, artifact_id ASC
`, tenantID, kind, status)
	if err != nil {
		return nil, fmt.Errorf("query postgres artifacts: %w", err)
	}
	defer rows.Close()
	var out []store.Artifact
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan postgres artifact: %w", err)
		}
		var artifact store.Artifact
		if err := json.Unmarshal(raw, &artifact); err != nil {
			return nil, fmt.Errorf("decode postgres artifact: %w", err)
		}
		out = append(out, artifact)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate postgres artifacts: %w", err)
	}
	return out, nil
}

func queryArtifact(ctx context.Context, db sqlExecutor, tenantID, artifactID string) (store.Artifact, bool, error) {
	if tenantID == "" {
		tenantID = "default"
	}
	if artifactID == "" {
		return store.Artifact{}, false, nil
	}
	row := db.QueryRowContext(ctx, `
SELECT data FROM artifacts
WHERE tenant_id = $1 AND artifact_id = $2
`, tenantID, artifactID)
	var raw []byte
	if err := row.Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return store.Artifact{}, false, nil
		}
		return store.Artifact{}, false, fmt.Errorf("query postgres artifact: %w", err)
	}
	var artifact store.Artifact
	if err := json.Unmarshal(raw, &artifact); err != nil {
		return store.Artifact{}, false, fmt.Errorf("decode postgres artifact: %w", err)
	}
	return artifact, true, nil
}

func queryChannels(ctx context.Context, db sqlExecutor, tenantID string) ([]store.ArtifactChannel, error) {
	if tenantID == "" {
		tenantID = "default"
	}
	rows, err := db.QueryContext(ctx, `
SELECT data FROM artifact_channels
WHERE tenant_id = $1
ORDER BY channel_name ASC
`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query postgres artifact channels: %w", err)
	}
	defer rows.Close()
	var out []store.ArtifactChannel
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan postgres artifact channel: %w", err)
		}
		var channel store.ArtifactChannel
		if err := json.Unmarshal(raw, &channel); err != nil {
			return nil, fmt.Errorf("decode postgres artifact channel: %w", err)
		}
		out = append(out, channel)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate postgres artifact channels: %w", err)
	}
	return out, nil
}

func queryChannel(ctx context.Context, db sqlExecutor, tenantID, channelName string) (store.ArtifactChannel, bool, error) {
	if tenantID == "" {
		tenantID = "default"
	}
	if channelName == "" {
		return store.ArtifactChannel{}, false, nil
	}
	row := db.QueryRowContext(ctx, `
SELECT data FROM artifact_channels
WHERE tenant_id = $1 AND channel_name = $2
`, tenantID, channelName)
	var raw []byte
	if err := row.Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return store.ArtifactChannel{}, false, nil
		}
		return store.ArtifactChannel{}, false, fmt.Errorf("query postgres artifact channel: %w", err)
	}
	var channel store.ArtifactChannel
	if err := json.Unmarshal(raw, &channel); err != nil {
		return store.ArtifactChannel{}, false, fmt.Errorf("decode postgres artifact channel: %w", err)
	}
	return channel, true, nil
}

func projectArtifacts(ctx context.Context, db sqlExecutor, artifacts []store.Artifact) error {
	for _, artifact := range artifacts {
		if err := upsertArtifact(ctx, db, artifact); err != nil {
			return err
		}
	}
	return nil
}

func upsertArtifact(ctx context.Context, db sqlExecutor, artifact store.Artifact) error {
	if artifact.ArtifactID == "" || artifact.Name == "" || artifact.Kind == "" || artifact.Version == "" || artifact.SHA256 == "" {
		return nil
	}
	tenantID := artifact.TenantID
	if tenantID == "" {
		tenantID = "default"
	}
	status := artifact.Status
	if status == "" {
		status = "draft"
	}
	data, err := json.Marshal(artifact)
	if err != nil {
		return fmt.Errorf("encode artifact projection: %w", err)
	}
	createdAt := artifact.CreatedAt
	updatedAt := artifact.UpdatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}
	_, err = db.ExecContext(ctx, `
INSERT INTO artifacts (tenant_id, artifact_id, artifact_name, artifact_kind, artifact_version, artifact_os, artifact_arch, sha256, size_bytes, status, storage_path, created_at, updated_at, data)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
ON CONFLICT (tenant_id, artifact_id) DO UPDATE SET
  artifact_name = EXCLUDED.artifact_name,
  artifact_kind = EXCLUDED.artifact_kind,
  artifact_version = EXCLUDED.artifact_version,
  artifact_os = EXCLUDED.artifact_os,
  artifact_arch = EXCLUDED.artifact_arch,
  sha256 = EXCLUDED.sha256,
  size_bytes = EXCLUDED.size_bytes,
  status = EXCLUDED.status,
  storage_path = EXCLUDED.storage_path,
  updated_at = EXCLUDED.updated_at,
  data = EXCLUDED.data
`, tenantID, artifact.ArtifactID, artifact.Name, artifact.Kind, artifact.Version, artifact.OS, artifact.Arch, artifact.SHA256, artifact.SizeBytes, status, artifact.StoragePath, createdAt, updatedAt, data)
	if err != nil {
		return fmt.Errorf("project artifact: %w", err)
	}
	return nil
}

func projectChannels(ctx context.Context, db sqlExecutor, channels []store.ArtifactChannel) error {
	for _, channel := range channels {
		if err := upsertChannel(ctx, db, channel); err != nil {
			return err
		}
	}
	return nil
}

func upsertChannel(ctx context.Context, db sqlExecutor, channel store.ArtifactChannel) error {
	if channel.Channel == "" || channel.ArtifactID == "" {
		return nil
	}
	tenantID := channel.TenantID
	if tenantID == "" {
		tenantID = "default"
	}
	now := time.Now().UTC()
	if channel.CreatedAt.IsZero() {
		channel.CreatedAt = now
	}
	if channel.UpdatedAt.IsZero() {
		channel.UpdatedAt = channel.CreatedAt
	}
	data, err := json.Marshal(channel)
	if err != nil {
		return fmt.Errorf("encode artifact channel projection: %w", err)
	}
	_, err = db.ExecContext(ctx, `
INSERT INTO artifact_channels (tenant_id, channel_name, artifact_id, created_at, updated_at, data)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (tenant_id, channel_name) DO UPDATE SET
  artifact_id = EXCLUDED.artifact_id,
  updated_at = EXCLUDED.updated_at,
  data = EXCLUDED.data
`, tenantID, channel.Channel, channel.ArtifactID, channel.CreatedAt, channel.UpdatedAt, data)
	if err != nil {
		return fmt.Errorf("project artifact channel: %w", err)
	}
	return nil
}
