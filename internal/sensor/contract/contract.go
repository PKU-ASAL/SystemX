package contract

import (
	"context"
	"time"

	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	sensorv1 "github.com/sysarmor/sysarmor-next-project/api/proto/sensor/v1"
)

type Sensor interface {
	Capability(ctx context.Context) (Capability, error)
	Subscribe(ctx context.Context, intent CollectionIntent) (<-chan EventEnvelope, error)
	Enforce(ctx context.Context, cmd EnforcementCmd) (EnforcementAck, error)
	Health(ctx context.Context) (Health, error)
}

type Capability struct {
	Backend         string
	Version         string
	SupportsExec    bool
	SupportsConnect bool
	SupportsFile    bool
	SupportsEnforce bool
	SupportsHealth  bool
	KernelRelease   string
	BTFAvailable    bool
	BPFFSAvailable  bool
}

type CollectionIntent struct {
	EventKinds     []eventv1.EventKind
	FilePrefixes   []string
	SocketFamilies []string
	ObserveOnly    bool
}

type EventEnvelope struct {
	SensorEvent *sensorv1.SensorEvent
	RawRef      string
	ReceivedAt  time.Time
}

type Health struct {
	Backend        string
	Running        bool
	Installed      bool
	Version        string
	PolicyLoaded   bool
	EventsSeen     uint64
	EventsDropped  uint64
	ParseErrors    uint64
	RestartCount   uint64
	LastEventAt    time.Time
	LastExitReason string
	LastError      string
}

type EnforcementCmd struct {
	ID          string
	Action      string
	Target      string
	ObserveOnly bool
	Reason      string
}

type EnforcementAck struct {
	ID          string
	Accepted    bool
	Unsupported bool
	ObserveOnly bool
	Message     string
}

func ObserveOnlyAck(cmd EnforcementCmd, message string) EnforcementAck {
	return EnforcementAck{
		ID:          cmd.ID,
		Accepted:    true,
		ObserveOnly: true,
		Message:     message,
	}
}

func UnsupportedAck(cmd EnforcementCmd, message string) EnforcementAck {
	return EnforcementAck{
		ID:          cmd.ID,
		Unsupported: true,
		Message:     message,
	}
}
