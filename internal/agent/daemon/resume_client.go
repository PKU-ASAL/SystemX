package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type ResumeClient struct {
	Manager  string
	Token    string
	TenantID string
	AgentID  string
	Client   *http.Client
}

type resumeCursorResponse struct {
	TenantID     string `json:"tenant_id"`
	AgentID      string `json:"agent_id"`
	SessionID    string `json:"session_id,omitempty"`
	ResumeCursor string `json:"resume_cursor,omitempty"`
}

func NewResumeClient(manager, token string, timeout time.Duration, tenantID, agentID string) *ResumeClient {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &ResumeClient{
		Manager:  normalizeManagerURL(manager),
		Token:    token,
		TenantID: tenantID,
		AgentID:  agentID,
		Client:   &http.Client{Timeout: timeout},
	}
}

func (c *ResumeClient) ResumeCursor(ctx context.Context) (string, error) {
	if c == nil {
		return "", fmt.Errorf("resume client is nil")
	}
	q := url.Values{}
	q.Set("tenant_id", c.TenantID)
	q.Set("agent_id", c.AgentID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Manager+"/api/v1/agent-gateway-resume?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	if c.Token != "" {
		req.Header.Set("X-SysArmor-Agent-Token", c.Token)
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("agentgateway resume failed: %s: %s", resp.Status, string(body))
	}
	var out resumeCursorResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	if out.AgentID != "" && out.AgentID != c.AgentID {
		return "", fmt.Errorf("agentgateway resume agent mismatch: got %q want %q", out.AgentID, c.AgentID)
	}
	return out.ResumeCursor, nil
}

func (c *ResumeClient) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return http.DefaultClient
}
