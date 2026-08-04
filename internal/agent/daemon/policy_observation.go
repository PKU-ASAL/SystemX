package daemon

import (
	"context"
	"fmt"

	controlplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/controlplane/v1"
	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/localstore"
	agentpolicy "github.com/sysarmor/sysarmor-next-project/internal/agent/policy"
)

func (r *AgentRuntime) pendingPolicyStatus(ctx context.Context) (agenthealth.PendingPolicyStatus, error) {
	if r.localStore == nil {
		return agenthealth.PendingPolicyStatus{}, nil
	}
	record, status, ok, err := r.localStore.DesiredPolicy(ctx, "endpoint", localstore.PolicySourceManaged)
	if err != nil || !ok {
		return agenthealth.PendingPolicyStatus{}, err
	}
	policy, err := agentpolicy.ParseEndpointPolicy(record.Document)
	if err != nil {
		return agenthealth.PendingPolicyStatus{}, fmt.Errorf("parse pending endpoint policy: %w", err)
	}
	return agenthealth.PendingPolicyStatus{
		Status: string(status), Source: string(localstore.PolicySourceManaged), PolicyID: policy.PolicyID,
		Version: record.Version, Digest: record.Digest,
	}, nil
}

func pendingPolicyMessage(status agenthealth.PendingPolicyStatus) *controlplanev1.PendingPolicyStatus {
	if status.Status == "" {
		return nil
	}
	return &controlplanev1.PendingPolicyStatus{
		Status: status.Status, Source: status.Source, PolicyId: status.PolicyID,
		Version: status.Version, Digest: status.Digest,
	}
}
