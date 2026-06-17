package contract

import (
	"context"
	"fmt"
	"strings"
	"time"

	sensorv1 "github.com/sysarmor/sysarmor-next-project/api/proto/sensor/v1"
)

type Sensor interface {
	Capability(ctx context.Context) (Capability, error)
	Apply(ctx context.Context, intent CollectionIntent) error
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
	Collection      []CollectionBehaviorCapability
}

type CollectionIntent struct {
	Behaviors       []string
	BinaryPrefixes  []string
	FilePrefixes    []string
	SocketFamilies  []string
	SocketAddrs     []string
	SocketPorts     []string
	BehaviorFilters []CollectionBehaviorFilter
	ScopeType       string
	ScopeSelector   string
	ObserveOnly     bool
	Capabilities    []CollectionBehaviorCapability
}

type CollectionBehaviorFilter struct {
	Behavior       string
	BinaryPrefixes []string
	FilePrefixes   []string
	SocketFamilies []string
	SocketAddrs    []string
	SocketPorts    []string
}

type CollectionBehaviorCapability struct {
	Behavior             string
	SensorMapping        string
	Fields               []string
	PushdownSelectors    []string
	AgentSideSelectors   []string
	UnsupportedSelectors []string
}

func NormalizeScope(scopeType, scopeSelector string) (string, string, error) {
	scopeType = strings.TrimSpace(scopeType)
	scopeSelector = strings.TrimSpace(scopeSelector)
	if scopeType == "" {
		scopeType = "host"
	}
	switch scopeType {
	case "host":
		if scopeSelector != "" {
			return "", "", fmt.Errorf("scope selector must be empty when scope type is host")
		}
	case "container", "cgroup", "namespace", "pod":
		if scopeSelector == "" {
			return "", "", fmt.Errorf("scope selector is required when scope type is %s", scopeType)
		}
	default:
		return "", "", fmt.Errorf("scope type must be one of host, container, cgroup, namespace, pod")
	}
	return scopeType, scopeSelector, nil
}

func ValidateScope(scopeType, scopeSelector string) error {
	_, _, err := NormalizeScope(scopeType, scopeSelector)
	return err
}

func (i CollectionIntent) NormalizeScope() (CollectionIntent, error) {
	scopeType, scopeSelector, err := NormalizeScope(i.ScopeType, i.ScopeSelector)
	if err != nil {
		return CollectionIntent{}, err
	}
	i.ScopeType = scopeType
	i.ScopeSelector = scopeSelector
	return i, nil
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
