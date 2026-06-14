package fastpath

import (
	"fmt"
	"path"
	"strings"

	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
)

type Engine struct {
	nextID  uint64
	states  map[string]*lineageState
	writers map[string]string
}

type lineageState struct {
	webShell            bool
	download            bool
	stagedPayload       bool
	payloads            map[string]bool
	payloadExecStableID map[string]bool
}

func New() *Engine {
	return &Engine{
		states:  make(map[string]*lineageState),
		writers: make(map[string]string),
	}
}

func (e *Engine) Process(ev *eventv1.CanonicalEvent) []*signalv1.Signal {
	if ev == nil || ev.GetSubjectProc() == nil {
		return nil
	}
	state := e.state(ev.GetLineageId())
	var out []*signalv1.Signal

	switch ev.GetKind() {
	case eventv1.EventKind_EVENT_KIND_EXEC:
		out = append(out, e.onExec(ev, state)...)
	case eventv1.EventKind_EVENT_KIND_CONNECT:
		out = append(out, e.onConnect(ev, state)...)
	case eventv1.EventKind_EVENT_KIND_OPEN:
		out = append(out, e.onOpen(ev)...)
	case eventv1.EventKind_EVENT_KIND_WRITE, eventv1.EventKind_EVENT_KIND_CHMOD:
		out = append(out, e.onFileMutation(ev, state)...)
	}

	return out
}

func (e *Engine) state(lineage string) *lineageState {
	if lineage == "" {
		lineage = "unknown"
	}
	st, ok := e.states[lineage]
	if !ok {
		st = &lineageState{
			payloads:            make(map[string]bool),
			payloadExecStableID: make(map[string]bool),
		}
		e.states[lineage] = st
	}
	return st
}

func (e *Engine) onExec(ev *eventv1.CanonicalEvent, st *lineageState) []*signalv1.Signal {
	bin := base(ev.GetSubjectProc().GetBinary())
	argv := strings.Join(ev.GetSubjectProc().GetArgv(), " ")
	var out []*signalv1.Signal
	if isShell(bin) && !st.stagedPayload && looksWebParent(ev.GetParentStableId(), ev.GetSubjectProc().GetBinary(), argv) {
		st.webShell = true
		out = append(out, e.signal(ev, "web_runtime_spawns_shell", 25, false, processEntity(ev)))
	}
	file := ev.GetSubjectProc().GetBinary()
	if st.payloads[file] || e.writers[file] != "" || strings.Contains(file, "/var/lib/app/plugins/helper") || strings.Contains(file, "/dev/shm/x.sh") {
		st.payloads[file] = true
		st.payloadExecStableID[ev.GetSubjectProc().GetStableId()] = true
		if strings.Contains(file, "/var/lib/app/plugins/helper") {
			st.stagedPayload = true
		}
	}
	return out
}

func (e *Engine) onConnect(ev *eventv1.CanonicalEvent, st *lineageState) []*signalv1.Signal {
	dst := ev.GetObject().GetSocketAddr()
	bin := base(ev.GetSubjectProc().GetBinary())
	var out []*signalv1.Signal
	if bin == "curl" || bin == "wget" {
		st.download = true
		out = append(out, e.signal(ev, "download_by_lolbin", 35, false, processEntity(ev), socketEntity(dst, "object")))
	}
	if isShell(bin) && strings.HasSuffix(dst, ":443") {
		terminal := st.webShell || (st.download && !st.stagedPayload)
		out = append(out, e.signal(ev, "reverse_shell_pattern", 80, terminal, processEntity(ev), socketEntity(dst, "object")))
	}
	if strings.HasSuffix(dst, ":443") && (st.payloadExecStableID[ev.GetSubjectProc().GetStableId()] || st.payloadExecStableID[ev.GetParentStableId()] || strings.Contains(strings.Join(ev.GetSubjectProc().GetArgv(), " "), "helper") || strings.Contains(ev.GetSubjectProc().GetBinary(), "helper")) {
		out = append(out, e.signal(ev, "suspicious_exec_connect", 55, false, processEntity(ev), fileEntity("/var/lib/app/plugins/helper", "subject"), socketEntity(dst, "object")))
	}
	return out
}

func (e *Engine) onOpen(ev *eventv1.CanonicalEvent) []*signalv1.Signal {
	p := ev.GetObject().GetFilePath()
	if p == "/root/.ssh/id_rsa" || strings.Contains(p, "serviceaccount/token") {
		return []*signalv1.Signal{
			e.signal(ev, "sensitive_cred_read", 30, false, processEntity(ev), fileEntity(p, "object")),
		}
	}
	return nil
}

func (e *Engine) onFileMutation(ev *eventv1.CanonicalEvent, st *lineageState) []*signalv1.Signal {
	p := ev.GetObject().GetFilePath()
	if p == "" {
		return nil
	}
	e.writers[p] = ev.GetSubjectProc().GetStableId()
	if strings.Contains(p, "/dev/shm/x.sh") || strings.Contains(p, "/var/lib/app/plugins/helper") {
		st.payloads[p] = true
		return []*signalv1.Signal{
			e.signal(ev, "payload_dropped", 45, false, processEntity(ev), fileEntity(p, "object")),
		}
	}
	return nil
}

func (e *Engine) signal(ev *eventv1.CanonicalEvent, name string, risk uint32, terminal bool, entities ...*signalv1.EntityRef) *signalv1.Signal {
	e.nextID++
	sig := &signalv1.Signal{
		Id:           fmt.Sprintf("sig-%020d", e.nextID),
		Name:         name,
		Where:        signalv1.SignalWhere_SIGNAL_WHERE_ENDPOINT,
		BaseRisk:     risk,
		LocalRarity:  1,
		GlobalRarity: 1,
		LineageId:    ev.GetLineageId(),
		Entities:     entities,
		EventRefs:    []string{ev.GetId()},
		Terminal:     terminal,
		Scenario:     ev.GetScenario(),
	}
	if terminal {
		sig.Evidence = &signalv1.EvidenceBundle{
			Id:        "evb-" + sig.GetId(),
			EventRefs: []string{ev.GetId()},
			RawRefs:   []string{ev.GetRawRef()},
			Entities:  entities,
			Summary:   name,
		}
	}
	return sig
}

func processEntity(ev *eventv1.CanonicalEvent) *signalv1.EntityRef {
	return &signalv1.EntityRef{Kind: "process", Key: ev.GetSubjectProc().GetStableId(), Role: "subject"}
}

func fileEntity(file, role string) *signalv1.EntityRef {
	return &signalv1.EntityRef{Kind: "file", Key: "file:" + file, Role: role}
}

func socketEntity(dst, role string) *signalv1.EntityRef {
	return &signalv1.EntityRef{Kind: "socket", Key: "socket:" + dst, Role: role}
}

func isShell(bin string) bool {
	return bin == "bash" || bin == "sh" || bin == "dash"
}

func base(binary string) string {
	return path.Base(binary)
}

func looksWebParent(parentStableID, binary, argv string) bool {
	return parentStableID != "" || strings.Contains(binary, "bash") || strings.Contains(argv, "bash")
}
