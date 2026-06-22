package daemon

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	controlv1 "github.com/sysarmor/sysarmor-next-project/api/proto/control/v1"
	dataplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/dataplane/v1"
	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	incidentv1 "github.com/sysarmor/sysarmor-next-project/api/proto/incident/v1"
	sensorv1 "github.com/sysarmor/sysarmor-next-project/api/proto/sensor/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/config"
	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/spool"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/tamper"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/uploadworker"
	"github.com/sysarmor/sysarmor-next-project/internal/agentplane"
	gatewaymodel "github.com/sysarmor/sysarmor-next-project/internal/agentplane/model"
	"github.com/sysarmor/sysarmor-next-project/internal/endpoint/normalize"
	"github.com/sysarmor/sysarmor-next-project/internal/managerapi"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
	responsemodel "github.com/sysarmor/sysarmor-next-project/internal/response"
	"github.com/sysarmor/sysarmor-next-project/internal/sensor/contract"
	sensorruntime "github.com/sysarmor/sysarmor-next-project/internal/sensor/runtime"
	"github.com/sysarmor/sysarmor-next-project/internal/sensor/tetragon"
	"github.com/sysarmor/sysarmor-next-project/internal/store"
	"github.com/sysarmor/sysarmor-next-project/internal/tlsconfig"
	"google.golang.org/grpc"
)

const testCollectionPolicyJSON = `{"behaviors":["process.exec","process.exit","process.fork","file.read","file.write","network.connect"],"observe_only":true}
`

func TestRunnerOnceWithFakeSensor(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(policyPath, []byte(testCollectionPolicyJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
		Manager: config.ManagerConfig{Address: "local", Transport: "local"},
		Sensor:  config.SensorConfig{Backend: "fake", Mode: "managed", PolicyPath: policyPath, ObserveOnly: true},
		Spool:   config.SpoolConfig{Path: filepath.Join(dir, "spool"), MaxBytes: 4096, BatchSize: 10, FlushInterval: time.Second},
		Upload:  config.UploadConfig{RetryInitial: time.Second, RetryMax: time.Second, RequestTimeout: time.Second},
		Health:  config.HealthConfig{Interval: time.Hour},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	runner, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	var out bytes.Buffer
	if err := runner.Run(context.Background(), Options{Once: true, Out: &out}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"agent daemon started", "sensor=fake", "agent daemon event"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output %q does not contain %q", got, want)
		}
	}
	assertSpoolBatch(t, cfg.Spool.Path)
}

func TestRunnerSpoolsConfiguredScenario(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(policyPath, []byte(testCollectionPolicyJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token", Scenario: "daemon-scenario"},
		Manager: config.ManagerConfig{Address: "local", Transport: "local"},
		Sensor:  config.SensorConfig{Backend: "fake", Mode: "managed", PolicyPath: policyPath, Scope: config.RuntimeScope{Type: "container", Selector: "abc123"}, ObserveOnly: true},
		Spool:   config.SpoolConfig{Path: filepath.Join(dir, "spool"), MaxBytes: 4096, BatchSize: 10, FlushInterval: time.Second},
		Upload:  config.UploadConfig{RetryInitial: time.Second, RetryMax: time.Second, RequestTimeout: time.Second},
		Health:  config.HealthConfig{Interval: time.Hour},
	}
	runner, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := runner.Run(context.Background(), Options{Once: true}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	batch := loadOnlySpoolBatch(t, cfg.Spool.Path)
	if got := batch.GetEvents()[0].GetEvent().GetScenario(); got != "daemon-scenario" {
		t.Fatalf("event scenario = %q", got)
	}
	for _, sig := range batch.GetSignals() {
		signal := sig.GetSignal()
		if got := signal.GetScenario(); got != "daemon-scenario" {
			t.Fatalf("signal %s scenario = %q", signal.GetName(), got)
		}
	}
}

func TestRunnerRefreshesEndpointPolicy(t *testing.T) {
	dir := t.TempDir()
	queue, err := spool.OpenWithLimit(filepath.Join(dir, "spool"), 4096)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		Agent: config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token", Scenario: "refresh-scenario"},
	}
	runner := &Runner{Config: cfg}
	runner.applyRuntimePolicy(policymodel.DefaultPolicy("default"))
	norm := normalize.New(cfg.Agent.ID, cfg.Agent.HostID, nil)
	if _, err := runner.spoolEvent(queue, norm, runner.currentDetection(), sensorEventEnvelope("file.write", 100, "/usr/bin/curl", "/dev/shm/x.sh", "")); err != nil {
		t.Fatal(err)
	}
	updated := policymodel.DefaultPolicy("default")
	updated.PolicyID = "no-payload-after-refresh"
	updated.Version = 2
	disabled := false
	updated.Detection.RuleOverrides = append(updated.Detection.RuleOverrides, policymodel.RuleOverride{RuleID: "payload_dropped", Enabled: &disabled})
	runner.applyRuntimePolicy(updated)
	if _, err := runner.spoolEvent(queue, norm, runner.currentDetection(), sensorEventEnvelope("file.write", 101, "/usr/bin/curl", "/dev/shm/x.sh", "")); err != nil {
		t.Fatal(err)
	}
	batches := loadSpoolBatches(t, filepath.Join(dir, "spool"))
	if len(batches) != 2 {
		t.Fatalf("batch count = %d", len(batches))
	}
	signalCounts := []int{len(batches[0].GetSignals()), len(batches[1].GetSignals())}
	if !containsInt(signalCounts, 1) || !containsInt(signalCounts, 0) {
		t.Fatalf("signal counts = %v, want one pre-refresh signal and one post-refresh suppressed signal", signalCounts)
	}
	if runner.activePolicy().PolicyID != "no-payload-after-refresh" || runner.activePolicy().Version != 2 {
		t.Fatalf("active policy = %+v", runner.activePolicy())
	}
}

func TestControlPolicyClientFetchesPolicy(t *testing.T) {
	st := &store.Store{}
	policy := policymodel.DefaultPolicy("default")
	policy.PolicyID = "stream-policy"
	policy.Version = 3
	disabled := false
	policy.Detection.RuleOverrides = append(policy.Detection.RuleOverrides, policymodel.RuleOverride{RuleID: "payload_dropped", Enabled: &disabled})
	st.UpsertPolicy(policy)
	st.AssignPolicy(policymodel.Assignment{
		TenantID:      "default",
		AgentID:       "agent-stream",
		PolicyID:      "stream-policy",
		PolicyVersion: 3,
	})
	linkSrv := managerapi.NewServer(st)
	grpcServer := grpc.NewServer()
	controlv1.RegisterAgentControlServiceServer(grpcServer, agentplane.NewControlServer(linkSrv))
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = grpcServer.Serve(lis)
	}()
	defer grpcServer.Stop()

	client := NewControlPolicyClient(lis.Addr().String(), "", time.Second)
	got, err := client.EffectivePolicy(context.Background(), EffectivePolicyRequest{
		TenantID: "default",
		AgentID:  "agent-stream",
	})
	if err != nil {
		t.Fatalf("EffectivePolicy() error = %v", err)
	}
	if got.PolicyID != "stream-policy" || got.Version != 3 || got.Mode != "observe" {
		t.Fatalf("policy = %+v", got)
	}
	if got.Detection == nil || len(got.Detection.RuleOverrides) == 0 {
		t.Fatalf("detection policy = %+v", got.Detection)
	}
}

func TestControlResponseClientFetchesCommandAndAcks(t *testing.T) {
	st := &store.Store{}
	st.CreateResponse(responsemodel.Command{
		ResponseID: "resp-stream-agent",
		TenantID:   "default",
		AgentID:    "agent-stream",
		Action:     "collect",
		Target:     "process:p1",
	})
	linkSrv := managerapi.NewServer(st)
	grpcServer := grpc.NewServer()
	analyticsv1.RegisterAgentDataServiceServer(grpcServer, agentplane.NewDataServer(linkSrv))
	controlv1.RegisterAgentControlServiceServer(grpcServer, agentplane.NewControlServer(linkSrv))
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = grpcServer.Serve(lis)
	}()
	defer grpcServer.Stop()

	runner := &Runner{
		Config: config.Config{
			Agent:   config.AgentConfig{ID: "agent-stream", HostID: "host-stream", TenantID: "default"},
			Manager: config.ManagerConfig{Address: lis.Addr().String(), Transport: "grpc"},
		},
		Sensor: &healthOnlySensor{health: contract.Health{Running: true, PolicyLoaded: true}},
	}
	client := NewControlResponseClient(lis.Addr().String(), "", time.Second)
	if err := runner.pollControlResponses(context.Background(), client); err != nil {
		t.Fatalf("pollControlResponses() error = %v", err)
	}
	audits := st.ListResponses("default", "agent-stream")
	if len(audits) != 1 || audits[0].Ack == nil {
		t.Fatalf("audits = %+v", audits)
	}
	ack := audits[0].Ack
	if ack.ResponseID != "resp-stream-agent" || !ack.ObserveOnly || ack.Executed || ack.Unsupported || !ack.Accepted {
		t.Fatalf("ack = %+v", ack)
	}
}

func TestControlEvidenceClientHandlesPullbackResult(t *testing.T) {
	st := &store.Store{}
	st.AddIncident(&incidentv1.Incident{Id: "inc-stream", Scenario: "pullback-stream", Summary: "stream incident"})
	st.CreateEvidencePullback(gatewaymodel.EvidencePullbackRequest{
		RequestID:  "evpb-stream-agent",
		TenantID:   "default",
		AgentID:    "agent-stream",
		IncidentID: "inc-stream",
		Scenario:   "pullback-stream",
		Target:     "process:p1",
		Reason:     "collect graph evidence",
	})
	linkSrv := managerapi.NewServer(st)
	grpcServer := grpc.NewServer()
	analyticsv1.RegisterAgentDataServiceServer(grpcServer, agentplane.NewDataServer(linkSrv))
	controlv1.RegisterAgentControlServiceServer(grpcServer, agentplane.NewControlServer(linkSrv))
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = grpcServer.Serve(lis)
	}()
	defer grpcServer.Stop()

	runner := &Runner{
		Config: config.Config{
			Agent:   config.AgentConfig{ID: "agent-stream", HostID: "host-stream", TenantID: "default"},
			Manager: config.ManagerConfig{Address: lis.Addr().String(), Transport: "grpc"},
		},
		Sensor: &healthOnlySensor{health: contract.Health{Running: true, PolicyLoaded: true}},
	}
	client := NewControlEvidenceClient(lis.Addr().String(), "", time.Second)
	if err := runner.pollControlEvidencePullbacks(context.Background(), client); err != nil {
		t.Fatalf("pollControlEvidencePullbacks() error = %v", err)
	}
	pullbacks := st.ListEvidencePullbacks("default", "agent-stream")
	if len(pullbacks) != 1 || pullbacks[0].Status != gatewaymodel.EvidencePullbackStatusCompleted || !pullbacks[0].ResultOK {
		t.Fatalf("pullbacks = %+v", pullbacks)
	}
	inc, ok := st.GetIncident("inc-stream", "pullback-stream")
	if !ok {
		t.Fatal("incident not found")
	}
	nodes := inc.GetEvidence().GetNodes()
	if len(nodes) != 1 || nodes[0].GetId() != "process:p1" || nodes[0].GetKind() != "process" {
		t.Fatalf("evidence nodes = %+v", nodes)
	}
}

func TestControlStreamHealthReporterSendsHeartbeatFrame(t *testing.T) {
	st := &store.Store{}
	linkSrv := managerapi.NewServer(st)
	grpcServer := grpc.NewServer()
	controlv1.RegisterAgentControlServiceServer(grpcServer, agentplane.NewControlServer(linkSrv))
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = grpcServer.Serve(lis)
	}()
	defer grpcServer.Stop()

	reporter := NewControlStreamHealthReporter(lis.Addr().String(), "", time.Second)
	err = reporter.Report(context.Background(), agenthealth.AgentHealth{
		AgentID:    "agent-control-health",
		HostID:     "host-control-health",
		TenantID:   "default",
		Status:     "ok",
		ObservedAt: time.Now().UTC(),
		Scope:      agenthealth.RuntimeScope{Type: "container", Selector: "api"},
		Capability: agenthealth.SensorCapability{Backend: "fake", Version: "dev", SupportsHealth: true},
		Sensor:     agenthealth.SensorHealth{Backend: "fake", Running: true, EventsSeen: 11},
	})
	if err != nil {
		t.Fatalf("Report() error = %v", err)
	}
	got, ok := st.GetAgentHealth("default", "agent-control-health")
	if !ok {
		t.Fatal("agent health not stored")
	}
	if got.Status != "ok" || got.Sensor.EventsSeen != 11 || got.Capability.Version != "dev" || got.Scope.Selector != "api" {
		t.Fatalf("stored health = %+v", got)
	}
}

func TestControlStreamSessionKeepsLongLivedContract(t *testing.T) {
	st := &store.Store{}
	linkSrv := managerapi.NewServer(st)
	grpcServer := grpc.NewServer()
	controlv1.RegisterAgentControlServiceServer(grpcServer, agentplane.NewControlServer(linkSrv))
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = grpcServer.Serve(lis)
	}()
	defer grpcServer.Stop()

	session := NewControlStreamSession(lis.Addr().String(), "", tlsconfig.ClientConfig{})
	defer session.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	frames, err := session.Hello(ctx, "default", "agent-long-control", "container", "api")
	if err != nil {
		t.Fatalf("Hello() error = %v", err)
	}
	if len(frames) != 2 || frames[0].GetType() != "policy_update" || frames[1].GetType() != "resume" {
		t.Fatalf("hello frames = %+v", frames)
	}
	if frames[0].GetContractVersion() != 1 || frames[0].GetSequence() != 1 || frames[1].GetSequence() != 2 {
		t.Fatalf("hello frame sequence = %d/%d contract=%d", frames[0].GetSequence(), frames[1].GetSequence(), frames[0].GetContractVersion())
	}
	if err := session.Send(ctx, &controlv1.ControlStreamFrame{
		Type:      "health_report",
		RequestId: "long-health",
		Context: &controlv1.RequestContext{
			TenantId: "default",
			AgentId:  "agent-long-control",
			Scope:    &controlv1.Scope{Type: "container", Selector: "api"},
		},
		Health: &controlv1.HealthResponse{
			AgentId:    "agent-long-control",
			HostId:     "host-long-control",
			TenantId:   "default",
			Status:     "ok",
			Scope:      &controlv1.Scope{Type: "container", Selector: "api"},
			ObservedAt: time.Now().UTC().Format(time.RFC3339Nano),
			Capability: &controlv1.SensorCapability{Backend: "fake", Version: "long", SupportsHealth: true},
			Sensor:     &controlv1.SensorHealth{Backend: "fake", Running: true, EventsSeen: 7},
		},
	}); err != nil {
		t.Fatalf("Send(health) error = %v", err)
	}
	ack, err := session.Recv()
	if err != nil {
		t.Fatalf("Recv(health ack) error = %v", err)
	}
	if ack.GetType() != "ack" || ack.GetSequence() != 3 || ack.GetAck().GetStatus() != "accepted" {
		t.Fatalf("health ack = %+v", ack)
	}
	got, ok := st.GetAgentHealth("default", "agent-long-control")
	if !ok || got.Capability.Version != "long" || got.Sensor.EventsSeen != 7 {
		t.Fatalf("stored long stream health = %+v ok=%t", got, ok)
	}
}

func TestRunnerControlStreamSessionProcessesPendingResponse(t *testing.T) {
	dir := t.TempDir()
	st := &store.Store{}
	st.CreateResponse(responsemodel.Command{
		ResponseID: "resp-runner-long",
		TenantID:   "default",
		AgentID:    "agent-runner-long",
		Action:     "collect",
		Target:     "process:p1",
	})
	linkSrv := managerapi.NewServer(st)
	grpcServer := grpc.NewServer()
	controlv1.RegisterAgentControlServiceServer(grpcServer, agentplane.NewControlServer(linkSrv))
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = grpcServer.Serve(lis)
	}()
	defer grpcServer.Stop()

	queue, err := spool.OpenWithLimit(filepath.Join(dir, "spool"), 4096)
	if err != nil {
		t.Fatal(err)
	}
	runner := &Runner{
		Config: config.Config{
			Agent:   config.AgentConfig{ID: "agent-runner-long", HostID: "host-runner-long", TenantID: "default"},
			Manager: config.ManagerConfig{Address: lis.Addr().String(), Transport: "grpc"},
			Upload:  config.UploadConfig{RetryInitial: 10 * time.Millisecond, RetryMax: 20 * time.Millisecond, RequestTimeout: time.Second},
			Health:  config.HealthConfig{Interval: 10 * time.Millisecond},
		},
		Sensor: &healthOnlySensor{health: contract.Health{Backend: "fake", Running: true, PolicyLoaded: true, EventsSeen: 3}},
		capability: contract.Capability{
			Backend:        "fake",
			Version:        "long",
			SupportsHealth: true,
		},
	}
	worker := &uploadworker.Worker{Queue: queue, Uploader: noopUploader{}}
	rt := sensorruntime.New(runner.Sensor)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runner.runControlStreamSession(ctx, rt, queue, worker, time.Now().UTC(), "host", "")
	}()
	deadline := time.After(time.Second)
	for {
		audits := st.ListResponses("default", "agent-runner-long")
		if len(audits) == 1 && audits[0].Ack != nil {
			cancel()
			err := <-done
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Fatalf("runControlStreamSession() error = %v", err)
			}
			ack := audits[0].Ack
			if ack.ResponseID != "resp-runner-long" || !ack.Accepted || !ack.ObserveOnly || ack.Executed {
				t.Fatalf("ack = %+v", ack)
			}
			if health, ok := st.GetAgentHealth("default", "agent-runner-long"); !ok || health.Capability.Version != "long" {
				t.Fatalf("health = %+v ok=%t", health, ok)
			}
			return
		}
		select {
		case <-deadline:
			cancel()
			t.Fatalf("response ack not observed; audits=%+v", audits)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestControlResumeClientAcksLocalSpoolThroughCursor(t *testing.T) {
	dir := t.TempDir()
	queue, err := spool.OpenWithLimit(filepath.Join(dir, "spool"), 4096)
	if err != nil {
		t.Fatal(err)
	}
	id1, err := queue.AppendDataBatch(testDataBatch("local-1", "agent-stream", "host-stream"))
	if err != nil {
		t.Fatal(err)
	}
	id2, err := queue.AppendDataBatch(testDataBatch("local-2", "agent-stream", "host-stream"))
	if err != nil {
		t.Fatal(err)
	}
	id3, err := queue.AppendDataBatch(testDataBatch("local-3", "agent-stream", "host-stream"))
	if err != nil {
		t.Fatal(err)
	}
	st := &store.Store{}
	st.RecordDataUpload(store.AgentIdentity{AgentID: "agent-stream", HostID: "host-stream", TenantID: "default"}, id2, "control", time.Now().UTC())
	linkSrv := managerapi.NewServer(st)
	grpcServer := grpc.NewServer()
	analyticsv1.RegisterAgentDataServiceServer(grpcServer, agentplane.NewDataServer(linkSrv))
	controlv1.RegisterAgentControlServiceServer(grpcServer, agentplane.NewControlServer(linkSrv))
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = grpcServer.Serve(lis)
	}()
	defer grpcServer.Stop()

	runner := &Runner{
		Config: config.Config{
			Agent:   config.AgentConfig{ID: "agent-stream", HostID: "host-stream", TenantID: "default"},
			Manager: config.ManagerConfig{Address: lis.Addr().String(), Transport: "grpc"},
			Upload:  config.UploadConfig{RequestTimeout: time.Second},
		},
	}
	worker, err := runner.uploadWorker(queue)
	if err != nil {
		t.Fatal(err)
	}
	stats, err := worker.ResumeOnce(context.Background())
	if err != nil {
		t.Fatalf("ResumeOnce() error = %v", err)
	}
	if stats.RemainingBatches != 1 || stats.LastError != "" {
		t.Fatalf("stats = %+v", stats)
	}
	entries, err := queue.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != id3 || entries[0].ID == id1 {
		t.Fatalf("remaining entries = %+v, cursor = %s", entries, id2)
	}
}

func TestRunnerOnceWithTetragonJSONLSource(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	eventPath := filepath.Join(dir, "events.jsonl")
	if err := os.WriteFile(policyPath, []byte(testCollectionPolicyJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	raw := `{"process_exec":{"process":{"pid":100,"uid":0,"binary":"/bin/bash","arguments":"-c id","start_time":"2026-06-14T10:00:00Z"},"parent":{"pid":99,"binary":"/sbin/init","start_time":"2026-06-14T09:59:59Z"}},"node_name":"node-a","time":"2026-06-14T10:00:00Z"}`
	if err := os.WriteFile(eventPath, []byte(raw+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
		Manager: config.ManagerConfig{Address: "local", Transport: "local"},
		Sensor:  config.SensorConfig{Backend: "tetragon", Mode: "managed", Version: "test", PolicyPath: policyPath, EventSource: eventPath, ObserveOnly: true},
		Spool:   config.SpoolConfig{Path: filepath.Join(dir, "spool"), MaxBytes: 4096, BatchSize: 10, FlushInterval: time.Second},
		Upload:  config.UploadConfig{RetryInitial: time.Second, RetryMax: time.Second, RequestTimeout: time.Second},
		Health:  config.HealthConfig{Interval: time.Hour},
	}
	runner, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	var out bytes.Buffer
	if err := runner.Run(context.Background(), Options{Once: true, Out: &out}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"sensor=tetragon", "agent daemon event"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output %q does not contain %q", got, want)
		}
	}
	assertSpoolBatch(t, cfg.Spool.Path)
}

func TestRunnerTetragonRequiresEventSource(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	if err := os.WriteFile(policyPath, []byte(testCollectionPolicyJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
		Manager: config.ManagerConfig{Address: "local", Transport: "local"},
		Sensor:  config.SensorConfig{Backend: "tetragon", Mode: "managed", PolicyPath: policyPath, EventTransport: "tetra", ObserveOnly: true},
		Spool:   config.SpoolConfig{Path: filepath.Join(dir, "spool"), MaxBytes: 1024, BatchSize: 10, FlushInterval: time.Second},
		Upload:  config.UploadConfig{RetryInitial: time.Second, RetryMax: time.Second, RequestTimeout: time.Second},
		Health:  config.HealthConfig{Interval: time.Hour},
	}
	runner, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	err = runner.Run(context.Background(), Options{Once: true})
	if err == nil {
		t.Fatal("Run() error = nil")
	}
}

func TestRunnerDrainOnceUploadsAndAcksSpool(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(policyPath, []byte(testCollectionPolicyJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
		Manager: config.ManagerConfig{Address: "local", Transport: "local"},
		Sensor:  config.SensorConfig{Backend: "fake", Mode: "managed", PolicyPath: policyPath, ObserveOnly: true},
		Spool:   config.SpoolConfig{Path: filepath.Join(dir, "spool"), MaxBytes: 1024, BatchSize: 10, FlushInterval: time.Second},
		Upload:  config.UploadConfig{RetryInitial: time.Second, RetryMax: time.Second, RequestTimeout: time.Second},
		Health:  config.HealthConfig{Interval: time.Hour},
	}
	runner, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runner.Run(context.Background(), Options{Once: true, DrainOnce: true, Out: &out}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(cfg.Spool.Path, "*.batch.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("spool batches after drain = %v", matches)
	}
	if !strings.Contains(out.String(), "agent upload drain: uploaded=1 remaining=0") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestRunnerBackgroundUploadLoopDrainsSpool(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(policyPath, []byte(testCollectionPolicyJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := config.Config{
		Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
		Manager: config.ManagerConfig{Address: "local", Transport: "local"},
		Sensor:  config.SensorConfig{Backend: "fake", Mode: "managed", PolicyPath: policyPath, ObserveOnly: true},
		Spool:   config.SpoolConfig{Path: filepath.Join(dir, "spool"), MaxBytes: 4096, BatchSize: 10, FlushInterval: 5 * time.Millisecond},
		Upload:  config.UploadConfig{RetryInitial: 5 * time.Millisecond, RetryMax: 10 * time.Millisecond, RequestTimeout: time.Second},
		Health:  config.HealthConfig{Interval: time.Hour},
	}
	runner, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- runner.Run(ctx, Options{Out: &bytes.Buffer{}})
	}()
	deadline := time.After(2 * time.Second)
	for {
		matches, err := filepath.Glob(filepath.Join(cfg.Spool.Path, "*.batch.json"))
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) == 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("spool batches after background drain = %v", matches)
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestRunnerGracefulShutdownDrainsSpoolWhenManagerAvailable(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(policyPath, []byte(testCollectionPolicyJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
		Manager: config.ManagerConfig{Address: "local", Transport: "local"},
		Sensor:  config.SensorConfig{Backend: "fake", Mode: "managed", PolicyPath: policyPath, Scope: config.RuntimeScope{Type: "container", Selector: "abc123"}, ObserveOnly: true},
		Spool:   config.SpoolConfig{Path: filepath.Join(dir, "spool"), MaxBytes: 4096, BatchSize: 10, FlushInterval: time.Hour},
		Upload:  config.UploadConfig{RetryInitial: 50 * time.Millisecond, RetryMax: 50 * time.Millisecond, RequestTimeout: time.Second},
		Health:  config.HealthConfig{Interval: time.Hour},
	}
	runner, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	var out bytes.Buffer
	go func() {
		errCh <- runner.Run(ctx, Options{Out: &out})
	}()

	deadline := time.After(2 * time.Second)
	for {
		matches, err := filepath.Glob(filepath.Join(cfg.Spool.Path, "*.batch.json"))
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) == 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for spool batch before shutdown")
		case <-time.After(10 * time.Millisecond):
		}
	}

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(cfg.Spool.Path, "*.batch.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("spool batches after shutdown drain = %v", matches)
	}
	if !strings.Contains(out.String(), "agent shutdown drain: uploaded=1 remaining=0") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestUploadWorkerRecoversUnackedBatchesAfterRestart(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(policyPath, []byte(testCollectionPolicyJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	spoolPath := filepath.Join(dir, "spool")
	cfg := config.Config{
		Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
		Manager: config.ManagerConfig{Address: "local", Transport: "local"},
		Sensor:  config.SensorConfig{Backend: "fake", Mode: "managed", PolicyPath: policyPath, ObserveOnly: true},
		Spool:   config.SpoolConfig{Path: spoolPath, MaxBytes: 4096, BatchSize: 10, FlushInterval: time.Second},
		Upload:  config.UploadConfig{RetryInitial: time.Second, RetryMax: time.Second, RequestTimeout: 5 * time.Millisecond},
		Health:  config.HealthConfig{Interval: time.Hour},
	}
	first, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Run(context.Background(), Options{Once: true}); err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(spoolPath, "*.batch.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("spool batches after first run = %v", matches)
	}

	oldBatch := loadOnlySpoolBatch(t, spoolPath)
	oldID := oldBatch.GetEvents()[0].GetEvent().GetId()

	worker, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	queue, err := spool.OpenWithLimit(spoolPath, cfg.Spool.MaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	up, err := worker.uploadWorker(queue)
	if err != nil {
		t.Fatal(err)
	}
	stats, err := up.DrainOnce(context.Background())
	if err != nil {
		t.Fatalf("DrainOnce() error = %v", err)
	}
	if stats.UploadedBatches != 1 || stats.RemainingBatches != 0 {
		t.Fatalf("stats = %+v", stats)
	}
	matches, err = filepath.Glob(filepath.Join(spoolPath, "*.batch.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("spool batches after recovery drain = %v", matches)
	}
	if oldID == "" {
		t.Fatal("old event id = empty")
	}
}

func TestRunnerReportsSpoolBackpressure(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(policyPath, []byte(testCollectionPolicyJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
		Manager: config.ManagerConfig{Address: "local", Transport: "local"},
		Sensor:  config.SensorConfig{Backend: "fake", Mode: "managed", PolicyPath: policyPath, ObserveOnly: true},
		Spool:   config.SpoolConfig{Path: filepath.Join(dir, "spool"), MaxBytes: 1, BatchSize: 10, FlushInterval: time.Second},
		Upload:  config.UploadConfig{RetryInitial: time.Second, RetryMax: time.Second, RequestTimeout: time.Second},
		Health:  config.HealthConfig{Interval: time.Hour},
	}
	runner, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runner.Run(context.Background(), Options{Once: true, Out: &out}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(out.String(), "agent spool backpressure") {
		t.Fatalf("output = %q", out.String())
	}
	matches, err := filepath.Glob(filepath.Join(cfg.Spool.Path, "*.batch.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("spool batches = %v", matches)
	}
}

func TestRunnerMarksHealthDegradedOnSpoolBackpressure(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(policyPath, []byte(testCollectionPolicyJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
		Manager: config.ManagerConfig{Address: "local", Transport: "local"},
		Sensor:  config.SensorConfig{Backend: "fake", Mode: "managed", PolicyPath: policyPath, ObserveOnly: true},
		Spool:   config.SpoolConfig{Path: filepath.Join(dir, "spool"), MaxBytes: 1, BatchSize: 10, FlushInterval: time.Hour},
		Upload:  config.UploadConfig{RetryInitial: time.Hour, RetryMax: time.Hour, RequestTimeout: 5 * time.Millisecond},
		Health:  config.HealthConfig{Interval: time.Hour},
	}
	runner, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	rt := sensorruntime.New(runner.Sensor)
	if _, err := rt.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := rt.Apply(context.Background(), contract.CollectionIntent{ObserveOnly: true}); err != nil {
		t.Fatal(err)
	}
	queue, err := spool.OpenWithLimit(cfg.Spool.Path, cfg.Spool.MaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queue.AppendDataBatch(&dataplanev1.DataBatch{
		Header: &dataplanev1.BatchHeader{AgentId: "agent-a", HostId: "host-a", TenantId: "default"},
		Events: []*dataplanev1.EventFrame{{
			Event: &eventv1.CanonicalEvent{
				Id:       "event-a",
				AgentId:  "agent-a",
				HostId:   "host-a",
				Behavior: "process.exec",
			},
		}},
	}); !spool.IsBackpressure(err) {
		t.Fatalf("Append() error = %v, want backpressure", err)
	}
	worker, err := runner.uploadWorker(queue)
	if err != nil {
		t.Fatal(err)
	}
	health, err := runner.collectHealth(context.Background(), rt, queue, worker, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if health.Status != "degraded" || health.Queue.BackpressureCount != 1 || health.Queue.DroppedBatches != 1 || health.Queue.LastError == "" {
		t.Fatalf("health = %+v", health)
	}
}

func TestRunnerSpoolsTamperSignalFromHealth(t *testing.T) {
	dir := t.TempDir()
	queue, err := spool.OpenWithLimit(filepath.Join(dir, "spool"), 4096)
	if err != nil {
		t.Fatal(err)
	}
	runner := &Runner{
		Config: config.Config{
			Agent:  config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
			Sensor: config.SensorConfig{Backend: "fake", Mode: "managed", ObserveOnly: true, RestartWindow: time.Hour},
			Spool:  config.SpoolConfig{Path: filepath.Join(dir, "spool"), MaxBytes: 4096, BatchSize: 10, FlushInterval: time.Hour},
			Upload: config.UploadConfig{RetryInitial: time.Hour, RetryMax: time.Hour, RequestTimeout: 5 * time.Millisecond},
			Health: config.HealthConfig{Interval: 5 * time.Millisecond},
		},
		Sensor: &healthOnlySensor{health: contract.Health{
			Backend:        "tetragon",
			Installed:      true,
			PolicyLoaded:   true,
			Running:        false,
			RestartCount:   3,
			LastExitReason: "exit status 7",
			LastError:      "exit status 7",
		}},
	}
	health := agenthealth.AgentHealth{
		AgentID:       runner.Config.Agent.ID,
		HostID:        runner.Config.Agent.HostID,
		TenantID:      runner.Config.Agent.TenantID,
		Status:        "degraded",
		PolicyID:      runner.activePolicy().PolicyID,
		PolicyVersion: runner.activePolicy().Version,
		PolicyMode:    runner.policyMode(),
		UptimeSeconds: 1,
		ObservedAt:    time.Now().UTC(),
		Sensor: agenthealth.SensorHealth{
			Backend:        "tetragon",
			Installed:      true,
			PolicyLoaded:   true,
			Running:        false,
			RestartCount:   3,
			LastExitReason: "exit status 7",
			LastError:      "exit status 7",
		},
	}
	sig := (&tamper.Detector{}).Evaluate(health, time.Now().UTC(), tamper.Options{
		MaxRestarts:        uint64(runner.Config.Sensor.MaxRestarts),
		MaxParseErrors:     runner.Config.Sensor.MaxParseErrors,
		MaxDroppedEvents:   runner.Config.Sensor.MaxDroppedEvents,
		NoEventGracePeriod: tamperNoEventGracePeriod(runner.Config.Sensor.RestartWindow, runner.Config.Health.Interval),
	})
	if sig == nil {
		t.Fatal("tamper Evaluate() = nil")
	}
	if _, err := runner.spoolSignals(queue, []*signalv1.Signal{sig}); err != nil {
		t.Fatal(err)
	}
	batch := loadOnlySpoolBatch(t, runner.Config.Spool.Path)
	sig = batch.GetSignals()[0].GetSignal()
	if sig.GetName() != tamper.SignalName || !sig.GetTerminal() || sig.GetWhere() != signalv1.SignalWhere_SIGNAL_WHERE_ENDPOINT {
		t.Fatalf("tamper signal = %+v", sig)
	}
}

func TestRunnerMarksHealthDegradedWhenParseThresholdExceeded(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(policyPath, []byte(testCollectionPolicyJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
		Manager: config.ManagerConfig{Address: "local", Transport: "local"},
		Sensor:  config.SensorConfig{Backend: "fake", Mode: "managed", PolicyPath: policyPath, ObserveOnly: true, MaxParseErrors: 1, RestartWindow: time.Hour},
		Spool:   config.SpoolConfig{Path: filepath.Join(dir, "spool"), MaxBytes: 4096, BatchSize: 10, FlushInterval: time.Hour},
		Upload:  config.UploadConfig{RetryInitial: time.Hour, RetryMax: time.Hour, RequestTimeout: 5 * time.Millisecond},
		Health:  config.HealthConfig{Interval: time.Hour},
	}
	runner := &Runner{
		Config: cfg,
		Sensor: &healthOnlySensor{health: contract.Health{
			Backend:      "tetragon",
			Installed:    true,
			PolicyLoaded: true,
			Running:      true,
			ParseErrors:  2,
		}},
	}
	rt := sensorruntime.New(runner.Sensor)
	if _, err := rt.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := rt.Apply(context.Background(), contract.CollectionIntent{ObserveOnly: true}); err != nil {
		t.Fatal(err)
	}
	queue, err := spool.OpenWithLimit(cfg.Spool.Path, cfg.Spool.MaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := runner.uploadWorker(queue)
	if err != nil {
		t.Fatal(err)
	}
	health, err := runner.collectHealth(context.Background(), rt, queue, worker, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if health.Status != "degraded" || health.Sensor.ParseErrors != 2 {
		t.Fatalf("health = %+v", health)
	}
}

func TestNewBatchUploaderAcceptsConfiguredTimeout(t *testing.T) {
	for _, tc := range []struct {
		name      string
		transport string
	}{
		{name: "grpc", transport: "grpc"},
		{name: "local", transport: "local"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			up, err := newBatchUploader("127.0.0.1:9443", tc.transport, 250*time.Millisecond, "dev-token", tlsconfig.ClientConfig{})
			if err != nil {
				t.Fatalf("newBatchUploader() error = %v", err)
			}
			if up == nil {
				t.Fatal("newBatchUploader() = nil")
			}
		})
	}
}

func TestTetragonRestartPolicyFromConfig(t *testing.T) {
	policy, err := tetragonRestartPolicy(config.SensorConfig{
		Restart:       "always",
		MaxRestarts:   7,
		RestartWindow: 25 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("tetragonRestartPolicy() error = %v", err)
	}
	if policy != (tetragon.ProcessRestartPolicy{Enabled: true, MaxRestarts: 7, Delay: 25 * time.Millisecond}) {
		t.Fatalf("policy = %+v", policy)
	}
	disabled, err := tetragonRestartPolicy(config.SensorConfig{Restart: "never"})
	if err != nil {
		t.Fatalf("tetragonRestartPolicy(never) error = %v", err)
	}
	if disabled.Enabled {
		t.Fatalf("disabled policy = %+v", disabled)
	}
	if _, err := tetragonRestartPolicy(config.SensorConfig{Restart: "sometimes"}); err == nil {
		t.Fatal("tetragonRestartPolicy(unknown) error = nil")
	}
}

func TestTamperNoEventGracePeriodHasFloor(t *testing.T) {
	if got := tamperNoEventGracePeriod(500*time.Millisecond, 500*time.Millisecond); got != 30*time.Second {
		t.Fatalf("tamper grace = %s, want 30s floor", got)
	}
	if got := tamperNoEventGracePeriod(time.Minute, 10*time.Second); got != 100*time.Second {
		t.Fatalf("tamper grace = %s, want 10 health intervals", got)
	}
}

type healthOnlySensor struct {
	health contract.Health
}

type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

type eventSensor struct {
	events []contract.EventEnvelope
	ch     chan contract.EventEnvelope
	health contract.Health
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type capabilityErrorSensor struct {
	err error
}

func newStreamingEventSensor() *eventSensor {
	s := newEventSensor(nil)
	s.ch = make(chan contract.EventEnvelope, 16)
	return s
}

func (s *eventSensor) emit(ev contract.EventEnvelope) {
	s.ch <- ev
}

func newEventSensor(events []contract.EventEnvelope) *eventSensor {
	return &eventSensor{
		events: events,
		health: contract.Health{
			Backend:      "test",
			Installed:    true,
			Running:      true,
			Version:      "test",
			PolicyLoaded: true,
		},
	}
}

func (s *capabilityErrorSensor) Capability(context.Context) (contract.Capability, error) {
	return contract.Capability{}, s.err
}

func (s *capabilityErrorSensor) Apply(context.Context, contract.CollectionIntent) error {
	return nil
}

func (s *capabilityErrorSensor) Subscribe(context.Context, contract.CollectionIntent) (<-chan contract.EventEnvelope, error) {
	return nil, nil
}

func (s *capabilityErrorSensor) Enforce(_ context.Context, cmd contract.EnforcementCmd) (contract.EnforcementAck, error) {
	return contract.UnsupportedAck(cmd, "test sensor"), nil
}

func (s *capabilityErrorSensor) Health(context.Context) (contract.Health, error) {
	return contract.Health{}, nil
}

func (s *healthOnlySensor) Capability(context.Context) (contract.Capability, error) {
	return contract.Capability{Backend: s.health.Backend, Version: "test", SupportsHealth: true}, nil
}

func (s *healthOnlySensor) Apply(context.Context, contract.CollectionIntent) error {
	return nil
}

func (s *healthOnlySensor) Subscribe(ctx context.Context, _ contract.CollectionIntent) (<-chan contract.EventEnvelope, error) {
	out := make(chan contract.EventEnvelope)
	go func() {
		defer close(out)
		<-ctx.Done()
	}()
	return out, nil
}

func (s *healthOnlySensor) Enforce(_ context.Context, cmd contract.EnforcementCmd) (contract.EnforcementAck, error) {
	return contract.UnsupportedAck(cmd, "test sensor"), nil
}

func (s *healthOnlySensor) Health(context.Context) (contract.Health, error) {
	return s.health, nil
}

func (s *eventSensor) Capability(context.Context) (contract.Capability, error) {
	return contract.Capability{Backend: "test", Version: "test", SupportsExec: true, SupportsFile: true, SupportsHealth: true}, nil
}

func (s *eventSensor) Apply(context.Context, contract.CollectionIntent) error {
	s.health.PolicyLoaded = true
	return nil
}

func (s *eventSensor) Subscribe(ctx context.Context, _ contract.CollectionIntent) (<-chan contract.EventEnvelope, error) {
	out := make(chan contract.EventEnvelope)
	events := append([]contract.EventEnvelope(nil), s.events...)
	go func() {
		defer close(out)
		for _, ev := range events {
			select {
			case <-ctx.Done():
				return
			case out <- ev:
				s.health.EventsSeen++
				s.health.LastEventAt = time.Now().UTC()
			}
		}
		if s.ch == nil {
			<-ctx.Done()
			return
		}
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-s.ch:
				select {
				case <-ctx.Done():
					return
				case out <- ev:
					s.health.EventsSeen++
					s.health.LastEventAt = time.Now().UTC()
				}
			}
		}
	}()
	return out, nil
}

func (s *eventSensor) Enforce(_ context.Context, cmd contract.EnforcementCmd) (contract.EnforcementAck, error) {
	return contract.UnsupportedAck(cmd, "test sensor"), nil
}

func (s *eventSensor) Health(context.Context) (contract.Health, error) {
	return s.health, nil
}

func sensorEventEnvelope(behavior string, pid uint32, binary, filePath, dst string) contract.EventEnvelope {
	return contract.EventEnvelope{
		SensorEvent: &sensorv1.SensorEvent{
			Behavior: behavior,
			Proc: &sensorv1.RawProcess{
				Pid:         pid,
				Binary:      binary,
				StartTimeNs: uint64(time.Now().UnixNano()),
			},
			Object: &sensorv1.RawObject{
				Path: filePath,
				Dst:  dst,
			},
			RawRef: "test-policy-event",
		},
		RawRef: "test-policy-event",
	}
}

func assertSpoolBatch(t *testing.T, dir string) {
	t.Helper()
	batch := loadOnlySpoolBatch(t, dir)
	if batch.GetHeader().GetAgentId() != "agent-a" {
		t.Fatalf("spool batch header = %+v", batch.GetHeader())
	}
}

func waitForOutput(t *testing.T, out interface{ String() string }, want string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		if strings.Contains(out.String(), want) {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for output %q; got %q", want, out.String())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func waitForSpoolBatches(t *testing.T, dir string, want int) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		matches, err := filepath.Glob(filepath.Join(dir, "*.batch.json"))
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) >= want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %d spool batches; got %v", want, matches)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func loadOnlySpoolBatch(t *testing.T, dir string) *dataplanev1.DataBatch {
	t.Helper()
	batches := loadSpoolBatches(t, dir)
	if len(batches) != 1 {
		t.Fatalf("spool batches = %d", len(batches))
	}
	return batches[0]
}

func loadSpoolBatches(t *testing.T, dir string) []*dataplanev1.DataBatch {
	t.Helper()
	queue, err := spool.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := queue.List()
	if err != nil {
		t.Fatal(err)
	}
	var batches []*dataplanev1.DataBatch
	for _, entry := range entries {
		batch, err := queue.LoadDataBatch(entry.ID)
		if err != nil {
			t.Fatalf("load spool batch %s: %v", entry.ID, err)
		}
		batches = append(batches, batch)
	}
	return batches
}

func testDataBatch(batchID, agentID, hostID string) *dataplanev1.DataBatch {
	return &dataplanev1.DataBatch{
		Header: &dataplanev1.BatchHeader{BatchId: batchID, AgentId: agentID, HostId: hostID, TenantId: "default"},
	}
}

func containsInt(items []int, want int) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
