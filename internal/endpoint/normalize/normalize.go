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
)

type Normalizer struct {
	agentID string
	hostID  string
	table   *endpointctx.Table
	seq     atomic.Uint64
}

func New(agentID, hostID string, table *endpointctx.Table) *Normalizer {
	if table == nil {
		table = endpointctx.NewTable()
	}
	return &Normalizer{agentID: agentID, hostID: hostID, table: table}
}

func (n *Normalizer) Normalize(ev *sensorv1.SensorEvent) *eventv1.CanonicalEvent {
	seq := n.seq.Add(1)
	stableID := StableID(n.hostID, ev.GetProc().GetPid(), ev.GetProc().GetStartTimeNs())
	if ev.GetProc().GetSensorExecId() != "" {
		stableID = StableIDFromSensor(n.hostID, ev.GetProc().GetSensorExecId())
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
		Id:       EventID(n.agentID, seq),
		Seq:      seq,
		AgentId:  n.agentID,
		HostId:   n.hostID,
		MonoNs:   ev.GetMonoNs(),
		Kind:     ev.GetKind(),
		Scenario: "",
		SubjectProc: &eventv1.ProcessRef{
			StableId:    stableID,
			Pid:         ev.GetProc().GetPid(),
			Binary:      cleanBinary(ev.GetProc().GetBinary()),
			Argv:        ev.GetProc().GetArgv(),
			Uid:         ev.GetProc().GetUid(),
			StartTimeNs: ev.GetProc().GetStartTimeNs(),
		},
		Object:         objectRef(ev),
		ParentStableId: parentStableID,
		LineageId:      lineageID,
		RawRef:         ev.GetRawRef(),
	}
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
	switch ev.GetKind() {
	case eventv1.EventKind_EVENT_KIND_CONNECT:
		return &eventv1.ObjectRef{Kind: "socket", SocketAddr: obj.GetDst()}
	case eventv1.EventKind_EVENT_KIND_OPEN, eventv1.EventKind_EVENT_KIND_WRITE, eventv1.EventKind_EVENT_KIND_CHMOD:
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
