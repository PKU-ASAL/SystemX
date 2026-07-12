package managerapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	managerauth "github.com/sysarmor/sysarmor-next-project/internal/manager/auth"
)

const maxAuthenticatedBody = 4 << 20

func (s *Server) HandlerWithAuth(verifier *managerauth.Verifier) http.Handler {
	return verifier.Middleware(bindPrincipalTenant(s.Handler()))
}

func bindPrincipalTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		principal, ok := managerauth.PrincipalFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		query := r.URL.Query()
		if tenant := query.Get("tenant_id"); tenant != "" && tenant != principal.TenantID {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		query.Set("tenant_id", principal.TenantID)
		r.URL.RawQuery = query.Encode()
		if !bindJSONTenant(w, r, principal.TenantID) {
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requireProductionPrincipal(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		if _, ok := managerauth.PrincipalFromContext(r.Context()); !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bindJSONTenant(w http.ResponseWriter, r *http.Request, tenantID string) bool {
	if r.Body == nil || !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		return true
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxAuthenticatedBody+1))
	if err != nil || len(raw) > maxAuthenticatedBody {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return false
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		r.Body = io.NopCloser(bytes.NewReader(raw))
		return true
	}
	var object map[string]any
	if json.Unmarshal(raw, &object) != nil {
		r.Body = io.NopCloser(bytes.NewReader(raw))
		return true
	}
	if tenant, _ := object["tenant_id"].(string); tenant != "" && tenant != tenantID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	object["tenant_id"] = tenantID
	bound, err := json.Marshal(object)
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return false
	}
	r.Body = io.NopCloser(bytes.NewReader(bound))
	r.ContentLength = int64(len(bound))
	return true
}
