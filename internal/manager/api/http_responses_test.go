package managerapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	managerauth "github.com/sysarmor/sysarmor-next-project/internal/manager/auth"
	"github.com/sysarmor/sysarmor-next-project/internal/store"
	policymodel "github.com/sysarmor/sysarmor-next-project/packages/policy"
	responsemodel "github.com/sysarmor/sysarmor-next-project/packages/response"
)

func TestResponsePolicyCanRequireApproval(t *testing.T) {
	st := &store.Store{}
	handler := newAdminTestServer(st).Handler()
	policy := policymodel.DefaultPolicy("default")
	policy.PolicyID = "approval-policy"
	policy.Version = 4
	policy.Response = responsemodel.Policy{
		AllowedActions:   []string{"collect"},
		AllowedModes:     []string{"observe"},
		ApprovalRequired: true,
	}
	policyData, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/policies", strings.NewReader(string(policyData)))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("policy post status = %d body=%s", rec.Code, rec.Body.String())
	}
	assignmentData := `{"tenant_id":"default","agent_id":"agent-response-policy","policy_id":"approval-policy","policy_version":4}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/policy-assignments", strings.NewReader(assignmentData))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("assignment post status = %d body=%s", rec.Code, rec.Body.String())
	}
	cmd := `{"tenant_id":"default","agent_id":"agent-response-policy","action":"collect","target":"process:p1"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/responses", strings.NewReader(cmd))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("response post status = %d body=%s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"policy_id":"approval-policy"`, `"policy_version":4`, `"status":"pending_approval"`, `"approval_required":true`, `"approval_status":"required"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("response policy output missing %s: %s", want, rec.Body.String())
		}
	}
	rec = get(t, handler, "/api/v1/responses?tenant_id=default&agent_id=agent-response-policy&pending=true")
	if strings.Contains(rec.Body.String(), `"approval-policy"`) {
		t.Fatalf("pending_approval response should not be pending before approval: %s", rec.Body.String())
	}
}

func TestResponsePolicyCanRequireMultiApprovalRoles(t *testing.T) {
	st := &store.Store{}
	testServer := newAdminTestServer(st)
	handler := testServer.Handler()
	policy := policymodel.DefaultPolicy("default")
	policy.PolicyID = "multi-approval-policy"
	policy.Version = 5
	policy.Response = responsemodel.Policy{
		AllowedActions:    []string{"collect"},
		AllowedModes:      []string{"observe"},
		ApprovalRequired:  true,
		ApprovalThreshold: 2,
		ApprovalRoles:     []string{"admin"},
	}
	policyData, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/policies", strings.NewReader(string(policyData)))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("policy post status = %d body=%s", rec.Code, rec.Body.String())
	}
	assignmentData := `{"tenant_id":"default","agent_id":"agent-multi-approval","policy_id":"multi-approval-policy","policy_version":5}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/policy-assignments", strings.NewReader(assignmentData))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("assignment post status = %d body=%s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/responses", strings.NewReader(`{"response_id":"resp-multi-http","tenant_id":"default","agent_id":"agent-multi-approval","action":"collect","target":"process:p1"}`))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("response post status = %d body=%s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"approval_threshold":2`, `"approval_roles":["admin"]`, `"status":"pending_approval"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("response policy output missing %s: %s", want, rec.Body.String())
		}
	}

	testServer.principal = managerauth.Principal{Subject: "operator-a", TenantID: "default", Roles: []string{"operator"}}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/response-approvals", strings.NewReader(`{"tenant_id":"default","agent_id":"agent-multi-approval","response_id":"resp-multi-http","approved":true}`))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("wrong-role approval status = %d body=%s", rec.Code, rec.Body.String())
	}
	testServer.principal = managerauth.Principal{Subject: "admin-a", TenantID: "default", Roles: []string{"admin"}}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/response-approvals", strings.NewReader(`{"tenant_id":"default","agent_id":"agent-multi-approval","response_id":"resp-multi-http","approved":true}`))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"approval_status":"partial"`) {
		t.Fatalf("first approval status = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = get(t, handler, "/api/v1/responses?tenant_id=default&agent_id=agent-multi-approval&pending=true")
	if strings.Contains(rec.Body.String(), `"resp-multi-http"`) {
		t.Fatalf("partial approval response should not be pending: %s", rec.Body.String())
	}
	testServer.principal = managerauth.Principal{Subject: "admin-b", TenantID: "default", Roles: []string{"admin"}}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/response-approvals", strings.NewReader(`{"tenant_id":"default","agent_id":"agent-multi-approval","response_id":"resp-multi-http","approved":true}`))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"approval_status":"approved"`) {
		t.Fatalf("second approval status = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = get(t, handler, "/api/v1/responses?tenant_id=default&agent_id=agent-multi-approval&pending=true")
	if !strings.Contains(rec.Body.String(), `"resp-multi-http"`) {
		t.Fatalf("approved response should be pending: %s", rec.Body.String())
	}
}
