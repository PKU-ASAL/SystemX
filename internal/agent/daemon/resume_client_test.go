package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestResumeClientFetchesCursor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/link1-resume" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("X-SysArmor-Agent-Token"); got != "dev-token" {
			t.Fatalf("token header = %q", got)
		}
		q := r.URL.Query()
		if q.Get("tenant_id") != "tenant-a" || q.Get("agent_id") != "agent-a" {
			t.Fatalf("query = %s", r.URL.RawQuery)
		}
		if err := json.NewEncoder(w).Encode(resumeCursorResponse{
			TenantID:     "tenant-a",
			AgentID:      "agent-a",
			SessionID:    "session-a",
			ResumeCursor: "00000000000000000042",
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()
	client := NewResumeClient(server.URL, "dev-token", time.Second, "tenant-a", "agent-a")
	cursor, err := client.ResumeCursor(t.Context())
	if err != nil {
		t.Fatalf("ResumeCursor() error = %v", err)
	}
	if cursor != "00000000000000000042" {
		t.Fatalf("cursor = %q", cursor)
	}
}

func TestResumeClientRejectsAgentMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if err := json.NewEncoder(w).Encode(resumeCursorResponse{
			TenantID:     "tenant-a",
			AgentID:      "agent-b",
			ResumeCursor: "00000000000000000042",
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()
	client := NewResumeClient(server.URL, "", time.Second, "tenant-a", "agent-a")
	if _, err := client.ResumeCursor(t.Context()); err == nil {
		t.Fatal("ResumeCursor() error = nil")
	}
}

func TestResumeClientReturnsStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no cursor", http.StatusNotFound)
	}))
	defer server.Close()
	client := NewResumeClient(server.URL, "", time.Second, "tenant-a", "agent-a")
	if _, err := client.ResumeCursor(t.Context()); err == nil {
		t.Fatal("ResumeCursor() error = nil")
	}
}
