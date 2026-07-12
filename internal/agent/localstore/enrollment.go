package localstore

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type EnrollmentState string

const (
	StateStandalone EnrollmentState = "standalone"
	StateManaged    EnrollmentState = "managed"
)

type Enrollment struct {
	State               EnrollmentState
	TenantID            string
	AgentID             string
	GatewayAddress      string
	TLSCAPath           string
	TLSCertPath         string
	TLSKeyPath          string
	TLSServerName       string
	UploadHistory       bool
	ManagedFromSequence uint64
	UpdatedAt           time.Time
}

func (s *Store) Enrollment(ctx context.Context) (Enrollment, error) {
	var enrollment Enrollment
	var uploadHistory int
	var updatedAt int64
	err := s.db.QueryRowContext(ctx, `SELECT state, COALESCE(tenant_id,''), COALESCE(agent_id,''), COALESCE(gateway_address,''),
COALESCE(tls_ca_path,''), COALESCE(tls_cert_path,''), COALESCE(tls_key_path,''), COALESCE(tls_server_name,''), upload_history,
COALESCE(managed_from_seq,0), updated_at_ns FROM enrollment WHERE singleton = 1`).Scan(
		&enrollment.State, &enrollment.TenantID, &enrollment.AgentID, &enrollment.GatewayAddress,
		&enrollment.TLSCAPath, &enrollment.TLSCertPath, &enrollment.TLSKeyPath, &enrollment.TLSServerName,
		&uploadHistory, &enrollment.ManagedFromSequence, &updatedAt,
	)
	enrollment.UploadHistory = uploadHistory != 0
	enrollment.UpdatedAt = time.Unix(0, updatedAt).UTC()
	return enrollment, err
}

func (s *Store) SetManaged(ctx context.Context, enrollment Enrollment) error {
	if err := validateManaged(enrollment); err != nil {
		return err
	}
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `UPDATE enrollment SET state='managed', tenant_id=?, agent_id=?, gateway_address=?, tls_ca_path=?,
tls_cert_path=?, tls_key_path=?, tls_server_name=?, upload_history=?, managed_from_seq=?, updated_at_ns=? WHERE singleton=1`,
		enrollment.TenantID, enrollment.AgentID, enrollment.GatewayAddress, enrollment.TLSCAPath, enrollment.TLSCertPath,
		enrollment.TLSKeyPath, enrollment.TLSServerName, enrollment.UploadHistory, enrollment.ManagedFromSequence, now.UnixNano())
	return err
}

func (s *Store) SetStandalone(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE enrollment SET state='standalone', tenant_id=NULL, agent_id=NULL, gateway_address=NULL,
tls_ca_path=NULL, tls_cert_path=NULL, tls_key_path=NULL, tls_server_name=NULL, upload_history=0, managed_from_seq=NULL, updated_at_ns=? WHERE singleton=1`, time.Now().UTC().UnixNano())
	return err
}

func validateManaged(enrollment Enrollment) error {
	values := []string{enrollment.TenantID, enrollment.AgentID, enrollment.GatewayAddress, enrollment.TLSCAPath, enrollment.TLSCertPath, enrollment.TLSKeyPath}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("managed enrollment fields are incomplete")
		}
	}
	return nil
}
