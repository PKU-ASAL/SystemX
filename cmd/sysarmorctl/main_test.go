package main

import (
	"encoding/json"
	"fmt"
	"io"
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

func TestQueryRarityBaseline(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		_, _ = fmt.Fprintln(w, "{}")
	}))
	defer server.Close()

	if _, err := query(server.URL, []string{"rarity-baseline", "--workload", "container:checkout-api", "--signal", "download_by_lolbin"}); err != nil {
		t.Fatalf("rarity-baseline query error = %v", err)
	}
	want := "/api/v1/rarity-baseline?signal=download_by_lolbin&workload=container%3Acheckout-api"
	if gotPath != want {
		t.Fatalf("path = %q, want %q", gotPath, want)
	}
}

func TestOperatorRoleBindingsCommand(t *testing.T) {
	var gotMethod string
	var gotPath string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.String()
		if r.Method == http.MethodPost {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if err := json.Unmarshal(body, &gotBody); err != nil {
				t.Fatalf("decode body: %v body=%s", err, string(body))
			}
		}
		_, _ = fmt.Fprintln(w, "{}")
	}))
	defer server.Close()

	if _, err := query(server.URL, []string{"operator-role-bindings", "--upsert", "--actor", "alice", "--roles", "policy_admin,responder"}); err != nil {
		t.Fatalf("operator-role-bindings upsert error = %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/operator-role-bindings" {
		t.Fatalf("upsert method/path = %s %s", gotMethod, gotPath)
	}
	if gotBody["actor"] != "alice" {
		t.Fatalf("actor = %v", gotBody["actor"])
	}
	roles, ok := gotBody["roles"].([]any)
	if !ok || len(roles) != 2 || roles[0] != "policy_admin" || roles[1] != "responder" {
		t.Fatalf("roles = %#v", gotBody["roles"])
	}

	if _, err := query(server.URL, []string{"operator-role-bindings", "--actor", "alice"}); err != nil {
		t.Fatalf("operator-role-bindings list error = %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/api/v1/operator-role-bindings?actor=alice" {
		t.Fatalf("list method/path = %s %s", gotMethod, gotPath)
	}
}

func TestEvidencePullbackCommand(t *testing.T) {
	var gotMethod string
	var gotPath string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.String()
		if r.Method == http.MethodPost {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if err := json.Unmarshal(body, &gotBody); err != nil {
				t.Fatalf("decode body: %v body=%s", err, string(body))
			}
		}
		_, _ = fmt.Fprintln(w, "{}")
	}))
	defer server.Close()

	if _, err := query(server.URL, []string{
		"evidence-pullbacks",
		"--create",
		"--request-id", "evpb-a",
		"--tenant-id", "default",
		"--agent-id", "agent-a",
		"--incident-id", "inc-a",
		"--target", "process:p1",
		"--reason", "collect process tree",
	}); err != nil {
		t.Fatalf("create query error = %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/evidence-pullbacks" {
		t.Fatalf("create method/path = %s %s", gotMethod, gotPath)
	}
	for key, want := range map[string]string{
		"request_id":  "evpb-a",
		"tenant_id":   "default",
		"agent_id":    "agent-a",
		"incident_id": "inc-a",
		"target":      "process:p1",
		"reason":      "collect process tree",
	} {
		if gotBody[key] != want {
			t.Fatalf("body[%s] = %v, want %s", key, gotBody[key], want)
		}
	}

	if _, err := query(server.URL, []string{"evidence-pullbacks", "--tenant-id", "default", "--agent-id", "agent-a"}); err != nil {
		t.Fatalf("list query error = %v", err)
	}
	wantPath := "/api/v1/evidence-pullbacks?agent_id=agent-a&tenant_id=default"
	if gotMethod != http.MethodGet || gotPath != wantPath {
		t.Fatalf("list method/path = %s %s, want GET %s", gotMethod, gotPath, wantPath)
	}
}
