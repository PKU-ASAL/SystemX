package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	controlv1 "github.com/sysarmor/sysarmor-next-project/api/proto/control/v1"
	gatewaymodel "github.com/sysarmor/sysarmor-next-project/internal/agentplane/model"
	"github.com/sysarmor/sysarmor-next-project/internal/tlsconfig"
)

type ControlEvidenceClient struct {
	Manager string
	Token   string
	Timeout time.Duration
	TLS     tlsconfig.ClientConfig
}

func NewControlEvidenceClient(manager, token string, timeout time.Duration) *ControlEvidenceClient {
	return NewControlEvidenceClientWithTLS(manager, token, timeout, tlsconfig.ClientConfig{})
}

func NewControlEvidenceClientWithTLS(manager, token string, timeout time.Duration, tlsCfg tlsconfig.ClientConfig) *ControlEvidenceClient {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &ControlEvidenceClient{Manager: normalizeGRPCAddress(manager), Token: token, Timeout: timeout, TLS: tlsCfg}
}

func (c *ControlEvidenceClient) Pending(ctx context.Context, tenantID, agentID string) ([]gatewaymodel.EvidencePullbackRequest, error) {
	frames, err := controlStreamHello(ctx, c.Manager, c.Token, c.Timeout, c.TLS, tenantID, agentID, "", "")
	if err != nil {
		return nil, err
	}
	var out []gatewaymodel.EvidencePullbackRequest
	for _, frame := range frames {
		if frame.GetType() != "evidence_pullback" {
			continue
		}
		req, err := evidencePullbackFromControl(frame.GetEvidencePullback())
		if err != nil {
			return nil, err
		}
		out = append(out, gatewaymodel.NormalizeEvidencePullback(req))
	}
	return out, nil
}

func (c *ControlEvidenceClient) Result(ctx context.Context, result gatewaymodel.EvidencePullbackResult) error {
	if c == nil {
		return fmt.Errorf("control evidence client is nil")
	}
	return controlStreamSend(ctx, c.Manager, c.Token, c.Timeout, c.TLS, &controlv1.ControlStreamFrame{
		Type:      "evidence_pullback_result",
		RequestId: result.RequestID,
		Context:   &controlv1.RequestContext{TenantId: result.TenantID, AgentId: result.AgentID},
		EvidenceResult: &controlv1.EvidencePullbackResult{
			RequestId:    result.RequestID,
			TenantId:     result.TenantID,
			AgentId:      result.AgentID,
			Ok:           result.OK,
			Message:      result.Message,
			EvidenceJson: append([]byte(nil), result.Evidence...),
			ObservedAt:   result.ObservedAt.UTC().Format(time.RFC3339Nano),
		},
	})
}

func evidencePullbackFromControl(in *controlv1.EvidencePullbackRequest) (gatewaymodel.EvidencePullbackRequest, error) {
	if in == nil {
		return gatewaymodel.EvidencePullbackRequest{}, fmt.Errorf("control stream missing evidence_pullback")
	}
	if in.GetRawJson() != "" {
		var req gatewaymodel.EvidencePullbackRequest
		if err := json.Unmarshal([]byte(in.GetRawJson()), &req); err != nil {
			return gatewaymodel.EvidencePullbackRequest{}, fmt.Errorf("decode control stream evidence pullback raw_json: %w", err)
		}
		return req, nil
	}
	return gatewaymodel.EvidencePullbackRequest{
		RequestID:  in.GetRequestId(),
		TenantID:   in.GetTenantId(),
		AgentID:    in.GetAgentId(),
		IncidentID: in.GetIncidentId(),
		Scenario:   in.GetScenario(),
		Target:     in.GetTarget(),
		Reason:     in.GetReason(),
		Status:     in.GetStatus(),
		Actor:      in.GetActor(),
	}, nil
}
