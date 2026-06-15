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
	"strings"
	"sync"
	"time"

	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	sensorv1 "github.com/sysarmor/sysarmor-next-project/api/proto/sensor/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/sensor/contract"
)

const runtimeTracingPolicyName = "sysarmor-runtime-collection"

type Backend struct {
	PolicyPath        string
	EventSource       string
	Version           string
	Bundle            BundleConfig
	Restart           ProcessRestartPolicy
	ScopeType         string
	ScopeSelector     string
	ContainerIDPrefix string
	BTFPath           string
	BPFFSPath         string
	RequireBTF        bool
	RequireBPFFS      bool

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
	}, nil
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
	if err := validateSupportedScope(normalized); err != nil {
		b.setError(err)
		return err
	}
	if _, err := os.Stat(b.PolicyPath); err != nil {
		b.setError(err)
		return fmt.Errorf("verify tetragon policy: %w", err)
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
	if err := b.applyPreparedPolicy(ctx); err != nil {
		stopSensor()
		b.setError(err)
		return nil, err
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

func isBenignEventSourceReadError(err error) bool {
	if err == nil {
		return true
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "file already closed") || strings.Contains(text, "closed pipe")
}

func (b *Backend) ensureIntent(ctx context.Context, intent contract.CollectionIntent) error {
	normalized, err := intent.NormalizeScope()
	if err != nil {
		return err
	}
	if err := validateSupportedScope(normalized); err != nil {
		return err
	}
	if strings.TrimSpace(normalized.ScopeType) != "" {
		b.ScopeType = normalized.ScopeType
		b.ScopeSelector = normalized.ScopeSelector
		if normalized.ScopeType == "container" && b.ContainerIDPrefix == "" {
			b.ContainerIDPrefix = normalized.ScopeSelector
		}
	}
	b.mu.Lock()
	loaded := b.policyLoaded
	hasIntent := len(b.intent.EventKinds) > 0 || b.intent.ObserveOnly || len(b.intent.FilePrefixes) > 0 || len(b.intent.SocketFamilies) > 0
	b.mu.Unlock()
	if loaded || hasIntent {
		return nil
	}
	return b.Apply(ctx, normalized)
}

func validateSupportedScope(intent contract.CollectionIntent) error {
	switch intent.ScopeType {
	case "", "host", "container", "cgroup":
		return nil
	case "namespace", "pod":
		return fmt.Errorf("tetragon backend scope %q is not supported in v2 without namespace/pod metadata", intent.ScopeType)
	default:
		return fmt.Errorf("tetragon backend scope %q is not supported", intent.ScopeType)
	}
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
	for _, kind := range intent.EventKinds {
		switch kind {
		case eventv1.EventKind_EVENT_KIND_CONNECT, eventv1.EventKind_EVENT_KIND_OPEN, eventv1.EventKind_EVENT_KIND_WRITE, eventv1.EventKind_EVENT_KIND_CHMOD:
			return true
		}
	}
	return false
}

func (b *Backend) renderTracingPolicy(intent contract.CollectionIntent) (string, error) {
	dir := filepath.Dir(b.PolicyPath)
	if dir == "." || dir == "" {
		dir = os.TempDir()
	}
	path := filepath.Join(dir, "sysarmor-runtime-tracingpolicy.yaml")
	data := buildTracingPolicy(intent)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write tetragon tracing policy: %w", err)
	}
	return path, nil
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
	if intentHasKind(intent, eventv1.EventKind_EVENT_KIND_CONNECT) {
		out.WriteString(`  - call: "security_socket_connect"
    syscall: false
    args:
    - index: 1
      type: "sockaddr"
    - index: 2
      type: "int"
    selectors:
    - matchArgs:
      - index: 1
        operator: "Family"
        values:
        - "AF_INET"
        - "AF_INET6"
`)
	}
	if intentHasAnyKind(intent, eventv1.EventKind_EVENT_KIND_OPEN, eventv1.EventKind_EVENT_KIND_WRITE, eventv1.EventKind_EVENT_KIND_CHMOD) {
		prefixes := intent.FilePrefixes
		if len(prefixes) == 0 {
			prefixes = []string{"/root/.ssh", "/var/run/secrets", "/etc/passwd"}
		}
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
    - matchArgs:
      - index: 0
        operator: "Prefix"
        values:
`)
		for _, prefix := range prefixes {
			out.WriteString("        - ")
			out.WriteString(fmt.Sprintf("%q", prefix))
			out.WriteString("\n")
		}
	}
	return out.Bytes()
}

func intentHasAnyKind(intent contract.CollectionIntent, kinds ...eventv1.EventKind) bool {
	for _, kind := range kinds {
		if intentHasKind(intent, kind) {
			return true
		}
	}
	return false
}

func intentHasKind(intent contract.CollectionIntent, kind eventv1.EventKind) bool {
	for _, got := range intent.EventKinds {
		if got == kind {
			return true
		}
	}
	return false
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
		Name: "tetragon",
		Path: b.Bundle.TetragonPath,
		Args: defaultTetragonArgs(),
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

func defaultTetragonArgs() []string {
	if _, err := os.Stat("/sys/kernel/btf/vmlinux"); err == nil {
		return []string{"--btf", "/sys/kernel/btf/vmlinux"}
	}
	return nil
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
		Args: []string{"getevents", "-o", "json"},
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

func (b *Backend) matchesScope(event *sensorv1.SensorEvent) bool {
	scopeType := strings.TrimSpace(b.ScopeType)
	scopeSelector := strings.TrimSpace(b.ScopeSelector)
	switch scopeType {
	case "", "host":
		if b.ContainerIDPrefix == "" {
			return true
		}
		return strings.HasPrefix(event.GetContainerId(), b.ContainerIDPrefix)
	case "container":
		return strings.HasPrefix(event.GetContainerId(), scopeSelector)
	case "cgroup":
		return strings.HasPrefix(event.GetProc().GetCgroup(), scopeSelector)
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
