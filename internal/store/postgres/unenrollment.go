package postgres

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sysarmor/sysarmor-next-project/internal/store"
)

func (b *tableBackend) GetUnenrollment(ctx context.Context, tenantID, enrollmentID string) (store.UnenrollmentRecord, bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return queryUnenrollment(ctx, b.db, tenantID, enrollmentID)
}

func (b *tableBackend) ListUnenrollments(ctx context.Context, tenantID string) ([]store.UnenrollmentRecord, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	rows, err := b.db.QueryContext(ctx, `SELECT data FROM agent_unenrollments WHERE tenant_id=$1 ORDER BY enrollment_id`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list agent unenrollments: %w", err)
	}
	defer rows.Close()
	var records []store.UnenrollmentRecord
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan agent unenrollment: %w", err)
		}
		var record store.UnenrollmentRecord
		if err := json.Unmarshal(raw, &record); err != nil {
			return nil, fmt.Errorf("decode agent unenrollment: %w", err)
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (b *tableBackend) AuthorizeAgentUnenrollment(ctx context.Context, tenantID, agentID, enrollmentID, serial, tokenHash string, revokedAt time.Time, receipt string) (store.AgentCertificate, store.UnenrollmentRecord, bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var cert store.AgentCertificate
	var record store.UnenrollmentRecord
	var found bool
	err := b.withTransaction(ctx, func(tx *sql.Tx) error {
		var err error
		cert, record, found, err = readUnenrollmentAuthorization(ctx, tx, tenantID, enrollmentID, serial)
		if err != nil || !found {
			return err
		}
		if err := validateCertificateIdentity(cert, tenantID, agentID, enrollmentID, serial); err != nil {
			return err
		}
		if record.EnrollmentID != "" {
			return validateUnenrollmentRecord(record, tenantID, agentID, enrollmentID, serial, receipt, tokenHash)
		}
		cert, record, err = prepareUnenrollmentRecord(cert, tokenHash, receipt, revokedAt)
		if err != nil {
			return err
		}
		if err := upsertAgentCertificate(ctx, tx, cert); err != nil {
			return err
		}
		return upsertUnenrollment(ctx, tx, record)
	})
	return cert, record, found, err
}

func (b *tableBackend) CompleteAgentUnenrollment(ctx context.Context, tenantID, agentID, enrollmentID, serial, receipt, tokenHash string, completedAt time.Time) (store.UnenrollmentRecord, bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var record store.UnenrollmentRecord
	var found bool
	err := b.withTransaction(ctx, func(tx *sql.Tx) error {
		var err error
		record, found, err = queryUnenrollmentForUpdate(ctx, tx, tenantID, enrollmentID)
		if err != nil || !found {
			return err
		}
		if err := validateUnenrollmentRecord(record, tenantID, agentID, enrollmentID, serial, receipt, tokenHash); err != nil {
			return err
		}
		if record.Status == store.UnenrollmentEndpointCompleted {
			return nil
		}
		record.Status = store.UnenrollmentEndpointCompleted
		record.EndpointCompletedAt, record.UpdatedAt = completedAt.UTC(), completedAt.UTC()
		return upsertUnenrollment(ctx, tx, record)
	})
	return record, found, err
}

func readUnenrollmentAuthorization(ctx context.Context, tx *sql.Tx, tenantID, enrollmentID, serial string) (store.AgentCertificate, store.UnenrollmentRecord, bool, error) {
	row := tx.QueryRowContext(ctx, `SELECT c.data, u.data FROM agent_certificates c
LEFT JOIN agent_unenrollments u ON u.tenant_id=c.tenant_id AND u.enrollment_id=c.enrollment_id
WHERE c.tenant_id=$1 AND c.serial_number=$2 FOR UPDATE OF c`, tenantID, serial)
	var certRaw, recordRaw []byte
	if err := row.Scan(&certRaw, &recordRaw); err == sql.ErrNoRows {
		return store.AgentCertificate{}, store.UnenrollmentRecord{}, false, nil
	} else if err != nil {
		return store.AgentCertificate{}, store.UnenrollmentRecord{}, false, fmt.Errorf("read certificate for unenrollment: %w", err)
	}
	var cert store.AgentCertificate
	if err := json.Unmarshal(certRaw, &cert); err != nil {
		return store.AgentCertificate{}, store.UnenrollmentRecord{}, false, fmt.Errorf("decode certificate for unenrollment: %w", err)
	}
	var record store.UnenrollmentRecord
	if len(recordRaw) > 0 {
		if err := json.Unmarshal(recordRaw, &record); err != nil {
			return store.AgentCertificate{}, store.UnenrollmentRecord{}, false, fmt.Errorf("decode unenrollment record: %w", err)
		}
	}
	return cert, record, true, nil
}

func prepareUnenrollmentRecord(cert store.AgentCertificate, tokenHash, receipt string, revokedAt time.Time) (store.AgentCertificate, store.UnenrollmentRecord, error) {
	if !cert.RevokedAt.IsZero() && cert.RevocationReceipt != "" && cert.RevocationReceipt != receipt {
		return store.AgentCertificate{}, store.UnenrollmentRecord{}, fmt.Errorf("%w: certificate revocation receipt mismatch", store.ErrConflict)
	}
	if cert.RevokedAt.IsZero() {
		cert.RevokedAt = revokedAt.UTC()
	}
	if cert.RevocationReceipt == "" {
		cert.RevocationReceipt = receipt
	}
	record := store.UnenrollmentRecord{TenantID: cert.TenantID, AgentID: cert.AgentID, EnrollmentID: cert.EnrollmentID,
		CertificateSerial: cert.SerialNumber, RevocationReceipt: cert.RevocationReceipt, CompletionTokenHash: tokenHash,
		Status: store.UnenrollmentRevokedEndpointPending, RevokedAt: cert.RevokedAt, CreatedAt: cert.RevokedAt, UpdatedAt: cert.RevokedAt}
	return cert, record, nil
}

func validateCertificateIdentity(cert store.AgentCertificate, tenantID, agentID, enrollmentID, serial string) error {
	if cert.TenantID != tenantID || cert.AgentID != agentID || cert.EnrollmentID != enrollmentID || cert.SerialNumber != serial {
		return fmt.Errorf("%w: certificate identity mismatch", store.ErrConflict)
	}
	return nil
}

func validateUnenrollmentRecord(record store.UnenrollmentRecord, tenantID, agentID, enrollmentID, serial, receipt, tokenHash string) error {
	hashMatches := subtle.ConstantTimeCompare([]byte(strings.ToLower(record.CompletionTokenHash)), []byte(strings.ToLower(tokenHash))) == 1
	if record.TenantID != tenantID || record.AgentID != agentID || record.EnrollmentID != enrollmentID ||
		record.CertificateSerial != serial || record.RevocationReceipt != receipt || !hashMatches {
		return fmt.Errorf("%w: unenrollment binding mismatch", store.ErrConflict)
	}
	return nil
}

func queryUnenrollment(ctx context.Context, db sqlExecutor, tenantID, enrollmentID string) (store.UnenrollmentRecord, bool, error) {
	return scanUnenrollmentRow(db.QueryRowContext(ctx, `SELECT data FROM agent_unenrollments WHERE tenant_id=$1 AND enrollment_id=$2`, tenantID, enrollmentID))
}

func queryUnenrollmentForUpdate(ctx context.Context, db sqlExecutor, tenantID, enrollmentID string) (store.UnenrollmentRecord, bool, error) {
	return scanUnenrollmentRow(db.QueryRowContext(ctx, `SELECT data FROM agent_unenrollments WHERE tenant_id=$1 AND enrollment_id=$2 FOR UPDATE`, tenantID, enrollmentID))
}

func scanUnenrollmentRow(row *sql.Row) (store.UnenrollmentRecord, bool, error) {
	var raw []byte
	if err := row.Scan(&raw); err == sql.ErrNoRows {
		return store.UnenrollmentRecord{}, false, nil
	} else if err != nil {
		return store.UnenrollmentRecord{}, false, fmt.Errorf("read agent unenrollment: %w", err)
	}
	var record store.UnenrollmentRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return store.UnenrollmentRecord{}, false, fmt.Errorf("decode agent unenrollment: %w", err)
	}
	return record, true, nil
}

func upsertUnenrollment(ctx context.Context, db sqlExecutor, record store.UnenrollmentRecord) error {
	raw, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode agent unenrollment: %w", err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO agent_unenrollments
(tenant_id, enrollment_id, agent_id, certificate_serial, status, revoked_at, endpoint_completed_at, created_at, updated_at, data)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT (tenant_id, enrollment_id) DO UPDATE SET status=EXCLUDED.status,
endpoint_completed_at=EXCLUDED.endpoint_completed_at, updated_at=EXCLUDED.updated_at, data=EXCLUDED.data`,
		record.TenantID, record.EnrollmentID, record.AgentID, record.CertificateSerial, record.Status, record.RevokedAt,
		formatOptionalTime(record.EndpointCompletedAt), record.CreatedAt, record.UpdatedAt, raw)
	if err != nil {
		return fmt.Errorf("upsert agent unenrollment: %w", err)
	}
	return nil
}
