package daemon

import (
	"context"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	controlv1 "github.com/sysarmor/sysarmor-next-project/api/proto/control/v1"
	dataplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/dataplane/v1"
	sensorv1 "github.com/sysarmor/sysarmor-next-project/api/proto/sensor/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/config"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/spool"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/uploadworker"
	"github.com/sysarmor/sysarmor-next-project/internal/endpoint/detection"
	"github.com/sysarmor/sysarmor-next-project/internal/endpoint/normalize"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
	"github.com/sysarmor/sysarmor-next-project/internal/sensor/contract"
	sensorruntime "github.com/sysarmor/sysarmor-next-project/internal/sensor/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestLocalControlServerOverUnixSocket(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "agent.sock")
	queue, err := spool.OpenWithLimit(filepath.Join(dir, "spool"), 16384)
	if err != nil {
		t.Fatal(err)
	}
	worker := &uploadworker.Worker{
		Queue:    queue,
		Uploader: noopUploader{},
	}
	runner := &Runner{
		Config: config.Config{
			Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default"},
			Control: config.ControlConfig{SocketPath: socketPath},
			Sensor:  config.SensorConfig{Scope: config.RuntimeScope{Type: "host"}},
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
	stop, err := runner.startLocalControlServer(ctx, rt, queue, worker, time.Now())
	if err != nil {
		t.Fatalf("startLocalControlServer() error = %v", err)
	}
	defer stop()

	client := newUnixControlClient(t, socketPath)
	health, err := client.Health(context.Background(), &controlv1.HealthRequest{Context: &controlv1.RequestContext{TenantId: "default", AgentId: "agent-a"}})
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if health.AgentId != "agent-a" || health.Sensor.Backend != "fake" || !health.Sensor.Running {
		t.Fatalf("health = %+v", health)
	}
	if health.GetStreams().GetEventCapacity() != 0 || health.GetStreams().GetEventNextSequence() != 0 {
		t.Fatalf("stream health = %+v", health.GetStreams())
	}
	if health.GetWal().GetMaxBytes() != 16384 || health.GetWal().GetQueuedBatches() != 0 {
		t.Fatalf("wal health = %+v", health.GetWal())
	}
	cap, err := client.Capability(context.Background(), &controlv1.CapabilityRequest{})
	if err != nil {
		t.Fatalf("Capability() error = %v", err)
	}
	if cap.AgentId != "agent-a" || !cap.Sensor.SupportsExec || len(cap.SupportedPolicySections) == 0 {
		t.Fatalf("capability = %+v", cap)
	}
	if len(cap.GetCollectionBehaviors()) != 1 || cap.GetCollectionBehaviors()[0].GetBehavior() != "network.connect" {
		t.Fatalf("collection behavior capability = %+v", cap.GetCollectionBehaviors())
	}
	policy, err := client.CurrentPolicy(context.Background(), &controlv1.CurrentPolicyRequest{})
	if err != nil {
		t.Fatalf("CurrentPolicy() error = %v", err)
	}
	if policy.PolicyId != policymodel.DefaultPolicyID || policy.RawJson == "" {
		t.Fatalf("policy = %+v", policy)
	}
}

func TestLocalControlExplainCollectionPolicyDryRunDoesNotApply(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "agent.sock")
	queue, err := spool.OpenWithLimit(filepath.Join(dir, "spool"), 16384)
	if err != nil {
		t.Fatal(err)
	}
	worker := &uploadworker.Worker{Queue: queue, Uploader: noopUploader{}}
	sensor := &recordingCollectionSensor{healthOnlySensor: healthOnlySensor{health: contract.Health{Backend: "fake", Running: true, Installed: true, PolicyLoaded: true}}}
	runner := &Runner{
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
	stop, err := runner.startLocalControlServer(ctx, rt, queue, worker, time.Now())
	if err != nil {
		t.Fatalf("startLocalControlServer() error = %v", err)
	}
	defer stop()

	client := newUnixControlClient(t, socketPath)
	ack, err := client.ApplyPolicy(context.Background(), &controlv1.ApplyPolicyRequest{
		Context:    &controlv1.RequestContext{TenantId: "default", AgentId: "agent-a", RequestId: "req-explain"},
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
	queue, err := spool.OpenWithLimit(filepath.Join(dir, "spool"), 4096)
	if err != nil {
		t.Fatal(err)
	}
	worker := &uploadworker.Worker{Queue: queue, Uploader: noopUploader{}}
	runner := &Runner{
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
	stop, err := runner.startLocalControlServer(ctx, rt, queue, worker, time.Now())
	if err != nil {
		t.Fatalf("startLocalControlServer() error = %v", err)
	}
	defer stop()

	client := newUnixControlClient(t, socketPath)
	ack, err := client.ApplyPolicy(context.Background(), &controlv1.ApplyPolicyRequest{
		Context:    &controlv1.RequestContext{TenantId: "default", AgentId: "agent-a", RequestId: "req-a"},
		PolicyType: "agent-runtime",
		PolicyJson: `{
			"policy_id":"local-policy",
			"version":7,
			"tenant_id":"default",
			"detection":{
				"policy_id":"local-detection",
				"version":2,
				"mode":"observe",
				"rulesets":[{"ref":"ruleset:endpoint-linux-builtin","version":"1","enabled":true}],
				"rule_overrides":[{"rule_id":"download_by_lolbin","enabled":false}]
			},
			"cloud_rules":[],
			"mode":"observe",
			"published":true
		}`,
	})
	if err != nil {
		t.Fatalf("ApplyPolicy() error = %v", err)
	}
	if ack.Status != "applied" || ack.PolicyId != "local-policy" || ack.PolicyVersion != 7 {
		t.Fatalf("ack = %+v", ack)
	}
	current, err := client.CurrentPolicy(context.Background(), &controlv1.CurrentPolicyRequest{})
	if err != nil {
		t.Fatalf("CurrentPolicy() error = %v", err)
	}
	if current.PolicyId != "local-policy" || current.RawJson == "" {
		t.Fatalf("current = %+v", current)
	}
}

func TestLocalControlApplyUploadPolicyContract(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "agent.sock")
	queue, err := spool.OpenWithLimit(filepath.Join(dir, "spool"), 4096)
	if err != nil {
		t.Fatal(err)
	}
	worker := &uploadworker.Worker{Queue: queue, Uploader: noopUploader{}}
	runner := &Runner{
		Config: config.Config{
			Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default"},
			Control: config.ControlConfig{SocketPath: socketPath},
			Manager: config.ManagerConfig{Address: "127.0.0.1:9443", Transport: "grpc"},
			Spool:   config.SpoolConfig{BatchSize: 10, FlushInterval: time.Second},
			Upload:  config.UploadConfig{RetryInitial: time.Second, RetryMax: 30 * time.Second, RequestTimeout: 10 * time.Second, MaxInflight: 1},
		},
		Sensor:     &healthOnlySensor{health: contract.Health{Backend: "fake", Running: true, Installed: true, PolicyLoaded: true}},
		capability: contract.Capability{Backend: "fake", SupportsExec: true},
	}
	rt := sensorruntime.New(runner.Sensor)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop, err := runner.startLocalControlServer(ctx, rt, queue, worker, time.Now())
	if err != nil {
		t.Fatalf("startLocalControlServer() error = %v", err)
	}
	defer stop()

	client := newUnixControlClient(t, socketPath)
	ack, err := client.ApplyPolicy(context.Background(), &controlv1.ApplyPolicyRequest{
		Context:    &controlv1.RequestContext{TenantId: "default", AgentId: "agent-a", RequestId: "req-upload"},
		PolicyType: "upload",
		PolicyJson: `{"transport":"grpc","endpoint":"manager:9443","batch_size":64,"flush_interval":"2s","retry_initial":"500ms","retry_max":"5s","request_timeout":"3s","max_inflight":2,"compression":"gzip","tls_profile":"mtls-prod"}`,
	})
	if err != nil {
		t.Fatalf("ApplyPolicy(upload) error = %v", err)
	}
	if ack.Status != "applied" || len(ack.Sections) != 1 || !ack.Sections[0].RequiresRestart {
		t.Fatalf("ack = %+v", ack)
	}
	if runner.Config.Manager.Transport != "grpc" || runner.Config.Manager.Address != "manager:9443" {
		t.Fatalf("manager config = %+v", runner.Config.Manager)
	}
	if runner.Config.Spool.BatchSize != 64 || runner.Config.Spool.FlushInterval != 2*time.Second {
		t.Fatalf("spool config = %+v", runner.Config.Spool)
	}
	if runner.Config.Upload.RetryInitial != 500*time.Millisecond || runner.Config.Upload.RetryMax != 5*time.Second || runner.Config.Upload.RequestTimeout != 3*time.Second || runner.Config.Upload.MaxInflight != 2 || runner.Config.Upload.Compression != "gzip" || runner.Config.Upload.TLSProfile != "mtls-prod" {
		t.Fatalf("upload config = %+v", runner.Config.Upload)
	}
}

func TestLocalControlApplyCollectionPolicyUpdatesSensorRuntime(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "agent.sock")
	queue, err := spool.OpenWithLimit(filepath.Join(dir, "spool"), 4096)
	if err != nil {
		t.Fatal(err)
	}
	worker := &uploadworker.Worker{Queue: queue, Uploader: noopUploader{}}
	sensor := &recordingCollectionSensor{healthOnlySensor: healthOnlySensor{health: contract.Health{Backend: "fake", Running: true, Installed: true, PolicyLoaded: true}}}
	runner := &Runner{
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
	stop, err := runner.startLocalControlServer(ctx, rt, queue, worker, time.Now())
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
			"metadata":{"id":"ioc:c2-port-feed","version":"2026.06.18.1"},
			"spec":{"value_type":"port","values":["443"]}
		}`,
	} {
		if _, err := client.ApplyContent(context.Background(), &controlv1.ApplyContentRequest{
			Context:       &controlv1.RequestContext{TenantId: "default", AgentId: "agent-a", RequestId: "req-content"},
			ContentJson:   contentJSON,
			AllowUnsigned: true,
		}); err != nil {
			t.Fatalf("ApplyContent() error = %v", err)
		}
	}
	ack, err := client.ApplyPolicy(context.Background(), &controlv1.ApplyPolicyRequest{
		Context:    &controlv1.RequestContext{TenantId: "default", AgentId: "agent-a", RequestId: "req-collection"},
		PolicyType: "collection",
		PolicyJson: `{
			"policy_id":"collection-a",
			"version":3,
			"behaviors":[
				{"id":"network.connect","selectors":{"socket":{"families":["AF_INET"],"addr_refs":["ioc:c2-ip-feed"],"port_refs":["ioc:c2-port-feed"]}}},
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
	queue, err := spool.OpenWithLimit(filepath.Join(dir, "spool"), 4096)
	if err != nil {
		t.Fatal(err)
	}
	worker := &uploadworker.Worker{Queue: queue, Uploader: noopUploader{}}
	sensor := &recordingCollectionSensor{healthOnlySensor: healthOnlySensor{health: contract.Health{Backend: "fake", Running: true, Installed: true, PolicyLoaded: true}}}
	runner := &Runner{
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
	stop, err := runner.startLocalControlServer(ctx, rt, queue, worker, time.Now())
	if err != nil {
		t.Fatalf("startLocalControlServer() error = %v", err)
	}
	defer stop()

	client := newUnixControlClient(t, socketPath)
	ack, err := client.ApplyPolicy(context.Background(), &controlv1.ApplyPolicyRequest{
		Context:    &controlv1.RequestContext{TenantId: "default", AgentId: "agent-a", RequestId: "req-network-binary"},
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
	queue, err := spool.OpenWithLimit(filepath.Join(dir, "spool"), 4096)
	if err != nil {
		t.Fatal(err)
	}
	worker := &uploadworker.Worker{Queue: queue, Uploader: noopUploader{}}
	runner := &Runner{
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
	stop, err := runner.startLocalControlServer(ctx, rt, queue, worker, time.Now())
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
	ack, err := client.ApplyContent(context.Background(), &controlv1.ApplyContentRequest{
		Context:       &controlv1.RequestContext{TenantId: "default", AgentId: "agent-a", RequestId: "req-content"},
		ContentJson:   contentJSON,
		AllowUnsigned: true,
	})
	if err != nil {
		t.Fatalf("ApplyContent() error = %v", err)
	}
	if ack.Status != "applied" || ack.PolicyId != "ioc:c2-ip-feed" {
		t.Fatalf("ack = %+v", ack)
	}
	list, err := client.ListContent(context.Background(), &controlv1.ListContentRequest{Kind: "iocpack"})
	if err != nil {
		t.Fatalf("ListContent() error = %v", err)
	}
	if len(list.GetRecords()) != 1 || list.GetRecords()[0].GetRef() != "ioc:c2-ip-feed" {
		t.Fatalf("list = %+v", list)
	}
	got, err := client.GetContent(context.Background(), &controlv1.GetContentRequest{Ref: "ioc:c2-ip-feed"})
	if err != nil {
		t.Fatalf("GetContent() error = %v", err)
	}
	if got.GetRecord().GetVersion() != "2026.06.17.1" || got.GetRecord().GetRawJson() == "" {
		t.Fatalf("get = %+v", got)
	}
}

func TestLocalControlContentApplyRebuildsDetection(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "agent.sock")
	queue, err := spool.OpenWithLimit(filepath.Join(dir, "spool"), 4096)
	if err != nil {
		t.Fatal(err)
	}
	worker := &uploadworker.Worker{Queue: queue, Uploader: noopUploader{}}
	runner := &Runner{
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
	stop, err := runner.startLocalControlServer(ctx, rt, queue, worker, time.Now())
	if err != nil {
		t.Fatalf("startLocalControlServer() error = %v", err)
	}
	defer stop()

	client := newUnixControlClient(t, socketPath)
	contentJSON := `{
		"api_version":"sysarmor.content/v1",
		"kind":"iocpack",
		"metadata":{"id":"ioc:c2-port-feed","version":"local-9443"},
		"spec":{"value_type":"port","values":["9443"]}
	}`
	if _, err := client.ApplyContent(context.Background(), &controlv1.ApplyContentRequest{
		Context:       &controlv1.RequestContext{TenantId: "default", AgentId: "agent-a", RequestId: "req-content-rebuild"},
		ContentJson:   contentJSON,
		AllowUnsigned: true,
	}); err != nil {
		t.Fatalf("ApplyContent() error = %v", err)
	}

	norm := normalize.New("agent-a", "host-a", nil)
	if _, err := runner.spoolEvent(queue, norm, runner.currentDetection(), sensorEventEnvelope("network.connect", 100, "/bin/bash", "", "10.66.0.99:9443")); err != nil {
		t.Fatal(err)
	}
	signalStream, err := client.WatchSignals(context.Background(), &controlv1.WatchSignalsRequest{IncludeRecent: true, Limit: 1, RuleId: "reverse_shell_pattern", Where: "endpoint"})
	if err != nil {
		t.Fatalf("WatchSignals() error = %v", err)
	}
	frame, err := signalStream.Recv()
	if err != nil {
		t.Fatalf("signal Recv() error = %v", err)
	}
	var gotVersion string
	for _, ref := range frame.GetSignal().GetIocRefs() {
		if ref.GetRef() == "ioc:c2-port-feed" {
			gotVersion = ref.GetVersion()
		}
	}
	if gotVersion != "local-9443" {
		t.Fatalf("signal ioc version = %q, want local-9443; signal=%+v", gotVersion, frame.GetSignal())
	}
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
	queue, err := spool.OpenWithLimit(filepath.Join(dir, "spool"), 4096)
	if err != nil {
		t.Fatal(err)
	}
	worker := &uploadworker.Worker{Queue: queue, Uploader: noopUploader{}}
	runner := &Runner{
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
	stop, err := runner.startLocalControlServer(ctx, rt, queue, worker, time.Now())
	if err != nil {
		t.Fatalf("startLocalControlServer() error = %v", err)
	}
	defer stop()

	norm := normalize.NewWithOptions("agent-a", "host-a", nil, normalize.Options{TenantID: "default", ScopeType: "host", Labels: map[string]string{"benchmark_run": "run-a"}})
	detector, _ := detection.New(policymodel.DefaultDetectionPolicy())
	if _, err := runner.spoolEvent(queue, norm, detector, sensorEventEnvelope("file.write", 100, "/usr/bin/curl", "/dev/shm/x.sh", "")); err != nil {
		t.Fatal(err)
	}

	client := newUnixControlClient(t, socketPath)
	eventStream, err := client.WatchEvents(context.Background(), &controlv1.WatchEventsRequest{IncludeRecent: true, Limit: 1, Behavior: "file.write"})
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
	eventGet, err := client.GetEvent(context.Background(), &controlv1.GetEventRequest{EventId: eventFrame.GetEvent().GetId()})
	if err != nil {
		t.Fatalf("GetEvent() error = %v", err)
	}
	if eventGet.GetFrame().GetEvent().GetId() != eventFrame.GetEvent().GetId() {
		t.Fatalf("GetEvent frame = %+v, want event id %q", eventGet.GetFrame(), eventFrame.GetEvent().GetId())
	}
	signalStream, err := client.WatchSignals(context.Background(), &controlv1.WatchSignalsRequest{IncludeRecent: true, Limit: 1, RuleId: "payload_dropped", Where: "endpoint"})
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
	labelStream, err := client.WatchSignals(context.Background(), &controlv1.WatchSignalsRequest{
		IncludeRecent: true,
		Limit:         1,
		Filter:        &controlv1.WatchFilter{Labels: map[string]string{"benchmark_run": "run-a"}},
	})
	if err != nil {
		t.Fatalf("WatchSignals(label) error = %v", err)
	}
	if _, err := labelStream.Recv(); err != nil {
		t.Fatalf("label signal Recv() error = %v", err)
	}
	afterStream, err := client.WatchSignals(context.Background(), &controlv1.WatchSignalsRequest{
		IncludeRecent: true,
		SnapshotOnly:  true,
		Filter:        &controlv1.WatchFilter{AfterSequence: signalFrame.GetSequence()},
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
	queue, err := spool.OpenWithLimit(filepath.Join(dir, "spool"), 16384)
	if err != nil {
		t.Fatal(err)
	}
	worker := &uploadworker.Worker{Queue: queue, Uploader: noopUploader{}}
	runner := &Runner{
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
	stop, err := runner.startLocalControlServer(ctx, rt, queue, worker, time.Now())
	if err != nil {
		t.Fatalf("startLocalControlServer() error = %v", err)
	}
	defer stop()

	client := newUnixControlClient(t, socketPath)
	contentAck, err := client.ApplyContent(context.Background(), &controlv1.ApplyContentRequest{
		Context:       &controlv1.RequestContext{TenantId: "default", AgentId: "agent-a"},
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
	policyAck, err := client.ApplyPolicy(context.Background(), &controlv1.ApplyPolicyRequest{
		Context:    &controlv1.RequestContext{TenantId: "default", AgentId: "agent-a"},
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
		if _, err := runner.spoolEvent(queue, norm, runner.currentDetection(), ev); err != nil {
			t.Fatal(err)
		}
	}

	signalStream, err := client.WatchSignals(context.Background(), &controlv1.WatchSignalsRequest{IncludeRecent: true, Limit: 1, RuleId: "cep_payload_lifecycle", Where: "endpoint"})
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
	health, err := client.Health(context.Background(), &controlv1.HealthRequest{Context: &controlv1.RequestContext{TenantId: "default", AgentId: "agent-a"}})
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
			]}
		}]}]}
	}`
}

type noopUploader struct{}

func (noopUploader) Upload(batch *dataplanev1.DataBatch) (*dataplanev1.DataAck, error) {
	return &dataplanev1.DataAck{Accepted: true, BatchId: batch.GetHeader().GetBatchId()}, nil
}

func newUnixControlClient(t *testing.T, socketPath string) controlv1.AgentControlServiceClient {
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
	return controlv1.NewAgentControlServiceClient(conn)
}
