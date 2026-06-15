package runtime

import (
	"context"
	"testing"
	"time"

	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	sensorv1 "github.com/sysarmor/sysarmor-next-project/api/proto/sensor/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/sensor/contract"
)

func TestManagerLifecycle(t *testing.T) {
	ctx := context.Background()
	fake := newFakeSensor()
	rt := New(fake)

	capability, err := rt.Probe(ctx)
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if capability.Backend != "fake" || !capability.SupportsExec {
		t.Fatalf("unexpected capability: %+v", capability)
	}

	intent := contract.CollectionIntent{
		EventKinds:  []eventv1.EventKind{eventv1.EventKind_EVENT_KIND_EXEC},
		ObserveOnly: true,
	}
	if err := rt.Apply(ctx, intent); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	events, err := rt.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	fake.emit(contract.EventEnvelope{SensorEvent: &sensorv1.SensorEvent{Kind: eventv1.EventKind_EVENT_KIND_EXEC}})

	select {
	case ev := <-events:
		if ev.SensorEvent.GetKind() != eventv1.EventKind_EVENT_KIND_EXEC {
			t.Fatalf("event kind = %v", ev.SensorEvent.GetKind())
		}
		if ev.ReceivedAt.IsZero() {
			t.Fatal("ReceivedAt was not set")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}

	health, err := rt.Health(ctx)
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if health.EventsSeen != 1 {
		t.Fatalf("EventsSeen = %d", health.EventsSeen)
	}
	if err := rt.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestManagerRequiresApplyBeforeSubscribe(t *testing.T) {
	rt := New(newFakeSensor())
	if _, err := rt.Subscribe(context.Background()); err == nil {
		t.Fatal("Subscribe() error = nil")
	}
}

type fakeSensor struct {
	events chan contract.EventEnvelope
}

func newFakeSensor() *fakeSensor {
	return &fakeSensor{events: make(chan contract.EventEnvelope, 1)}
}

func (f *fakeSensor) Capability(context.Context) (contract.Capability, error) {
	return contract.Capability{Backend: "fake", Version: "test", SupportsExec: true, SupportsHealth: true}, nil
}

func (f *fakeSensor) Apply(context.Context, contract.CollectionIntent) error {
	return nil
}

func (f *fakeSensor) Subscribe(ctx context.Context, _ contract.CollectionIntent) (<-chan contract.EventEnvelope, error) {
	out := make(chan contract.EventEnvelope)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-f.events:
				out <- ev
			}
		}
	}()
	return out, nil
}

func (f *fakeSensor) Enforce(_ context.Context, cmd contract.EnforcementCmd) (contract.EnforcementAck, error) {
	return contract.UnsupportedAck(cmd, "fake sensor is observe-only"), nil
}

func (f *fakeSensor) Health(context.Context) (contract.Health, error) {
	return contract.Health{Backend: "fake", Running: true, Installed: true, PolicyLoaded: true}, nil
}

func (f *fakeSensor) emit(ev contract.EventEnvelope) {
	f.events <- ev
}
