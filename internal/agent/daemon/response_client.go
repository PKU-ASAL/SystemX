package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	responsemodel "github.com/sysarmor/sysarmor-next-project/internal/response"
)

type ResponseClient struct {
	Manager string
	Token   string
	Client  *http.Client
}

func NewResponseClient(manager, token string, timeout time.Duration) *ResponseClient {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &ResponseClient{
		Manager: normalizeManagerURL(manager),
		Token:   token,
		Client:  &http.Client{Timeout: timeout},
	}
}

func (c *ResponseClient) Pending(ctx context.Context, tenantID, agentID string) ([]responsemodel.Command, error) {
	q := url.Values{}
	q.Set("pending", "true")
	q.Set("tenant_id", tenantID)
	q.Set("agent_id", agentID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Manager+"/api/v1/responses?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("pending responses failed: %s: %s", resp.Status, string(body))
	}
	var commands []responsemodel.Command
	if err := json.Unmarshal(body, &commands); err != nil {
		return nil, err
	}
	return commands, nil
}

func (c *ResponseClient) Ack(ctx context.Context, ack responsemodel.Ack) error {
	data, err := json.Marshal(ack)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Manager+"/api/v1/response-acks", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("X-SysArmor-Agent-Token", c.Token)
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("response ack failed: %s: %s", resp.Status, string(body))
	}
	return nil
}

func (c *ResponseClient) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return http.DefaultClient
}
