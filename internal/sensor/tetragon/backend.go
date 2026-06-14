package tetragon

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/sysarmor/sysarmor-next-project/internal/sensor/contract"
)

type Backend struct {
	PolicyPath  string
	EventSource string
	Version     string
	Bundle      BundleConfig

	mu           sync.Mutex
	policyLoaded bool
	running      bool
	installed    bool
	eventsSeen   uint64
	parseErrors  uint64
	lastEventAt  time.Time
	lastError    string
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
	}
	return contract.Capability{
		Backend:         "tetragon",
		Version:         b.Version,
		SupportsExec:    true,
		SupportsConnect: true,
		SupportsFile:    true,
		SupportsHealth:  true,
	}, nil
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

func (b *Backend) Subscribe(ctx context.Context, _ contract.CollectionIntent) (<-chan contract.EventEnvelope, error) {
	if b.PolicyPath == "" {
		return nil, fmt.Errorf("tetragon policy path is required")
	}
	if _, err := os.Stat(b.PolicyPath); err != nil {
		b.setError(err)
		return nil, fmt.Errorf("verify tetragon policy: %w", err)
	}
	source, closeSource, err := b.openEventSource()
	if err != nil {
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
		defer closeSource()
		defer b.setRunning(false)
		scanner := bufio.NewScanner(source)
		for scanner.Scan() {
			line := append([]byte(nil), scanner.Bytes()...)
			if len(line) == 0 {
				continue
			}
			events, ok := ParseLine(line)
			if !ok {
				b.incParseError(fmt.Errorf("unrecognized tetragon event"))
				continue
			}
			rawRef := rawRefForLine(line)
			for _, event := range events {
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
		if err := scanner.Err(); err != nil {
			b.setError(err)
		}
	}()
	return out, nil
}

func (b *Backend) Enforce(_ context.Context, cmd contract.EnforcementCmd) (contract.EnforcementAck, error) {
	return contract.UnsupportedAck(cmd, "tetragon backend is observe-only in v2"), nil
}

func (b *Backend) Health(context.Context) (contract.Health, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return contract.Health{
		Backend:      "tetragon",
		Running:      b.running,
		Installed:    b.installed,
		Version:      b.Version,
		PolicyLoaded: b.policyLoaded,
		EventsSeen:   b.eventsSeen,
		ParseErrors:  b.parseErrors,
		LastEventAt:  b.lastEventAt,
		LastError:    b.lastError,
	}, nil
}

func (b *Backend) openEventSource() (io.Reader, func(), error) {
	switch b.EventSource {
	case "":
		return nil, func() {}, fmt.Errorf("tetragon event_source is required until managed process subscription is implemented")
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

func rawRefForLine(line []byte) string {
	sum := sha256.Sum256(line)
	return "tetragon:" + hex.EncodeToString(sum[:8])
}
