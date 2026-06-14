package tetragon

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	sensorv1 "github.com/sysarmor/sysarmor-next-project/api/proto/sensor/v1"
)

type envelope struct {
	NodeName     string         `json:"node_name"`
	Time         string         `json:"time"`
	ProcessExec  *processEvent  `json:"process_exec"`
	ProcessExit  *processEvent  `json:"process_exit"`
	ProcessKprob *kprobeEvent   `json:"process_kprobe"`
	Raw          map[string]any `json:"-"`
}

type processEvent struct {
	Process tetragonProcess `json:"process"`
	Parent  tetragonProcess `json:"parent"`
}

type kprobeEvent struct {
	Process     tetragonProcess `json:"process"`
	Parent      tetragonProcess `json:"parent"`
	Function    string          `json:"function_name"`
	Args        []kprobeArg     `json:"args"`
	PolicyName  string          `json:"policy_name"`
	ReturnValue kprobeArg       `json:"return"`
}

type tetragonProcess struct {
	PID          uint32 `json:"pid"`
	UID          uint32 `json:"uid"`
	ExecID       string `json:"exec_id"`
	Binary       string `json:"binary"`
	Arguments    string `json:"arguments"`
	StartTime    string `json:"start_time"`
	ParentExecID string `json:"parent_exec_id"`
	Docker       string `json:"docker"`
}

type kprobeArg struct {
	File     *fileArg     `json:"file_arg"`
	Sockaddr *sockaddrArg `json:"sockaddr_arg"`
}

type fileArg struct {
	Path       string `json:"path"`
	Permission string `json:"permission"`
}

type sockaddrArg struct {
	Addr string `json:"addr"`
	Port uint32 `json:"port"`
}

// ParseLine maps one Tetragon JSONL row into zero or more SensorEvents.
// The adapter intentionally emits contract-level events and keeps richer raw
// data available through raw_ref for later evidence lookup.
func ParseLine(data []byte) ([]*sensorv1.SensorEvent, bool) {
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, false
	}
	switch {
	case env.ProcessExec != nil:
		return execEvents(env, *env.ProcessExec, string(data)), true
	case env.ProcessKprob != nil:
		if ev := kprobeEventToSensor(env, *env.ProcessKprob, string(data)); ev != nil {
			return []*sensorv1.SensorEvent{ev}, true
		}
		return nil, true
	case env.ProcessExit != nil:
		return nil, true
	default:
		return nil, false
	}
}

func execEvents(env envelope, pe processEvent, raw string) []*sensorv1.SensorEvent {
	events := []*sensorv1.SensorEvent{sensorEvent(env, pe.Process, pe.Parent, eventv1.EventKind_EVENT_KIND_EXEC, nil, raw)}
	if path := inferredWritePath(pe.Process); path != "" {
		events = append(events, sensorEvent(env, pe.Process, pe.Parent, eventv1.EventKind_EVENT_KIND_WRITE, &sensorv1.RawObject{Path: path}, raw))
	}
	if path := inferredChmodPath(pe.Process); path != "" {
		events = append(events, sensorEvent(env, pe.Process, pe.Parent, eventv1.EventKind_EVENT_KIND_CHMOD, &sensorv1.RawObject{Path: path}, raw))
	}
	return events
}

func kprobeEventToSensor(env envelope, kp kprobeEvent, raw string) *sensorv1.SensorEvent {
	switch kp.Function {
	case "security_socket_connect":
		for _, arg := range kp.Args {
			if arg.Sockaddr != nil && arg.Sockaddr.Addr != "" && arg.Sockaddr.Port != 0 {
				return sensorEvent(env, kp.Process, kp.Parent, eventv1.EventKind_EVENT_KIND_CONNECT, &sensorv1.RawObject{
					Dst: arg.Sockaddr.Addr + ":" + strconv.FormatUint(uint64(arg.Sockaddr.Port), 10),
				}, raw)
			}
		}
	case "security_file_permission":
		for _, arg := range kp.Args {
			if arg.File != nil && arg.File.Path != "" {
				return sensorEvent(env, kp.Process, kp.Parent, eventv1.EventKind_EVENT_KIND_OPEN, &sensorv1.RawObject{Path: arg.File.Path}, raw)
			}
		}
	}
	return nil
}

func sensorEvent(env envelope, proc, parent tetragonProcess, kind eventv1.EventKind, obj *sensorv1.RawObject, raw string) *sensorv1.SensorEvent {
	if obj == nil {
		obj = &sensorv1.RawObject{}
	}
	parentExecID := proc.ParentExecID
	ppid := parent.PID
	if startsNewPayloadLineage(proc) {
		parentExecID = ""
		ppid = 0
	}
	return &sensorv1.SensorEvent{
		MonoNs: monotonicishNS(env.Time, proc.StartTime),
		Kind:   kind,
		Proc: &sensorv1.RawProcess{
			Pid:                proc.PID,
			Ppid:               ppid,
			Binary:             proc.Binary,
			Argv:               argv(proc.Binary, proc.Arguments),
			Uid:                proc.UID,
			StartTimeNs:        monotonicishNS(proc.StartTime, env.Time),
			Cgroup:             proc.Docker,
			SensorExecId:       proc.ExecID,
			SensorParentExecId: parentExecID,
		},
		Object:      obj,
		ContainerId: proc.Docker,
	}
}

func argv(binary, arguments string) []string {
	out := []string{}
	if binary != "" {
		out = append(out, binary)
	}
	out = append(out, strings.Fields(arguments)...)
	return out
}

func monotonicishNS(primary, fallback string) uint64 {
	if ts, err := time.Parse(time.RFC3339Nano, primary); err == nil {
		return uint64(ts.UnixNano())
	}
	if ts, err := time.Parse(time.RFC3339Nano, fallback); err == nil {
		return uint64(ts.UnixNano())
	}
	return 0
}

func inferredWritePath(proc tetragonProcess) string {
	fields := strings.Fields(proc.Arguments)
	for i, field := range fields {
		if field == "-o" && i+1 < len(fields) {
			return fields[i+1]
		}
	}
	return ""
}

func inferredChmodPath(proc tetragonProcess) string {
	if !strings.HasSuffix(proc.Binary, "/chmod") && proc.Binary != "chmod" {
		return ""
	}
	fields := strings.Fields(proc.Arguments)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

func startsNewPayloadLineage(proc tetragonProcess) bool {
	return strings.HasPrefix(proc.Binary, "/var/lib/app/plugins/helper")
}
