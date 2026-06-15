package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestQueryAgentsFilters(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		_, _ = fmt.Fprintln(w, "[]")
	}))
	defer server.Close()

	if _, err := query(server.URL, []string{"agents", "--tenant-id", "default", "--scope-type", "container", "--scope-selector", "abc123", "--health-status", "ok"}); err != nil {
		t.Fatalf("query() error = %v", err)
	}
	want := "/api/v1/agents?health_status=ok&scope_selector=abc123&scope_type=container&tenant_id=default"
	if gotPath != want {
		t.Fatalf("path = %q, want %q", gotPath, want)
	}
}

func TestQueryPolicyCommands(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		_, _ = fmt.Fprintln(w, "{}")
	}))
	defer server.Close()

	if _, err := query(server.URL, []string{"rules", "--where", "cloud"}); err != nil {
		t.Fatalf("rules query error = %v", err)
	}
	if gotPath != "/api/v1/rules?where=cloud" {
		t.Fatalf("rules path = %q", gotPath)
	}

	if _, err := query(server.URL, []string{"effective-policy", "--tenant-id", "default", "--agent-id", "agent-a", "--scope-type", "container", "--scope-selector", "abc123"}); err != nil {
		t.Fatalf("effective-policy query error = %v", err)
	}
	want := "/api/v1/effective-policy?agent_id=agent-a&scope_selector=abc123&scope_type=container&tenant_id=default"
	if gotPath != want {
		t.Fatalf("effective-policy path = %q, want %q", gotPath, want)
	}
}
