package agentplane

import (
	"context"
	"fmt"

	controlplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/controlplane/v1"
	dataplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/dataplane/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/store"
	"github.com/sysarmor/sysarmor-next-project/internal/tlsconfig"
)

func validatePeerDataIdentity(ctx context.Context, header *dataplanev1.BatchHeader) (tlsconfig.PeerIdentity, bool, error) {
	peerID, ok := tlsconfig.PeerAgentIdentity(ctx)
	if !ok {
		return tlsconfig.PeerIdentity{}, false, nil
	}
	tenantID := header.GetTenantId()
	if tenantID == "" {
		tenantID = "default"
	}
	if peerID.TenantID != tenantID || peerID.AgentID != header.GetAgentId() {
		return peerID, true, fmt.Errorf("mTLS identity mismatch: cert=%s/%s batch=%s/%s", peerID.TenantID, peerID.AgentID, tenantID, header.GetAgentId())
	}
	return peerID, true, nil
}

func validatePeerControlIdentity(ctx context.Context, reqCtx *controlplanev1.RequestContext) (tlsconfig.PeerIdentity, bool, error) {
	peerID, ok := tlsconfig.PeerAgentIdentity(ctx)
	if !ok {
		return tlsconfig.PeerIdentity{}, false, nil
	}
	tenantID := reqCtx.GetTenantId()
	if tenantID == "" {
		tenantID = "default"
	}
	if peerID.TenantID != tenantID || peerID.AgentID != reqCtx.GetAgentId() {
		return peerID, true, fmt.Errorf("mTLS identity mismatch: cert=%s/%s frame=%s/%s", peerID.TenantID, peerID.AgentID, tenantID, reqCtx.GetAgentId())
	}
	return peerID, true, nil
}

func agentIdentityFromPeer(peerID tlsconfig.PeerIdentity, hostID, version string) store.AgentIdentity {
	return store.AgentIdentity{
		TenantID:     peerID.TenantID,
		AgentID:      peerID.AgentID,
		HostID:       hostID,
		Version:      version,
		AuthType:     "mtls",
		CertIdentity: peerID.Principal,
	}
}
