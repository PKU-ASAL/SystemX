package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	controlv1 "github.com/sysarmor/sysarmor-next-project/api/proto/control/v1"
	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	"google.golang.org/grpc"
)

func newLocalHTTPServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	lis, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.Listener = lis
	server.Start()
	return server
}

func TestQueryAgentsFilters(t *testing.T) {
	var gotPath string
	server := newLocalHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	server := newLocalHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	server := newLocalHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	server := newLocalHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	server := newLocalHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestQueryLocalAgentCapabilityOverUnixSocket(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "agent.sock")
	lis, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	controlv1.RegisterAgentControlServiceServer(server, &fakeAgentControlServer{})
	go func() {
		_ = server.Serve(lis)
	}()
	defer server.Stop()

	body, err := queryLocalAgent(socketPath, []string{"agent", "capability", "--tenant-id", "default", "--agent-id", "agent-a"})
	if err != nil {
		t.Fatalf("queryLocalAgent() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode body: %v body=%s", err, string(body))
	}
	if got["agentId"] != "agent-a" || got["tenantId"] != "default" {
		t.Fatalf("body = %s", string(body))
	}
	sensor, ok := got["sensor"].(map[string]any)
	if !ok || sensor["backend"] != "fake" || sensor["supportsExec"] != true {
		t.Fatalf("sensor = %#v body=%s", got["sensor"], string(body))
	}
}

func TestQueryLocalAgentPolicyApplyOverUnixSocket(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "agent.sock")
	lis, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	fake := &fakeAgentControlServer{}
	controlv1.RegisterAgentControlServiceServer(server, fake)
	go func() {
		_ = server.Serve(lis)
	}()
	defer server.Stop()

	policyFile := filepath.Join(t.TempDir(), "policy.json")
	if err := os.WriteFile(policyFile, []byte(`{"policy_id":"local","version":2,"tenant_id":"default"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	body, err := queryLocalAgent(socketPath, []string{
		"policy", "apply",
		"--tenant-id", "default",
		"--agent-id", "agent-a",
		"--type", "agent-runtime",
		"--file", policyFile,
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("queryLocalAgent() error = %v", err)
	}
	if fake.applyReq == nil || fake.applyReq.GetPolicyType() != "agent-runtime" || !fake.applyReq.GetDryRun() {
		t.Fatalf("apply request = %+v", fake.applyReq)
	}
	if fake.applyReq.GetContext().GetAgentId() != "agent-a" || fake.applyReq.GetPolicyJson() == "" {
		t.Fatalf("apply request = %+v", fake.applyReq)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode body: %v body=%s", err, string(body))
	}
	if got["status"] != "validated" || got["policyId"] != "local" {
		t.Fatalf("body = %s", string(body))
	}
}

func TestQueryLocalAgentPolicyApplyCollectionFlags(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "agent.sock")
	lis, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	fake := &fakeAgentControlServer{}
	controlv1.RegisterAgentControlServiceServer(server, fake)
	go func() {
		_ = server.Serve(lis)
	}()
	defer server.Stop()

	_, err = queryLocalAgent(socketPath, []string{
		"policy", "apply", "collection",
		"--tenant-id", "default",
		"--agent-id", "agent-a",
		"--behavior", "network.connect",
		"--behavior", "file.write",
		"--file-prefix", "/dev/shm",
		"--socket-family", "AF_INET",
	})
	if err != nil {
		t.Fatalf("queryLocalAgent() error = %v", err)
	}
	if fake.applyReq == nil || fake.applyReq.GetPolicyType() != "collection" {
		t.Fatalf("apply request = %+v", fake.applyReq)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(fake.applyReq.GetPolicyJson()), &payload); err != nil {
		t.Fatalf("decode policy json: %v", err)
	}
	behaviors, ok := payload["behaviors"].([]any)
	if !ok || len(behaviors) != 2 {
		t.Fatalf("behaviors = %#v", payload["behaviors"])
	}
	first, ok := behaviors[0].(map[string]any)
	if !ok || first["id"] != "network.connect" {
		t.Fatalf("first behavior = %#v", behaviors[0])
	}
	second, ok := behaviors[1].(map[string]any)
	if !ok || second["id"] != "file.write" {
		t.Fatalf("second behavior = %#v", behaviors[1])
	}
	selectors, ok := second["selectors"].(map[string]any)
	if !ok {
		t.Fatalf("second selectors = %#v", second["selectors"])
	}
	file, ok := selectors["file"].(map[string]any)
	if !ok {
		t.Fatalf("file selector = %#v", selectors["file"])
	}
	prefixes, ok := file["prefixes"].([]any)
	if !ok || len(prefixes) != 1 || prefixes[0] != "/dev/shm" {
		t.Fatalf("file prefixes = %#v", file["prefixes"])
	}
}

func TestQueryLocalAgentWatchStreamsOverUnixSocket(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "agent.sock")
	lis, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	controlv1.RegisterAgentControlServiceServer(server, &fakeAgentControlServer{})
	go func() {
		_ = server.Serve(lis)
	}()
	defer server.Stop()

	eventBody, err := queryLocalAgent(socketPath, []string{"event", "watch", "--include-recent", "--limit", "1", "--behavior", "process.exec"})
	if err != nil {
		t.Fatalf("event watch error = %v", err)
	}
	if lines := nonEmptyLines(string(eventBody)); len(lines) != 1 {
		t.Fatalf("event body = %s", string(eventBody))
	}
	signalBody, err := queryLocalAgent(socketPath, []string{"signal", "watch", "--include-recent", "--limit", "1", "--rule-id", "payload_dropped", "--where", "endpoint"})
	if err != nil {
		t.Fatalf("signal watch error = %v", err)
	}
	if lines := nonEmptyLines(string(signalBody)); len(lines) != 1 {
		t.Fatalf("signal body = %s", string(signalBody))
	}
}

func TestQueryLocalAgentSignalWatchIncludesEvents(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "agent.sock")
	lis, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	fake := &fakeAgentControlServer{}
	controlv1.RegisterAgentControlServiceServer(server, fake)
	go func() {
		_ = server.Serve(lis)
	}()
	defer server.Stop()

	body, err := queryLocalAgent(socketPath, []string{"signal", "watch", "--include-recent", "--limit", "1", "--rule-id", "payload_dropped", "--where", "endpoint", "--include-events"})
	if err != nil {
		t.Fatalf("signal watch error = %v", err)
	}
	lines := nonEmptyLines(string(body))
	if len(lines) != 1 {
		t.Fatalf("signal body = %s", string(body))
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("decode body: %v body=%s", err, string(body))
	}
	events, ok := got["eventFrames"].([]any)
	if !ok || len(events) != 1 {
		t.Fatalf("eventFrames = %#v body=%s", got["eventFrames"], string(body))
	}
	if fake.getEventReq == nil || fake.getEventReq.GetEventId() != "ev-a" {
		t.Fatalf("get event request = %+v", fake.getEventReq)
	}
}

func TestQueryLocalAgentEventGetOverUnixSocket(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "agent.sock")
	lis, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	fake := &fakeAgentControlServer{}
	controlv1.RegisterAgentControlServiceServer(server, fake)
	go func() {
		_ = server.Serve(lis)
	}()
	defer server.Stop()

	body, err := queryLocalAgent(socketPath, []string{"event", "get", "--event-id", "ev-a"})
	if err != nil {
		t.Fatalf("event get error = %v", err)
	}
	if fake.getEventReq == nil || fake.getEventReq.GetEventId() != "ev-a" {
		t.Fatalf("get event request = %+v", fake.getEventReq)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode body: %v body=%s", err, string(body))
	}
	frame := got["frame"].(map[string]any)
	event := frame["event"].(map[string]any)
	if event["id"] != "ev-a" {
		t.Fatalf("body = %s", string(body))
	}
}

func TestQueryLocalAgentContentCommands(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "agent.sock")
	contentPath := filepath.Join(dir, "ioc.json")
	if err := os.WriteFile(contentPath, []byte(`{
		"api_version":"sysarmor.content/v1",
		"kind":"iocpack",
		"metadata":{"id":"ioc:c2-ip-feed","version":"2026.06.17.1"},
		"spec":{"value_type":"ip","values":["203.0.113.10"]}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	lis, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	fake := &fakeAgentControlServer{}
	controlv1.RegisterAgentControlServiceServer(server, fake)
	go func() {
		_ = server.Serve(lis)
	}()
	defer server.Stop()

	if _, err := queryLocalAgent(socketPath, []string{"content", "apply", "--file", contentPath, "--allow-unsigned"}); err != nil {
		t.Fatalf("content apply error = %v", err)
	}
	if fake.contentReq == nil || !fake.contentReq.GetAllowUnsigned() || !strings.Contains(fake.contentReq.GetContentJson(), "ioc:c2-ip-feed") {
		t.Fatalf("content request = %+v", fake.contentReq)
	}
	if _, err := queryLocalAgent(socketPath, []string{"content", "list", "--kind", "iocpack"}); err != nil {
		t.Fatalf("content list error = %v", err)
	}
	if _, err := queryLocalAgent(socketPath, []string{"content", "get", "--ref", "ioc:c2-ip-feed"}); err != nil {
		t.Fatalf("content get error = %v", err)
	}
}

func TestContentDiffBuildsPatch(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.json")
	newPath := filepath.Join(dir, "new.json")
	if err := os.WriteFile(oldPath, []byte(`{
		"api_version":"sysarmor.content/v1",
		"kind":"iocpack",
		"metadata":{"id":"ioc:c2-port-feed","version":"v1"},
		"spec":{"value_type":"port","values":["443","8443"]}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, []byte(`{
		"api_version":"sysarmor.content/v1",
		"kind":"iocpack",
		"metadata":{"id":"ioc:c2-port-feed","version":"v2"},
		"spec":{"value_type":"port","values":["9443","8443"]}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	body, err := contentDiff([]string{"content", "diff", "--file", oldPath, "--file", newPath, "--version", "v2"})
	if err != nil {
		t.Fatal(err)
	}
	var patch map[string]any
	if err := json.Unmarshal(body, &patch); err != nil {
		t.Fatal(err)
	}
	spec := patch["spec"].(map[string]any)
	ops := spec["ops"].([]any)
	if len(ops) != 2 {
		t.Fatalf("ops = %#v", ops)
	}
}

func TestQueryLocalAgentWatchReturnsPartialFramesOnTimeout(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "agent.sock")
	lis, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	controlv1.RegisterAgentControlServiceServer(server, &blockingAgentControlServer{})
	go func() {
		_ = server.Serve(lis)
	}()
	defer server.Stop()

	body, err := queryLocalAgent(socketPath, []string{"signal", "watch", "--include-recent", "--limit", "200", "--timeout", "10ms"})
	if err != nil {
		t.Fatalf("signal watch partial timeout error = %v", err)
	}
	if lines := nonEmptyLines(string(body)); len(lines) != 1 {
		t.Fatalf("signal body = %s", string(body))
	}
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

type fakeAgentControlServer struct {
	controlv1.UnimplementedAgentControlServiceServer
	applyReq    *controlv1.ApplyPolicyRequest
	contentReq  *controlv1.ApplyContentRequest
	getEventReq *controlv1.GetEventRequest
}

func (fakeAgentControlServer) Capability(ctx context.Context, req *controlv1.CapabilityRequest) (*controlv1.CapabilityResponse, error) {
	return &controlv1.CapabilityResponse{
		AgentId:  req.GetContext().GetAgentId(),
		TenantId: req.GetContext().GetTenantId(),
		Sensor:   &controlv1.SensorCapability{Backend: "fake", SupportsExec: true},
	}, nil
}

func (s *fakeAgentControlServer) ApplyPolicy(ctx context.Context, req *controlv1.ApplyPolicyRequest) (*controlv1.ControlAck, error) {
	s.applyReq = req
	return &controlv1.ControlAck{
		RequestId:     req.GetContext().GetRequestId(),
		TenantId:      req.GetContext().GetTenantId(),
		AgentId:       req.GetContext().GetAgentId(),
		Status:        "validated",
		PolicyId:      "local",
		PolicyVersion: 2,
	}, nil
}

func (s *fakeAgentControlServer) ApplyContent(ctx context.Context, req *controlv1.ApplyContentRequest) (*controlv1.ControlAck, error) {
	s.contentReq = req
	return &controlv1.ControlAck{
		RequestId: req.GetContext().GetRequestId(),
		TenantId:  req.GetContext().GetTenantId(),
		AgentId:   req.GetContext().GetAgentId(),
		Status:    "applied",
		Message:   "content applied",
	}, nil
}

func (s *fakeAgentControlServer) ListContent(ctx context.Context, req *controlv1.ListContentRequest) (*controlv1.ListContentResponse, error) {
	return &controlv1.ListContentResponse{Records: []*controlv1.ContentRecord{{
		Ref:     "ioc:c2-ip-feed",
		Kind:    "iocpack",
		Version: "2026.06.17.1",
	}}}, nil
}

func (s *fakeAgentControlServer) GetContent(ctx context.Context, req *controlv1.GetContentRequest) (*controlv1.ContentGetResponse, error) {
	return &controlv1.ContentGetResponse{Record: &controlv1.ContentRecord{
		Ref:     req.GetRef(),
		Kind:    "iocpack",
		Version: "2026.06.17.1",
	}}, nil
}

func (s *fakeAgentControlServer) GetEvent(ctx context.Context, req *controlv1.GetEventRequest) (*controlv1.EventGetResponse, error) {
	s.getEventReq = req
	return &controlv1.EventGetResponse{Frame: &controlv1.EventFrame{
		TenantId: "default",
		AgentId:  "agent-a",
		Sequence: 1,
		Event: &eventv1.CanonicalEvent{
			Id:       req.GetEventId(),
			Behavior: "process.exec",
		},
	}}, nil
}

func (s *fakeAgentControlServer) WatchEvents(req *controlv1.WatchEventsRequest, stream controlv1.AgentControlService_WatchEventsServer) error {
	return stream.Send(&controlv1.EventFrame{
		TenantId: "default",
		AgentId:  "agent-a",
		Sequence: 1,
		Event: &eventv1.CanonicalEvent{
			Id:       "ev-a",
			Behavior: "process.exec",
		},
	})
}

func (s *fakeAgentControlServer) WatchSignals(req *controlv1.WatchSignalsRequest, stream controlv1.AgentControlService_WatchSignalsServer) error {
	return stream.Send(&controlv1.SignalFrame{
		TenantId: "default",
		AgentId:  "agent-a",
		Sequence: 1,
		Signal: &signalv1.Signal{
			Id:        "sig-a",
			Name:      "payload_dropped",
			Where:     signalv1.SignalWhere_SIGNAL_WHERE_ENDPOINT,
			EventRefs: []string{"ev-a"},
		},
	})
}

type blockingAgentControlServer struct {
	controlv1.UnimplementedAgentControlServiceServer
}

func (s *blockingAgentControlServer) WatchSignals(req *controlv1.WatchSignalsRequest, stream controlv1.AgentControlService_WatchSignalsServer) error {
	if err := stream.Send(&controlv1.SignalFrame{
		TenantId: "default",
		AgentId:  "agent-a",
		Sequence: 1,
		Signal: &signalv1.Signal{
			Id:    "sig-a",
			Name:  "payload_dropped",
			Where: signalv1.SignalWhere_SIGNAL_WHERE_ENDPOINT,
		},
	}); err != nil {
		return err
	}
	<-stream.Context().Done()
	return stream.Context().Err()
}
