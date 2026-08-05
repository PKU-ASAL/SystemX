package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sysarmor/sysarmor-next-project/apps/manager/internal/store"
)

func (b *tableBackend) ListEnrollments(ctx context.Context, tenantID, status string) ([]store.Enrollment, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return queryEnrollments(ctx, b.db, tenantID, status)
}

func formatOptionalTime(t time.Time) string {
	if t.IsZero() {
		return "0001-01-01T00:00:00Z"
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func (b *tableBackend) GetEnrollmentByTokenHash(ctx context.Context, tokenHash string) (store.Enrollment, bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return queryEnrollmentByTokenHash(ctx, b.db, tokenHash)
}

func (b *tableBackend) GetEnrollmentByBootstrapTokenHash(ctx context.Context, tokenHash string) (store.Enrollment, bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return queryEnrollmentByBootstrapTokenHash(ctx, b.db, tokenHash, false)
}

func (b *tableBackend) ConsumeEnrollmentBootstrap(ctx context.Context, bootstrapHash, enrollmentHash, enrollmentPreview string, fetchedAt time.Time) (store.Enrollment, bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return store.Enrollment{}, false, err
	}
	defer tx.Rollback()
	enrollment, ok, err := queryEnrollmentByBootstrapTokenHash(ctx, tx, bootstrapHash, true)
	if err != nil || !ok {
		return store.Enrollment{}, false, err
	}
	if enrollment.Status != "active" || !enrollment.BootstrapFetchedAt.IsZero() ||
		(!enrollment.ExpiresAt.IsZero() && fetchedAt.After(enrollment.ExpiresAt)) {
		return store.Enrollment{}, false, nil
	}
	enrollment.TokenHash = enrollmentHash
	enrollment.TokenPreview = enrollmentPreview
	enrollment.BootstrapFetchedAt = fetchedAt
	if err := upsertEnrollment(ctx, tx, enrollment); err != nil {
		return store.Enrollment{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return store.Enrollment{}, false, err
	}
	return enrollment, true, nil
}

func (b *tableBackend) CommitEnrollmentIssue(ctx context.Context, tokenHash, keyHash string, proposed store.Enrollment, cert store.AgentCertificate) (store.Enrollment, store.EnrollmentIssueResult, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return store.Enrollment{}, store.EnrollmentIssueMissing, err
	}
	defer tx.Rollback()
	current, ok, err := queryEnrollmentByTokenHashForUpdate(ctx, tx, tokenHash)
	if err != nil || !ok {
		return store.Enrollment{}, store.EnrollmentIssueMissing, err
	}
	if current.Status == "issued" {
		if current.IssuedKeySHA256 == keyHash {
			return current, store.EnrollmentIssueReplay, tx.Commit()
		}
		return store.Enrollment{}, store.EnrollmentIssueConflict, nil
	}
	if current.Status != "active" || (!current.ExpiresAt.IsZero() && time.Now().UTC().After(current.ExpiresAt)) {
		return store.Enrollment{}, store.EnrollmentIssueConflict, nil
	}
	proposed.TokenHash = current.TokenHash
	proposed.Status = "issued"
	proposed.IssuedKeySHA256 = keyHash
	if err := upsertEnrollment(ctx, tx, proposed); err != nil {
		return store.Enrollment{}, store.EnrollmentIssueMissing, err
	}
	if err := upsertAgentCertificate(ctx, tx, cert); err != nil {
		return store.Enrollment{}, store.EnrollmentIssueMissing, err
	}
	if err := tx.Commit(); err != nil {
		return store.Enrollment{}, store.EnrollmentIssueMissing, err
	}
	return proposed, store.EnrollmentIssued, nil
}

func (b *tableBackend) GetAgentCertificate(ctx context.Context, tenantID, serial string) (store.AgentCertificate, bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var raw []byte
	err := b.db.QueryRowContext(ctx, `SELECT data FROM agent_certificates WHERE tenant_id=$1 AND serial_number=$2`, tenantID, serial).Scan(&raw)
	if err == sql.ErrNoRows {
		return store.AgentCertificate{}, false, nil
	}
	if err != nil {
		return store.AgentCertificate{}, false, fmt.Errorf("query agent certificate: %w", err)
	}
	var cert store.AgentCertificate
	if err := json.Unmarshal(raw, &cert); err != nil {
		return store.AgentCertificate{}, false, fmt.Errorf("decode agent certificate: %w", err)
	}
	return cert, true, nil
}

func (b *tableBackend) WriteEnrollment(ctx context.Context, enrollment store.Enrollment) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return upsertEnrollment(ctx, b.db, enrollment)
}

func (b *tableBackend) WriteAgentCertificate(ctx context.Context, cert store.AgentCertificate) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return upsertAgentCertificate(ctx, b.db, cert)
}

func (b *tableBackend) RevokeAgentCertificate(ctx context.Context, tenantID, agentID, enrollmentID, serial string, revokedAt time.Time, receipt string) (store.AgentCertificate, bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var revoked store.AgentCertificate
	var found bool
	err := b.withTransaction(ctx, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `SELECT data FROM agent_certificates WHERE tenant_id=$1 AND serial_number=$2 FOR UPDATE`, tenantID, serial)
		var raw []byte
		if err := row.Scan(&raw); err == sql.ErrNoRows {
			return nil
		} else if err != nil {
			return fmt.Errorf("read agent certificate for revocation: %w", err)
		}
		if err := json.Unmarshal(raw, &revoked); err != nil {
			return fmt.Errorf("decode agent certificate for revocation: %w", err)
		}
		found = true
		if revoked.AgentID != agentID || revoked.EnrollmentID != enrollmentID {
			return fmt.Errorf("%w: certificate identity mismatch", store.ErrConflict)
		}
		if revoked.RevokedAt.IsZero() {
			revoked.RevokedAt = revokedAt.UTC()
		}
		if revoked.RevocationReceipt == "" {
			revoked.RevocationReceipt = receipt
		}
		if err := upsertAgentCertificate(ctx, tx, revoked); err != nil {
			return err
		}
		return insertLegacyUnenrollment(ctx, tx, revoked)
	})
	return revoked, found, err
}

func queryEnrollments(ctx context.Context, db sqlExecutor, tenantID, status string) ([]store.Enrollment, error) {
	if tenantID == "" {
		tenantID = "default"
	}
	rows, err := db.QueryContext(ctx, `
SELECT data FROM enrollments
WHERE tenant_id = $1 AND ($2 = '' OR status = $2)
ORDER BY created_at ASC, enrollment_id ASC
`, tenantID, status)
	if err != nil {
		return nil, fmt.Errorf("query postgres enrollments: %w", err)
	}
	defer rows.Close()
	var out []store.Enrollment
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan postgres enrollment: %w", err)
		}
		var enrollment store.Enrollment
		if err := json.Unmarshal(raw, &enrollment); err != nil {
			return nil, fmt.Errorf("decode postgres enrollment: %w", err)
		}
		out = append(out, enrollment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate postgres enrollments: %w", err)
	}
	return out, nil
}

func queryEnrollmentByTokenHash(ctx context.Context, db sqlExecutor, tokenHash string) (store.Enrollment, bool, error) {
	if tokenHash == "" {
		return store.Enrollment{}, false, nil
	}
	row := db.QueryRowContext(ctx, `
SELECT data FROM enrollments
WHERE token_hash = $1
ORDER BY created_at DESC
LIMIT 1
`, tokenHash)
	var raw []byte
	if err := row.Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return store.Enrollment{}, false, nil
		}
		return store.Enrollment{}, false, fmt.Errorf("query postgres enrollment by token: %w", err)
	}
	var enrollment store.Enrollment
	if err := json.Unmarshal(raw, &enrollment); err != nil {
		return store.Enrollment{}, false, fmt.Errorf("decode postgres enrollment by token: %w", err)
	}
	return enrollment, true, nil
}

func queryEnrollmentByTokenHashForUpdate(ctx context.Context, db sqlExecutor, tokenHash string) (store.Enrollment, bool, error) {
	row := db.QueryRowContext(ctx, `
SELECT data FROM enrollments
WHERE token_hash = $1
ORDER BY created_at DESC
LIMIT 1
FOR UPDATE
`, tokenHash)
	var raw []byte
	if err := row.Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return store.Enrollment{}, false, nil
		}
		return store.Enrollment{}, false, fmt.Errorf("lock postgres enrollment by token: %w", err)
	}
	var enrollment store.Enrollment
	if err := json.Unmarshal(raw, &enrollment); err != nil {
		return store.Enrollment{}, false, fmt.Errorf("decode locked postgres enrollment: %w", err)
	}
	return enrollment, true, nil
}

func queryEnrollmentByBootstrapTokenHash(ctx context.Context, db sqlExecutor, tokenHash string, forUpdate bool) (store.Enrollment, bool, error) {
	query := `
SELECT data FROM enrollments
WHERE data->>'bootstrap_token_hash' = $1
ORDER BY created_at DESC
LIMIT 1`
	if forUpdate {
		query += " FOR UPDATE"
	}
	row := db.QueryRowContext(ctx, query, tokenHash)
	var raw []byte
	if err := row.Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return store.Enrollment{}, false, nil
		}
		return store.Enrollment{}, false, fmt.Errorf("query postgres enrollment by bootstrap token: %w", err)
	}
	var enrollment store.Enrollment
	if err := json.Unmarshal(raw, &enrollment); err != nil {
		return store.Enrollment{}, false, fmt.Errorf("decode postgres bootstrap enrollment: %w", err)
	}
	return enrollment, true, nil
}

func projectEnrollments(ctx context.Context, db sqlExecutor, enrollments []store.Enrollment) error {
	for _, enrollment := range enrollments {
		if err := upsertEnrollment(ctx, db, enrollment); err != nil {
			return err
		}
	}
	return nil
}

func upsertEnrollment(ctx context.Context, db sqlExecutor, enrollment store.Enrollment) error {
	if enrollment.EnrollmentID == "" || enrollment.TokenHash == "" {
		return nil
	}
	tenantID := enrollment.TenantID
	if tenantID == "" {
		tenantID = "default"
	}
	status := enrollment.Status
	if status == "" {
		status = "active"
	}
	data, err := json.Marshal(enrollment)
	if err != nil {
		return fmt.Errorf("encode enrollment projection: %w", err)
	}
	_, err = db.ExecContext(ctx, `
INSERT INTO enrollments (tenant_id, enrollment_id, agent_id, host_id, token_hash, status, created_at, expires_at, used_at, data)
VALUES ($1, $2, $3, $4, $5, $6, $7, nullif($8, '0001-01-01T00:00:00Z')::timestamptz, nullif($9, '0001-01-01T00:00:00Z')::timestamptz, $10)
ON CONFLICT (tenant_id, enrollment_id) DO UPDATE SET
  agent_id = EXCLUDED.agent_id,
  host_id = EXCLUDED.host_id,
  token_hash = EXCLUDED.token_hash,
  status = EXCLUDED.status,
  expires_at = EXCLUDED.expires_at,
  used_at = EXCLUDED.used_at,
  data = EXCLUDED.data
`, tenantID, enrollment.EnrollmentID, enrollment.AgentID, enrollment.HostID, enrollment.TokenHash, status, enrollment.CreatedAt, formatOptionalTime(enrollment.ExpiresAt), formatOptionalTime(enrollment.UsedAt), data)
	if err != nil {
		return fmt.Errorf("project enrollment: %w", err)
	}
	return nil
}

func projectAgentCertificates(ctx context.Context, db sqlExecutor, certs []store.AgentCertificate) error {
	for _, cert := range certs {
		if err := upsertAgentCertificate(ctx, db, cert); err != nil {
			return err
		}
	}
	return nil
}

func upsertAgentCertificate(ctx context.Context, db sqlExecutor, cert store.AgentCertificate) error {
	if cert.TenantID == "" {
		cert.TenantID = "default"
	}
	if cert.AgentID == "" || cert.SerialNumber == "" {
		return nil
	}
	if cert.CreatedAt.IsZero() {
		cert.CreatedAt = time.Now().UTC()
	}
	data, err := json.Marshal(cert)
	if err != nil {
		return fmt.Errorf("encode agent certificate projection: %w", err)
	}
	_, err = db.ExecContext(ctx, `
INSERT INTO agent_certificates (tenant_id, agent_id, serial_number, enrollment_id, not_before, not_after, created_at, revoked_at, data)
VALUES ($1, $2, $3, $4, $5, $6, $7, nullif($8, '0001-01-01T00:00:00Z')::timestamptz, $9)
ON CONFLICT (tenant_id, serial_number) DO UPDATE SET
  agent_id = EXCLUDED.agent_id,
  enrollment_id = EXCLUDED.enrollment_id,
  not_before = EXCLUDED.not_before,
  not_after = EXCLUDED.not_after,
  revoked_at = EXCLUDED.revoked_at,
  data = EXCLUDED.data
`, cert.TenantID, cert.AgentID, cert.SerialNumber, cert.EnrollmentID, cert.NotBefore, cert.NotAfter, cert.CreatedAt, formatOptionalTime(cert.RevokedAt), data)
	if err != nil {
		return fmt.Errorf("project agent certificate: %w", err)
	}
	return nil
}
