package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
)

func TestPolicyClientFetchesEffectivePolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/effective-policy" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("X-SysArmor-Agent-Token"); got != "dev-token" {
			t.Fatalf("token header = %q", got)
		}
		q := r.URL.Query()
		if q.Get("tenant_id") != "tenant-a" || q.Get("agent_id") != "agent-a" || q.Get("scope_type") != "container" || q.Get("scope_selector") != "abc123" {
			t.Fatalf("query = %s", r.URL.RawQuery)
		}
		policy := policymodel.DefaultPolicy("tenant-a")
		policy.PolicyID = "policy-a"
		policy.Version = 7
		if err := json.NewEncoder(w).Encode(policy); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()
	client := NewPolicyClient(server.URL, "dev-token", time.Second)
	policy, err := client.EffectivePolicy(t.Context(), EffectivePolicyRequest{
		TenantID:      "tenant-a",
		AgentID:       "agent-a",
		ScopeType:     "container",
		ScopeSelector: "abc123",
	})
	if err != nil {
		t.Fatalf("EffectivePolicy() error = %v", err)
	}
	if policy.PolicyID != "policy-a" || policy.Version != 7 || policy.TenantID != "tenant-a" {
		t.Fatalf("policy = %+v", policy)
	}
}

func TestPolicyClientReturnsStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no policy", http.StatusNotFound)
	}))
	defer server.Close()
	client := NewPolicyClient(server.URL, "", time.Second)
	if _, err := client.EffectivePolicy(t.Context(), EffectivePolicyRequest{}); err == nil {
		t.Fatal("EffectivePolicy() error = nil")
	}
}
