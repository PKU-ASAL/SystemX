package daemon

import (
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	controlplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/controlplane/v1"
	dataplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/dataplane/v1"
	sensorv1 "github.com/sysarmor/sysarmor-next-project/api/proto/sensor/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/config"
	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/localstore"
	agentpolicy "github.com/sysarmor/sysarmor-next-project/internal/agent/policy"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/telemetry"
	"github.com/sysarmor/sysarmor-next-project/internal/endpoint/normalize"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
	"github.com/sysarmor/sysarmor-next-project/internal/sensors/contract"
	sensorruntime "github.com/sysarmor/sysarmor-next-project/internal/sensors/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestLocalControlServerOverUnixSocket(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "agent.sock")
	runner := &AgentRuntime{
		Config: config.Config{
			Agent:     config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default"},
			Control:   config.ControlConfig{SocketPath: socketPath},
			Sensor:    config.SensorConfig{Scope: config.RuntimeScope{Type: "host"}},
			Telemetry: config.DefaultTelemetryConfig(),
		},
		Sensor: &healthOnlySensor{health: contract.Health{
			Backend:      "fake",
			Running:      true,
			Installed:    true,
			PolicyLoaded: true,
		}},
		capability: contract.Capability{
			Backend:         "fake",
			Version:         "test",
			SupportsExec:    true,
			SupportsConnect: true,
			SupportsHealth:  true,
			Collection: []contract.CollectionBehaviorCapability{{
				Behavior: "network.connect",
				Fields:   []string{"socket.port", "process.binary", "lineage_id"},
			}},
		},
	}
	runner.applyRuntimePolicy(policymodel.DefaultPolicy("default"))
	rt := sensorruntime.New(runner.Sensor)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bus, batcher, sender := newTestTelemetry(t, runner)
	stop, err := runner.startLocalControlServer(ctx, rt, bus, batcher, sender, time.Now())
	if err != nil {
		t.Fatalf("startLocalControlServer() error = %v", err)
	}
	defer stop()

	client := newUnixControlClient(t, socketPath)
	health, err := client.Health(context.Background(), &controlplanev1.HealthRequest{Context: &controlplanev1.RequestContext{TenantId: "default", AgentId: "agent-a"}})
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if health.AgentId != "agent-a" || health.Sensor.Backend != "fake" || !health.Sensor.Running {
		t.Fatalf("health = %+v", health)
	}
	if health.GetStreams().GetEventCapacity() == 0 || health.GetStreams().GetSignalCapacity() == 0 {
		t.Fatalf("stream health = %+v", health.GetStreams())
	}
	if health.GetTelemetryBatcher().GetQueuedBatches() != 0 {
		t.Fatalf("telemetry batcher health = %+v", health.GetTelemetryBatcher())
	}
	cap, err := client.Capability(context.Background(), &controlplanev1.CapabilityRequest{})
	if err != nil {
		t.Fatalf("Capability() error = %v", err)
	}
	if cap.AgentId != "agent-a" || !cap.Sensor.SupportsExec || len(cap.SupportedPolicySections) == 0 {
		t.Fatalf("capability = %+v", cap)
	}
	if len(cap.GetCollectionBehaviors()) != 1 || cap.GetCollectionBehaviors()[0].GetBehavior() != "network.connect" {
		t.Fatalf("collection behavior capability = %+v", cap.GetCollectionBehaviors())
	}
	policy, err := client.CurrentPolicy(context.Background(), &controlplanev1.CurrentPolicyRequest{})
	if err != nil {
		t.Fatalf("CurrentPolicy() error = %v", err)
	}
	if policy.PolicyId != policymodel.DefaultPolicyID || policy.RawJson == "" {
		t.Fatalf("policy = %+v", policy)
	}
	profile, err := client.DebugProfile(context.Background(), &controlplanev1.DebugProfileRequest{
		Context:     &controlplanev1.RequestContext{TenantId: "default", AgentId: "agent-a"},
		ProfileType: "cpu",
		Seconds:     1,
		Label:       "unit-test",
	})
	if err != nil {
		t.Fatalf("DebugProfile() error = %v", err)
	}
	if profile.GetProfileType() != "cpu" || profile.GetSeconds() != 1 || profile.GetLabel() != "unit-test" || len(profile.GetProfile()) == 0 {
		t.Fatalf("profile = %+v len=%d", profile, len(profile.GetProfile()))
	}
}

func TestHealthResponseIncludesDefaultManifestVersion(t *testing.T) {
	response := healthResponse(agenthealth.AgentHealth{Detection: agenthealth.DetectionHealth{DefaultManifestVersion: "release-v1"}})
	if got := response.GetDetection().GetDefaultManifestVersion(); got != "release-v1" {
		t.Fatalf("health manifest version = %q, want release-v1", got)
	}
}

func TestLocalControlExplainCollectionPolicyDryRunDoesNotApply(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "agent.sock")
	sensor := &recordingCollectionSensor{healthOnlySensor: healthOnlySensor{health: contract.Health{Backend: "fake", Running: true, Installed: true, PolicyLoaded: true}}}
	runner := &AgentRuntime{
		Config: config.Config{
			Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default"},
			Control: config.ControlConfig{SocketPath: socketPath},
			Sensor:  config.SensorConfig{Scope: config.RuntimeScope{Type: "host"}, ObserveOnly: true},
		},
		Sensor:     sensor,
		capability: contract.Capability{Backend: "fake", SupportsExec: true},
	}
	rt := sensorruntime.New(runner.Sensor)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bus, batcher, sender := newTestTelemetry(t, runner)
	stop, err := runner.startLocalControlServer(ctx, rt, bus, batcher, sender, time.Now())
	if err != nil {
		t.Fatalf("startLocalControlServer() error = %v", err)
	}
	defer stop()

	client := newUnixControlClient(t, socketPath)
	ack, err := client.ApplyPolicy(context.Background(), &controlplanev1.ApplyPolicyRequest{
		Context:    &controlplanev1.RequestContext{TenantId: "default", AgentId: "agent-a", RequestId: "req-explain"},
		PolicyType: "collection",
		DryRun:     true,
		PolicyJson: `{"policy_id":"collection-explain","version":1,"behaviors":[{"id":"process.exec"}],"observe_only":true}`,
	})
	if err != nil {
		t.Fatalf("ApplyPolicy(collection dry-run) error = %v", err)
	}
	if ack.Status != "degraded" || ack.PolicyId != "collection-explain" {
		t.Fatalf("ack = %+v", ack)
	}
	if sensor.lastIntent.Behaviors != nil {
		t.Fatalf("dry-run applied sensor intent = %+v", sensor.lastIntent)
	}
	for _, want := range []string{`"behavior_mappings"`, `"detection_coverage"`, `"missing_behaviors"`} {
		if !strings.Contains(ack.ReportJson, want) {
			t.Fatalf("report_json missing %s: %s", want, ack.ReportJson)
		}
	}
}

func TestLocalControlApplyPolicyUpdatesCurrentPolicy(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "agent.sock")
	runner := &AgentRuntime{
		Config: config.Config{
			Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default"},
			Control: config.ControlConfig{SocketPath: socketPath},
			Sensor:  config.SensorConfig{Scope: config.RuntimeScope{Type: "host"}},
		},
		Sensor:     &healthOnlySensor{health: contract.Health{Backend: "fake", Running: true, Installed: true, PolicyLoaded: true}},
		capability: contract.Capability{Backend: "fake", SupportsExec: true},
	}
	runner.applyRuntimePolicy(policymodel.DefaultPolicy("default"))
	rt := sensorruntime.New(runner.Sensor)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bus, batcher, sender := newTestTelemetry(t, runner)
	stop, err := runner.startLocalControlServer(ctx, rt, bus, batcher, sender, time.Now())
	if err != nil {
		t.Fatalf("startLocalControlServer() error = %v", err)
	}
	defer stop()

	client := newUnixControlClient(t, socketPath)
	ack, err := client.ApplyPolicy(context.Background(), &controlplanev1.ApplyPolicyRequest{
		Context:    &controlplanev1.RequestContext{TenantId: "default", AgentId: "agent-a", RequestId: "req-a"},
		PolicyType: "endpoint",
		PolicyJson: `{
			"policy_id":"local-policy",
			"version":7,
			"collection":{"behaviors":["process.exec"]},
			"detection":{"policy_id":"local-detection","version":1,"rulesets":[{"ref":"ruleset:cep-endpoint","enabled":true}]},
			"telemetry":{"max_batch_items":256,"max_batch_bytes":262144,"flush_interval":"1s"},
			"response":{}
		}`,
	})
	if err != nil {
		t.Fatalf("ApplyPolicy() error = %v", err)
	}
	if ack.Status != "degraded" || ack.PolicyId != "local-policy" || ack.PolicyVersion != 7 {
		t.Fatalf("ack = %+v", ack)
	}
	current, err := client.CurrentPolicy(context.Background(), &controlplanev1.CurrentPolicyRequest{})
	if err != nil {
		t.Fatalf("CurrentPolicy() error = %v", err)
	}
	if current.PolicyId != "local-policy" || current.RawJson == "" {
		t.Fatalf("current = %+v", current)
	}
}

func TestLocalControlApplyTelemetryPolicyContract(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "agent.sock")
	runner := &AgentRuntime{
		Config: config.Config{
			Agent:     config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default"},
			Control:   config.ControlConfig{SocketPath: socketPath},
			Manager:   config.ManagerConfig{Address: "127.0.0.1:9443", Transport: "grpc"},
			Telemetry: config.TelemetryConfig{MaxBatchItems: 10, MaxBatchBytes: 256 << 10, FlushInterval: time.Second},
			Local:     config.LocalConfig{Export: config.LocalExportConfig{RetryInitial: time.Second, RetryMax: 30 * time.Second, RequestTimeout: 10 * time.Second, MaxInflight: 1}},
		},
		Sensor:     &healthOnlySensor{health: contract.Health{Backend: "fake", Running: true, Installed: true, PolicyLoaded: true}},
		capability: contract.Capability{Backend: "fake", SupportsExec: true},
	}
	rt := sensorruntime.New(runner.Sensor)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bus, batcher, sender := newTestTelemetry(t, runner)
	stop, err := runner.startLocalControlServer(ctx, rt, bus, batcher, sender, time.Now())
	if err != nil {
		t.Fatalf("startLocalControlServer() error = %v", err)
	}
	defer stop()

	client := newUnixControlClient(t, socketPath)
	ack, err := client.ApplyPolicy(context.Background(), &controlplanev1.ApplyPolicyRequest{
		Context:    &controlplanev1.RequestContext{TenantId: "default", AgentId: "agent-a", RequestId: "req-telemetry"},
		PolicyType: "telemetry",
		PolicyJson: `{"max_batch_items":64,"max_batch_bytes":131072,"flush_interval":"2s"}`,
	})
	if err != nil {
		t.Fatalf("ApplyPolicy(telemetry) error = %v", err)
	}
	if ack.Status != "applied" || len(ack.Sections) != 1 || ack.Sections[0].RequiresRestart {
		t.Fatalf("ack = %+v", ack)
	}
	if runner.Config.Manager.Transport != "grpc" || runner.Config.Manager.Address != "127.0.0.1:9443" {
		t.Fatalf("manager config = %+v", runner.Config.Manager)
	}
	effective := runner.currentEffectiveTelemetry()
	if effective.MaxBatchItems != 64 || effective.MaxBatchBytes != 131072 || effective.FlushInterval != 2*time.Second {
		t.Fatalf("effective telemetry = %+v", effective)
	}
	if runner.Config.Telemetry.MaxBatchItems != 10 {
		t.Fatalf("telemetry config baseline was mutated: %+v", runner.Config.Telemetry)
	}
	if runner.Config.Local.Export.RetryInitial != time.Second || runner.Config.Local.Export.RetryMax != 30*time.Second || runner.Config.Local.Export.RequestTimeout != 10*time.Second || runner.Config.Local.Export.MaxInflight != 1 {
		t.Fatalf("export config changed by telemetry policy: %+v", runner.Config.Local.Export)
	}
}

func TestApplyTelemetryPolicyPersistsUnifiedEndpointPolicy(t *testing.T) {
	store, err := localstore.Open(t.Context(), localstore.Options{RootDir: filepath.Join(t.TempDir(), "state")})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	runner := &AgentRuntime{localStore: store, Config: config.Config{Telemetry: config.TelemetryConfig{MaxBatchItems: 256, MaxBatchBytes: 256 << 10, FlushInterval: time.Second}}}
	runner.setEndpointPolicy(agentpolicy.EndpointPolicy{PolicyID: "endpoint-a", Version: 1})
	ack := (&localControlServer{runner: runner}).applyTelemetryPolicy(t.Context(), &controlplanev1.ApplyPolicyRequest{
		PolicyType: "telemetry", PolicyJson: `{"max_batch_items":512,"max_batch_bytes":524288,"flush_interval":"2s"}`,
	}, nil)
	if ack.GetStatus() != "applied" {
		t.Fatalf("ack=%+v", ack)
	}
	record, ok, err := store.Policy(t.Context(), "endpoint")
	if err != nil || !ok {
		t.Fatalf("policy ok=%t err=%v", ok, err)
	}
	var persisted agentpolicy.EndpointPolicy
	if err := json.Unmarshal(record.Document, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Telemetry.MaxBatchItems != 512 || persisted.Version != 2 {
		t.Fatalf("persisted=%+v", persisted.Telemetry)
	}
}

func TestLocalControlApplyCollectionPolicyUpdatesSensorRuntime(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "agent.sock")
	sensor := &recordingCollectionSensor{healthOnlySensor: healthOnlySensor{health: contract.Health{Backend: "fake", Running: true, Installed: true, PolicyLoaded: true}}}
	runner := &AgentRuntime{
		Config: config.Config{
			Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default"},
			Control: config.ControlConfig{SocketPath: socketPath},
			Sensor:  config.SensorConfig{Scope: config.RuntimeScope{Type: "host"}, ObserveOnly: true},
		},
		Sensor:     sensor,
		capability: contract.Capability{Backend: "fake", SupportsExec: true},
	}
	rt := sensorruntime.New(runner.Sensor)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bus, batcher, sender := newTestTelemetry(t, runner)
	stop, err := runner.startLocalControlServer(ctx, rt, bus, batcher, sender, time.Now())
	if err != nil {
		t.Fatalf("startLocalControlServer() error = %v", err)
	}
	defer stop()

	client := newUnixControlClient(t, socketPath)
	for _, contentJSON := range []string{
		`{
			"api_version":"sysarmor.content/v1",
			"kind":"contextset",
			"metadata":{"id":"ctx:payload-path-prefixes","version":"2026.06.18.1"},
			"spec":{"value_type":"path_prefix","values":["/dev/shm"]}
		}`,
		`{
			"api_version":"sysarmor.content/v1",
			"kind":"iocpack",
			"metadata":{"id":"ioc:c2-ip-feed","version":"2026.06.18.1"},
			"spec":{"value_type":"ip","values":["10.66.0.99"]}
		}`,
		`{
			"api_version":"sysarmor.content/v1",
			"kind":"iocpack",
			"metadata":{"id":"ioc:c2-control-port-feed","version":"2026.06.18.1"},
			"spec":{"value_type":"port","values":["443"]}
		}`,
	} {
		if _, err := client.ApplyContent(context.Background(), &controlplanev1.ApplyContentRequest{
			Context:       &controlplanev1.RequestContext{TenantId: "default", AgentId: "agent-a", RequestId: "req-content"},
			ContentJson:   contentJSON,
			AllowUnsigned: true,
		}); err != nil {
			t.Fatalf("ApplyContent() error = %v", err)
		}
	}
	ack, err := client.ApplyPolicy(context.Background(), &controlplanev1.ApplyPolicyRequest{
		Context:    &controlplanev1.RequestContext{TenantId: "default", AgentId: "agent-a", RequestId: "req-collection"},
		PolicyType: "collection",
		PolicyJson: `{
			"policy_id":"collection-a",
			"version":3,
			"behaviors":[
				{"id":"network.connect","selectors":{"socket":{"families":["AF_INET"],"addr_refs":["ioc:c2-ip-feed"],"port_refs":["ioc:c2-control-port-feed"]}}},
				{"id":"file.write","selectors":{"file":{"prefix_refs":["ctx:payload-path-prefixes"]}}}
			],
			"observe_only":true
		}`,
	})
	if err != nil {
		t.Fatalf("ApplyPolicy(collection) error = %v", err)
	}
	if ack.Status != "degraded" || ack.PolicyId != "collection-a" || ack.PolicyVersion != 3 {
		t.Fatalf("ack = %+v", ack)
	}
	if ack.ReportJson == "" || !strings.Contains(ack.ReportJson, `"pushed_down_selectors"`) || !strings.Contains(ack.ReportJson, `"generated_policy_hash"`) {
		t.Fatalf("ack report_json = %q", ack.ReportJson)
	}
	if !strings.Contains(ack.ReportJson, `"resolved_refs"`) || !strings.Contains(ack.ReportJson, `"ioc:c2-ip-feed"`) || !strings.Contains(ack.ReportJson, `"ctx:payload-path-prefixes"`) {
		t.Fatalf("ack report_json = %q", ack.ReportJson)
	}
	if !containsString(ack.Details, "unsupported_selectors=0") {
		t.Fatalf("ack details = %v", ack.Details)
	}
	if !containsString(ack.Details, "resolved_refs=3") {
		t.Fatalf("ack details = %v", ack.Details)
	}
	got := sensor.lastIntent
	if len(got.Behaviors) != 2 || got.Behaviors[0] != "network.connect" {
		t.Fatalf("intent behaviors = %v", got.Behaviors)
	}
	if len(got.BehaviorFilters) != 2 {
		t.Fatalf("intent behavior filters = %+v", got.BehaviorFilters)
	}
	if got.BehaviorFilters[0].Behavior != "network.connect" || got.BehaviorFilters[0].SocketFamilies[0] != "AF_INET" {
		t.Fatalf("network filter = %+v", got.BehaviorFilters[0])
	}
	if got.BehaviorFilters[1].Behavior != "file.write" || got.BehaviorFilters[1].FilePrefixes[0] != "/dev/shm" {
		t.Fatalf("file filter = %+v", got.BehaviorFilters[1])
	}
}

func TestLocalControlPushesNetworkProcessBinarySelector(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "agent.sock")
	sensor := &recordingCollectionSensor{healthOnlySensor: healthOnlySensor{health: contract.Health{Backend: "fake", Running: true, Installed: true, PolicyLoaded: true}}}
	runner := &AgentRuntime{
		Config: config.Config{
			Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default"},
			Control: config.ControlConfig{SocketPath: socketPath},
			Sensor:  config.SensorConfig{Scope: config.RuntimeScope{Type: "host"}, ObserveOnly: true},
		},
		Sensor:     sensor,
		capability: contract.Capability{Backend: "fake", SupportsExec: true},
	}
	rt := sensorruntime.New(runner.Sensor)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bus, batcher, sender := newTestTelemetry(t, runner)
	stop, err := runner.startLocalControlServer(ctx, rt, bus, batcher, sender, time.Now())
	if err != nil {
		t.Fatalf("startLocalControlServer() error = %v", err)
	}
	defer stop()

	client := newUnixControlClient(t, socketPath)
	ack, err := client.ApplyPolicy(context.Background(), &controlplanev1.ApplyPolicyRequest{
		Context:    &controlplanev1.RequestContext{TenantId: "default", AgentId: "agent-a", RequestId: "req-network-binary"},
		PolicyType: "collection",
		PolicyJson: `{
			"policy_id":"collection-network-binary",
			"version":1,
			"behaviors":[
				{"id":"network.connect","selectors":{"process":{"binary_prefixes":["/tmp"]},"socket":{"families":["AF_INET"]}}}
			],
			"observe_only":true
		}`,
	})
	if err != nil {
		t.Fatalf("ApplyPolicy(collection) error = %v", err)
	}
	if ack.Status != "degraded" {
		t.Fatalf("ack status = %q, want degraded from detection dependencies: %+v", ack.Status, ack)
	}
	if strings.Contains(ack.ReportJson, `"unsupported_selectors"`) || !strings.Contains(ack.ReportJson, `"process.binary_prefix"`) || !strings.Contains(ack.ReportJson, `"pushed_down"`) {
		t.Fatalf("ack report_json = %q", ack.ReportJson)
	}
	if len(sensor.lastIntent.Behaviors) != 1 || sensor.lastIntent.BehaviorFilters[0].BinaryPrefixes[0] != "/tmp" {
		t.Fatalf("sensor intent = %+v", sensor.lastIntent)
	}
}

func TestLocalControlApplyListGetContent(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "agent.sock")
	runner := &AgentRuntime{
		Config: config.Config{
			Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default"},
			Control: config.ControlConfig{SocketPath: socketPath},
			Sensor:  config.SensorConfig{Scope: config.RuntimeScope{Type: "host"}},
		},
		Sensor:     &healthOnlySensor{health: contract.Health{Backend: "fake", Running: true, Installed: true, PolicyLoaded: true}},
		capability: contract.Capability{Backend: "fake", SupportsExec: true},
	}
	rt := sensorruntime.New(runner.Sensor)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bus, batcher, sender := newTestTelemetry(t, runner)
	stop, err := runner.startLocalControlServer(ctx, rt, bus, batcher, sender, time.Now())
	if err != nil {
		t.Fatalf("startLocalControlServer() error = %v", err)
	}
	defer stop()

	client := newUnixControlClient(t, socketPath)
	contentJSON := `{
		"api_version":"sysarmor.content/v1",
		"kind":"iocpack",
		"metadata":{"id":"ioc:c2-ip-feed","version":"2026.06.17.1"},
		"spec":{"value_type":"ip","values":["203.0.113.10"]}
	}`
	ack, err := client.ApplyContent(context.Background(), &controlplanev1.ApplyContentRequest{
		Context:       &controlplanev1.RequestContext{TenantId: "default", AgentId: "agent-a", RequestId: "req-content"},
		ContentJson:   contentJSON,
		AllowUnsigned: true,
	})
	if err != nil {
		t.Fatalf("ApplyContent() error = %v", err)
	}
	if ack.Status != "applied" || ack.PolicyId != "ioc:c2-ip-feed" {
		t.Fatalf("ack = %+v", ack)
	}
	list, err := client.ListContent(context.Background(), &controlplanev1.ListContentRequest{Kind: "iocpack"})
	if err != nil {
		t.Fatalf("ListContent() error = %v", err)
	}
	found := false
	for _, record := range list.GetRecords() {
		found = found || record.GetRef() == "ioc:c2-ip-feed"
	}
	if !found {
		t.Fatalf("list = %+v", list)
	}
	got, err := client.GetContent(context.Background(), &controlplanev1.GetContentRequest{Ref: "ioc:c2-ip-feed"})
	if err != nil {
		t.Fatalf("GetContent() error = %v", err)
	}
	if got.GetRecord().GetVersion() != "2026.06.17.1" || got.GetRecord().GetRawJson() == "" {
		t.Fatalf("get = %+v", got)
	}
}

func TestContentUpdateTransactionSerializesCallbacks(t *testing.T) {
	runner := &AgentRuntime{}
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	done := make(chan struct{}, 2)
	callback := func() {
		entered <- struct{}{}
		<-release
		done <- struct{}{}
	}

	go runner.withContentUpdateTransaction(callback)
	<-entered
	go runner.withContentUpdateTransaction(callback)
	select {
	case <-entered:
		t.Fatal("second content transaction entered before first completed")
	case <-time.After(20 * time.Millisecond):
	}
	release <- struct{}{}
	<-done
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("second content transaction did not enter after first completed")
	}
	release <- struct{}{}
	<-done
}

func TestLocalControlContentApplyRebuildsDetection(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "agent.sock")
	runner := &AgentRuntime{
		Config: config.Config{
			Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default"},
			Control: config.ControlConfig{SocketPath: socketPath},
			Sensor:  config.SensorConfig{Scope: config.RuntimeScope{Type: "host"}},
		},
		Sensor:     &healthOnlySensor{health: contract.Health{Backend: "fake", Running: true, Installed: true, PolicyLoaded: true}},
		capability: contract.Capability{Backend: "fake", SupportsExec: true},
	}
	runner.applyRuntimePolicy(policymodel.DefaultPolicy("default"))
	rt := sensorruntime.New(runner.Sensor)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bus, batcher, sender := newTestTelemetry(t, runner)
	stop, err := runner.startLocalControlServer(ctx, rt, bus, batcher, sender, time.Now())
	if err != nil {
		t.Fatalf("startLocalControlServer() error = %v", err)
	}
	defer stop()

	client := newUnixControlClient(t, socketPath)
	contentJSON := `{
		"api_version":"sysarmor.content/v1",
		"kind":"iocpack",
		"metadata":{"id":"ioc:c2-control-port-feed","version":"local-9443"},
		"spec":{"value_type":"port","values":["9443"]}
	}`
	if _, err := client.ApplyContent(context.Background(), &controlplanev1.ApplyContentRequest{
		Context:       &controlplanev1.RequestContext{TenantId: "default", AgentId: "agent-a", RequestId: "req-content-rebuild"},
		ContentJson:   contentJSON,
		AllowUnsigned: true,
	}); err != nil {
		t.Fatalf("ApplyContent() error = %v", err)
	}

	norm := normalize.New("agent-a", "host-a", nil)
	appendEndpointEventForTest(t, runner, bus, norm, sensorEventEnvelope("network.connect", 100, "/bin/bash", "", "10.66.0.99:9443"))
	signalStream, err := client.WatchSignals(context.Background(), &controlplanev1.WatchSignalsRequest{IncludeRecent: true, Limit: 1, RuleId: "reverse_shell_pattern", Where: "endpoint"})
	if err != nil {
		t.Fatalf("WatchSignals() error = %v", err)
	}
	frame, err := signalStream.Recv()
	if err != nil {
		t.Fatalf("signal Recv() error = %v", err)
	}
	var gotVersion string
	for _, ref := range frame.GetSignal().GetIocRefs() {
		if ref.GetRef() == "ioc:c2-control-port-feed" {
			gotVersion = ref.GetVersion()
		}
	}
	if gotVersion != "local-9443" {
		t.Fatalf("signal ioc version = %q, want local-9443; signal=%+v", gotVersion, frame.GetSignal())
	}
}

func TestLocalControlContentRebuildFailureKeepsPreviousDetection(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "agent.sock")
	runner := &AgentRuntime{
		Config: config.Config{
			Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default"},
			Control: config.ControlConfig{SocketPath: socketPath},
			Sensor:  config.SensorConfig{Scope: config.RuntimeScope{Type: "host"}},
		},
		Sensor:     &healthOnlySensor{health: contract.Health{Backend: "fake", Running: true, Installed: true, PolicyLoaded: true}},
		capability: contract.Capability{Backend: "fake", SupportsExec: true, SupportsFile: true, SupportsConnect: true},
	}
	runner.applyRuntimePolicy(policymodel.DefaultPolicy("default"))
	rt := sensorruntime.New(runner.Sensor)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bus, batcher, sender := newTestTelemetry(t, runner)
	stop, err := runner.startLocalControlServer(ctx, rt, bus, batcher, sender, time.Now())
	if err != nil {
		t.Fatalf("startLocalControlServer() error = %v", err)
	}
	defer stop()

	client := newUnixControlClient(t, socketPath)
	good := `{
		"api_version":"sysarmor.content/v1",
		"kind":"iocpack",
		"metadata":{"id":"ioc:c2-control-port-feed","version":"good-9443"},
		"spec":{"value_type":"port","values":["9443"]}
	}`
	goodAck, err := client.ApplyContent(context.Background(), &controlplanev1.ApplyContentRequest{
		Context:       &controlplanev1.RequestContext{TenantId: "default", AgentId: "agent-a", RequestId: "req-good-content"},
		ContentJson:   good,
		AllowUnsigned: true,
	})
	if err != nil {
		t.Fatalf("ApplyContent(good) error = %v", err)
	}
	if goodAck.GetStatus() != "applied" {
		t.Fatalf("good content ack = %+v", goodAck)
	}
	badAck, err := client.ApplyContent(context.Background(), &controlplanev1.ApplyContentRequest{
		Context:       &controlplanev1.RequestContext{TenantId: "default", AgentId: "agent-a", RequestId: "req-bad-content"},
		ContentJson:   badRuntimeRulePackJSON(),
		AllowUnsigned: true,
	})
	if err != nil {
		t.Fatalf("ApplyContent(bad) error = %v", err)
	}
	if badAck.GetStatus() != "rejected" || !strings.Contains(badAck.GetMessage(), "detection rebuild failed") {
		t.Fatalf("bad content ack = %+v", badAck)
	}
	if _, ok := runner.contentStore().Get("rulepack:bad-runtime"); ok {
		t.Fatalf("rejected content was committed")
	}

	norm := normalize.New("agent-a", "host-a", nil)
	batch := appendEndpointEventForTest(t, runner, bus, norm, sensorEventEnvelope("file.write", 100, "/usr/bin/curl", "/dev/shm/kept.sh", ""))
	if len(batch.GetSignals()) != 1 || batch.GetSignals()[0].GetSignal().GetName() != "payload_dropped" {
		t.Fatalf("signals after rejected rebuild = %+v, want previous detection engine still active", batch.GetSignals())
	}
	health, err := client.Health(context.Background(), &controlplanev1.HealthRequest{Context: &controlplanev1.RequestContext{TenantId: "default", AgentId: "agent-a"}})
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if health.GetDetection().GetLastApplyStatus() != "rejected" || !strings.Contains(health.GetDetection().GetLastApplyError(), "bad_runtime_rule") {
		t.Fatalf("detection health = %+v", health.GetDetection())
	}
	for _, ref := range health.GetDetection().GetContentRefs() {
		if ref.GetRef() == "rulepack:bad-runtime" {
			t.Fatalf("detection health includes rejected content ref: %+v", health.GetDetection())
		}
	}
}

func TestLocalControlDetectionPolicyRebuildFailureKeepsPreviousPolicy(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "agent.sock")
	runner := &AgentRuntime{
		Config: config.Config{
			Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default"},
			Control: config.ControlConfig{SocketPath: socketPath},
			Sensor:  config.SensorConfig{Scope: config.RuntimeScope{Type: "host"}},
		},
		Sensor:     &healthOnlySensor{health: contract.Health{Backend: "fake", Running: true, Installed: true, PolicyLoaded: true}},
		capability: contract.Capability{Backend: "fake", SupportsExec: true, SupportsFile: true, SupportsConnect: true},
	}
	runner.applyRuntimePolicy(policymodel.DefaultPolicy("default"))
	rt := sensorruntime.New(runner.Sensor)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bus, batcher, sender := newTestTelemetry(t, runner)
	stop, err := runner.startLocalControlServer(ctx, rt, bus, batcher, sender, time.Now())
	if err != nil {
		t.Fatalf("startLocalControlServer() error = %v", err)
	}
	defer stop()

	client := newUnixControlClient(t, socketPath)
	if _, err := client.ApplyContent(context.Background(), &controlplanev1.ApplyContentRequest{
		Context:       &controlplanev1.RequestContext{TenantId: "default", AgentId: "agent-a", RequestId: "req-bad-rulepack"},
		ContentJson:   badRuntimeRulePackJSON(),
		AllowUnsigned: true,
	}); err != nil {
		t.Fatalf("ApplyContent(bad rulepack not enabled) error = %v", err)
	}
	ack, err := client.ApplyPolicy(context.Background(), &controlplanev1.ApplyPolicyRequest{
		Context:    &controlplanev1.RequestContext{TenantId: "default", AgentId: "agent-a", RequestId: "req-bad-detection-policy"},
		PolicyType: "detection",
		PolicyJson: `{
			"policy_id":"bad-runtime-policy",
			"version":9,
			"mode":"observe",
			"rulesets":[{"ref":"ruleset:bad-runtime","enabled":true}]
		}`,
	})
	if err != nil {
		t.Fatalf("ApplyPolicy(bad detection) error = %v", err)
	}
	if ack.GetStatus() != "rejected" {
		t.Fatalf("ack = %+v, want rejected", ack)
	}
	if runner.activePolicy().Detection.PolicyID == "bad-runtime-policy" {
		t.Fatalf("bad detection policy replaced active policy")
	}
	batch := appendEndpointEventForTest(t, runner, bus, normalize.New("agent-a", "host-a", nil), sensorEventEnvelope("file.write", 100, "/usr/bin/curl", "/dev/shm/kept-policy.sh", ""))
	if len(batch.GetSignals()) != 1 || batch.GetSignals()[0].GetSignal().GetName() != "payload_dropped" {
		t.Fatalf("signals after rejected policy = %+v, want previous detection policy still active", batch.GetSignals())
	}
}

func badRuntimeRulePackJSON() string {
	return `{
		"api_version":"sysarmor.content/v1",
		"kind":"rulepack",
		"metadata":{"id":"rulepack:bad-runtime","version":"bad-v1"},
		"spec":{"rulesets":[{"id":"ruleset:cep-endpoint","version":"v1","rules":[{
			"rule_id":"bad_runtime_rule",
			"version":1,
			"severity":"high",
			"runtime":{"type":"made_up_runtime"}
		}]}]}
	}`
}

type recordingCollectionSensor struct {
	healthOnlySensor
	lastIntent contract.CollectionIntent
}

func (s *recordingCollectionSensor) Apply(ctx context.Context, intent contract.CollectionIntent) error {
	s.lastIntent = intent
	return s.healthOnlySensor.Apply(ctx, intent)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestLocalControlWatchRecentEventsAndSignals(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "agent.sock")
	runner := &AgentRuntime{
		Config: config.Config{
			Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default"},
			Control: config.ControlConfig{SocketPath: socketPath},
			Sensor:  config.SensorConfig{Scope: config.RuntimeScope{Type: "host"}},
		},
		Sensor:     &healthOnlySensor{health: contract.Health{Backend: "fake", Running: true, Installed: true, PolicyLoaded: true}},
		capability: contract.Capability{Backend: "fake", SupportsExec: true},
	}
	rt := sensorruntime.New(runner.Sensor)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bus, batcher, sender := newTestTelemetry(t, runner)
	stop, err := runner.startLocalControlServer(ctx, rt, bus, batcher, sender, time.Now())
	if err != nil {
		t.Fatalf("startLocalControlServer() error = %v", err)
	}
	defer stop()

	norm := normalize.NewWithOptions("agent-a", "host-a", nil, normalize.Options{TenantID: "default", ScopeType: "host", Labels: map[string]string{"benchmark_run": "run-a"}})
	runner.applyRuntimePolicy(policymodel.DefaultPolicy("default"))
	appendEndpointEventForTest(t, runner, bus, norm, sensorEventEnvelope("file.write", 100, "/usr/bin/curl", "/dev/shm/x.sh", ""))

	client := newUnixControlClient(t, socketPath)
	eventStream, err := client.WatchEvents(context.Background(), &controlplanev1.WatchEventsRequest{IncludeRecent: true, Limit: 1, Behavior: "file.write"})
	if err != nil {
		t.Fatalf("WatchEvents() error = %v", err)
	}
	eventFrame, err := eventStream.Recv()
	if err != nil {
		t.Fatalf("event Recv() error = %v", err)
	}
	if eventFrame.GetEvent().GetBehavior() != "file.write" || eventFrame.GetAgentId() != "agent-a" {
		t.Fatalf("event frame = %+v", eventFrame)
	}
	if eventFrame.GetEvent().GetTenantId() != "default" || eventFrame.GetEvent().GetScope().GetType() != "host" {
		t.Fatalf("event provenance tags = %+v", eventFrame.GetEvent())
	}
	if eventFrame.GetEvent().GetLabels()["benchmark_run"] != "run-a" {
		t.Fatalf("event labels = %+v", eventFrame.GetEvent().GetLabels())
	}
	eventGet, err := client.GetEvent(context.Background(), &controlplanev1.GetEventRequest{EventId: eventFrame.GetEvent().GetId()})
	if err != nil {
		t.Fatalf("GetEvent() error = %v", err)
	}
	if eventGet.GetFrame().GetEvent().GetId() != eventFrame.GetEvent().GetId() {
		t.Fatalf("GetEvent frame = %+v, want event id %q", eventGet.GetFrame(), eventFrame.GetEvent().GetId())
	}
	signalStream, err := client.WatchSignals(context.Background(), &controlplanev1.WatchSignalsRequest{IncludeRecent: true, Limit: 1, RuleId: "payload_dropped", Where: "endpoint"})
	if err != nil {
		t.Fatalf("WatchSignals() error = %v", err)
	}
	signalFrame, err := signalStream.Recv()
	if err != nil {
		t.Fatalf("signal Recv() error = %v", err)
	}
	if signalFrame.GetSignal().GetName() != "payload_dropped" || signalFrame.GetAgentId() != "agent-a" {
		t.Fatalf("signal frame = %+v", signalFrame)
	}
	if len(signalFrame.GetSignal().GetEventRefs()) != 1 || signalFrame.GetSignal().GetEventRefs()[0] != eventFrame.GetEvent().GetId() {
		t.Fatalf("signal event refs = %v, want %s", signalFrame.GetSignal().GetEventRefs(), eventFrame.GetEvent().GetId())
	}
	if signalFrame.GetSignal().GetLabels()["benchmark_run"] != "run-a" {
		t.Fatalf("signal labels = %+v", signalFrame.GetSignal().GetLabels())
	}
	labelStream, err := client.WatchSignals(context.Background(), &controlplanev1.WatchSignalsRequest{
		IncludeRecent: true,
		Limit:         1,
		Filter:        &controlplanev1.WatchFilter{Labels: map[string]string{"benchmark_run": "run-a"}},
	})
	if err != nil {
		t.Fatalf("WatchSignals(label) error = %v", err)
	}
	if _, err := labelStream.Recv(); err != nil {
		t.Fatalf("label signal Recv() error = %v", err)
	}
	afterStream, err := client.WatchSignals(context.Background(), &controlplanev1.WatchSignalsRequest{
		IncludeRecent: true,
		SnapshotOnly:  true,
		Filter:        &controlplanev1.WatchFilter{AfterSequence: signalFrame.GetSequence()},
	})
	if err != nil {
		t.Fatalf("WatchSignals(after sequence) error = %v", err)
	}
	if frame, err := afterStream.Recv(); err == nil {
		t.Fatalf("after sequence returned frame %+v, want no recent frames", frame)
	}
}

func TestLocalControlContentApplyEnablesCEPRulePack(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "agent.sock")
	runner := &AgentRuntime{
		Config: config.Config{
			Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default"},
			Control: config.ControlConfig{SocketPath: socketPath},
			Sensor:  config.SensorConfig{Scope: config.RuntimeScope{Type: "host"}},
		},
		Sensor:     &healthOnlySensor{health: contract.Health{Backend: "fake", Running: true, Installed: true, PolicyLoaded: true}},
		capability: contract.Capability{Backend: "fake", SupportsExec: true, SupportsFile: true, SupportsConnect: true},
	}
	rt := sensorruntime.New(runner.Sensor)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bus, batcher, sender := newTestTelemetry(t, runner)
	stop, err := runner.startLocalControlServer(ctx, rt, bus, batcher, sender, time.Now())
	if err != nil {
		t.Fatalf("startLocalControlServer() error = %v", err)
	}
	defer stop()

	client := newUnixControlClient(t, socketPath)
	contentAck, err := client.ApplyContent(context.Background(), &controlplanev1.ApplyContentRequest{
		Context:       &controlplanev1.RequestContext{TenantId: "default", AgentId: "agent-a"},
		ContentJson:   cepRulePackJSON(),
		AllowUnsigned: true,
	})
	if err != nil {
		t.Fatalf("ApplyContent() error = %v", err)
	}
	if contentAck.GetStatus() != "applied" {
		t.Fatalf("content ack = %+v", contentAck)
	}
	detectionPolicy := `{
		"policy_id":"local-cep-detection",
		"version":1,
		"mode":"observe",
		"rulesets":[{"ref":"ruleset:cep","enabled":true}]
	}`
	policyAck, err := client.ApplyPolicy(context.Background(), &controlplanev1.ApplyPolicyRequest{
		Context:    &controlplanev1.RequestContext{TenantId: "default", AgentId: "agent-a"},
		PolicyType: "detection",
		PolicyJson: detectionPolicy,
	})
	if err != nil {
		t.Fatalf("ApplyPolicy() error = %v", err)
	}
	if policyAck.GetStatus() != "applied" {
		t.Fatalf("policy ack = %+v", policyAck)
	}

	norm := normalize.NewWithOptions("agent-a", "host-a", nil, normalize.Options{TenantID: "default", ScopeType: "host"})
	for _, ev := range []contract.EventEnvelope{
		cepSensorEventEnvelope("file.write", "/usr/bin/curl", "/dev/shm/cep-x", ""),
		cepSensorEventEnvelope("file.chmod", "/usr/bin/chmod", "/dev/shm/cep-x", ""),
		cepSensorEventEnvelope("process.exec", "/dev/shm/cep-x", "", ""),
		cepSensorEventEnvelope("network.connect", "/dev/shm/cep-x", "", "10.66.0.99:443"),
	} {
		appendEndpointEventForTest(t, runner, bus, norm, ev)
	}

	signalStream, err := client.WatchSignals(context.Background(), &controlplanev1.WatchSignalsRequest{IncludeRecent: true, Limit: 1, RuleId: "cep_payload_lifecycle", Where: "endpoint"})
	if err != nil {
		t.Fatalf("WatchSignals() error = %v", err)
	}
	frame, err := signalStream.Recv()
	if err != nil {
		t.Fatalf("signal Recv() error = %v", err)
	}
	if got := frame.GetSignal().GetEventRefs(); len(got) != 4 {
		t.Fatalf("event refs = %v, want 4", got)
	}
	if frame.GetSignal().GetTerminal() {
		t.Fatalf("signal = %+v, want rulepack terminal=false", frame.GetSignal())
	}
	health, err := client.Health(context.Background(), &controlplanev1.HealthRequest{Context: &controlplanev1.RequestContext{TenantId: "default", AgentId: "agent-a"}})
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if health.GetCep().GetEmittedSignals() == 0 {
		t.Fatalf("cep health = %+v, want emitted signals", health.GetCep())
	}
}

func cepSensorEventEnvelope(behavior, binary, filePath, dst string) contract.EventEnvelope {
	return contract.EventEnvelope{
		SensorEvent: &sensorv1.SensorEvent{
			Behavior: behavior,
			Proc: &sensorv1.RawProcess{
				Pid:          200,
				Binary:       binary,
				StartTimeNs:  200,
				SensorExecId: "cep-exec",
			},
			Object: &sensorv1.RawObject{
				Path: filePath,
				Dst:  dst,
			},
			RawRef: "cep-test-event",
		},
		RawRef: "cep-test-event",
	}
}

func cepRulePackJSON() string {
	return `{
		"api_version":"sysarmor.content/v1",
		"kind":"rulepack",
		"metadata":{"id":"rulepack:cep","version":"v1"},
		"spec":{"rulesets":[{"id":"ruleset:cep","version":"v1","rules":[{
			"rule_id":"cep_payload_lifecycle",
			"version":1,
			"severity":"critical",
			"runtime":{
				"type":"sequence",
				"sequence":{
					"within":"60s",
					"by":["lineage_id"],
					"steps":[
						{"id":"drop","event":"file.write","conditions":[{"field":"file.path","op":"prefix","value":"/dev/shm/"}]},
						{"id":"chmod","event":"file.chmod","conditions":[{"field":"file.path","op":"same_as","step":"drop"}]},
						{"id":"exec","event":"process.exec","conditions":[{"field":"process.binary","op":"same_as","step":"drop","step_field":"file.path"}]},
						{"id":"connect","event":"network.connect","conditions":[{"field":"socket.port","op":"in","values":["443"]}]}
					]
				}
			},
			"requires":{"events":[
				{"behavior":"file.write","fields":["file.path","process.binary","lineage_id"]},
				{"behavior":"file.chmod","fields":["file.path","lineage_id"]},
				{"behavior":"process.exec","fields":["process.binary","lineage_id"]},
				{"behavior":"network.connect","fields":["socket.port","lineage_id"]}
			]},
			"output":{"terminal":false}
		}]}]}
	}`
}

type noopUploader struct{}

func (noopUploader) SendBatch(batch *dataplanev1.DataBatch) (*dataplanev1.DataAck, error) {
	return &dataplanev1.DataAck{Accepted: true, BatchId: batch.GetHeader().GetBatchId()}, nil
}

type recordingUploader struct {
	ch chan *dataplanev1.DataBatch
}

func newRecordingUploader() *recordingUploader {
	return &recordingUploader{ch: make(chan *dataplanev1.DataBatch, 16)}
}

func (u *recordingUploader) SendBatch(batch *dataplanev1.DataBatch) (*dataplanev1.DataAck, error) {
	if u != nil && batch != nil {
		u.ch <- batch
	}
	return (&noopUploader{}).SendBatch(batch)
}

func newTestTelemetry(t testing.TB, runner *AgentRuntime) (*telemetry.Bus, *telemetry.Batcher, *telemetry.Sender) {
	t.Helper()
	installTestDetection(t, runner)
	bus := telemetry.NewBus(1024)
	batcher := telemetry.NewBatcher(runner.newDataBatch, 10, time.Hour, 16)
	sender := &telemetry.Sender{Appender: noopUploader{}, Batcher: batcher}
	return bus, batcher, sender
}

func newUnixControlClient(t *testing.T, socketPath string) controlplanev1.AgentControlPlaneServiceClient {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)
	conn, err := grpc.DialContext(ctx, "unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		}),
		grpc.WithBlock(),
	)
	if err != nil {
		t.Fatalf("dial unix control socket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return controlplanev1.NewAgentControlPlaneServiceClient(conn)
}
