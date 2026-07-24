package tetragon

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	sensorv1 "github.com/sysarmor/sysarmor-next-project/api/proto/sensor/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/eventmodel"
	"github.com/sysarmor/sysarmor-next-project/internal/sensors/contract"
)

const runtimeTracingPolicyName = "sysarmor-runtime-collection"

type Backend struct {
	PolicyPath               string
	EventSource              string
	EventTransport           string
	ServerAddress            string
	Version                  string
	Bundle                   BundleConfig
	Restart                  ProcessRestartPolicy
	CgroupRate               string
	PprofAddress             string
	GopsAddress              string
	ProcessCacheSize         int
	DataCacheSize            int
	EventQueueSize           int
	RBQueueSize              string
	ScopeType                string
	ScopeSelector            string
	ContainerIDPrefix        string
	namespaceSelfContainerID string
	BTFPath                  string
	BPFFSPath                string
	RequireBTF               bool
	RequireBPFFS             bool

	mu                   sync.Mutex
	intent               contract.CollectionIntent
	policyLoaded         bool
	running              bool
	installed            bool
	eventsSeen           uint64
	eventsDropped        uint64
	parseErrors          uint64
	lastEventAt          time.Time
	lastError            string
	runtimePolicyApplied bool

	sensorSupervisor ProcessSupervisor
	eventSupervisor  ProcessSupervisor
}

type ProcessRestartPolicy struct {
	Enabled     bool
	MaxRestarts int
	Delay       time.Duration
}

func NewBackend(policyPath, eventSource, version string) *Backend {
	return NewBackendWithBundle(policyPath, eventSource, version, BundleConfig{})
}

func NewBackendWithBundle(policyPath, eventSource, version string, bundle BundleConfig) *Backend {
	if version == "" {
		version = "unknown"
	}
	return &Backend{PolicyPath: policyPath, EventSource: eventSource, Version: version, Bundle: bundle}
}

func NewBackendWithOptions(policyPath, eventSource, version string, bundle BundleConfig, restart ProcessRestartPolicy) *Backend {
	backend := NewBackendWithBundle(policyPath, eventSource, version, bundle)
	backend.Restart = restart
	return backend
}

func (b *Backend) Capability(context.Context) (contract.Capability, error) {
	if b.Bundle.BundleDir != "" {
		verified, err := b.prepareBundle()
		if err != nil {
			b.setError(err)
			return contract.Capability{}, err
		}
		b.mu.Lock()
		b.installed = true
		b.lastError = ""
		if b.Version == "unknown" {
			b.Version = verified.Version
		}
		b.mu.Unlock()
	} else if b.EventSource != "" {
		b.mu.Lock()
		b.installed = true
		b.mu.Unlock()
	} else if b.Bundle.TetraPath != "" || b.Bundle.TetragonPath != "" {
		b.mu.Lock()
		b.installed = true
		b.mu.Unlock()
	}
	kernelRelease, btfAvailable, bpffsAvailable, err := b.probeHostCapabilities()
	if err != nil {
		b.setError(err)
		return contract.Capability{}, err
	}
	if err := b.verifyConfiguredExecutables(); err != nil {
		b.setError(err)
		return contract.Capability{}, err
	}
	return contract.Capability{
		Backend:         "tetragon",
		Version:         b.Version,
		SupportsExec:    true,
		SupportsConnect: true,
		SupportsFile:    true,
		SupportsHealth:  true,
		KernelRelease:   kernelRelease,
		BTFAvailable:    btfAvailable,
		BPFFSAvailable:  bpffsAvailable,
		Collection:      CollectionCapabilities(),
	}, nil
}

func CollectionCapabilities() []contract.CollectionBehaviorCapability {
	commonProcess := []string{"event.id", "event.behavior", "lineage_id", "process.stable_id", "process.binary", "process.argv", "process.uid", "parent.stable_id", "scope.type", "scope.selector", "container.id", "cgroup"}
	agentSideScope := []string{"scope.container", "scope.cgroup", "scope.pod"}
	with := func(base []string, fields ...string) []string {
		out := append([]string(nil), base...)
		out = append(out, fields...)
		return out
	}
	pushdown := func(selectors ...string) []string {
		return append([]string{"scope.namespace"}, selectors...)
	}
	return []contract.CollectionBehaviorCapability{
		{
			Behavior:           eventmodel.BehaviorProcessExec.String(),
			SensorMapping:      "tetragon:process_exec/security_bprm_creds_from_file",
			Fields:             commonProcess,
			PushdownSelectors:  pushdown("process.binary_prefix"),
			AgentSideSelectors: agentSideScope,
		},
		{
			Behavior:           eventmodel.BehaviorProcessFork.String(),
			SensorMapping:      "tetragon:process_exec.clone",
			Fields:             commonProcess,
			PushdownSelectors:  pushdown("process.binary_prefix"),
			AgentSideSelectors: agentSideScope,
		},
		{
			Behavior:           eventmodel.BehaviorProcessExit.String(),
			SensorMapping:      "tetragon:process_exit/do_exit",
			Fields:             commonProcess,
			PushdownSelectors:  pushdown(),
			AgentSideSelectors: agentSideScope,
		},
		{
			Behavior:           eventmodel.BehaviorNetworkConnect.String(),
			SensorMapping:      "tetragon:kprobe/security_socket_connect",
			Fields:             with(commonProcess, "socket", "socket.addr", "socket.port", "object.socket_addr"),
			PushdownSelectors:  pushdown("process.binary_prefix", "socket.family", "socket.addr", "socket.port"),
			AgentSideSelectors: agentSideScope,
		},
		{
			Behavior:           eventmodel.BehaviorFileOpen.String(),
			SensorMapping:      "tetragon:kprobe/security_file_permission",
			Fields:             with(commonProcess, "file.path", "object.file_path"),
			PushdownSelectors:  pushdown("process.binary_prefix", "file.path.prefix"),
			AgentSideSelectors: agentSideScope,
		},
		{
			Behavior:           eventmodel.BehaviorFileRead.String(),
			SensorMapping:      "tetragon:kprobe/security_file_permission",
			Fields:             with(commonProcess, "file.path", "object.file_path"),
			PushdownSelectors:  pushdown("process.binary_prefix", "file.path.prefix", "file.access"),
			AgentSideSelectors: agentSideScope,
		},
		{
			Behavior:           eventmodel.BehaviorFileWrite.String(),
			SensorMapping:      "tetragon:process_exec.inferred_write/security_file_permission",
			Fields:             with(commonProcess, "file.path", "object.file_path"),
			PushdownSelectors:  pushdown("process.binary_prefix", "file.path.prefix", "file.access"),
			AgentSideSelectors: agentSideScope,
		},
		{
			Behavior:           eventmodel.BehaviorFileChmod.String(),
			SensorMapping:      "tetragon:process_exec.inferred_chmod",
			Fields:             with(commonProcess, "file.path", "object.file_path"),
			PushdownSelectors:  pushdown("process.binary_prefix", "file.path.prefix", "file.access"),
			AgentSideSelectors: agentSideScope,
		},
	}
}

func CompileReport(intent contract.CollectionIntent) contract.CollectionCompileReport {
	report := contract.CollectionCompileReport{
		Status:  "ok",
		Backend: "tetragon",
	}
	if needsTracingPolicy(intent) {
		sum := sha256.Sum256(buildTracingPolicy(intent))
		report.GeneratedPolicyHash = hex.EncodeToString(sum[:])
	}
	for _, behavior := range intent.Behaviors {
		behavior = eventmodel.NormalizeBehavior(behavior).String()
		if behavior == "" {
			continue
		}
		report.BehaviorMappings = append(report.BehaviorMappings, contract.CollectionBehaviorMap{
			Behavior: behavior,
			Backend:  "tetragon",
			Hook:     hookForBehavior(behavior),
		})
		filter := behaviorFilter(intent, behavior)
		report.PushedDownSelectors = append(report.PushedDownSelectors, pushedDownSelectorsForFilter(filter)...)
		report.AgentSideSelectors = append(report.AgentSideSelectors, agentSideSelectorsForFilter(filter)...)
		for _, scopeReport := range scopeSelectorReports(intent, behavior) {
			if scopeReport.Status == "pushed_down" {
				report.PushedDownSelectors = append(report.PushedDownSelectors, scopeReport)
				continue
			}
			report.AgentSideSelectors = append(report.AgentSideSelectors, scopeReport)
		}
		report.UnsupportedSelectors = append(report.UnsupportedSelectors, unsupportedSelectorsForFilter(filter)...)
		if !hasPushdownSelectors(intent, behavior, filter) {
			report.Warnings = append(report.Warnings, fmt.Sprintf("behavior %s has no pushdown selectors; kernel BPF filter will pass all events of this type, resulting in high event volume", behavior))
		}
	}
	if len(report.AgentSideSelectors) > 0 {
		report.Warnings = append(report.Warnings, "some selectors are enforced agent-side after Tetragon emission; behavior is correct but event volume can be higher")
	}
	if len(report.UnsupportedSelectors) > 0 {
		report.Status = "unsupported"
		report.Warnings = append(report.Warnings, "collection policy contains selectors that are not compiled to Tetragon or enforced agent-side")
	}
	return report
}

func hookForBehavior(behavior string) string {
	switch behavior {
	case eventmodel.BehaviorProcessExec.String(), eventmodel.BehaviorProcessFork.String():
		return "security_bprm_creds_from_file"
	case eventmodel.BehaviorProcessExit.String():
		return "do_exit"
	case eventmodel.BehaviorNetworkConnect.String():
		return "security_socket_connect"
	case eventmodel.BehaviorFileOpen.String(), eventmodel.BehaviorFileRead.String(), eventmodel.BehaviorFileWrite.String(), eventmodel.BehaviorFileChmod.String():
		return "security_file_permission"
	default:
		return ""
	}
}

func hasPushdownSelectors(intent contract.CollectionIntent, behavior string, filter contract.CollectionBehaviorFilter) bool {
	if hasNamespacePushdown(intent) {
		return true
	}
	switch eventmodel.NormalizeBehavior(behavior) {
	case eventmodel.BehaviorProcessExec, eventmodel.BehaviorProcessFork:
		return len(filter.BinaryPrefixes) > 0
	case eventmodel.BehaviorNetworkConnect:
		return len(filter.BinaryPrefixes) > 0 || len(filter.SocketAddrs) > 0 || len(filter.SocketPorts) > 0
	case eventmodel.BehaviorFileOpen, eventmodel.BehaviorFileRead, eventmodel.BehaviorFileWrite, eventmodel.BehaviorFileChmod:
		return len(filter.BinaryPrefixes) > 0 || len(filter.FilePrefixes) > 0
	default:
		return true
	}
}

func pushedDownSelectorsForFilter(filter contract.CollectionBehaviorFilter) []contract.CollectionSelectorReport {
	var out []contract.CollectionSelectorReport
	behavior := eventmodel.NormalizeBehavior(filter.Behavior).String()
	add := func(selector, mapping string) {
		out = append(out, contract.CollectionSelectorReport{Behavior: behavior, Selector: selector, Status: "pushed_down", Location: "tetragon", Mapping: mapping})
	}
	switch behavior {
	case eventmodel.BehaviorProcessExec.String(), eventmodel.BehaviorProcessFork.String():
		if len(filter.BinaryPrefixes) > 0 {
			add("process.binary_prefix", "selectors.matchArgs[index=1,operator=Prefix]")
		}
	case eventmodel.BehaviorNetworkConnect.String():
		if len(filter.BinaryPrefixes) > 0 {
			add("process.binary_prefix", "selectors.matchBinaries[operator=Prefix]")
		}
		if len(filter.SocketFamilies) > 0 {
			add("socket.family", "selectors.matchArgs[index=1,operator=Family]")
		}
	case eventmodel.BehaviorFileOpen.String(), eventmodel.BehaviorFileRead.String(), eventmodel.BehaviorFileWrite.String(), eventmodel.BehaviorFileChmod.String():
		if len(filter.BinaryPrefixes) > 0 {
			add("process.binary_prefix", "selectors.matchBinaries[operator=Prefix]")
		}
		if len(filter.FilePrefixes) > 0 {
			add("file.path.prefix", "selectors.matchArgs[index=0,operator=Prefix]")
		}
		switch behavior {
		case eventmodel.BehaviorFileRead.String():
			add("file.access", "selectors.matchArgs[index=1,operator=Equal,value=4/MAY_READ]")
		case eventmodel.BehaviorFileWrite.String(), eventmodel.BehaviorFileChmod.String():
			add("file.access", "selectors.matchArgs[index=1,operator=Equal,value=2/MAY_WRITE]")
		}
	}
	return out
}

func scopeSelectorReports(intent contract.CollectionIntent, behavior string) []contract.CollectionSelectorReport {
	if intent.ScopeType == "" || intent.ScopeType == "host" {
		return nil
	}
	if hasNamespacePushdown(intent) {
		return []contract.CollectionSelectorReport{{
			Behavior: behavior,
			Selector: "scope.namespace",
			Status:   "pushed_down",
			Location: "tetragon",
			Mapping:  "selectors.matchNamespaces",
		}}
	}
	return []contract.CollectionSelectorReport{{
		Behavior: behavior,
		Selector: "scope." + intent.ScopeType,
		Status:   "agent_side",
		Location: "agent",
		Reason:   "runtime scope is enforced after Tetragon emission; selector is correct but may collect extra events until backend pushdown is implemented",
	}}
}

func hasNamespacePushdown(intent contract.CollectionIntent) bool {
	return intent.ScopeType == "namespace" && (intent.ScopeSelector == "self" || len(intent.NamespaceSelectors) > 0)
}

func agentSideSelectorsForFilter(filter contract.CollectionBehaviorFilter) []contract.CollectionSelectorReport {
	behavior := eventmodel.NormalizeBehavior(filter.Behavior).String()
	if behavior != eventmodel.BehaviorNetworkConnect.String() {
		return nil
	}
	var out []contract.CollectionSelectorReport
	add := func(selector, reason string) {
		out = append(out, contract.CollectionSelectorReport{
			Behavior: behavior,
			Selector: selector,
			Status:   "agent_side",
			Location: "agent",
			Reason:   reason,
		})
	}
	if len(filter.SocketAddrs) > 0 {
		add("socket.addr", "destination address IOC is enforced after Tetragon emission until sockaddr destination pushdown is verified")
	}
	if len(filter.SocketPorts) > 0 {
		add("socket.port", "destination port IOC is enforced after Tetragon emission until sockaddr destination pushdown is verified")
	}
	return out
}

func unsupportedSelectorsForFilter(filter contract.CollectionBehaviorFilter) []contract.CollectionSelectorReport {
	return nil
}

func (b *Backend) probeHostCapabilities() (string, bool, bool, error) {
	kernelRelease := runtime.GOOS
	if data, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		kernelRelease = strings.TrimSpace(string(data))
	}
	btfPath := firstNonEmpty(b.BTFPath, "/sys/kernel/btf/vmlinux")
	btfAvailable := fileExists(btfPath)
	bpffsPath := firstNonEmpty(b.BPFFSPath, "/sys/fs/bpf")
	bpffsAvailable := dirExists(bpffsPath)
	if b.RequireBTF && !btfAvailable {
		return kernelRelease, false, bpffsAvailable, fmt.Errorf("btf unavailable at %s", btfPath)
	}
	if b.RequireBPFFS && !bpffsAvailable {
		return kernelRelease, btfAvailable, false, fmt.Errorf("bpffs unavailable at %s", bpffsPath)
	}
	return kernelRelease, btfAvailable, bpffsAvailable, nil
}

func (b *Backend) verifyConfiguredExecutables() error {
	for name, path := range map[string]string{
		"tetra":    b.Bundle.TetraPath,
		"tetragon": b.Bundle.TetragonPath,
	} {
		if strings.TrimSpace(path) == "" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("verify %s executable %s: %w", name, path, err)
		}
		if info.IsDir() {
			return fmt.Errorf("verify %s executable %s: is a directory", name, path)
		}
		if info.Mode()&0o111 == 0 {
			return fmt.Errorf("verify %s executable %s: not executable", name, path)
		}
	}
	return nil
}

func (b *Backend) Apply(ctx context.Context, intent contract.CollectionIntent) error {
	if b.PolicyPath == "" {
		err := fmt.Errorf("tetragon policy path is required")
		b.setError(err)
		return err
	}
	normalized, err := intent.NormalizeScope()
	if err != nil {
		b.setError(err)
		return err
	}
	normalized, err = resolveNamespaceScope(normalized)
	if err != nil {
		b.setError(err)
		return err
	}
	if err := validateSupportedScope(normalized); err != nil {
		b.setError(err)
		return err
	}
	selfContainerID, err := resolveNamespaceSelfContainerID(normalized)
	if err != nil {
		b.setError(err)
		return err
	}
	if _, err := os.Stat(b.PolicyPath); err != nil {
		b.setError(err)
		return fmt.Errorf("verify tetragon policy: %w", err)
	}
	b.mu.Lock()
	b.namespaceSelfContainerID = selfContainerID
	loaded := b.policyLoaded
	oldIntent := b.intent
	oldRuntimePolicyApplied := b.runtimePolicyApplied
	b.mu.Unlock()
	if loaded && b.Bundle.TetraPath != "" && b.EventSource == "" {
		if err := b.liveApplyTracingPolicy(ctx, oldIntent, normalized, oldRuntimePolicyApplied); err != nil {
			b.setError(err)
			return err
		}
		b.mu.Lock()
		b.intent = normalized
		b.policyLoaded = true
		b.runtimePolicyApplied = needsTracingPolicy(normalized)
		b.lastError = ""
		b.mu.Unlock()
		return nil
	}
	b.mu.Lock()
	b.intent = normalized
	b.policyLoaded = false
	b.runtimePolicyApplied = false
	b.lastError = ""
	b.mu.Unlock()
	return nil
}

func (b *Backend) prepareBundle() (BundleVerification, error) {
	if b.Bundle.InstallDir != "" {
		installed, err := InstallBundle(b.Bundle)
		if err != nil {
			return BundleVerification{}, err
		}
		b.Bundle.TetraPath = installed.TetraPath
		b.Bundle.TetragonPath = installed.TetragonPath
		return BundleVerification{
			Version:      installed.Version,
			TetraPath:    installed.TetraPath,
			TetragonPath: installed.TetragonPath,
		}, nil
	}
	return VerifyBundle(b.Bundle)
}

func (b *Backend) Subscribe(ctx context.Context, intent contract.CollectionIntent) (<-chan contract.EventEnvelope, error) {
	if err := b.ensureIntent(ctx, intent); err != nil {
		return nil, err
	}
	stopSensor, err := b.startManagedSensor(ctx)
	if err != nil {
		b.setError(err)
		return nil, err
	}
	if b.requiresManagedSensorReady() {
		if err := b.waitManagedSensorReady(ctx); err != nil {
			stopSensor()
			b.setError(err)
			return nil, err
		}
	}
	if err := b.applyPreparedPolicy(ctx); err != nil {
		stopSensor()
		b.setError(err)
		return nil, err
	}
	if b.useGRPCEventSource() {
		return b.subscribeManagedGRPC(ctx, stopSensor)
	}
	source, closeSource, err := b.openEventSource(ctx)
	if err != nil {
		stopSensor()
		b.setError(err)
		return nil, err
	}
	b.mu.Lock()
	b.policyLoaded = true
	b.running = true
	b.lastError = ""
	b.mu.Unlock()

	out := make(chan contract.EventEnvelope)
	go func() {
		defer close(out)
		defer b.setRunning(false)
		defer b.cleanupRuntimePolicy()
		defer stopSensor()
		defer closeSource()
		scanner := bufio.NewScanner(source)
		scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for scanner.Scan() {
			line := append([]byte(nil), scanner.Bytes()...)
			if len(line) == 0 {
				continue
			}
			if dropped, ok := ParseDroppedEvents(line); ok {
				b.incDroppedEvents(dropped)
				continue
			}
			events, ok := ParseLine(line)
			if !ok {
				b.incParseError(fmt.Errorf("unrecognized tetragon event"))
				continue
			}
			rawRef := rawRefForLine(line)
			for _, event := range events {
				if !b.matchesIntentBehavior(event.GetBehavior()) {
					continue
				}
				if !b.matchesScope(event) {
					continue
				}
				if event.RawRef == "" {
					event.RawRef = rawRef
				}
				ev := contract.EventEnvelope{
					SensorEvent: event,
					RawRef:      event.GetRawRef(),
					ReceivedAt:  time.Now().UTC(),
				}
				select {
				case <-ctx.Done():
					return
				case out <- ev:
					b.incEvent(ev.ReceivedAt)
				}
			}
		}
		if err := scanner.Err(); err != nil && !isBenignEventSourceReadError(err) {
			b.setError(err)
		}
	}()
	return out, nil
}

func (b *Backend) matchesIntentBehavior(behavior string) bool {
	b.mu.Lock()
	intent := b.intent
	b.mu.Unlock()
	if len(intent.Behaviors) == 0 {
		return true
	}
	return intentHasBehavior(intent, behavior)
}

func isBenignEventSourceReadError(err error) bool {
	if err == nil {
		return true
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "file already closed") || strings.Contains(text, "closed pipe")
}

func (b *Backend) ensureIntent(ctx context.Context, intent contract.CollectionIntent) error {
	explicitScope := strings.TrimSpace(intent.ScopeType) != "" || strings.TrimSpace(intent.ScopeSelector) != ""
	normalized, err := intent.NormalizeScope()
	if err != nil {
		return err
	}
	normalized, err = resolveNamespaceScope(normalized)
	if err != nil {
		return err
	}
	if err := validateSupportedScope(normalized); err != nil {
		return err
	}
	if explicitScope {
		b.ScopeType = normalized.ScopeType
		b.ScopeSelector = normalized.ScopeSelector
		if normalized.ScopeType == "container" && b.ContainerIDPrefix == "" {
			b.ContainerIDPrefix = normalized.ScopeSelector
		}
	}
	b.mu.Lock()
	loaded := b.policyLoaded
	hasIntent := len(b.intent.Behaviors) > 0 || b.intent.ObserveOnly || len(b.intent.FilePrefixes) > 0 || len(b.intent.SocketFamilies) > 0
	b.mu.Unlock()
	if loaded || hasIntent {
		return nil
	}
	return b.Apply(ctx, normalized)
}

func validateSupportedScope(intent contract.CollectionIntent) error {
	switch intent.ScopeType {
	case "", "host", "container", "cgroup", "namespace", "pod":
		return nil
	default:
		return fmt.Errorf("tetragon backend scope %q is not supported", intent.ScopeType)
	}
}

func resolveNamespaceScope(intent contract.CollectionIntent) (contract.CollectionIntent, error) {
	if intent.ScopeType != "namespace" || intent.ScopeSelector != "self" || len(intent.NamespaceSelectors) > 0 {
		return intent, nil
	}
	selectors, err := selfNamespaceSelectors()
	if err != nil {
		return contract.CollectionIntent{}, err
	}
	intent.NamespaceSelectors = selectors
	return intent, nil
}

func selfNamespaceSelectors() ([]contract.NamespaceSelector, error) {
	namespaces := []struct {
		name string
		tp   string
	}{
		{name: "pid", tp: "Pid"},
		{name: "mnt", tp: "Mnt"},
	}
	out := make([]contract.NamespaceSelector, 0, len(namespaces))
	for _, ns := range namespaces {
		link, err := os.Readlink(filepath.Join("/proc/self/ns", ns.name))
		if err != nil {
			return nil, fmt.Errorf("resolve namespace scope %s: %w", ns.name, err)
		}
		inode, ok := namespaceInode(link)
		if !ok {
			return nil, fmt.Errorf("resolve namespace scope %s: unexpected link %q", ns.name, link)
		}
		out = append(out, contract.NamespaceSelector{Namespace: ns.tp, Values: []string{inode}})
	}
	return out, nil
}

func namespaceInode(link string) (string, bool) {
	start := strings.Index(link, "[")
	end := strings.Index(link, "]")
	if start < 0 || end <= start+1 {
		return "", false
	}
	inode := link[start+1 : end]
	if _, err := strconv.ParseUint(inode, 10, 64); err != nil {
		return "", false
	}
	return inode, true
}

func (b *Backend) applyPreparedPolicy(ctx context.Context) error {
	b.mu.Lock()
	if b.policyLoaded {
		b.mu.Unlock()
		return nil
	}
	intent := b.intent
	b.mu.Unlock()

	if b.Bundle.TetraPath != "" && b.EventSource == "" && needsTracingPolicy(intent) {
		path, err := b.renderTracingPolicy(intent)
		if err != nil {
			return err
		}
		if err := b.applyTracingPolicy(ctx, path); err != nil {
			return err
		}
	}
	b.mu.Lock()
	b.policyLoaded = true
	if b.Bundle.TetraPath != "" && b.EventSource == "" && needsTracingPolicy(intent) {
		b.runtimePolicyApplied = true
	}
	b.lastError = ""
	b.mu.Unlock()
	return nil
}

func (b *Backend) cleanupRuntimePolicy() {
	b.mu.Lock()
	shouldCleanup := b.runtimePolicyApplied
	b.runtimePolicyApplied = false
	b.mu.Unlock()
	if !shouldCleanup || b.Bundle.TetraPath == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, b.Bundle.TetraPath, "tracingpolicy", "delete", runtimeTracingPolicyName)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return
	}
	trimmed := strings.TrimSpace(string(output))
	lower := strings.ToLower(trimmed)
	if strings.Contains(lower, "not found") || strings.Contains(lower, "notfound") || strings.Contains(lower, "not exist") {
		return
	}
	if trimmed == "" {
		b.setError(fmt.Errorf("delete tetragon tracing policy %s: %w", runtimeTracingPolicyName, err))
		return
	}
	b.setError(fmt.Errorf("delete tetragon tracing policy %s: %w: %s", runtimeTracingPolicyName, err, trimmed))
}

func needsTracingPolicy(intent contract.CollectionIntent) bool {
	for _, behavior := range intent.Behaviors {
		switch eventmodel.NormalizeBehavior(behavior) {
		case eventmodel.BehaviorProcessExec, eventmodel.BehaviorProcessExit, eventmodel.BehaviorProcessFork, eventmodel.BehaviorNetworkConnect, eventmodel.BehaviorFileOpen, eventmodel.BehaviorFileRead, eventmodel.BehaviorFileWrite, eventmodel.BehaviorFileChmod:
			return true
		}
	}
	return false
}

func (b *Backend) renderTracingPolicy(intent contract.CollectionIntent) (string, error) {
	return b.writeTracingPolicy(intent, "sysarmor-runtime-tracingpolicy.yaml")
}

func (b *Backend) writeTracingPolicy(intent contract.CollectionIntent, name string) (string, error) {
	dir := filepath.Dir(b.PolicyPath)
	if dir == "." || dir == "" {
		dir = os.TempDir()
	}
	path := filepath.Join(dir, name)
	data := buildTracingPolicy(intent)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write tetragon tracing policy: %w", err)
	}
	return path, nil
}

func (b *Backend) liveApplyTracingPolicy(ctx context.Context, oldIntent, newIntent contract.CollectionIntent, oldRuntimePolicyApplied bool) error {
	oldNeeds := oldRuntimePolicyApplied && needsTracingPolicy(oldIntent)
	newNeeds := needsTracingPolicy(newIntent)
	if !oldNeeds && !newNeeds {
		return nil
	}
	var oldPath string
	var err error
	if oldNeeds {
		oldPath, err = b.writeTracingPolicy(oldIntent, "sysarmor-runtime-tracingpolicy-rollback.yaml")
		if err != nil {
			return err
		}
		if err := b.deleteTracingPolicy(ctx); err != nil {
			return err
		}
	}
	if !newNeeds {
		return nil
	}
	newPath, err := b.renderTracingPolicy(newIntent)
	if err != nil {
		return err
	}
	if err := b.applyTracingPolicy(ctx, newPath); err != nil {
		if oldNeeds && oldPath != "" {
			_ = b.applyTracingPolicy(context.Background(), oldPath)
		}
		return err
	}
	return nil
}

func (b *Backend) deleteTracingPolicy(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, b.Bundle.TetraPath, "tracingpolicy", "delete", runtimeTracingPolicyName)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	trimmed := strings.TrimSpace(string(output))
	lower := strings.ToLower(trimmed)
	if strings.Contains(lower, "not found") || strings.Contains(lower, "notfound") || strings.Contains(lower, "not exist") {
		return nil
	}
	if trimmed == "" {
		return fmt.Errorf("delete tetragon tracing policy %s: %w", runtimeTracingPolicyName, err)
	}
	return fmt.Errorf("delete tetragon tracing policy %s: %w: %s", runtimeTracingPolicyName, err, trimmed)
}

func (b *Backend) applyTracingPolicy(ctx context.Context, path string) error {
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for {
		cmd := exec.CommandContext(ctx, b.Bundle.TetraPath, "tracingpolicy", "add", path)
		output, err := cmd.CombinedOutput()
		if err == nil {
			return b.verifyTracingPolicy(ctx, runtimeTracingPolicyName)
		}
		trimmed := strings.TrimSpace(string(output))
		if strings.Contains(strings.ToLower(trimmed), "already exists") {
			return b.verifyTracingPolicy(ctx, runtimeTracingPolicyName)
		}
		if trimmed == "" {
			lastErr = fmt.Errorf("apply tetragon tracing policy: %w", err)
		} else {
			lastErr = fmt.Errorf("apply tetragon tracing policy: %w: %s", err, trimmed)
		}
		if time.Now().After(deadline) {
			return lastErr
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (b *Backend) verifyTracingPolicy(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, b.Bundle.TetraPath, "tracingpolicy", "list")
	output, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if err != nil {
		if trimmed == "" {
			return fmt.Errorf("verify tetragon tracing policy %s: %w", name, err)
		}
		return fmt.Errorf("verify tetragon tracing policy %s: %w: %s", name, err, trimmed)
	}
	if !strings.Contains(trimmed, name) {
		return fmt.Errorf("verify tetragon tracing policy %s: not listed", name)
	}
	return nil
}

func buildTracingPolicy(intent contract.CollectionIntent) []byte {
	var out bytes.Buffer
	out.WriteString("apiVersion: cilium.io/v1alpha1\n")
	out.WriteString("kind: TracingPolicy\n")
	out.WriteString("metadata:\n")
	out.WriteString("  name: ")
	out.WriteString(fmt.Sprintf("%q", runtimeTracingPolicyName))
	out.WriteString("\n")
	out.WriteString("spec:\n")
	out.WriteString("  kprobes:\n")
	if intentHasAnyBehavior(intent, eventmodel.BehaviorProcessExec.String(), eventmodel.BehaviorProcessFork.String()) {
		prefixes := mergeFilterStrings(
			behaviorFilter(intent, eventmodel.BehaviorProcessExec.String()).BinaryPrefixes,
			behaviorFilter(intent, eventmodel.BehaviorProcessFork.String()).BinaryPrefixes,
		)
		out.WriteString(`  - call: "security_bprm_creds_from_file"
    syscall: false
    args:
    - index: 0
      type: "nop"
    - index: 1
      type: "file"
`)
		if len(prefixes) > 0 || len(intent.NamespaceSelectors) > 0 {
			out.WriteString("    selectors:\n")
			out.WriteString("    -\n")
			writeNamespaceSelectors(&out, intent.NamespaceSelectors, "      ")
		}
		if len(prefixes) > 0 {
			out.WriteString(`      matchArgs:
      - index: 1
        operator: "Prefix"
        values:
`)
			for _, prefix := range prefixes {
				out.WriteString("        - ")
				out.WriteString(fmt.Sprintf("%q", prefix))
				out.WriteString("\n")
			}
		}
	}
	if intentHasBehavior(intent, eventmodel.BehaviorProcessExit.String()) {
		out.WriteString(`  - call: "do_exit"
    syscall: false
    args:
    - index: 0
      type: "int"
`)
	}
	if intentHasBehavior(intent, eventmodel.BehaviorNetworkConnect.String()) {
		filter := behaviorFilter(intent, eventmodel.BehaviorNetworkConnect.String())
		families := filter.SocketFamilies
		if len(families) == 0 {
			families = []string{"AF_INET", "AF_INET6"}
		}
		out.WriteString(`  - call: "security_socket_connect"
    syscall: false
    args:
    - index: 1
      type: "sockaddr"
    - index: 2
      type: "int"
    selectors:
    -
`)
		writeMatchBinaries(&out, filter.BinaryPrefixes, "      ")
		writeNamespaceSelectors(&out, intent.NamespaceSelectors, "      ")
		out.WriteString(`      matchArgs:
      - index: 1
        operator: "Family"
        values:
`)
		for _, family := range families {
			out.WriteString("        - ")
			out.WriteString(fmt.Sprintf("%q", family))
			out.WriteString("\n")
		}
	}
	fileSelectors := filePermissionSelectors(intent)
	if len(fileSelectors) > 0 {
		out.WriteString(`  - call: "security_file_permission"
    syscall: false
    return: true
    args:
    - index: 0
      type: "file"
    - index: 1
      type: "int"
    returnArg:
      index: 0
      type: "int"
    selectors:
`)
		for _, selector := range fileSelectors {
			writeFilePermissionSelector(&out, selector)
		}
	}
	return out.Bytes()
}

type filePermissionSelector struct {
	Behavior           string
	Access             int32
	BinaryPrefixes     []string
	FilePrefixes       []string
	NamespaceSelectors []contract.NamespaceSelector
}

func filePermissionSelectors(intent contract.CollectionIntent) []filePermissionSelector {
	var selectors []filePermissionSelector
	add := func(behavior string, access int32) {
		if !intentHasBehavior(intent, behavior) {
			return
		}
		filter := behaviorFilter(intent, behavior)
		prefixes := filter.FilePrefixes
		if len(prefixes) == 0 {
			prefixes = defaultFilePrefixesForBehavior(behavior)
		}
		selectors = append(selectors, filePermissionSelector{
			Behavior:           behavior,
			Access:             access,
			BinaryPrefixes:     filter.BinaryPrefixes,
			FilePrefixes:       prefixes,
			NamespaceSelectors: append([]contract.NamespaceSelector(nil), intent.NamespaceSelectors...),
		})
	}
	add(eventmodel.BehaviorFileOpen.String(), 0)
	add(eventmodel.BehaviorFileRead.String(), 4)
	add(eventmodel.BehaviorFileWrite.String(), 2)
	add(eventmodel.BehaviorFileChmod.String(), 2)
	return selectors
}

func defaultFilePrefixesForBehavior(behavior string) []string {
	switch eventmodel.NormalizeBehavior(behavior) {
	case eventmodel.BehaviorFileRead:
		return []string{"/root/.ssh", "/var/run/secrets", "/etc/passwd", "/etc/shadow", "/etc/sudoers"}
	case eventmodel.BehaviorFileWrite, eventmodel.BehaviorFileChmod:
		return []string{"/dev/shm", "/tmp", "/var/tmp"}
	default:
		return []string{"/root/.ssh", "/var/run/secrets", "/etc/passwd"}
	}
}

func writeFilePermissionSelector(out *bytes.Buffer, selector filePermissionSelector) {
	out.WriteString("    -\n")
	writeMatchBinaries(out, selector.BinaryPrefixes, "      ")
	writeNamespaceSelectors(out, selector.NamespaceSelectors, "      ")
	out.WriteString(`      matchArgs:
      - index: 0
        operator: "Prefix"
        values:
`)
	for _, prefix := range mergeFilterStrings(selector.FilePrefixes) {
		out.WriteString("        - ")
		out.WriteString(fmt.Sprintf("%q", prefix))
		out.WriteString("\n")
	}
	if selector.Access != 0 {
		out.WriteString(`      - index: 1
        operator: "Equal"
        values:
`)
		out.WriteString("        - ")
		out.WriteString(fmt.Sprintf("%q", strconv.FormatInt(int64(selector.Access), 10)))
		out.WriteString("\n")
	}
}

func writeMatchBinaries(out *bytes.Buffer, prefixes []string, indent string) {
	prefixes = mergeFilterStrings(prefixes)
	if len(prefixes) == 0 {
		return
	}
	out.WriteString(indent)
	out.WriteString("matchBinaries:\n")
	out.WriteString(indent)
	out.WriteString("- operator: \"Prefix\"\n")
	out.WriteString(indent)
	out.WriteString("  values:\n")
	for _, prefix := range prefixes {
		out.WriteString(indent)
		out.WriteString("  - ")
		out.WriteString(fmt.Sprintf("%q", prefix))
		out.WriteString("\n")
	}
}

func writeNamespaceSelectors(out *bytes.Buffer, selectors []contract.NamespaceSelector, indent string) {
	if len(selectors) == 0 {
		return
	}
	out.WriteString(indent)
	out.WriteString("matchNamespaces:\n")
	for _, selector := range selectors {
		values := mergeFilterStrings(selector.Values)
		if selector.Namespace == "" || len(values) == 0 {
			continue
		}
		out.WriteString(indent)
		out.WriteString("- namespace: ")
		out.WriteString(selector.Namespace)
		out.WriteString("\n")
		out.WriteString(indent)
		out.WriteString("  operator: \"In\"\n")
		out.WriteString(indent)
		out.WriteString("  values:\n")
		for _, value := range values {
			out.WriteString(indent)
			out.WriteString("  - ")
			out.WriteString(fmt.Sprintf("%q", value))
			out.WriteString("\n")
		}
	}
}

func intentHasAnyBehavior(intent contract.CollectionIntent, behaviors ...string) bool {
	for _, behavior := range behaviors {
		if intentHasBehavior(intent, behavior) {
			return true
		}
	}
	return false
}

func intentHasBehavior(intent contract.CollectionIntent, behavior string) bool {
	behavior = eventmodel.NormalizeBehavior(behavior).String()
	for _, got := range intent.Behaviors {
		if eventmodel.NormalizeBehavior(got).String() == behavior {
			return true
		}
	}
	return false
}

func behaviorFilter(intent contract.CollectionIntent, behavior string) contract.CollectionBehaviorFilter {
	behavior = eventmodel.NormalizeBehavior(behavior).String()
	for _, filter := range intent.BehaviorFilters {
		if eventmodel.NormalizeBehavior(filter.Behavior).String() == behavior {
			return filter
		}
	}
	filter := contract.CollectionBehaviorFilter{Behavior: behavior}
	switch behavior {
	case eventmodel.BehaviorProcessExec.String(), eventmodel.BehaviorProcessFork.String():
		filter.BinaryPrefixes = intent.BinaryPrefixes
	case eventmodel.BehaviorNetworkConnect.String():
		filter.BinaryPrefixes = intent.BinaryPrefixes
		filter.SocketFamilies = intent.SocketFamilies
		filter.SocketAddrs = intent.SocketAddrs
		filter.SocketPorts = intent.SocketPorts
	case eventmodel.BehaviorFileOpen.String(), eventmodel.BehaviorFileRead.String(), eventmodel.BehaviorFileWrite.String(), eventmodel.BehaviorFileChmod.String():
		filter.BinaryPrefixes = intent.BinaryPrefixes
		filter.FilePrefixes = intent.FilePrefixes
	}
	return filter
}

func mergeFilterStrings(lists ...[]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, list := range lists {
		for _, value := range list {
			value = strings.TrimSpace(value)
			if value == "" || seen[value] {
				continue
			}
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func (b *Backend) Enforce(_ context.Context, cmd contract.EnforcementCmd) (contract.EnforcementAck, error) {
	return contract.UnsupportedAck(cmd, "tetragon backend is observe-only in v2"), nil
}

func (b *Backend) Health(context.Context) (contract.Health, error) {
	b.mu.Lock()
	running := b.running
	lastError := b.lastError
	b.mu.Unlock()
	status := b.eventSupervisor.Status()
	sensorStatus := b.sensorSupervisor.Status()
	if status.Running || sensorStatus.Running {
		running = true
	}
	restarts := status.RestartCount + sensorStatus.RestartCount
	lastExit, processError := mergedProcessExit(status, sensorStatus)
	if processError != "" {
		lastError = processError
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return contract.Health{
		Backend:        "tetragon",
		Running:        running,
		Installed:      b.installed,
		Version:        b.Version,
		PolicyLoaded:   b.policyLoaded,
		EventsSeen:     b.eventsSeen,
		EventsDropped:  b.eventsDropped,
		ParseErrors:    b.parseErrors,
		RestartCount:   restarts,
		LastEventAt:    b.lastEventAt,
		LastExitReason: lastExit,
		LastError:      lastError,
	}, nil
}

func mergedProcessExit(statuses ...ProcessStatus) (string, string) {
	for _, status := range statuses {
		if status.LastError != "" {
			return status.LastExit, status.LastError
		}
	}
	for _, status := range statuses {
		if status.LastExit != "" {
			return status.LastExit, ""
		}
	}
	return "", ""
}

func (b *Backend) startManagedSensor(ctx context.Context) (func(), error) {
	if b.Bundle.TetragonPath == "" {
		return func() {}, nil
	}
	_ = os.MkdirAll("/var/run/tetragon", 0o755)
	_ = os.Remove("/var/run/tetragon/tetragon.pid")
	spec := ProcessSpec{
		Name:    "tetragon",
		Path:    b.Bundle.TetragonPath,
		Args:    b.tetragonArgs(b.Bundle.TetragonPath),
		Dir:     bundleRuntimeDir(b.Bundle.TetragonPath),
		LogPath: "/var/log/sysarmor/tetragon.log",
	}
	if b.Restart.Enabled {
		if err := b.sensorSupervisor.StartRestarting(ctx, spec, RestartPolicy{
			MaxRestarts: b.Restart.MaxRestarts,
			Delay:       b.Restart.Delay,
		}); err != nil {
			return nil, err
		}
	} else if err := b.sensorSupervisor.Start(ctx, spec); err != nil {
		return nil, err
	}
	return func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = b.sensorSupervisor.Stop(stopCtx)
	}, nil
}

func (b *Backend) waitManagedSensorReady(ctx context.Context) error {
	if b.Bundle.TetragonPath == "" || b.Bundle.TetraPath == "" {
		return nil
	}
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for {
		cmd := exec.CommandContext(ctx, b.Bundle.TetraPath, "tracingpolicy", "list")
		output, err := cmd.CombinedOutput()
		if err == nil {
			return nil
		}
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			lastErr = fmt.Errorf("wait tetragon ready: %w", err)
		} else {
			lastErr = fmt.Errorf("wait tetragon ready: %w: %s", err, trimmed)
		}
		if time.Now().After(deadline) {
			return lastErr
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (b *Backend) requiresManagedSensorReady() bool {
	return b.Bundle.BundleDir != "" && b.Bundle.InstallDir != "" && b.Bundle.TetragonPath != ""
}

func tetragonArgs(tetragonPath string) []string {
	args := defaultTetragonArgs()
	if libPath := bundleTetragonLibDir(tetragonPath); libPath != "" {
		args = append(args, "--bpf-lib", libPath)
	}
	return args
}

func (b *Backend) tetragonArgs(tetragonPath string) []string {
	args := tetragonArgs(tetragonPath)
	if b.useGRPCEventSource() {
		args = append(args, "--server-address", firstNonEmpty(b.ServerAddress, defaultTetragonServerAddress))
	}
	if strings.TrimSpace(b.CgroupRate) != "" {
		args = append(args, "--cgroup-rate", strings.TrimSpace(b.CgroupRate))
	}
	if strings.TrimSpace(b.PprofAddress) != "" {
		args = append(args, "--pprof-address", strings.TrimSpace(b.PprofAddress))
	}
	if strings.TrimSpace(b.GopsAddress) != "" {
		args = append(args, "--gops-address", strings.TrimSpace(b.GopsAddress))
	}
	if b.ProcessCacheSize > 0 {
		args = append(args, "--process-cache-size", strconv.Itoa(b.ProcessCacheSize))
	}
	if b.DataCacheSize > 0 {
		args = append(args, "--data-cache-size", strconv.Itoa(b.DataCacheSize))
	}
	if b.EventQueueSize > 0 {
		args = append(args, "--event-queue-size", strconv.Itoa(b.EventQueueSize))
	}
	if strings.TrimSpace(b.RBQueueSize) != "" {
		args = append(args, "--rb-queue-size", strings.TrimSpace(b.RBQueueSize))
	}
	return args
}

func defaultTetragonArgs() []string {
	args := []string{
		"--log-level", "warn",
		"--metrics-server", "",
		"--health-server-address", "",
		"--enable-tracing-policy-crd=false",
		"--enable-process-cred=false",
		"--enable-process-ns=false",
		"--enable-process-environment-variables=false",
		"--enable-ancestors", "",
		"--enable-k8s-api=false",
		"--enable-pod-annotations=false",
	}
	if _, err := os.Stat("/sys/kernel/btf/vmlinux"); err == nil {
		args = append(args, "--btf", "/sys/kernel/btf/vmlinux")
	}
	return args
}

func tetraGetEventsArgs(intent contract.CollectionIntent) []string {
	args := []string{"getevents", "-o", "json", "--policy-names", runtimeTracingPolicyName}
	eventTypes := tetraEventTypesForIntent(intent)
	if len(eventTypes) > 0 {
		args = append(args, "--event-types", strings.Join(eventTypes, ","))
	}
	return args
}

func tetraEventTypesForIntent(intent contract.CollectionIntent) []string {
	types := map[string]bool{}
	for _, behavior := range intent.Behaviors {
		switch eventmodel.NormalizeBehavior(behavior).String() {
		case eventmodel.BehaviorProcessExec.String(),
			eventmodel.BehaviorProcessFork.String(),
			eventmodel.BehaviorProcessExit.String(),
			eventmodel.BehaviorNetworkConnect.String(),
			eventmodel.BehaviorFileOpen.String(),
			eventmodel.BehaviorFileRead.String(),
			eventmodel.BehaviorFileWrite.String(),
			eventmodel.BehaviorFileChmod.String():
			types["PROCESS_KPROBE"] = true
		}
	}
	var out []string
	for _, eventType := range []string{"PROCESS_KPROBE"} {
		if types[eventType] {
			out = append(out, eventType)
		}
	}
	return out
}

func bundleTetragonLibDir(tetragonPath string) string {
	if strings.TrimSpace(tetragonPath) == "" {
		return ""
	}
	root := filepath.Dir(filepath.Dir(tetragonPath))
	candidates := []string{
		filepath.Join(root, "lib", "tetragon", "bpf"),
		filepath.Join(root, "usr", "local", "lib", "tetragon", "bpf"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func (b *Backend) openEventSource(ctx context.Context) (io.Reader, func(), error) {
	switch b.EventSource {
	case "":
		return b.openManagedEventSource(ctx)
	case "-":
		return os.Stdin, func() {}, nil
	default:
		f, err := os.Open(b.EventSource)
		if err != nil {
			return nil, func() {}, err
		}
		return f, func() { _ = f.Close() }, nil
	}
}

func (b *Backend) openManagedEventSource(ctx context.Context) (io.Reader, func(), error) {
	if b.Bundle.TetraPath == "" {
		return nil, func() {}, fmt.Errorf("tetragon tetra_path is required for managed event subscription")
	}
	stdout, err := b.eventSupervisor.StartWithStdout(ctx, ProcessSpec{
		Name: "tetra-getevents",
		Path: b.Bundle.TetraPath,
		Args: tetraGetEventsArgs(b.intent),
		Dir:  bundleRuntimeDir(b.Bundle.TetraPath),
	})
	if err != nil {
		return nil, func() {}, err
	}
	return stdout, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = b.eventSupervisor.Stop(ctx)
		_ = stdout.Close()
	}, nil
}

func (b *Backend) useGRPCEventSource() bool {
	if b.EventSource != "" {
		return false
	}
	transport := strings.TrimSpace(b.EventTransport)
	return transport == "" || transport == "grpc"
}

func bundleRuntimeDir(binaryPath string) string {
	if strings.TrimSpace(binaryPath) == "" {
		return ""
	}
	dir := filepath.Dir(binaryPath)
	if filepath.Base(dir) == "bin" {
		return filepath.Dir(dir)
	}
	return dir
}

func (b *Backend) matchesScope(event *sensorv1.SensorEvent) bool {
	scopeType := strings.TrimSpace(b.ScopeType)
	scopeSelector := strings.TrimSpace(b.ScopeSelector)
	switch scopeType {
	case "":
		if b.ContainerIDPrefix == "" {
			return true
		}
		return strings.HasPrefix(event.GetContainerId(), b.ContainerIDPrefix)
	case "host":
		return true
	case "container":
		return strings.HasPrefix(event.GetContainerId(), scopeSelector)
	case "cgroup":
		return strings.HasPrefix(event.GetProc().GetCgroup(), scopeSelector)
	case "namespace":
		b.mu.Lock()
		hasNamespaceSelectors := len(b.intent.NamespaceSelectors) > 0
		selfContainerID := b.namespaceSelfContainerID
		b.mu.Unlock()
		if scopeSelector == "self" && hasNamespaceSelectors {
			if selfContainerID == "" {
				return event.GetContainerId() == ""
			}
			return containerIDsMatch(event.GetContainerId(), selfContainerID)
		}
		return strings.HasPrefix(event.GetProc().GetCgroup(), scopeSelector)
	case "pod":
		return strings.HasPrefix(event.GetContainerId(), scopeSelector)
	default:
		return false
	}
}

func (b *Backend) incEvent(at time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.eventsSeen++
	b.lastEventAt = at
}

func (b *Backend) incParseError(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.parseErrors++
	b.lastError = err.Error()
}

func (b *Backend) incDroppedEvents(count uint64) {
	if count == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.eventsDropped += count
	b.lastError = fmt.Sprintf("tetragon dropped events: %d", b.eventsDropped)
}

func (b *Backend) setError(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err != nil {
		b.lastError = err.Error()
	}
}

func (b *Backend) setRunning(running bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.running = running
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func rawRefForLine(line []byte) string {
	sum := sha256.Sum256(line)
	return "tetragon:" + hex.EncodeToString(sum[:8])
}
