package store

import (
	"strings"

	dataplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/dataplane/v1"
)

type AgentIdentity struct {
	AgentID      string `json:"agent_id"`
	HostID       string `json:"host_id,omitempty"`
	Version      string `json:"version,omitempty"`
	TenantID     string `json:"tenant_id,omitempty"`
	AuthType     string `json:"auth_type,omitempty"`
	CertIdentity string `json:"cert_identity,omitempty"`
}

func AgentIdentityFromDataBatch(batch *dataplanev1.DataBatch) AgentIdentity {
	header := batch.GetHeader()
	agent := AgentIdentity{
		AgentID:  header.GetAgentId(),
		HostID:   header.GetHostId(),
		TenantID: header.GetTenantId(),
	}
	if header.GetLabels() != nil {
		agent.Version = header.GetLabels()["agent_version"]
	}
	return agent.Normalized()
}

func (a AgentIdentity) Normalized() AgentIdentity {
	a.AgentID = strings.TrimSpace(a.AgentID)
	a.HostID = strings.TrimSpace(a.HostID)
	a.Version = strings.TrimSpace(a.Version)
	a.TenantID = strings.TrimSpace(a.TenantID)
	a.AuthType = strings.TrimSpace(a.AuthType)
	a.CertIdentity = strings.TrimSpace(a.CertIdentity)
	if a.TenantID == "" {
		a.TenantID = "default"
	}
	return a
}

func (a AgentIdentity) Valid() bool {
	return strings.TrimSpace(a.AgentID) != ""
}
