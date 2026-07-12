package localstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type PolicyRecord struct {
	Kind      string
	Version   uint64
	Document  []byte
	Digest    string
	UpdatedAt time.Time
}

func (s *Store) PutPolicy(ctx context.Context, policy PolicyRecord) error {
	if strings.TrimSpace(policy.Kind) == "" || len(policy.Document) == 0 || strings.TrimSpace(policy.Digest) == "" {
		return fmt.Errorf("policy kind, document, and digest are required")
	}
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `INSERT INTO policy(kind, version, document_json, digest, updated_at_ns)
VALUES (?, ?, ?, ?, ?) ON CONFLICT(kind) DO UPDATE SET version=excluded.version, document_json=excluded.document_json, digest=excluded.digest, updated_at_ns=excluded.updated_at_ns`,
		policy.Kind, policy.Version, policy.Document, policy.Digest, now.UnixNano())
	return err
}

func (s *Store) Policy(ctx context.Context, kind string) (PolicyRecord, bool, error) {
	var policy PolicyRecord
	var updatedAt int64
	err := s.db.QueryRowContext(ctx, `SELECT kind, version, document_json, digest, updated_at_ns FROM policy WHERE kind = ?`, kind).Scan(
		&policy.Kind, &policy.Version, &policy.Document, &policy.Digest, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return PolicyRecord{}, false, nil
	}
	if err != nil {
		return PolicyRecord{}, false, err
	}
	policy.UpdatedAt = time.Unix(0, updatedAt).UTC()
	return policy, true, nil
}
