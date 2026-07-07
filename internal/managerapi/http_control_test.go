package managerapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	controlmodel "github.com/sysarmor/sysarmor-next-project/internal/controlmodel"
	"github.com/sysarmor/sysarmor-next-project/internal/store"
)

func TestControlCommandsAPICreatesAuditableDownlink(t *testing.T) {
	st := &store.Store{}
	handler := NewServerWithOperatorToken(st, "operator-token").Handler()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/control-commands", strings.NewReader(`{
		"command_id":"ctrl-content-api",
		"tenant_id":"default",
		"agent_id":"agent-a",
		"type":"content_update",
		"payload_json":{"api_version":"sysarmor.content/v1","kind":"iocpack","metadata":{"id":"ioc:test","version":"v1"},"spec":{"value_type":"ip","values":["10.0.0.1"]}},
		"reason":"refresh ioc"
	}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("control command without operator token status = %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/control-commands", strings.NewReader(`{
		"command_id":"ctrl-content-api",
		"tenant_id":"default",
		"agent_id":"agent-a",
		"type":"content_update",
		"payload_json":{"api_version":"sysarmor.content/v1","kind":"iocpack","metadata":{"id":"ioc:test","version":"v1"},"spec":{"value_type":"ip","values":["10.0.0.1"]}},
		"reason":"refresh ioc"
	}`))
	req.Header.Set("X-SysArmor-Operator-Token", "operator-token")
	req.Header.Set("X-SysArmor-Role", "control_admin")
	req.Header.Set("X-SysArmor-Actor", "control-operator")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"command_id":"ctrl-content-api"`) || !strings.Contains(rec.Body.String(), `"actor":"control-operator"`) {
		t.Fatalf("control command create status = %d body=%s", rec.Code, rec.Body.String())
	}

	got := st.PendingControlCommands("default", "agent-a")
	if len(got) != 1 || got[0].Type != controlmodel.ControlCommandTypeContentUpdate || got[0].Reason != "refresh ioc" || got[0].ContentRef != "ioc:test" || got[0].ContentKind != "iocpack" || got[0].ContentVersion != "v1" {
		t.Fatalf("pending control commands = %+v", got)
	}
	rec = get(t, handler, "/api/v1/control-commands?tenant_id=default&agent_id=agent-a&type=content_update")
	if !strings.Contains(rec.Body.String(), `"status":"pending"`) || !strings.Contains(rec.Body.String(), `"reason":"refresh ioc"`) || !strings.Contains(rec.Body.String(), `"content_ref":"ioc:test"`) {
		t.Fatalf("control command audit response = %s", rec.Body.String())
	}
}

func TestControlCommandActionsUpdateLifecycle(t *testing.T) {
	st := &store.Store{}
	handler := NewServer(st).Handler()
	st.CreateControlCommand(controlmodel.ControlCommand{
		CommandID:   "ctrl-action",
		TenantID:    "default",
		AgentID:     "agent-a",
		Type:        controlmodel.ControlCommandTypeContentUpdate,
		PayloadJSON: []byte(`{"kind":"iocpack"}`),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/control-commands", strings.NewReader(`{
		"action":"cancel",
		"command_id":"ctrl-action",
		"tenant_id":"default",
		"agent_id":"agent-a",
		"actor":"operator",
		"reason":"bad rollout"
	}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"canceled"`) || !strings.Contains(rec.Body.String(), `"error":"bad rollout"`) {
		t.Fatalf("cancel status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := st.PendingControlCommands("default", "agent-a"); len(got) != 0 {
		t.Fatalf("pending after cancel = %+v", got)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/control-commands", strings.NewReader(`{
		"action":"retry",
		"command_id":"ctrl-action",
		"tenant_id":"default",
		"agent_id":"agent-a",
		"actor":"operator",
		"reason":"retry rollout"
	}`))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"pending"`) || !strings.Contains(rec.Body.String(), `"reason":"retry rollout"`) {
		t.Fatalf("retry status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := st.PendingControlCommands("default", "agent-a"); len(got) != 1 || got[0].CommandID != "ctrl-action" {
		t.Fatalf("pending after retry = %+v", got)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/control-commands", strings.NewReader(`{
		"action":"expire",
		"command_id":"ctrl-action",
		"tenant_id":"default",
		"agent_id":"agent-a",
		"reason":"ttl elapsed"
	}`))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"expired"`) || !strings.Contains(rec.Body.String(), `"error":"ttl elapsed"`) {
		t.Fatalf("expire status = %d body=%s", rec.Code, rec.Body.String())
	}
}
