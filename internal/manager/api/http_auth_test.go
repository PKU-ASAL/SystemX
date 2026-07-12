package managerapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	managerauth "github.com/sysarmor/sysarmor-next-project/internal/manager/auth"
)

func TestBindPrincipalTenantRejectsMismatch(t *testing.T) {
	handler := bindPrincipalTenant(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next handler called")
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents?tenant_id=tenant-b", nil)
	req = req.WithContext(managerauth.WithPrincipal(req.Context(), managerauth.Principal{Subject: "user", TenantID: "tenant-a", Roles: []string{"viewer"}}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestBindPrincipalTenantInjectsQueryAndJSONBody(t *testing.T) {
	handler := bindPrincipalTenant(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.URL.Query().Get("tenant_id") != "tenant-a" || !strings.Contains(string(body), `"tenant_id":"tenant-a"`) {
			t.Fatalf("request tenant not bound: query=%s body=%s", r.URL.RawQuery, body)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/policies", strings.NewReader(`{"policy_id":"p1"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(managerauth.WithPrincipal(req.Context(), managerauth.Principal{Subject: "user", TenantID: "tenant-a", Roles: []string{"operator"}}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestOperatorPrincipalCannotSatisfyAdminRequirement(t *testing.T) {
	server := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin", nil)
	req = req.WithContext(managerauth.WithPrincipal(req.Context(), managerauth.Principal{Subject: "user", TenantID: "tenant-a", Roles: []string{"operator"}}))
	rec := httptest.NewRecorder()
	allowed := server.requireOperator(rec, req, "admin")
	if allowed || rec.Code != http.StatusForbidden {
		t.Fatalf("operator satisfied admin requirement: allowed=%t status=%d", allowed, rec.Code)
	}
}
