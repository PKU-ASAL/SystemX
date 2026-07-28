package normalize

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"

	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	sensorv1 "github.com/sysarmor/sysarmor-next-project/api/proto/sensor/v1"
	endpointctx "github.com/sysarmor/sysarmor-next-project/internal/endpoint/context"
	"github.com/sysarmor/sysarmor-next-project/internal/eventmodel"
)

type Normalizer struct {
	identity      atomic.Pointer[Identity]
	scopeType     string
	scopeSelector string
	labels        map[string]string
	table         *endpointctx.Table
	seq           atomic.Uint64
}

type Identity struct {
	AgentID  string
	HostID   string
	TenantID string
}

type Options struct {
	TenantID        string
	ScopeType       string
	ScopeSelector   string
	Labels          map[string]string
	InitialSequence uint64
}

func New(agentID, hostID string, table *endpointctx.Table) *Normalizer {
	return NewWithOptions(agentID, hostID, table, Options{})
}

func NewWithOptions(agentID, hostID string, table *endpointctx.Table, opts Options) *Normalizer {
	if table == nil {
		table = endpointctx.NewTable()
	}
	if opts.ScopeType == "" {
		opts.ScopeType = "host"
	}
	n := &Normalizer{
		scopeType:     opts.ScopeType,
		scopeSelector: opts.ScopeSelector,
		labels:        cloneLabels(opts.Labels),
		table:         table,
	}
	n.seq.Store(opts.InitialSequence)
	n.SetIdentity(agentID, hostID, opts.TenantID)
	return n
}

func (n *Normalizer) SetIdentity(agentID, hostID, tenantID string) {
	n.identity.Store(&Identity{AgentID: agentID, HostID: hostID, TenantID: tenantID})
}

func (n *Normalizer) Normalize(ev *sensorv1.SensorEvent) *eventv1.CanonicalEvent {
	identity := n.identity.Load()
	seq := n.seq.Add(1)
	stableID := StableID(identity.HostID, ev.GetProc().GetPid(), ev.GetProc().GetStartTimeNs())
	if ev.GetProc().GetSensorExecId() != "" {
		stableID = StableIDFromSensor(identity.HostID, ev.GetProc().GetSensorExecId())
	}
	parentStableID := ""
	lineageID := stableID
	if parent, ok := n.parentProcess(ev); ok {
		parentStableID = parent.StableID
		if parent.LineageID != "" {
			lineageID = parent.LineageID
		}
	}
	proc := endpointctx.Process{
		StableID:     stableID,
		SensorExecID: ev.GetProc().GetSensorExecId(),
		PID:          ev.GetProc().GetPid(),
		PPID:         ev.GetProc().GetPpid(),
		Binary:       ev.GetProc().GetBinary(),
		LineageID:    lineageID,
	}
	n.table.Upsert(proc)

	return &eventv1.CanonicalEvent{
		Id:           EventID(identity.AgentID, seq),
		Seq:          seq,
		AgentId:      identity.AgentID,
		HostId:       identity.HostID,
		TenantId:     identity.TenantID,
		MonoNs:       ev.GetMonoNs(),
		OccurredAtNs: ev.GetMonoNs(),
		Behavior:     eventBehavior(ev),
		SubjectProc: &eventv1.ProcessRef{
			StableId:              stableID,
			Pid:                   ev.GetProc().GetPid(),
			Binary:                cleanBinary(ev.GetProc().GetBinary()),
			Argv:                  ev.GetProc().GetArgv(),
			ArgvBoundariesTrusted: ev.GetProc().GetArgvBoundariesTrusted(),
			Uid:                   ev.GetProc().GetUid(),
			StartTimeNs:           ev.GetProc().GetStartTimeNs(),
		},
		Object:         objectRef(ev),
		ParentStableId: parentStableID,
		LineageId:      lineageID,
		RawRef:         ev.GetRawRef(),
		Scope:          &eventv1.RuntimeScope{Type: n.scopeType, Selector: n.scopeSelector},
		ContainerId:    ev.GetContainerId(),
		Cgroup:         ev.GetProc().GetCgroup(),
		Labels:         cloneLabels(n.labels),
	}
}

func cloneLabels(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
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

func eventBehavior(ev *sensorv1.SensorEvent) string {
	if behavior := strings.TrimSpace(ev.GetBehavior()); behavior != "" {
		return eventmodel.NormalizeBehavior(behavior).String()
	}
	return ""
}

func (n *Normalizer) parentProcess(ev *sensorv1.SensorEvent) (endpointctx.Process, bool) {
	if ev.GetProc().GetSensorParentExecId() != "" {
		if parent, ok := n.table.BySensorExecID(ev.GetProc().GetSensorParentExecId()); ok {
			return parent, true
		}
	}
	return n.table.ByPID(ev.GetProc().GetPpid())
}

func StableID(hostID string, pid uint32, startNS uint64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d", hostID, pid, startNS)))
	return hex.EncodeToString(sum[:12])
}

func StableIDFromSensor(hostID, execID string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%s", hostID, execID)))
	return hex.EncodeToString(sum[:12])
}

func EventID(agentID string, seq uint64) string {
	return fmt.Sprintf("%s-%020d", agentID, seq)
}

func objectRef(ev *sensorv1.SensorEvent) *eventv1.ObjectRef {
	obj := ev.GetObject()
	switch eventBehavior(ev) {
	case "network.connect":
		return &eventv1.ObjectRef{Kind: "socket", SocketAddr: obj.GetDst()}
	case "file.open", "file.read", "file.write", "file.chmod":
		return &eventv1.ObjectRef{Kind: "file", FilePath: obj.GetPath()}
	default:
		return &eventv1.ObjectRef{Kind: "process"}
	}
}

func cleanBinary(binary string) string {
	if binary == "" {
		return ""
	}
	if strings.Contains(binary, "/") {
		return filepath.Clean(binary)
	}
	return binary
}
