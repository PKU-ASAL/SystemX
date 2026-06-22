package daemon

import (
	"context"
	"fmt"
	"time"

	"github.com/sysarmor/sysarmor-next-project/internal/tlsconfig"
)

type ControlResumeClient struct {
	Manager  string
	Token    string
	Timeout  time.Duration
	TenantID string
	AgentID  string
	TLS      tlsconfig.ClientConfig
}

func NewControlResumeClient(manager, token string, timeout time.Duration, tenantID, agentID string) *ControlResumeClient {
	return NewControlResumeClientWithTLS(manager, token, timeout, tenantID, agentID, tlsconfig.ClientConfig{})
}

func NewControlResumeClientWithTLS(manager, token string, timeout time.Duration, tenantID, agentID string, tlsCfg tlsconfig.ClientConfig) *ControlResumeClient {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &ControlResumeClient{
		Manager:  normalizeGRPCAddress(manager),
		Token:    token,
		Timeout:  timeout,
		TenantID: tenantID,
		AgentID:  agentID,
		TLS:      tlsCfg,
	}
}

func (c *ControlResumeClient) ResumeCursor(ctx context.Context) (string, error) {
	if c == nil {
		return "", fmt.Errorf("control resume client is nil")
	}
	frames, err := controlStreamHello(ctx, c.Manager, c.Token, c.Timeout, c.TLS, c.TenantID, c.AgentID, "", "")
	if err != nil {
		return "", err
	}
	for _, frame := range frames {
		if frame.GetType() != "resume" {
			continue
		}
		payload := frame.GetResume()
		if payload.GetAgentId() != "" && payload.GetAgentId() != c.AgentID {
			return "", fmt.Errorf("control resume agent mismatch: got %q want %q", payload.GetAgentId(), c.AgentID)
		}
		return payload.GetResumeCursor(), nil
	}
	return "", nil
}
