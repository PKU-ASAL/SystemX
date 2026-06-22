package daemon

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
	"time"

	dataplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/dataplane/v1"
	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	incidentv1 "github.com/sysarmor/sysarmor-next-project/api/proto/incident/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/config"
	agentcontent "github.com/sysarmor/sysarmor-next-project/internal/agent/content"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/databatchworker"
	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/policy"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/spool"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/tamper"
	controlmodel "github.com/sysarmor/sysarmor-next-project/internal/agentplane/model"
	"github.com/sysarmor/sysarmor-next-project/internal/endpoint/dataappend"
	"github.com/sysarmor/sysarmor-next-project/internal/endpoint/detection"
	"github.com/sysarmor/sysarmor-next-project/internal/endpoint/normalize"
	"github.com/sysarmor/sysarmor-next-project/internal/eventmodel"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
	responsemodel "github.com/sysarmor/sysarmor-next-project/internal/response"
	"github.com/sysarmor/sysarmor-next-project/internal/sensor/contract"
	"github.com/sysarmor/sysarmor-next-project/internal/sensor/fake"
	sensorruntime "github.com/sysarmor/sysarmor-next-project/internal/sensor/runtime"
	"github.com/sysarmor/sysarmor-next-project/internal/sensor/tetragon"
	"github.com/sysarmor/sysarmor-next-project/internal/tlsconfig"
	"google.golang.org/protobuf/encoding/protojson"
)

type localHealthReporter struct{}

func (localHealthReporter) Report(context.Context, agenthealth.AgentHealth) error {
	return nil
}

type localBatchAppender struct{}

func (localBatchAppender) AppendBatch(batch *dataplanev1.DataBatch) (*dataplanev1.DataAck, error) {
	if batch == nil {
		return &dataplanev1.DataAck{Accepted: true, Status: dataplanev1.DataAck_STATUS_ACCEPTED}, nil
	}
	return &dataplanev1.DataAck{
		Accepted:        true,
		Status:          dataplanev1.DataAck_STATUS_ACCEPTED,
		BatchId:         batch.GetHeader().GetBatchId(),
		CommittedCursor: batch.GetHeader().GetBatchId(),
	}, nil
}

type Options struct {
	Out io.Writer
}

type AgentRuntime struct {
	Config     config.Config
	Sensor     contract.Sensor
	Out        io.Writer
	capability contract.Capability
	mu         sync.RWMutex
	policy     policymodel.Policy
	detection  *detection.Engine
	collection contract.CollectionIntent
	content    *agentcontent.Store
	signalSeq  uint64
}

type healthReporter interface {
	Report(context.Context, agenthealth.AgentHealth) error
}

func New(cfg config.Config) (*AgentRuntime, error) {
	sensor, err := sensorFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	contentStore, err := agentcontent.NewStoreWithOptions(agentcontent.Options{Dir: cfg.Content.Path, TrustedKeys: parseTrustKeys(cfg.Content.TrustKeys)})
	if err != nil {
		return nil, err
	}
	return &AgentRuntime{Config: cfg, Sensor: sensor, content: contentStore}, nil
}

func NewAgentRuntime(cfg config.Config) (*AgentRuntime, error) {
	return New(cfg)
}

func (r *AgentRuntime) Run(ctx context.Context, opts Options) error {
	if opts.Out != nil {
		r.Out = opts.Out
	}
	startedAt := time.Now()
	reporter := r.healthReporter()
	failStartup := func(stage string, err error) error {
		r.reportStartupFailure(reporter, startedAt, stage, err)
		return err
	}
	rt := sensorruntime.New(r.Sensor)
	capability, err := rt.Probe(ctx)
	if err != nil {
		return failStartup("probe", err)
	}
	r.capability = capability
	intent, err := policy.LoadCollectionIntent(r.Config.Sensor.PolicyPath, r.Config.Sensor.ObserveOnly)
	if err != nil {
		return failStartup("policy", err)
	}
	scope, err := r.Config.Sensor.EffectiveScope()
	if err != nil {
		return err
	}
	scopeType := scope.Type
	scopeSelector := scope.Selector
	intent = policy.WithScope(intent, scopeType, scopeSelector)
	r.setCollectionIntent(r.withCollectionCapabilities(intent))
	if err := rt.Apply(ctx, intent); err != nil {
		return failStartup("apply", err)
	}
	longControl := r.Config.Manager.Transport == "grpc"
	effectivePolicy := policymodel.DefaultPolicy(r.Config.Agent.TenantID)
	r.setPolicy(effectivePolicy)
	events, err := rt.Subscribe(ctx)
	if err != nil {
		return failStartup("subscribe", err)
	}
	queue, err := spool.OpenWithLimit(r.Config.Spool.Path, r.Config.Spool.MaxBytes)
	if err != nil {
		return failStartup("spool", err)
	}
	agentSpool := NewAgentSpool(queue)
	worker, err := r.dataBatchWorker(queue)
	if err != nil {
		return failStartup("data_plane", err)
	}
	stopLocalControl, err := r.startLocalControlServer(ctx, rt, queue, worker, startedAt)
	if err != nil {
		return failStartup("local_control", err)
	}
	localRuntime := NewLocalRuntime(stopLocalControl)
	defer localRuntime.Close()
	dataPlaneCtx, cancelDataPlane := context.WithCancel(ctx)
	defer cancelDataPlane()
	norm := normalize.NewWithOptions(r.Config.Agent.ID, r.Config.Agent.HostID, nil, normalize.Options{
		TenantID:      r.Config.Agent.TenantID,
		ScopeType:     scopeType,
		ScopeSelector: scopeSelector,
		Labels:        r.runtimeLabels(scopeType, scopeSelector, capability.Backend),
	})
	endpointRuntime := NewEndpointRuntime(r, norm)
	transportRuntime := NewTransportRuntime(r, rt, agentSpool, worker, startedAt, scopeType, scopeSelector)
	go transportRuntime.RunDataFlow(dataPlaneCtx)
	go transportRuntime.RunControlFlow(dataPlaneCtx)
	r.applyRuntimePolicy(effectivePolicy)
	if r.Out != nil {
		fmt.Fprintf(r.Out, "agent daemon started: agent=%s host=%s tenant=%s sensor=%s version=%s behaviors=%d policy=%s version=%d mode=%s\n",
			r.Config.Agent.ID, r.Config.Agent.HostID, r.Config.Agent.TenantID, capability.Backend, capability.Version, len(intent.Behaviors), effectivePolicy.PolicyID, effectivePolicy.Version, effectivePolicy.Mode)
	}

	ticker := time.NewTicker(r.Config.Health.Interval)
	defer ticker.Stop()
	tamperDetector := &tamper.Detector{}
	stopped := false
	stopRuntime := func() {
		if stopped {
			return
		}
		stopped = true
		_ = rt.Stop(context.Background())
	}
	defer stopRuntime()

	for {
		select {
		case <-ctx.Done():
			drainErr := r.shutdownAndReport(context.Background(), rt, queue, worker, reporter, startedAt, cancelDataPlane, stopRuntime)
			if drainErr != nil && !errors.Is(drainErr, context.DeadlineExceeded) && !errors.Is(drainErr, context.Canceled) {
				return drainErr
			}
			return ctx.Err()
		case ev, ok := <-events:
			if !ok {
				drainErr := r.shutdownAndReport(context.Background(), rt, queue, worker, reporter, startedAt, cancelDataPlane, stopRuntime)
				if drainErr != nil && !errors.Is(drainErr, context.DeadlineExceeded) && !errors.Is(drainErr, context.Canceled) {
					return drainErr
				}
				return nil
			}
			var batchID string
			batchID, err := agentSpool.AppendEndpointEvent(endpointRuntime, ev)
			if err == nil && r.Out != nil {
				fmt.Fprintf(r.Out, "agent daemon event: behavior=%s raw_ref=%s spool_batch=%s\n", ev.SensorEvent.GetBehavior(), ev.RawRef, batchID)
			}
			if err != nil && !spool.IsBackpressure(err) {
				return err
			}
			if err != nil && spool.IsBackpressure(err) && r.Out != nil {
				stats, statErr := queue.Stats()
				if statErr != nil {
					return statErr
				}
				fmt.Fprintf(r.Out, "agent spool backpressure: dropped_batches=%d dropped_bytes=%d last_error=%q\n", stats.DroppedBatches, stats.DroppedBytes, stats.LastError)
			}
		case <-ticker.C:
			health, err := r.collectHealth(ctx, rt, queue, worker, startedAt)
			if err != nil {
				return err
			}
			if sig := tamperDetector.Evaluate(health, time.Now().UTC(), tamper.Options{
				MaxRestarts:        uint64(r.Config.Sensor.MaxRestarts),
				MaxParseErrors:     r.Config.Sensor.MaxParseErrors,
				MaxDroppedEvents:   r.Config.Sensor.MaxDroppedEvents,
				NoEventGracePeriod: tamperNoEventGracePeriod(r.Config.Sensor.RestartWindow, r.Config.Health.Interval),
			}); sig != nil {
				var batchID string
				batchID, err = agentSpool.AppendEndpointSignals(endpointRuntime, []*signalv1.Signal{sig})
				if err != nil && !spool.IsBackpressure(err) {
					return err
				}
				if r.Out != nil {
					fmt.Fprintf(r.Out, "agent tamper signal: name=%s reason=%q spool_batch=%s\n", sig.GetName(), sig.GetEvidence().GetSummary(), batchID)
				}
			}
			if !longControl {
				if err := reporter.Report(ctx, health); err != nil && r.Out != nil {
					fmt.Fprintf(r.Out, "agent health report error: %v\n", err)
				}
			}
			if r.Out != nil {
				fmt.Fprintf(r.Out, "agent health: sensor=%s running=%t policy_loaded=%t events_seen=%d queued_batches=%d queued_bytes=%d dropped_batches=%d last_spool_error=%q last_data_plane_error=%q\n",
					health.Sensor.Backend, health.Sensor.Running, health.Sensor.PolicyLoaded, health.Sensor.EventsSeen, health.Queue.QueuedBatches, health.Queue.QueuedBytes, health.Queue.DroppedBatches, health.Queue.LastError, health.DataPlane.LastError)
			}
		}
	}
}

func tamperNoEventGracePeriod(restartWindow, healthInterval time.Duration) time.Duration {
	grace := restartWindow
	if grace < 30*time.Second {
		grace = 30 * time.Second
	}
	if intervalGrace := healthInterval * 10; intervalGrace > grace {
		grace = intervalGrace
	}
	return grace
}

func (r *AgentRuntime) shutdownAndReport(ctx context.Context, rt sensorruntime.Runtime, queue *spool.Queue, worker *databatchworker.Worker, reporter healthReporter, startedAt time.Time, cancelDataPlane func(), stopRuntime func()) error {
	cancelDataPlane()
	stopRuntime()
	var drainErr error
	var stats databatchworker.Stats
	drainCtx, cancel := context.WithTimeout(context.Background(), shutdownDrainTimeout(r.Config))
	stats, drainErr = worker.DrainOnce(drainCtx)
	cancel()
	if r.Out != nil {
		fmt.Fprintf(r.Out, "agent shutdown drain: appended=%d remaining=%d last_error=%q\n", stats.AppendedBatches, stats.RemainingBatches, stats.LastError)
	}
	finalHealth, healthErr := r.collectShutdownHealth(ctx, rt, queue, worker, startedAt)
	if healthErr == nil {
		if err := reporter.Report(context.Background(), finalHealth); err != nil && r.Out != nil {
			fmt.Fprintf(r.Out, "agent final health report error: %v\n", err)
		}
		if r.Out != nil {
			fmt.Fprintf(r.Out, "agent final health: sensor=%s running=%t policy_loaded=%t status=%s queued_batches=%d last_data_plane_error=%q\n",
				finalHealth.Sensor.Backend, finalHealth.Sensor.Running, finalHealth.Sensor.PolicyLoaded, finalHealth.Status, finalHealth.Queue.QueuedBatches, finalHealth.DataPlane.LastError)
		}
	}
	return drainErr
}

func (r *AgentRuntime) reportStartupFailure(reporter healthReporter, startedAt time.Time, stage string, startupErr error) {
	if reporter == nil || startupErr == nil {
		return
	}
	health := agenthealth.AgentHealth{
		AgentID:       r.Config.Agent.ID,
		HostID:        r.Config.Agent.HostID,
		TenantID:      r.Config.Agent.TenantID,
		Scope:         r.runtimeScope(),
		Status:        "degraded",
		PolicyID:      r.activePolicy().PolicyID,
		PolicyVersion: r.activePolicy().Version,
		PolicyMode:    r.policyMode(),
		UptimeSeconds: int64(time.Since(startedAt).Seconds()),
		ObservedAt:    time.Now().UTC(),
		Sensor: agenthealth.SensorHealth{
			Backend:      r.Config.Sensor.Backend,
			Installed:    false,
			Running:      false,
			PolicyLoaded: false,
			LastError:    fmt.Sprintf("%s: %v", stage, startupErr),
		},
		Capability: r.runtimeCapability(),
		Queue:      agenthealth.QueueHealth{},
		DataPlane:  agenthealth.DataPlaneHealth{},
	}
	if err := reporter.Report(context.Background(), health); err != nil && r.Out != nil {
		fmt.Fprintf(r.Out, "agent startup health report error: %v\n", err)
	}
	if r.Out != nil {
		fmt.Fprintf(r.Out, "agent startup failure: stage=%s error=%q\n", stage, startupErr)
	}
}

func (r *AgentRuntime) healthReporter() healthReporter {
	return localHealthReporter{}
}

func (r *AgentRuntime) collectShutdownHealth(ctx context.Context, rt sensorruntime.Runtime, queue *spool.Queue, worker *databatchworker.Worker, startedAt time.Time) (agenthealth.AgentHealth, error) {
	health, err := r.collectHealth(ctx, rt, queue, worker, startedAt)
	if err != nil {
		return agenthealth.AgentHealth{}, err
	}
	health.Sensor.Running = false
	if health.Status == "ok" {
		health.Status = "degraded"
	}
	health.ObservedAt = time.Now().UTC()
	return health, nil
}

func shutdownDrainTimeout(cfg config.Config) time.Duration {
	timeout := cfg.DataPlane.RequestTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if cfg.DataPlane.RetryInitial > timeout {
		timeout = cfg.DataPlane.RetryInitial
	}
	return timeout
}

func (r *AgentRuntime) collectHealth(ctx context.Context, rt sensorruntime.Runtime, queue *spool.Queue, worker *databatchworker.Worker, startedAt time.Time) (agenthealth.AgentHealth, error) {
	sensor, err := rt.Health(ctx)
	if err != nil {
		return agenthealth.AgentHealth{}, err
	}
	queueStats, err := queue.Stats()
	if err != nil {
		return agenthealth.AgentHealth{}, err
	}
	dataPlaneStats, err := worker.Stats()
	if err != nil {
		return agenthealth.AgentHealth{}, err
	}
	status := "ok"
	if !sensor.Running || sensor.LastError != "" || queueStats.LastError != "" || dataPlaneStats.LastError != "" {
		status = "degraded"
	}
	if queueStats.BackpressureCount > 0 || queueStats.DroppedBatches > 0 || queueStats.DroppedBytes > 0 {
		status = "degraded"
	}
	if r.Config.Sensor.MaxParseErrors > 0 && sensor.ParseErrors > r.Config.Sensor.MaxParseErrors {
		status = "degraded"
	}
	if r.Config.Sensor.MaxDroppedEvents > 0 && sensor.EventsDropped > r.Config.Sensor.MaxDroppedEvents {
		status = "degraded"
	}
	cepMetrics := r.currentDetection().Metrics()
	cepDegraded := cepMetrics.EvictedCEPGroups > 0 || cepMetrics.DroppedEventRefs > 0 || cepMetrics.CEPEvalErrors > 0
	if cepDegraded {
		status = "degraded"
	}
	now := time.Now().UTC()
	return agenthealth.AgentHealth{
		AgentID:       r.Config.Agent.ID,
		HostID:        r.Config.Agent.HostID,
		TenantID:      r.Config.Agent.TenantID,
		Scope:         r.runtimeScope(),
		Status:        status,
		PolicyID:      r.activePolicy().PolicyID,
		PolicyVersion: r.activePolicy().Version,
		PolicyMode:    r.policyMode(),
		UptimeSeconds: int64(time.Since(startedAt).Seconds()),
		ObservedAt:    now,
		Sensor: agenthealth.SensorHealth{
			Backend:        sensor.Backend,
			Installed:      sensor.Installed,
			Running:        sensor.Running,
			Version:        sensor.Version,
			PolicyLoaded:   sensor.PolicyLoaded,
			EventsSeen:     sensor.EventsSeen,
			EventsDropped:  sensor.EventsDropped,
			ParseErrors:    sensor.ParseErrors,
			RestartCount:   sensor.RestartCount,
			LastEventAt:    sensor.LastEventAt,
			LastExitReason: sensor.LastExitReason,
			LastError:      sensor.LastError,
		},
		Capability: r.runtimeCapability(),
		Queue: agenthealth.QueueHealth{
			QueuedBatches:     queueStats.QueuedBatches,
			QueuedBytes:       queueStats.QueuedBytes,
			MaxBytes:          queueStats.MaxBytes,
			BackpressureCount: queueStats.BackpressureCount,
			DroppedBatches:    queueStats.DroppedBatches,
			DroppedBytes:      queueStats.DroppedBytes,
			LastError:         queueStats.LastError,
		},
		WAL: agenthealth.WALHealth{
			QueuedBatches:     queueStats.QueuedBatches,
			QueuedBytes:       queueStats.QueuedBytes,
			MaxBytes:          queueStats.MaxBytes,
			OldestBatchID:     queueStats.OldestBatchID,
			NewestBatchID:     queueStats.NewestBatchID,
			LastAckedBatchID:  queueStats.LastAckedBatchID,
			WatchSubscribers:  queueStats.WatchSubscribers,
			BackpressureCount: queueStats.BackpressureCount,
			DroppedBatches:    queueStats.DroppedBatches,
			DroppedBytes:      queueStats.DroppedBytes,
			LastError:         queueStats.LastError,
		},
		DataPlane: agenthealth.DataPlaneHealth{
			AppendedBatches:  dataPlaneStats.AppendedBatches,
			RemainingBatches: dataPlaneStats.RemainingBatches,
			RemainingBytes:   dataPlaneStats.RemainingBytes,
			LastError:        dataPlaneStats.LastError,
		},
		CEP: agenthealth.CEPHealth{
			ActiveGroups:     cepMetrics.ActiveCEPGroups,
			EvictedGroups:    cepMetrics.EvictedCEPGroups,
			ExpiredGroups:    cepMetrics.ExpiredCEPGroups,
			DroppedEventRefs: cepMetrics.DroppedEventRefs,
			EvalErrors:       cepMetrics.CEPEvalErrors,
			EmittedSignals:   cepMetrics.EmittedSignals,
			Degraded:         cepDegraded,
		},
	}, nil
}

func (r *AgentRuntime) runtimeCapability() agenthealth.SensorCapability {
	collection := make([]agenthealth.CollectionBehaviorCapability, 0, len(r.capability.Collection))
	for _, behavior := range r.capability.Collection {
		collection = append(collection, agenthealth.CollectionBehaviorCapability{
			Behavior:             behavior.Behavior,
			SensorMapping:        behavior.SensorMapping,
			Fields:               append([]string(nil), behavior.Fields...),
			PushdownSelectors:    append([]string(nil), behavior.PushdownSelectors...),
			AgentSideSelectors:   append([]string(nil), behavior.AgentSideSelectors...),
			UnsupportedSelectors: append([]string(nil), behavior.UnsupportedSelectors...),
		})
	}
	return agenthealth.SensorCapability{
		Backend:         r.capability.Backend,
		Version:         r.capability.Version,
		SupportsExec:    r.capability.SupportsExec,
		SupportsConnect: r.capability.SupportsConnect,
		SupportsFile:    r.capability.SupportsFile,
		SupportsEnforce: r.capability.SupportsEnforce,
		SupportsHealth:  r.capability.SupportsHealth,
		KernelRelease:   r.capability.KernelRelease,
		BTFAvailable:    r.capability.BTFAvailable,
		BPFFSAvailable:  r.capability.BPFFSAvailable,
		Collection:      collection,
	}
}

func (r *AgentRuntime) runtimeScope() agenthealth.RuntimeScope {
	scope, err := r.Config.Sensor.EffectiveScope()
	if err != nil {
		return agenthealth.RuntimeScope{Type: "host"}
	}
	return agenthealth.RuntimeScope{Type: scope.Type, Selector: scope.Selector}
}

func (r *AgentRuntime) policyMode() string {
	if mode := r.activePolicy().Mode; mode != "" {
		return mode
	}
	if r.Config.Sensor.ObserveOnly {
		return "observe"
	}
	return "enforce"
}

func (r *AgentRuntime) activePolicy() policymodel.Policy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.policy.PolicyID != "" {
		return r.policy
	}
	return policymodel.DefaultPolicy(r.Config.Agent.TenantID)
}

func (r *AgentRuntime) setPolicy(policy policymodel.Policy) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.policy = policy
}

func (r *AgentRuntime) currentDetection() *detection.Engine {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.detection != nil {
		return r.detection
	}
	engine, _ := detection.NewWithRuntimeLimits(policymodel.DefaultDetectionPolicy(), r.collection, detection.ContentSnapshot{}, r.detectionLimits())
	return engine
}

func (r *AgentRuntime) setDetection(engine *detection.Engine) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.detection = engine
}

func (r *AgentRuntime) setCollectionIntent(intent contract.CollectionIntent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.collection = r.withCollectionCapabilities(intent)
}

func (r *AgentRuntime) currentCollectionIntent() contract.CollectionIntent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.collection
}

func (r *AgentRuntime) withCollectionCapabilities(intent contract.CollectionIntent) contract.CollectionIntent {
	if len(intent.Capabilities) == 0 && len(r.capability.Collection) > 0 {
		intent.Capabilities = append([]contract.CollectionBehaviorCapability(nil), r.capability.Collection...)
	}
	return intent
}

func (r *AgentRuntime) applyRuntimePolicy(policy policymodel.Policy) {
	policy = policymodel.Normalize(policy)
	engine, _ := detection.NewWithRuntimeLimits(policy.Detection, r.currentCollectionIntent(), r.detectionContentSnapshot(), r.detectionLimits())
	r.setPolicy(policy)
	r.setDetection(engine)
}

func (r *AgentRuntime) rebuildDetection() detection.ApplyReport {
	policy := policymodel.Normalize(r.activePolicy())
	engine, report := detection.NewWithRuntimeLimits(policy.Detection, r.currentCollectionIntent(), r.detectionContentSnapshot(), r.detectionLimits())
	r.setDetection(engine)
	return report
}

func (r *AgentRuntime) detectionLimits() detection.EngineLimits {
	return detection.EngineLimits{
		MaxCEPGroups: r.Config.Resource.MaxActiveCEPGroups,
		MaxCEPRefs:   r.Config.Resource.MaxEventRefsPerSignal,
	}
}

func samePolicyRuntime(a, b policymodel.Policy) bool {
	return a.PolicyID == b.PolicyID &&
		a.Version == b.Version &&
		a.Mode == b.Mode &&
		reflect.DeepEqual(a.Detection, b.Detection)
}

func (r *AgentRuntime) dataBatchWorker(queue *spool.Queue) (*databatchworker.Worker, error) {
	up, err := newBatchAppender(r.Config.Manager.Address, r.Config.Manager.Transport, r.Config.DataPlane.RequestTimeout, r.Config.Agent.Token, r.managerTLS())
	if err != nil {
		return nil, err
	}
	worker := &databatchworker.Worker{
		Queue:    queue,
		Uploader: up,
		Backoff:  databatchworker.Backoff{Initial: r.Config.DataPlane.RetryInitial, Max: r.Config.DataPlane.RetryMax},
	}
	return worker, nil
}

func (r *AgentRuntime) managerTLS() tlsconfig.ClientConfig {
	return tlsconfig.ClientConfig{
		CAFile:     r.Config.Manager.TLSCA,
		CertFile:   r.Config.Manager.TLSCert,
		KeyFile:    r.Config.Manager.TLSKey,
		ServerName: r.Config.Manager.TLSServerName,
		Insecure:   r.Config.Manager.TLSInsecure,
	}
}

func newBatchAppender(manager, transport string, timeout time.Duration, token string, tlsCfg tlsconfig.ClientConfig) (dataappend.BatchAppender, error) {
	switch transport {
	case "grpc":
		return dataappend.NewGRPCAppenderWithTLS(manager, timeout, token, tlsCfg), nil
	case "local":
		return localBatchAppender{}, nil
	default:
		return nil, fmt.Errorf("unknown transport %q", transport)
	}
}

func parseTrustKeys(raw string) map[string]ed25519.PublicKey {
	out := map[string]ed25519.PublicKey{}
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		keyID, encoded, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		keyID = strings.TrimSpace(keyID)
		encoded = strings.TrimSpace(encoded)
		if keyID == "" || encoded == "" {
			continue
		}
		if data, err := base64.StdEncoding.DecodeString(encoded); err == nil && len(data) == ed25519.PublicKeySize {
			out[keyID] = ed25519.PublicKey(data)
		}
	}
	return out
}

func (r *AgentRuntime) runtimeLabels(scopeType, scopeSelector, sensorRuntime string) map[string]string {
	labels := cloneStringMap(r.Config.Agent.Labels)
	if r.Config.Agent.Scenario != "" {
		labels["scenario"] = r.Config.Agent.Scenario
	}
	if sensorRuntime != "" {
		labels["sensor_runtime"] = sensorRuntime
	}
	if scopeType != "" {
		labels["scope_type"] = scopeType
	}
	if scopeSelector != "" {
		labels["scope_selector"] = scopeSelector
	}
	if len(labels) == 0 {
		return nil
	}
	return labels
}

func (r *AgentRuntime) policyLabels() map[string]string {
	policy := r.activePolicy()
	labels := map[string]string{}
	if policy.PolicyID != "" {
		labels["policy_id"] = policy.PolicyID
	}
	if policy.Version > 0 {
		labels["policy_version"] = fmt.Sprintf("%d", policy.Version)
	}
	if policy.Mode != "" {
		labels["policy_mode"] = policy.Mode
	}
	if len(labels) == 0 {
		return nil
	}
	return labels
}

func cloneStringMap(in map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range in {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func mergeLabels(base, extra map[string]string) map[string]string {
	out := cloneStringMap(base)
	for key, value := range extra {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (r *AgentRuntime) dataBatchForEvent(event *eventv1.CanonicalEvent, signals []*signalv1.Signal) *dataplanev1.DataBatch {
	now := time.Now().UTC()
	batch := r.newDataBatch(now)
	if event != nil {
		batch.Events = append(batch.Events, &dataplanev1.EventFrame{
			Sequence:   event.GetSeq(),
			ObservedAt: now.Format(time.RFC3339Nano),
			Event:      event,
		})
	}
	for _, sig := range signals {
		batch.Signals = append(batch.Signals, &dataplanev1.SignalFrame{
			Sequence:   r.nextSignalSequence(),
			ObservedAt: now.Format(time.RFC3339Nano),
			Signal:     sig,
		})
	}
	return batch
}

func (r *AgentRuntime) dataBatchForSignals(signals []*signalv1.Signal) *dataplanev1.DataBatch {
	now := time.Now().UTC()
	batch := r.newDataBatch(now)
	for _, sig := range signals {
		batch.Signals = append(batch.Signals, &dataplanev1.SignalFrame{
			Sequence:   r.nextSignalSequence(),
			ObservedAt: now.Format(time.RFC3339Nano),
			Signal:     sig,
		})
	}
	return batch
}

func (r *AgentRuntime) newDataBatch(now time.Time) *dataplanev1.DataBatch {
	policy := r.activePolicy()
	labels := cloneStringMap(r.Config.Agent.Labels)
	if labels == nil {
		labels = map[string]string{}
	}
	for key, value := range r.policyLabels() {
		labels[key] = value
	}
	if r.Config.Agent.Scenario != "" {
		labels["scenario"] = r.Config.Agent.Scenario
	}
	if len(labels) == 0 {
		labels = nil
	}
	return &dataplanev1.DataBatch{
		Header: &dataplanev1.BatchHeader{
			TenantId:          r.Config.Agent.TenantID,
			AgentId:           r.Config.Agent.ID,
			HostId:            r.Config.Agent.HostID,
			PolicyId:          policy.PolicyID,
			PolicyVersion:     policy.Version,
			PolicyMode:        policy.Mode,
			CreatedAtUnixNano: now.UnixNano(),
			Labels:            labels,
		},
	}
}

func (r *AgentRuntime) nextSignalSequence() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.signalSeq++
	return r.signalSeq
}

func (r *AgentRuntime) contentStore() *agentcontent.Store {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.content == nil {
		r.content = agentcontent.NewStore()
	}
	return r.content
}

func (r *AgentRuntime) detectionContentSnapshot() detection.ContentSnapshot {
	snapshot := r.contentStore().Snapshot()
	out := detection.ContentSnapshot{
		ContextRefs: make(map[string]detection.ContentRef),
		IOCRefs:     make(map[string]detection.ContentRef),
	}
	for ref, set := range snapshot.ContextSets {
		out.ContextRefs[ref] = detection.ContentRef{
			Ref:     set.Ref,
			Version: set.Version,
			Digest:  set.Digest,
			Values:  append([]string(nil), set.Values...),
		}
	}
	for ref, set := range snapshot.IOCPacks {
		out.IOCRefs[ref] = detection.ContentRef{
			Ref:     set.Ref,
			Version: set.Version,
			Digest:  set.Digest,
			Values:  append([]string(nil), set.Values...),
		}
	}
	for _, rule := range snapshot.Rules {
		out.Rules = append(out.Rules, detection.RuleSpec{
			RuleID:            rule.RuleID,
			Version:           rule.Version,
			RuleSetRef:        rule.RuleSetRef,
			Severity:          rule.Severity,
			Runtime:           rule.RuntimeEntry,
			RuntimeType:       rule.RuntimeType,
			Expr:              detectionExpr(rule.Expr),
			Sequence:          detectionSequence(rule.Sequence),
			RequiredEvents:    detectionRequiredEvents(rule.RequiredEvents),
			RequiredBehaviors: requiredBehaviors(rule.RequiredEvents),
			ContextRefs:       append([]string(nil), rule.ContextRefs...),
			IOCRefs:           append([]string(nil), rule.IOCRefs...),
			ResponseIntent: &policymodel.ResponseIntentRef{
				Action:     rule.ResponseIntent.Action,
				Confidence: rule.ResponseIntent.Confidence,
				Reason:     rule.ResponseIntent.Reason,
			},
		})
	}
	return out
}

func detectionRequiredEvents(events []agentcontent.RequiredEvent) []detection.RequiredEventSpec {
	out := make([]detection.RequiredEventSpec, 0, len(events))
	for _, event := range events {
		out = append(out, detection.RequiredEventSpec{
			Behavior: event.Behavior,
			Fields:   append([]string(nil), event.Fields...),
		})
	}
	return out
}

func detectionExpr(expr agentcontent.RuntimeExpr) detection.ExprSpec {
	out := detection.ExprSpec{Conditions: make([]detection.ConditionSpec, 0, len(expr.Conditions))}
	for _, cond := range expr.Conditions {
		out.Conditions = append(out.Conditions, detectionCondition(cond))
	}
	return out
}

func detectionSequence(seq agentcontent.RuntimeSequence) detection.SequenceSpec {
	within, _ := time.ParseDuration(seq.Within)
	out := detection.SequenceSpec{
		Within: within,
		By:     append([]string(nil), seq.By...),
		Steps:  make([]detection.StepSpec, 0, len(seq.Steps)),
	}
	for _, step := range seq.Steps {
		behavior := step.Behavior
		if behavior == "" {
			behavior = step.Event
		}
		next := detection.StepSpec{
			ID:         step.ID,
			Behavior:   eventmodel.NormalizeBehavior(behavior).String(),
			Conditions: make([]detection.ConditionSpec, 0, len(step.Conditions)),
		}
		for _, cond := range step.Conditions {
			next.Conditions = append(next.Conditions, detectionCondition(cond))
		}
		out.Steps = append(out.Steps, next)
	}
	return out
}

func detectionCondition(cond agentcontent.RuntimeCondition) detection.ConditionSpec {
	return detection.ConditionSpec{
		Field:     cond.Field,
		Op:        cond.Op,
		Value:     cond.Value,
		Values:    append([]string(nil), cond.Values...),
		Ref:       cond.Ref,
		Step:      cond.Step,
		StepField: cond.StepField,
	}
}

func requiredBehaviors(events []agentcontent.RequiredEvent) []string {
	seen := map[string]bool{}
	var out []string
	for _, event := range events {
		behavior := eventmodel.NormalizeBehavior(event.Behavior).String()
		if behavior == "" || seen[behavior] {
			continue
		}
		seen[behavior] = true
		out = append(out, behavior)
	}
	return out
}

func (r *AgentRuntime) executeResponse(ctx context.Context, cmd responsemodel.Command) responsemodel.Ack {
	cmd = responsemodel.NormalizeCommand(cmd)
	if cmd.Mode == "" {
		cmd.Mode = responsemodel.DefaultMode
	}
	if cmd.Mode != "enforce" {
		return responsemodel.Ack{
			ResponseID:  cmd.ResponseID,
			TenantID:    r.Config.Agent.TenantID,
			AgentID:     r.Config.Agent.ID,
			Accepted:    true,
			ObserveOnly: true,
			Executed:    false,
			Message:     fmt.Sprintf("observe-only response accepted; would execute action=%s target=%s", cmd.Action, cmd.Target),
			ObservedAt:  time.Now().UTC(),
		}
	}
	ack, err := r.Sensor.Enforce(ctx, responsemodel.ToEnforcement(cmd))
	if err != nil {
		ack = contract.UnsupportedAck(responsemodel.ToEnforcement(cmd), err.Error())
	}
	out := responsemodel.FromEnforcementAck(cmd, ack)
	out.TenantID = r.Config.Agent.TenantID
	out.AgentID = r.Config.Agent.ID
	return out
}

func (r *AgentRuntime) collectEvidencePullback(req controlmodel.EvidencePullbackRequest) controlmodel.EvidencePullbackResult {
	result := controlmodel.EvidencePullbackResult{
		RequestID:  req.RequestID,
		TenantID:   r.Config.Agent.TenantID,
		AgentID:    r.Config.Agent.ID,
		OK:         true,
		Message:    "collected target evidence",
		ObservedAt: time.Now().UTC(),
	}
	if req.Target == "" {
		result.Message = "collected no target evidence"
		return result
	}
	evidence := &incidentv1.EvidenceSubgraph{
		Nodes: []*incidentv1.GraphNode{{
			Id:    req.Target,
			Kind:  evidenceKindFromTarget(req.Target),
			Label: req.Target,
		}},
	}
	data, err := protojson.Marshal(evidence)
	if err != nil {
		result.OK = false
		result.Message = fmt.Sprintf("encode evidence: %v", err)
		return result
	}
	result.Evidence = json.RawMessage(data)
	return result
}

func evidenceKindFromTarget(target string) string {
	if idx := strings.Index(target, ":"); idx > 0 {
		return target[:idx]
	}
	return "entity"
}

func sensorFromConfig(cfg config.Config) (contract.Sensor, error) {
	switch cfg.Sensor.Backend {
	case "fake":
		count := cfg.Sensor.FakeStartupEvents
		if count == 0 {
			count = 1
		}
		return fake.NewWithStartupEvents(count), nil
	case "tetragon":
		restart, err := tetragonRestartPolicy(cfg.Sensor)
		if err != nil {
			return nil, err
		}
		backend := tetragon.NewBackendWithOptions(cfg.Sensor.PolicyPath, cfg.Sensor.EventSource, cfg.Sensor.Version, tetragon.BundleConfig{
			BundleDir:    cfg.Sensor.BundleDir,
			InstallDir:   cfg.Sensor.InstallDir,
			TetraPath:    cfg.Sensor.TetraPath,
			TetragonPath: cfg.Sensor.TetragonPath,
		}, restart)
		backend.EventTransport = cfg.Sensor.EventTransport
		backend.ServerAddress = cfg.Sensor.ServerAddress
		backend.CgroupRate = cfg.Sensor.CgroupRate
		backend.PprofAddress = cfg.Sensor.PprofAddress
		backend.GopsAddress = cfg.Sensor.GopsAddress
		backend.ProcessCacheSize = cfg.Sensor.ProcessCacheSize
		backend.DataCacheSize = cfg.Sensor.DataCacheSize
		backend.EventQueueSize = cfg.Sensor.EventQueueSize
		backend.RBQueueSize = cfg.Sensor.RBQueueSize
		scope, err := cfg.Sensor.EffectiveScope()
		if err != nil {
			return nil, err
		}
		backend.ScopeType = scope.Type
		backend.ScopeSelector = scope.Selector
		backend.ContainerIDPrefix = cfg.Sensor.ContainerIDPrefix
		backend.BTFPath = cfg.Sensor.BTFPath
		backend.BPFFSPath = cfg.Sensor.BPFFSPath
		backend.RequireBTF = cfg.Sensor.RequireBTF
		backend.RequireBPFFS = cfg.Sensor.RequireBPFFS
		return backend, nil
	default:
		return nil, fmt.Errorf("unsupported sensor backend %q", cfg.Sensor.Backend)
	}
}

func tetragonRestartPolicy(cfg config.SensorConfig) (tetragon.ProcessRestartPolicy, error) {
	switch cfg.Restart {
	case "", "never", "off", "false":
		return tetragon.ProcessRestartPolicy{}, nil
	case "always":
		return tetragon.ProcessRestartPolicy{
			Enabled:     true,
			MaxRestarts: cfg.MaxRestarts,
			Delay:       cfg.RestartWindow,
		}, nil
	default:
		return tetragon.ProcessRestartPolicy{}, fmt.Errorf("unsupported sensor.restart %q", cfg.Restart)
	}
}
