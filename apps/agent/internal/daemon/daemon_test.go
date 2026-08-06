package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sysarmor/sysarmor-next-project/apps/agent/internal/config"
	agentcontent "github.com/sysarmor/sysarmor-next-project/apps/agent/internal/content"
	"github.com/sysarmor/sysarmor-next-project/apps/agent/internal/detection"
	"github.com/sysarmor/sysarmor-next-project/apps/agent/internal/detection/matcher"
	"github.com/sysarmor/sysarmor-next-project/apps/agent/internal/event/normalize"
	"github.com/sysarmor/sysarmor-next-project/apps/agent/internal/localstore"
	agentpolicy "github.com/sysarmor/sysarmor-next-project/apps/agent/internal/policy"
	"github.com/sysarmor/sysarmor-next-project/apps/agent/internal/remoteapi"
	"github.com/sysarmor/sysarmor-next-project/apps/agent/internal/sensors/linux/tetragon"
	sensorruntime "github.com/sysarmor/sysarmor-next-project/apps/agent/internal/sensors/runtime"
	"github.com/sysarmor/sysarmor-next-project/apps/agent/internal/tamper"
	"github.com/sysarmor/sysarmor-next-project/apps/agent/internal/telemetry"
	"github.com/sysarmor/sysarmor-next-project/apps/agent/internal/telemetry/dataappend"
	agenthealth "github.com/sysarmor/sysarmor-next-project/packages/contracts/health"
	controlplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/controlplane/v1"
	dataplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/dataplane/v1"
	eventv1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/event/v1"
	sensorv1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/sensor/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/signal/v1"
	policymodel "github.com/sysarmor/sysarmor-next-project/packages/policy"
	"github.com/sysarmor/sysarmor-next-project/packages/sensor-sdk/contract"
	"github.com/sysarmor/sysarmor-next-project/packages/tlsconfig"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestDetectionSuppressionConversion(t *testing.T) {
	got := detectionSuppression(agentcontent.RuntimeSuppression{
		Within: "5m",
		By:     []string{"process.stable_id", "file.path"},
	})
	if got.Within != 5*time.Minute || !slices.Equal(got.By, []string{"process.stable_id", "file.path"}) {
		t.Fatalf("suppression = %+v", got)
	}
}

func TestApplySupervisorHealthMarksSensorDegraded(t *testing.T) {
	sensor := contract.Health{Backend: "tetragon", Running: true, PolicyLoaded: true, RestartCount: 2}
	got := applySupervisorHealth(sensor, sensorruntime.SupervisorStatus{State: "degraded", LastError: "apply: unavailable", RestartCount: 3})
	if got.Running || got.LastError != "apply: unavailable" || got.RestartCount != 5 {
		t.Fatalf("sensor health = %+v", got)
	}
}

func TestResolveSensorHealthKeepsHealthAvailableWhenSensorFails(t *testing.T) {
	status := sensorruntime.SupervisorStatus{State: "degraded", LastError: "apply: unavailable", RestartCount: 2}
	got, err := resolveSensorHealth(contract.Health{}, errors.New("health unavailable"), &status, "tetragon")
	if err != nil || got.Backend != "tetragon" || got.Running || !strings.Contains(got.LastError, "health unavailable") {
		t.Fatalf("sensor health = %+v err=%v", got, err)
	}
}

func TestDetectionConditionTreeConversion(t *testing.T) {
	node := &agentcontent.RuntimeConditionNode{Any: []agentcontent.RuntimeConditionNode{
		{Condition: &agentcontent.RuntimeCondition{Field: "process.binary_name", Op: "in", Ref: "ctx:test-tools"}},
		{Not: &agentcontent.RuntimeConditionNode{Condition: &agentcontent.RuntimeCondition{Field: "socket.port", Op: "in", Values: []string{"80"}}}},
		{All: []agentcontent.RuntimeConditionNode{
			{Condition: &agentcontent.RuntimeCondition{Field: "behavior", Op: "eq", Value: "process.exec"}},
		}},
	}}
	got := detectionConditionNode(node)
	if got == nil || len(got.Any) != 3 || got.Any[0].Condition == nil || got.Any[1].Not == nil || len(got.Any[2].All) != 1 {
		t.Fatalf("condition tree = %+v", got)
	}
	if got.All != nil || got.Not != nil || got.Condition != nil {
		t.Fatalf("any node gained unrelated kinds: %+v", got)
	}
	if got.Any[0].All != nil || got.Any[0].Any != nil || got.Any[0].Not != nil {
		t.Fatalf("condition leaf gained unrelated kinds: %+v", got.Any[0])
	}
	if got.Any[0].Condition.Ref != "ctx:test-tools" || !slices.Equal(got.Any[1].Not.Condition.Values, []string{"80"}) {
		t.Fatalf("condition tree leaves = %+v", got)
	}
	if got.Any[2].Any != nil || got.Any[2].Not != nil || got.Any[2].Condition != nil {
		t.Fatalf("all node gained unrelated kinds: %+v", got.Any[2])
	}
}

func TestDetectionCorrelateConversion(t *testing.T) {
	got := detectionCorrelate(agentcontent.RuntimeCorrelate{
		Within: "2m", By: []string{"lineage_id"},
		Facts: []agentcontent.RuntimeFact{
			{ID: "change", Events: []string{"file.write", "file.chmod"}},
			{ID: "run", Event: "process.exec", Conditions: []agentcontent.RuntimeCondition{{Field: "process.binary", Op: "exists"}}},
		},
	})
	if got.Within != 2*time.Minute || got.WithinText != "2m" || !slices.Equal(got.By, []string{"lineage_id"}) || len(got.Facts) != 2 {
		t.Fatalf("correlate = %+v", got)
	}
	if !slices.Equal(got.Facts[0].Events, []string{"file.write", "file.chmod"}) || got.Facts[1].Event != "process.exec" || len(got.Facts[1].Conditions) != 1 {
		t.Fatalf("facts = %+v", got.Facts)
	}
}

const testCollectionPolicyJSON = `{"behaviors":["process.exec","process.exit","process.fork","file.read","file.write","network.connect"],"observe_only":true}
`

func appendEndpointEventForTest(t testing.TB, runner *AgentRuntime, bus *telemetry.Bus, norm *normalize.Normalizer, ev contract.EventEnvelope) *dataplanev1.DataBatch {
	t.Helper()
	batch, err := NewEndpointRuntime(runner, norm).ProcessEvent(ev)
	if err != nil {
		t.Fatal(err)
	}
	if bus != nil {
		bus.PublishBatch(batch)
	}
	return batch
}

func appendEndpointSignalsForTest(t testing.TB, runner *AgentRuntime, bus *telemetry.Bus, signals []*signalv1.Signal) *dataplanev1.DataBatch {
	t.Helper()
	batch, err := NewEndpointRuntime(runner, nil).ProcessSignals(signals)
	if err != nil {
		t.Fatal(err)
	}
	if bus != nil {
		bus.PublishBatch(batch)
	}
	return batch
}

func installTestDetection(t testing.TB, runner *AgentRuntime) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "..", "..", "..", "deployments", "agent", "content", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := runner.contentStore().Apply(string(raw), true, false); err != nil {
			t.Fatalf("load test content %s: %v", path, err)
		}
	}
	policy := policymodel.DefaultPolicy("default")
	policy.Detection = &policymodel.DetectionPolicy{
		PolicyID: "daemon-test-detection", Version: 1, Mode: "observe",
		RuleSets: []policymodel.RuleSetRef{{Ref: "ruleset:cep-endpoint"}},
	}
	if report, ok := runner.tryApplyRuntimePolicy(policy); !ok {
		t.Fatalf("install test detection: %+v", report)
	}
}

func TestAgentRuntimeSwitchesBatchIdentityAfterEnrollment(t *testing.T) {
	runner := &AgentRuntime{Config: config.Config{Agent: config.AgentConfig{ID: "device-a", HostID: "host-a", TenantID: "local"}}}
	runner.setRuntimeIdentity(runtimeIdentity{AgentID: "device-a", HostID: "host-a", TenantID: "local"})
	runner.telemetryBatcher = telemetry.NewBatcher(runner.newDataBatch, 10, time.Hour, 2)
	runner.telemetryBatcher.Add(&dataplanev1.DataBatch{Events: []*dataplanev1.EventFrame{{Sequence: 1}}})

	runner.applyEnrollmentIdentity(localstore.Enrollment{State: localstore.StateManaged, AgentID: "agent-a", TenantID: "tenant-a"})
	boundary := <-runner.telemetryBatcher.Batches()
	if boundary.GetHeader().GetAgentId() != "device-a" || boundary.GetHeader().GetTenantId() != "local" {
		t.Fatalf("boundary batch identity = %+v", boundary.GetHeader())
	}
	managed := runner.newDataBatch(time.Now())
	if managed.GetHeader().GetAgentId() != "agent-a" || managed.GetHeader().GetTenantId() != "tenant-a" {
		t.Fatalf("managed batch identity = %+v", managed.GetHeader())
	}
	managedContext := &controlplanev1.RequestContext{AgentId: "agent-a", TenantId: "tenant-a"}
	if err := runner.validateControlContext(managedContext); err != nil {
		t.Fatalf("managed control context rejected: %v", err)
	}
	ack := runner.bindControlAckIdentity(&controlplanev1.ControlAck{AgentId: "device-a", TenantId: "local"})
	if ack.GetAgentId() != "agent-a" || ack.GetTenantId() != "tenant-a" {
		t.Fatalf("managed ack identity = %+v", ack)
	}

	runner.applyEnrollmentIdentity(localstore.Enrollment{State: localstore.StateStandalone})
	standalone := runner.newDataBatch(time.Now())
	if standalone.GetHeader().GetAgentId() != "device-a" || standalone.GetHeader().GetTenantId() != "local" {
		t.Fatalf("standalone batch identity = %+v", standalone.GetHeader())
	}
}

func TestManagedSessionUsesEnrollmentIdentityWhilePolicyPending(t *testing.T) {
	sessionIdentity := runtimeIdentity{AgentID: "agent-a", HostID: "host-a", TenantID: "tenant-a"}
	health := bindHealthToSession(agenthealth.AgentHealth{
		AgentID: "device-a", HostID: "host-a", TenantID: "local",
		PendingPolicy: agenthealth.PendingPolicyStatus{Status: "pending", Source: "managed"},
	}, sessionIdentity)
	if health.AgentID != "agent-a" || health.TenantID != "tenant-a" || health.HostID != "host-a" {
		t.Fatalf("managed session health identity = %+v", health)
	}
	ack := bindControlAckToSession(&controlplanev1.ControlAck{
		AgentId: "device-a", TenantId: "local", Status: "pending",
	}, sessionIdentity)
	if ack.GetAgentId() != "agent-a" || ack.GetTenantId() != "tenant-a" {
		t.Fatalf("managed session ack identity = %+v", ack)
	}
}

func TestAgentRuntimeAppliesMatcherFeatureFlag(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SYSARMOR_TEST_MATCHER_STRATEGY", "")
	t.Cleanup(func() { matcher.SetDefaultStrategy(matcher.StrategyLinear) })
	cfg := config.Config{
		Agent:     config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
		Manager:   config.ManagerConfig{Address: "local", Transport: "local"},
		Runtime:   config.RuntimeConfig{FeatureFlags: config.RuntimeFeatureFlags{MatcherStrategy: "optimized"}},
		Sensor:    config.SensorConfig{Backend: "fake", Mode: "managed", PolicyPath: filepath.Join(dir, "collection.yaml"), ObserveOnly: true},
		Telemetry: config.TelemetryConfig{MaxBatchItems: 10, MaxBatchBytes: 256 << 10, FlushInterval: time.Second},
	}
	if err := os.WriteFile(cfg.Sensor.PolicyPath, []byte(testCollectionPolicyJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	runner, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if got := matcher.DefaultStrategy(); got != matcher.StrategyOptimized {
		t.Fatalf("matcher strategy = %q, want optimized", got)
	}
	if got := runner.detectionHealth().FeatureFlags.MatcherStrategy; got != "optimized" {
		t.Fatalf("health matcher strategy = %q, want optimized", got)
	}
}

func TestAgentRuntimeRejectsMissingDefaultContentManifest(t *testing.T) {
	cfg := config.Config{
		Manager: config.ManagerConfig{Address: "local", Transport: "local"},
		Runtime: config.RuntimeConfig{FeatureFlags: config.RuntimeFeatureFlags{MatcherStrategy: "linear"}},
		Sensor:  config.SensorConfig{Backend: "fake", Mode: "managed", ObserveOnly: true},
		Content: config.ContentConfig{DefaultPath: t.TempDir(), Path: t.TempDir()},
	}
	_, err := New(cfg)
	if err == nil || !strings.Contains(err.Error(), "default content manifest") {
		t.Fatalf("New() error = %v, want default content manifest error", err)
	}
}

func TestAgentRuntimeRejectsStartupDetectionWithoutRuleSet(t *testing.T) {
	runner := &AgentRuntime{content: agentcontent.NewStore()}
	err := runner.applyStartupDetection(policymodel.DefaultPolicy("default"))
	if err == nil || !strings.Contains(err.Error(), "explicit ruleset") {
		t.Fatalf("applyStartupDetection() error = %v, want explicit ruleset error", err)
	}
}

func TestDetectionStatusIncludesDefaultManifestVersion(t *testing.T) {
	runner := &AgentRuntime{}
	policy := policymodel.DefaultPolicy("default")
	policy.Detection = &policymodel.DetectionPolicy{PolicyID: "detection-test"}
	runner.setDetectionStatus(policy, detection.ApplyReport{Status: "applied"}, agentcontent.Snapshot{DefaultManifestVersion: "release-v1"})
	if got := runner.detectionHealth().DefaultManifestVersion; got != "release-v1" {
		t.Fatalf("detection manifest version = %q, want release-v1", got)
	}
}

func TestAgentRuntimeMatcherFeatureFlagTestOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SYSARMOR_TEST_MATCHER_STRATEGY", "optimized")
	t.Cleanup(func() { matcher.SetDefaultStrategy(matcher.StrategyLinear) })
	cfg := config.Config{
		Agent:     config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
		Manager:   config.ManagerConfig{Address: "local", Transport: "local"},
		Runtime:   config.RuntimeConfig{FeatureFlags: config.RuntimeFeatureFlags{MatcherStrategy: "linear"}},
		Sensor:    config.SensorConfig{Backend: "fake", Mode: "managed", PolicyPath: filepath.Join(dir, "collection.yaml"), ObserveOnly: true},
		Telemetry: config.TelemetryConfig{MaxBatchItems: 10, MaxBatchBytes: 256 << 10, FlushInterval: time.Second},
	}
	if err := os.WriteFile(cfg.Sensor.PolicyPath, []byte(testCollectionPolicyJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	runner, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if got := matcher.DefaultStrategy(); got != matcher.StrategyOptimized {
		t.Fatalf("matcher strategy = %q, want optimized", got)
	}
	if got := runner.detectionHealth().FeatureFlags.MatcherStrategy; got != "optimized" {
		t.Fatalf("health matcher strategy = %q, want optimized", got)
	}
}

func TestAgentRuntimeRejectsInvalidMatcherFeatureFlagOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SYSARMOR_TEST_MATCHER_STRATEGY", "auto")
	t.Cleanup(func() { matcher.SetDefaultStrategy(matcher.StrategyLinear) })
	cfg := config.Config{
		Agent:     config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
		Manager:   config.ManagerConfig{Address: "local", Transport: "local"},
		Runtime:   config.RuntimeConfig{FeatureFlags: config.RuntimeFeatureFlags{MatcherStrategy: "linear"}},
		Sensor:    config.SensorConfig{Backend: "fake", Mode: "managed", PolicyPath: filepath.Join(dir, "collection.yaml"), ObserveOnly: true},
		Telemetry: config.TelemetryConfig{MaxBatchItems: 10, MaxBatchBytes: 256 << 10, FlushInterval: time.Second},
	}
	if err := os.WriteFile(cfg.Sensor.PolicyPath, []byte(testCollectionPolicyJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := New(cfg); err == nil || !strings.Contains(err.Error(), "matcher_strategy") {
		t.Fatalf("New() error = %v, want matcher strategy validation error", err)
	}
}

func TestAgentRuntimeUploadsFakeSensorEvent(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(policyPath, []byte(testCollectionPolicyJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		Agent:     config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
		Manager:   config.ManagerConfig{Address: "local", Transport: "local"},
		Sensor:    config.SensorConfig{Backend: "fake", Mode: "managed", PolicyPath: policyPath, ObserveOnly: true},
		Telemetry: config.TelemetryConfig{MaxBatchItems: 10, MaxBatchBytes: 256 << 10, FlushInterval: time.Second},
		Local:     config.LocalConfig{Export: config.LocalExportConfig{RetryInitial: time.Second, RetryMax: time.Second, RequestTimeout: time.Second, MaxInflight: 1}},
		Health:    config.HealthConfig{Interval: time.Hour},
	}
	runner, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	installTestDetection(t, runner)
	var out bytes.Buffer
	runDaemonUntilUploadedBatch(t, runner, &out)
	got := out.String()
	for _, want := range []string{"agent daemon started", "sensor=fake", "agent daemon event"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output %q does not contain %q", got, want)
		}
	}
}

func TestStandaloneRuntimePersistsBeforeAcknowledging(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.json")
	if err := os.WriteFile(policyPath, []byte(testCollectionPolicyJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		Local:     config.LocalConfig{StatePath: filepath.Join(dir, "state"), Export: config.LocalExportConfig{RetryInitial: time.Second, RetryMax: time.Second, RequestTimeout: time.Second, MaxInflight: 1}, Storage: config.LocalStorageConfig{MaxBytes: 1 << 30, MinFreeBytes: 1, SegmentSize: 1 << 20, SignalMaxCount: 1000}},
		Sensor:    config.SensorConfig{Backend: "fake", Mode: "managed", PolicyPath: policyPath, ObserveOnly: true},
		Telemetry: config.TelemetryConfig{MaxBatchItems: 256, MaxBatchBytes: 256 << 10, FlushInterval: time.Second},
		Health:    config.HealthConfig{Interval: time.Second},
	}
	runner, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer runner.localStore.Close()
	if runner.Config.Agent.ID == "" || runner.Config.Agent.HostID == "" || runner.Config.Agent.TenantID != "local" {
		t.Fatalf("identity=%+v", runner.Config.Agent)
	}
	sender, err := runner.batchSender()
	if err != nil {
		t.Fatal(err)
	}
	batch := &dataplanev1.DataBatch{Header: &dataplanev1.BatchHeader{BatchId: "standalone-1", EventSeqStart: 1, EventSeqEnd: 1}}
	ack, err := sender.SendBatch(batch)
	if err != nil || !dataappend.AckCommitted(ack) {
		t.Fatalf("ack=%+v err=%v", ack, err)
	}
	batches, err := runner.localStore.ReadBatches(t.Context(), localstore.ReadOptions{Limit: 10})
	if err != nil || len(batches) != 1 || batches[0].Batch.GetHeader().GetBatchId() != "standalone-1" {
		t.Fatalf("batches=%+v err=%v", batches, err)
	}
}

func TestStandaloneRuntimeResumesPersistentSequences(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state")
	store := openSequenceStore(t, statePath)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	cfg := standaloneTestConfig(t, dir, statePath)
	runner, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer runner.localStore.Close()
	if runner.eventSeq != 41 || runner.signalSeq != 17 {
		t.Fatalf("eventSeq=%d signalSeq=%d, want 41/17", runner.eventSeq, runner.signalSeq)
	}
}

func TestEndpointSignalIDsContinueAcrossDetectionReplacement(t *testing.T) {
	runner := &AgentRuntime{signalSeq: 17}
	parent := &eventv1.CanonicalEvent{
		Id: "event-parent", Behavior: "process.exec",
		SubjectProc: &eventv1.ProcessRef{StableId: "node-parent", Binary: "/usr/bin/node"},
	}
	event := &eventv1.CanonicalEvent{
		Id: "event-a", Behavior: "process.exec", ParentStableId: "node-parent",
		SubjectProc: &eventv1.ProcessRef{StableId: "shell-child", Binary: "/bin/bash"},
	}
	policy := &policymodel.DetectionPolicy{RuleSets: []policymodel.RuleSetRef{{Ref: "ruleset:signal-sequence"}}}
	content := detection.ContentSnapshot{Rules: []detection.RuleSpec{{
		RuleID: "signal_sequence_test", RuleSetRef: "ruleset:signal-sequence", RuntimeType: "expr",
		Expr: detection.ExprSpec{Conditions: []detection.ConditionSpec{{Field: "process.binary_name", Op: "eq", Value: "bash"}}},
	}}}
	firstEngine, _ := detection.NewWithRuntime(policy, contract.CollectionIntent{}, content)
	secondEngine, _ := detection.NewWithRuntime(policy, contract.CollectionIntent{}, content)
	firstEngine.Process(parent)
	secondEngine.Process(parent)
	first := runner.dataBatchForEvent(event, firstEngine.Process(event)).GetSignals()[0]
	second := runner.dataBatchForEvent(event, secondEngine.Process(event)).GetSignals()[0]
	if first.GetSequence() != 18 || first.GetSignal().GetId() != "sig-00000000000000000018" {
		t.Fatalf("first signal=%+v", first)
	}
	if second.GetSequence() != 19 || second.GetSignal().GetId() != "sig-00000000000000000019" {
		t.Fatalf("second signal=%+v", second)
	}
}

func TestLocalStoreBatchSenderEnforcesCapacityBeforeAck(t *testing.T) {
	root := t.TempDir()
	store, err := localstore.Open(t.Context(), localstore.Options{
		RootDir: root, MaxBytes: 1, MinFreeBytes: 1, SegmentSize: 128,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	for sequence := uint64(1); sequence <= 3; sequence++ {
		batch := &dataplanev1.DataBatch{Header: &dataplanev1.BatchHeader{
			BatchId: fmt.Sprintf("seed-%d", sequence), EventSeqStart: sequence, EventSeqEnd: sequence,
		}}
		if _, err := store.AppendBatch(t.Context(), batch); err != nil {
			t.Fatal(err)
		}
		if err := store.Seal(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.SaveCheckpoint(t.Context(), localstore.Checkpoint{SegmentID: 3}); err != nil {
		t.Fatal(err)
	}

	sender := &localStoreBatchSender{store: store}
	ack, err := sender.SendBatch(&dataplanev1.DataBatch{Header: &dataplanev1.BatchHeader{
		BatchId: "new-batch", EventSeqStart: 4, EventSeqEnd: 4,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !ack.GetAccepted() {
		t.Fatalf("ack=%+v", ack)
	}
	if _, err := os.Stat(filepath.Join(root, "spool", "0000000000000001.seg")); !os.IsNotExist(err) {
		t.Fatalf("uploaded segment was not reclaimed, stat error=%v", err)
	}
}

func openSequenceStore(t *testing.T, statePath string) *localstore.Store {
	t.Helper()
	store, err := localstore.Open(t.Context(), localstore.Options{RootDir: statePath})
	if err != nil {
		t.Fatal(err)
	}
	signal := &dataplanev1.SignalFrame{Sequence: 17, ObservedAt: "2026-07-24T00:00:00Z", Signal: &signalv1.Signal{Id: "sig-00000000000000000017"}}
	batch := &dataplanev1.DataBatch{
		Header:  &dataplanev1.BatchHeader{BatchId: "seed", EventSeqStart: 41, EventSeqEnd: 41, SignalSeqStart: 17, SignalSeqEnd: 17},
		Signals: []*dataplanev1.SignalFrame{signal},
	}
	if _, err := store.AppendBatch(t.Context(), batch); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendSignals(t.Context(), []*dataplanev1.SignalFrame{signal}); err != nil {
		t.Fatal(err)
	}
	return store
}

func standaloneTestConfig(t *testing.T, dir, statePath string) config.Config {
	t.Helper()
	policyPath := filepath.Join(dir, "collection.json")
	if err := os.WriteFile(policyPath, []byte(testCollectionPolicyJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	return config.Config{
		Local:     config.LocalConfig{StatePath: statePath, Storage: config.LocalStorageConfig{MaxBytes: 1 << 30, MinFreeBytes: 1, SegmentSize: 1 << 20, SignalMaxCount: 1000}},
		Sensor:    config.SensorConfig{Backend: "fake", Mode: "managed", PolicyPath: policyPath, ObserveOnly: true},
		Telemetry: config.TelemetryConfig{MaxBatchItems: 256, MaxBatchBytes: 256 << 10, FlushInterval: time.Second},
	}
}

func TestAgentRuntimeUploadsConfiguredLabels(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(policyPath, []byte(testCollectionPolicyJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		Agent:     config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token", Labels: map[string]string{"scenario": "daemon-scenario"}},
		Manager:   config.ManagerConfig{Address: "local", Transport: "local"},
		Sensor:    config.SensorConfig{Backend: "fake", Mode: "managed", PolicyPath: policyPath, Scope: config.RuntimeScope{Type: "container", Selector: "abc123"}, ObserveOnly: true},
		Telemetry: config.TelemetryConfig{MaxBatchItems: 10, MaxBatchBytes: 256 << 10, FlushInterval: time.Second},
		Local:     config.LocalConfig{Export: config.LocalExportConfig{RetryInitial: time.Second, RetryMax: time.Second, RequestTimeout: time.Second, MaxInflight: 1}},
		Health:    config.HealthConfig{Interval: time.Hour},
	}
	runner, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	installTestDetection(t, runner)
	batch := runDaemonUntilUploadedBatch(t, runner, nil)
	if got := batch.GetEvents()[0].GetEvent().GetLabels()["scenario"]; got != "daemon-scenario" {
		t.Fatalf("event label scenario = %q", got)
	}
	for _, sig := range batch.GetSignals() {
		signal := sig.GetSignal()
		if got := signal.GetLabels()["scenario"]; got != "daemon-scenario" {
			t.Fatalf("signal %s label scenario = %q", signal.GetName(), got)
		}
	}
}

func TestAgentRuntimeShutdownFlushesTelemetryBestEffort(t *testing.T) {
	runner := &AgentRuntime{
		Config: config.Config{
			Agent:     config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
			Sensor:    config.SensorConfig{Scope: config.RuntimeScope{Type: "host"}},
			Telemetry: config.TelemetryConfig{MaxBatchItems: 10, MaxBatchBytes: 256 << 10, FlushInterval: time.Hour},
			Local:     config.LocalConfig{Export: config.LocalExportConfig{RequestTimeout: 200 * time.Millisecond, MaxInflight: 1}},
		},
		Sensor:     &healthOnlySensor{health: contract.Health{Backend: "fake", Running: true, Installed: true, PolicyLoaded: true}},
		capability: contract.Capability{Backend: "fake", SupportsHealth: true},
	}
	bus := telemetry.NewBus(16)
	batcher := telemetry.NewBatcher(runner.newDataBatch, 10, time.Hour, 4)
	uploader := newRecordingUploader()
	sender := &telemetry.Sender{Appender: uploader, Batcher: batcher}
	ctx, cancelSender := context.WithCancel(context.Background())
	defer cancelSender()
	go sender.Run(ctx)
	batcher.Add(&dataplanev1.DataBatch{Events: []*dataplanev1.EventFrame{{Event: &eventv1.CanonicalEvent{Id: "event-a", AgentId: "agent-a", HostId: "host-a"}}}})

	var out bytes.Buffer
	runner.Out = &out
	rt := sensorruntime.New(runner.Sensor)
	if err := runner.shutdownAndReport(context.Background(), rt, bus, batcher, sender, localHealthReporter{}, time.Now(), cancelSender, func() {}); err != nil {
		t.Fatalf("shutdownAndReport() error = %v", err)
	}
	stats := sender.Stats()
	if !stats.Drained || stats.SentBatches != 1 || len(uploader.ch) != 1 {
		t.Fatalf("sender stats = %+v uploaded=%d", stats, len(uploader.ch))
	}
	if got := out.String(); !strings.Contains(got, "drained=true") || !strings.Contains(got, "timeout=false") {
		t.Fatalf("shutdown output = %q", got)
	}
}

func TestAgentRuntimeRefreshesEndpointPolicy(t *testing.T) {
	cfg := config.Config{
		Agent: config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token", Labels: map[string]string{"scenario": "refresh-scenario"}},
	}
	runner := &AgentRuntime{Config: cfg}
	installTestDetection(t, runner)
	runner.applyRuntimePolicy(policymodel.DefaultPolicy("default"))
	norm := normalize.New(cfg.Agent.ID, cfg.Agent.HostID, nil)
	first := appendEndpointEventForTest(t, runner, nil, norm, sensorEventEnvelope("file.write", 100, "/usr/bin/curl", "/dev/shm/x.sh", ""))
	updated := runner.activePolicy()
	updated.PolicyID = "no-payload-after-refresh"
	updated.Version = 2
	disabled := false
	updated.Detection.RuleOverrides = append(updated.Detection.RuleOverrides, policymodel.RuleOverride{RuleID: "payload_dropped", Enabled: &disabled})
	runner.applyRuntimePolicy(updated)
	second := appendEndpointEventForTest(t, runner, nil, norm, sensorEventEnvelope("file.write", 101, "/usr/bin/curl", "/dev/shm/x.sh", ""))
	signalCounts := []int{len(first.GetSignals()), len(second.GetSignals())}
	if !containsInt(signalCounts, 1) || !containsInt(signalCounts, 0) {
		t.Fatalf("signal counts = %v, want one pre-refresh signal and one post-refresh suppressed signal", signalCounts)
	}
	if runner.activePolicy().PolicyID != "no-payload-after-refresh" || runner.activePolicy().Version != 2 {
		t.Fatalf("active policy = %+v", runner.activePolicy())
	}
}

func TestControlChannelKeepsLongLivedContract(t *testing.T) {
	server := &healthControlContractServer{received: make(chan *controlplanev1.HealthResponse, 1)}
	address := startControlContractServer(t, server)

	session := remoteapi.NewControlChannel(address, "", tlsconfig.ClientConfig{})
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
	if err := session.Send(ctx, &controlplanev1.ControlFrame{
		Type:      "health_report",
		RequestId: "long-health",
		Context: &controlplanev1.RequestContext{
			TenantId: "default",
			AgentId:  "agent-long-control",
			Scope:    &controlplanev1.Scope{Type: "container", Selector: "api"},
		},
		Health: &controlplanev1.HealthResponse{
			AgentId:    "agent-long-control",
			HostId:     "host-long-control",
			TenantId:   "default",
			Status:     "ok",
			Scope:      &controlplanev1.Scope{Type: "container", Selector: "api"},
			ObservedAt: time.Now().UTC().Format(time.RFC3339Nano),
			Capability: &controlplanev1.SensorCapability{Backend: "fake", Version: "long", SupportsHealth: true},
			Sensor:     &controlplanev1.SensorHealth{Backend: "fake", Running: true, EventsSeen: 7},
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
	got := <-server.received
	if got.GetCapability().GetVersion() != "long" || got.GetSensor().GetEventsSeen() != 7 {
		t.Fatalf("received long stream health = %+v", got)
	}
}

func TestControlChannelHelloStopsWhenSessionContextIsCanceled(t *testing.T) {
	received := make(chan struct{})
	grpcServer := grpc.NewServer()
	controlplanev1.RegisterAgentControlPlaneServiceServer(grpcServer, &blockingHelloControlServer{received: received})
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = grpcServer.Serve(lis) }()
	defer grpcServer.Stop()

	session := remoteapi.NewControlChannel(lis.Addr().String(), "", tlsconfig.ClientConfig{})
	defer session.Close()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		_, err := session.Hello(ctx, "default", "agent-a", "host", "")
		done <- err
	}()
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("server did not receive hello")
	}
	cancel()
	select {
	case err := <-done:
		if status.Code(err) != codes.Canceled {
			t.Fatalf("Hello() error=%v, want context canceled", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Hello() did not stop after session cancellation")
	}
}

type blockingHelloControlServer struct {
	controlplanev1.UnimplementedAgentControlPlaneServiceServer
	received chan struct{}
}

func (s *blockingHelloControlServer) Connect(stream controlplanev1.AgentControlPlaneService_ConnectServer) error {
	if _, err := stream.Recv(); err != nil {
		return err
	}
	close(s.received)
	<-stream.Context().Done()
	return stream.Context().Err()
}

func TestAgentRuntimeControlChannelProcessesPendingResponse(t *testing.T) {
	server := &responseControlContractServer{observed: make(chan responseControlObservation, 1)}
	address := startControlContractServer(t, server)

	runner := &AgentRuntime{
		Config: config.Config{
			Agent:   config.AgentConfig{ID: "agent-runner-long", HostID: "host-runner-long", TenantID: "default"},
			Manager: config.ManagerConfig{Address: address, Transport: "grpc"},
			Local:   config.LocalConfig{Export: config.LocalExportConfig{RetryInitial: 10 * time.Millisecond, RetryMax: 20 * time.Millisecond, RequestTimeout: time.Second, MaxInflight: 1}},
			Health:  config.HealthConfig{Interval: 10 * time.Millisecond},
		},
		Sensor: &healthOnlySensor{health: contract.Health{Backend: "fake", Running: true, PolicyLoaded: true, EventsSeen: 3}},
		capability: contract.Capability{
			Backend:        "fake",
			Version:        "long",
			SupportsHealth: true,
		},
	}
	rt := sensorruntime.New(runner.Sensor)
	batcher := telemetry.NewBatcher(runner.newDataBatch, 10, time.Hour, 16)
	sender := &telemetry.Sender{Appender: noopUploader{}, Batcher: batcher}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- NewTransportRuntime(runner, rt, batcher, sender, time.Now().UTC(), "host", "").RunControlChannel(ctx)
	}()
	var observation responseControlObservation
	select {
	case observation = <-server.observed:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("response ack not observed")
	}
	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("RunControlChannel() error = %v", err)
	}
	ack := observation.ack
	if ack.GetResponseId() != "resp-runner-long" || !ack.GetAccepted() || !ack.GetObserveOnly() || ack.GetExecuted() {
		t.Fatalf("ack = %+v", ack)
	}
	if observation.capability.GetSensor().GetVersion() != "long" {
		t.Fatalf("capability = %+v", observation.capability)
	}
}

func TestAgentRuntimeControlChannelAppliesContentUpdate(t *testing.T) {
	dir := t.TempDir()
	server := &contentUpdateControlServer{
		tenantID: "default",
		agentID:  "agent-content-update",
		contentJSON: `{
			"api_version":"sysarmor.content/v1",
			"kind":"iocpack",
			"metadata":{"id":"ioc:c2-control-port-feed","version":"control-9443"},
			"spec":{"value_type":"port","values":["9443"]}
		}`,
	}
	runner, done, cancel := runTestControlChannel(t, dir, server, "agent-content-update")
	defer cancel()

	ack := waitForControlAck(t, server, "content-update-1")
	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("RunControlChannel() error = %v", err)
	}
	if ack.GetStatus() != "applied" || ack.GetPolicyId() != "ioc:c2-control-port-feed" {
		t.Fatalf("content ack = %+v", ack)
	}
	if record, ok := runner.contentStore().Get("ioc:c2-control-port-feed"); !ok || record.Version != "control-9443" {
		t.Fatalf("content record = %+v ok=%t", record, ok)
	}

	batch := appendEndpointEventForTest(t, runner, nil, normalize.New("agent-content-update", "host-content-update", nil), sensorEventEnvelope("network.connect", 100, "/bin/bash", "", "10.66.0.99:9443"))
	if len(batch.GetSignals()) == 0 {
		t.Fatalf("signals after content update = none, want detection runtime to use updated content")
	}
}

func TestAgentRuntimeControlChannelRejectsBadContentUpdateWithoutReplacingDetection(t *testing.T) {
	dir := t.TempDir()
	server := &contentUpdateControlServer{
		tenantID: "default",
		agentID:  "agent-bad-content-update",
		contentJSON: `{
			"api_version":"sysarmor.content/v1",
			"kind":"rulepack",
			"metadata":{"id":"rulepack:bad-runtime","version":"bad-v1"},
			"spec":{"rulesets":[{"id":"ruleset:cep-endpoint","version":"v1","rules":[{
				"rule_id":"bad_runtime_rule",
				"version":1,
				"severity":"high",
				"runtime":{"type":"made_up_runtime"}
			}]}]}
		}`,
		policy: badRuntimeCandidatePolicy("default"),
	}
	runner, done, cancel := runTestControlChannel(t, dir, server, "agent-bad-content-update")
	defer cancel()

	ack := waitForControlAck(t, server, "content-update-1")
	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("RunControlChannel() error = %v", err)
	}
	if ack.GetStatus() != "rejected" || !strings.Contains(ack.GetMessage(), "detection rebuild failed") {
		t.Fatalf("bad content ack = %+v", ack)
	}
	if _, ok := runner.contentStore().Get("rulepack:bad-runtime"); ok {
		t.Fatalf("rejected content update was committed")
	}
	batch := appendEndpointEventForTest(t, runner, nil, normalize.New("agent-bad-content-update", "host-bad-content-update", nil), sensorEventEnvelope("file.write", 101, "/usr/bin/curl", "/dev/shm/kept-control.sh", ""))
	if len(batch.GetSignals()) != 1 || batch.GetSignals()[0].GetSignal().GetName() != "payload_dropped" {
		t.Fatalf("signals after rejected content update = %+v, want previous detection engine active", batch.GetSignals())
	}
}

func TestAgentRuntimeRunsWithTetragonJSONLSource(t *testing.T) {
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
		Agent:     config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
		Manager:   config.ManagerConfig{Address: "local", Transport: "local"},
		Sensor:    config.SensorConfig{Backend: "tetragon", Mode: "managed", Version: "test", PolicyPath: policyPath, EventSource: eventPath, ObserveOnly: true},
		Telemetry: config.TelemetryConfig{MaxBatchItems: 10, MaxBatchBytes: 256 << 10, FlushInterval: time.Second},
		Local:     config.LocalConfig{Export: config.LocalExportConfig{RetryInitial: time.Second, RetryMax: time.Second, RequestTimeout: time.Second, MaxInflight: 1}},
		Health:    config.HealthConfig{Interval: time.Hour},
	}
	runner, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	installTestDetection(t, runner)
	var out safeBuffer
	runDaemonUntilOutput(t, runner, &out, "agent daemon event")
	got := out.String()
	for _, want := range []string{"sensor=tetragon", "agent daemon event"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output %q does not contain %q", got, want)
		}
	}
}

func TestAgentRuntimeTetragonRequiresEventSource(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	if err := os.WriteFile(policyPath, []byte(testCollectionPolicyJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		Agent:     config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
		Manager:   config.ManagerConfig{Address: "local", Transport: "local"},
		Sensor:    config.SensorConfig{Backend: "tetragon", Mode: "managed", PolicyPath: policyPath, EventTransport: "tetra", ObserveOnly: true},
		Telemetry: config.TelemetryConfig{MaxBatchItems: 10, MaxBatchBytes: 256 << 10, FlushInterval: time.Second},
		Local:     config.LocalConfig{Export: config.LocalExportConfig{RetryInitial: time.Second, RetryMax: time.Second, RequestTimeout: time.Second, MaxInflight: 1}},
		Health:    config.HealthConfig{Interval: time.Hour},
	}
	runner, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = runner.Run(ctx, Options{})
	if err == nil {
		t.Fatal("Run() error = nil")
	}
}

func TestAgentRuntimeProcessesTamperSignalFromHealth(t *testing.T) {
	runner := &AgentRuntime{
		Config: config.Config{
			Agent:     config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
			Sensor:    config.SensorConfig{Backend: "fake", Mode: "managed", ObserveOnly: true, RestartWindow: time.Hour},
			Telemetry: config.TelemetryConfig{MaxBatchItems: 10, MaxBatchBytes: 256 << 10, FlushInterval: time.Hour},
			Local:     config.LocalConfig{Export: config.LocalExportConfig{RetryInitial: time.Hour, RetryMax: time.Hour, RequestTimeout: 5 * time.Millisecond, MaxInflight: 1}},
			Health:    config.HealthConfig{Interval: 5 * time.Millisecond},
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
		MaxRestarts:      uint64(runner.Config.Sensor.MaxRestarts),
		MaxParseErrors:   runner.Config.Sensor.MaxParseErrors,
		MaxDroppedEvents: runner.Config.Sensor.MaxDroppedEvents,
	})
	if sig == nil {
		t.Fatal("tamper Evaluate() = nil")
	}
	batch := appendEndpointSignalsForTest(t, runner, nil, []*signalv1.Signal{sig})
	sig = batch.GetSignals()[0].GetSignal()
	if sig.GetName() != tamper.SignalName || !sig.GetTerminal() || sig.GetWhere() != signalv1.SignalWhere_SIGNAL_WHERE_ENDPOINT {
		t.Fatalf("tamper signal = %+v", sig)
	}
}

func TestAgentRuntimeMarksHealthDegradedWhenParseThresholdExceeded(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(policyPath, []byte(testCollectionPolicyJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		Agent:     config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
		Manager:   config.ManagerConfig{Address: "local", Transport: "local"},
		Sensor:    config.SensorConfig{Backend: "fake", Mode: "managed", PolicyPath: policyPath, ObserveOnly: true, MaxParseErrors: 1, RestartWindow: time.Hour},
		Telemetry: config.TelemetryConfig{MaxBatchItems: 10, MaxBatchBytes: 256 << 10, FlushInterval: time.Hour},
		Local:     config.LocalConfig{Export: config.LocalExportConfig{RetryInitial: time.Hour, RetryMax: time.Hour, RequestTimeout: 5 * time.Millisecond, MaxInflight: 1}},
		Health:    config.HealthConfig{Interval: time.Hour},
	}
	runner := &AgentRuntime{
		Config: cfg,
		Sensor: &healthOnlySensor{health: contract.Health{
			Backend:      "tetragon",
			Installed:    true,
			PolicyLoaded: true,
			Running:      true,
			ParseErrors:  2,
		}},
	}
	runner.setRuntimeIdentity(runtimeIdentity{AgentID: "device-a", HostID: "host-a", TenantID: "local"})
	runner.applyEnrollmentIdentity(localstore.Enrollment{State: localstore.StateManaged, AgentID: "managed-agent", TenantID: "managed-tenant"})
	rt := sensorruntime.New(runner.Sensor)
	if _, err := rt.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Apply(context.Background(), contract.CollectionIntent{ObserveOnly: true}); err != nil {
		t.Fatal(err)
	}
	bus := telemetry.NewBus(1024)
	batcher := telemetry.NewBatcher(runner.newDataBatch, cfg.Telemetry.MaxBatchItems, cfg.Telemetry.FlushInterval, 16)
	sender := &telemetry.Sender{Appender: noopUploader{}, Batcher: batcher}
	health, err := runner.collectHealth(context.Background(), rt, bus, batcher, sender, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if health.Status != "degraded" || health.Sensor.ParseErrors != 2 {
		t.Fatalf("health = %+v", health)
	}
	if health.AgentID != "managed-agent" || health.TenantID != "managed-tenant" || health.HostID != "host-a" {
		t.Fatalf("managed health identity = %+v", health)
	}
}

func TestNewBatchSenderAcceptsConfiguredTimeout(t *testing.T) {
	for _, tc := range []struct {
		name      string
		transport string
	}{
		{name: "grpc", transport: "grpc"},
		{name: "local", transport: "local"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			up, err := newBatchSender("127.0.0.1:9443", tc.transport, 250*time.Millisecond, "dev-token", tlsconfig.ClientConfig{})
			if err != nil {
				t.Fatalf("newBatchSender() error = %v", err)
			}
			if up == nil {
				t.Fatal("newBatchSender() = nil")
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

type healthOnlySensor struct {
	health contract.Health
}

type contentUpdateControlServer struct {
	controlplanev1.UnimplementedAgentControlPlaneServiceServer
	tenantID    string
	agentID     string
	contentJSON string
	policy      policymodel.Policy
	acks        chan *controlplanev1.ControlAck
}

func (s *contentUpdateControlServer) Connect(stream controlplanev1.AgentControlPlaneService_ConnectServer) error {
	hello, err := stream.Recv()
	if err != nil {
		return err
	}
	if hello.GetType() != "hello" {
		return fmt.Errorf("first frame type = %q, want hello", hello.GetType())
	}
	tenantID := firstNonEmptyString(s.tenantID, "default")
	agentID := firstNonEmptyString(s.agentID, hello.GetContext().GetAgentId())
	policy := s.policy
	if policy.PolicyID == "" {
		policy = policymodel.DefaultPolicy(tenantID)
	}
	policy.TenantID = tenantID
	endpointPolicy := agentpolicy.EndpointPolicy{PolicyID: policy.PolicyID, Version: policy.Version,
		Collection: agentpolicy.CollectionPolicy{Behaviors: []string{"process.exec"}}, Detection: *policy.Detection,
		Telemetry: policymodel.TelemetryPolicy{MaxBatchItems: 256, MaxBatchBytes: 256 << 10, FlushInterval: "1s"}, Response: policy.Response}
	rawPolicy, _ := json.Marshal(endpointPolicy)
	for _, frame := range []*controlplanev1.ControlFrame{{
		Type:            "policy_update",
		RequestId:       hello.GetRequestId(),
		ContractVersion: 1,
		Sequence:        1,
		Context:         &controlplanev1.RequestContext{TenantId: tenantID, AgentId: agentID},
		PolicyUpdate: &controlplanev1.CurrentPolicyResponse{
			PolicyId: policy.PolicyID,
			Version:  policy.Version,
			TenantId: tenantID,
			Mode:     policy.Mode,
			RawJson:  string(rawPolicy),
		},
	}, {
		Type:            "resume",
		RequestId:       hello.GetRequestId(),
		ContractVersion: 1,
		Sequence:        2,
		Context:         &controlplanev1.RequestContext{TenantId: tenantID, AgentId: agentID},
		Resume:          &controlplanev1.ResumeCursor{TenantId: tenantID, AgentId: agentID},
	}, {
		Type:            "content_update",
		RequestId:       "content-update-1",
		ContractVersion: 1,
		Sequence:        3,
		Context:         &controlplanev1.RequestContext{TenantId: tenantID, AgentId: agentID, RequestId: "content-update-1"},
		ContentUpdate: &controlplanev1.ApplyContentRequest{
			Context:       &controlplanev1.RequestContext{TenantId: tenantID, AgentId: agentID, RequestId: "content-update-1"},
			ContentJson:   s.contentJSON,
			AllowUnsigned: true,
		},
	}} {
		if err := stream.Send(frame); err != nil {
			return err
		}
	}
	for {
		frame, err := stream.Recv()
		if err != nil {
			return err
		}
		if frame.GetType() == "ack" && frame.GetAck().GetRequestId() == "content-update-1" {
			s.acks <- frame.GetAck()
			return nil
		}
	}
}

func runTestControlChannel(t *testing.T, dir string, server *contentUpdateControlServer, agentID string) (*AgentRuntime, <-chan error, context.CancelFunc) {
	t.Helper()
	if server.acks == nil {
		server.acks = make(chan *controlplanev1.ControlAck, 1)
	}
	grpcServer := grpc.NewServer()
	controlplanev1.RegisterAgentControlPlaneServiceServer(grpcServer, server)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = grpcServer.Serve(lis)
	}()
	t.Cleanup(grpcServer.Stop)

	runner := &AgentRuntime{
		Config: config.Config{
			Agent:   config.AgentConfig{ID: agentID, HostID: "host-" + agentID, TenantID: "default"},
			Manager: config.ManagerConfig{Address: lis.Addr().String(), Transport: "grpc"},
			Local:   config.LocalConfig{Export: config.LocalExportConfig{RetryInitial: 10 * time.Millisecond, RetryMax: 20 * time.Millisecond, RequestTimeout: time.Second, MaxInflight: 1}},
			Health:  config.HealthConfig{Interval: time.Hour},
		},
		Sensor: &healthOnlySensor{health: contract.Health{Backend: "fake", Running: true, PolicyLoaded: true, EventsSeen: 3}},
		capability: contract.Capability{
			Backend:        "fake",
			Version:        "long",
			SupportsHealth: true,
		},
	}
	installTestDetection(t, runner)
	runner.applyRuntimePolicy(policymodel.DefaultPolicy("default"))
	rt := sensorruntime.New(runner.Sensor)
	batcher := telemetry.NewBatcher(runner.newDataBatch, 10, time.Hour, 16)
	sender := &telemetry.Sender{Appender: noopUploader{}, Batcher: batcher}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- NewTransportRuntime(runner, rt, batcher, sender, time.Now().UTC(), "host", "").RunControlChannel(ctx)
	}()
	return runner, done, cancel
}

func waitForControlAck(t *testing.T, server *contentUpdateControlServer, requestID string) *controlplanev1.ControlAck {
	t.Helper()
	select {
	case ack := <-server.acks:
		if ack.GetRequestId() != requestID {
			t.Fatalf("ack request_id = %q, want %q", ack.GetRequestId(), requestID)
		}
		return ack
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for control ack %q", requestID)
		return nil
	}
}

func badRuntimeCandidatePolicy(tenantID string) policymodel.Policy {
	policy := policymodel.DefaultPolicy(tenantID)
	policy.PolicyID = "bad-runtime-candidate"
	policy.Version = 2
	enabled := true
	policy.Detection.RuleSets = append(policy.Detection.RuleSets, policymodel.RuleSetRef{Ref: "ruleset:bad-runtime", Enabled: &enabled})
	return policy
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

func (s *capabilityErrorSensor) Apply(context.Context, contract.CollectionIntent) (contract.ApplyResult, error) {
	return contract.ApplyResult{State: contract.ApplyStateApplied}, nil
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

func (s *healthOnlySensor) Apply(context.Context, contract.CollectionIntent) (contract.ApplyResult, error) {
	return contract.ApplyResult{State: contract.ApplyStateApplied}, nil
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

func (s *eventSensor) Apply(context.Context, contract.CollectionIntent) (contract.ApplyResult, error) {
	s.health.PolicyLoaded = true
	return contract.ApplyResult{State: contract.ApplyStateApplied}, nil
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

func runDaemonUntilUploadedBatch(t *testing.T, runner *AgentRuntime, out *bytes.Buffer) *dataplanev1.DataBatch {
	t.Helper()
	uploader := newRecordingUploader()
	prev := newLocalBatchSender
	newLocalBatchSender = func() dataappend.BatchSender { return uploader }
	t.Cleanup(func() { newLocalBatchSender = prev })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		opts := Options{}
		if out != nil {
			opts.Out = out
		}
		errCh <- runner.Run(ctx, opts)
	}()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case batch := <-uploader.ch:
			cancel()
			if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
				t.Fatalf("Run() error = %v", err)
			}
			return batch
		case err := <-errCh:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Fatalf("Run() error = %v", err)
			}
			t.Fatalf("daemon exited before uploading a telemetry batch")
		case <-deadline:
			cancel()
			t.Fatalf("timed out waiting for uploaded telemetry batch")
		}
	}
}

func runDaemonUntilOutput(t *testing.T, runner *AgentRuntime, out interface {
	Write([]byte) (int, error)
	String() string
}, want string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- runner.Run(ctx, Options{Out: out})
	}()
	deadline := time.After(2 * time.Second)
	for {
		if strings.Contains(out.String(), want) {
			cancel()
			if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
				t.Fatalf("Run() error = %v", err)
			}
			return
		}
		select {
		case err := <-errCh:
			if strings.Contains(out.String(), want) {
				return
			}
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Fatalf("Run() error = %v", err)
			}
			t.Fatalf("daemon exited before output %q; got %q", want, out.String())
		case <-deadline:
			cancel()
			t.Fatalf("timed out waiting for output %q; got %q", want, out.String())
		case <-time.After(10 * time.Millisecond):
		}
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
