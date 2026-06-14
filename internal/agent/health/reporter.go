package health

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Reporter struct {
	Manager string
	Token   string
	Client  *http.Client
}

func NewReporter(manager, token string, timeout time.Duration) *Reporter {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Reporter{
		Manager: normalizeManagerURL(manager),
		Token:   token,
		Client:  &http.Client{Timeout: timeout},
	}
}

func (r *Reporter) Report(ctx context.Context, health AgentHealth) error {
	if r == nil {
		return fmt.Errorf("health reporter is nil")
	}
	data, err := json.Marshal(health)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.Manager+"/api/v1/agent-health", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if r.Token != "" {
		req.Header.Set("X-SysArmor-Agent-Token", r.Token)
	}
	resp, err := r.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("agent health report failed: %s: %s", resp.Status, string(body))
	}
	return nil
}

func (r *Reporter) client() *http.Client {
	if r.Client != nil {
		return r.Client
	}
	return http.DefaultClient
}

func normalizeManagerURL(manager string) string {
	if strings.HasPrefix(manager, "http://") || strings.HasPrefix(manager, "https://") {
		return strings.TrimRight(manager, "/")
	}
	return "http://" + strings.TrimRight(manager, "/")
}
