package managerapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sysarmor/sysarmor-next-project/internal/store"
	controlmodel "github.com/sysarmor/sysarmor-next-project/packages/contracts/controlmodel"
)

func TestControlCommandsAPICreatesAuditableDownlink(t *testing.T) {
	st := &store.Store{}
	handler := NewServer(st).Handler()

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
		t.Fatalf("control command without principal status = %d body=%s", rec.Code, rec.Body.String())
	}
	handler = newAdminTestServer(st).Handler()

	req = httptest.NewRequest(http.MethodPost, "/api/v1/control-commands", strings.NewReader(`{
		"command_id":"ctrl-content-api",
		"tenant_id":"default",
		"agent_id":"agent-a",
		"type":"content_update",
		"payload_json":{"api_version":"sysarmor.content/v1","kind":"iocpack","metadata":{"id":"ioc:test","version":"v1"},"spec":{"value_type":"ip","values":["10.0.0.1"]}},
		"reason":"refresh ioc"
	}`))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"command_id":"ctrl-content-api"`) || !strings.Contains(rec.Body.String(), `"actor":"test-admin"`) {
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

func TestControlCommandCreateDoesNotRepeatPersistence(t *testing.T) {
	st := &saveCountingManagerStore{Store: &store.Store{}}
	handler := newAdminTestServer(st).Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/control-commands", strings.NewReader(`{
		"command_id":"ctrl-single-write",
		"tenant_id":"default",
		"agent_id":"agent-a",
		"type":"content_update",
		"payload_json":{"kind":"iocpack"}
	}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || st.saves != 0 {
		t.Fatalf("control command create status=%d saves=%d body=%s, want success without full save", rec.Code, st.saves, rec.Body.String())
	}
}

func TestEvidencePullbackCreatePersistsStore(t *testing.T) {
	st := &saveCountingManagerStore{Store: &store.Store{}}
	handler := newAdminTestServer(st).Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/evidence-pullbacks", strings.NewReader(`{
		"request_id":"pullback-persist",
		"tenant_id":"default",
		"agent_id":"agent-a"
	}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || st.saves != 1 {
		t.Fatalf("evidence pullback create status=%d saves=%d body=%s, want one save", rec.Code, st.saves, rec.Body.String())
	}
}

type saveCountingManagerStore struct {
	*store.Store
	saves int
}

func (s *saveCountingManagerStore) Save() error {
	s.saves++
	return nil
}

func TestControlCommandActionsUpdateLifecycle(t *testing.T) {
	st := &store.Store{}
	handler := newAdminTestServer(st).Handler()
	if _, err := st.CreateControlCommand(controlmodel.ControlCommand{
		CommandID:   "ctrl-action",
		TenantID:    "default",
		AgentID:     "agent-a",
		Type:        controlmodel.ControlCommandTypeContentUpdate,
		PayloadJSON: []byte(`{"kind":"iocpack"}`),
	}); err != nil {
		t.Fatal(err)
	}

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
