package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	incidentv1 "github.com/sysarmor/sysarmor-next-project/api/proto/incident/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
	"google.golang.org/protobuf/encoding/protojson"
)

type Store struct {
	mu        sync.RWMutex
	path      string
	Agents    []*analyticsv1.AgentHello
	Events    []*eventv1.CanonicalEvent
	Signals   []*signalv1.Signal
	Incidents []*incidentv1.Incident
	Health    map[string]agenthealth.AgentHealth
	Metrics   Metrics
}

type Metrics struct {
	UploadBatches             uint64  `json:"upload_batches"`
	EventsIngested            uint64  `json:"events_ingested"`
	EndpointSignalsIngested   uint64  `json:"endpoint_signals_ingested"`
	CloudSignalsEmitted       uint64  `json:"cloud_signals_emitted"`
	SignalsEmitted            uint64  `json:"signals_emitted"`
	IncidentsCreated          uint64  `json:"incidents_created"`
	DroppedEvents             uint64  `json:"dropped_events"`
	DuplicateEvents           uint64  `json:"duplicate_events"`
	LastConvergenceLatencyMs  uint64  `json:"last_convergence_latency_ms"`
	MaxConvergenceLatencyMs   uint64  `json:"max_convergence_latency_ms"`
	TotalConvergenceLatencyMs uint64  `json:"total_convergence_latency_ms"`
	AverageConvergenceLatency float64 `json:"average_convergence_latency_ms"`
}

type diskState struct {
	Agents    []json.RawMessage `json:"agents"`
	Events    []json.RawMessage `json:"events"`
	Signals   []json.RawMessage `json:"signals"`
	Incidents []json.RawMessage `json:"incidents"`
	Health    []json.RawMessage `json:"health"`
	Metrics   Metrics           `json:"metrics"`
}

func Open(path string) (*Store, error) {
	s := &Store{path: path, Health: map[string]agenthealth.AgentHealth{}}
	if path == "" {
		return s, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var state diskState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	for _, raw := range state.Agents {
		msg := &analyticsv1.AgentHello{}
		if err := protojson.Unmarshal(raw, msg); err != nil {
			return nil, err
		}
		s.Agents = append(s.Agents, msg)
	}
	for _, raw := range state.Events {
		msg := &eventv1.CanonicalEvent{}
		if err := protojson.Unmarshal(raw, msg); err != nil {
			return nil, err
		}
		s.Events = append(s.Events, msg)
	}
	for _, raw := range state.Signals {
		msg := &signalv1.Signal{}
		if err := protojson.Unmarshal(raw, msg); err != nil {
			return nil, err
		}
		s.Signals = append(s.Signals, msg)
	}
	for _, raw := range state.Incidents {
		msg := &incidentv1.Incident{}
		if err := protojson.Unmarshal(raw, msg); err != nil {
			return nil, err
		}
		s.Incidents = append(s.Incidents, msg)
	}
	for _, raw := range state.Health {
		var msg agenthealth.AgentHealth
		if err := json.Unmarshal(raw, &msg); err != nil {
			return nil, err
		}
		s.Health[agentHealthKey(msg.TenantID, msg.AgentID)] = msg
	}
	s.Metrics = state.Metrics
	return s, nil
}

func (s *Store) AddAgent(agent *analyticsv1.AgentHello) {
	if agent == nil || agent.GetAgentId() == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.Agents {
		if existing.GetTenantId() == agent.GetTenantId() && existing.GetAgentId() == agent.GetAgentId() {
			s.Agents[i] = agent
			return
		}
	}
	s.Agents = append(s.Agents, agent)
}

func (s *Store) AddEvent(ev *eventv1.CanonicalEvent) bool {
	if ev == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if ev.GetId() != "" {
		for i, existing := range s.Events {
			if existing.GetId() == ev.GetId() {
				s.Events[i] = ev
				return false
			}
		}
	}
	s.Events = append(s.Events, ev)
	return true
}

func (s *Store) AddSignal(sig *signalv1.Signal) bool {
	if sig == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := signalKey(sig)
	for i, existing := range s.Signals {
		if key != "" && signalKey(existing) == key {
			s.Signals[i] = sig
			return false
		}
	}
	s.Signals = append(s.Signals, sig)
	return true
}

func (s *Store) AddIncident(inc *incidentv1.Incident) bool {
	if inc == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := incidentKey(inc)
	for i, existing := range s.Incidents {
		if key != "" && incidentKey(existing) == key {
			s.Incidents[i] = inc
			return false
		}
	}
	s.Incidents = append(s.Incidents, inc)
	return true
}

func (s *Store) UpsertAgentHealth(health agenthealth.AgentHealth) {
	if health.AgentID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Health == nil {
		s.Health = map[string]agenthealth.AgentHealth{}
	}
	s.Health[agentHealthKey(health.TenantID, health.AgentID)] = health
}

func (s *Store) ReplaceDerivedForScenario(scenario string, cloudSignals []*signalv1.Signal, incidents []*incidentv1.Incident) {
	if scenario == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	signals := s.Signals[:0]
	for _, sig := range s.Signals {
		if sig.GetScenario() == scenario && layerName(sig.GetWhere()) == "cloud" {
			continue
		}
		signals = append(signals, sig)
	}
	s.Signals = signals
	keptIncidents := s.Incidents[:0]
	for _, inc := range s.Incidents {
		if inc.GetScenario() == scenario {
			continue
		}
		keptIncidents = append(keptIncidents, inc)
	}
	s.Incidents = keptIncidents
	s.Signals = append(s.Signals, cloudSignals...)
	s.Incidents = append(s.Incidents, incidents...)
}

func (s *Store) RecordUpload(events, endpointSignals, cloudSignals, incidents int, convergenceLatency time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	latencyMs := uint64(convergenceLatency.Milliseconds())
	s.Metrics.UploadBatches++
	s.Metrics.EventsIngested += uint64(events)
	s.Metrics.EndpointSignalsIngested += uint64(endpointSignals)
	s.Metrics.CloudSignalsEmitted += uint64(cloudSignals)
	s.Metrics.SignalsEmitted += uint64(endpointSignals + cloudSignals)
	s.Metrics.IncidentsCreated += uint64(incidents)
	s.Metrics.LastConvergenceLatencyMs = latencyMs
	s.Metrics.TotalConvergenceLatencyMs += latencyMs
	if latencyMs > s.Metrics.MaxConvergenceLatencyMs {
		s.Metrics.MaxConvergenceLatencyMs = latencyMs
	}
	s.Metrics.AverageConvergenceLatency = float64(s.Metrics.TotalConvergenceLatencyMs) / float64(s.Metrics.UploadBatches)
}

func (s *Store) ListAgents() []*analyticsv1.AgentHello {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*analyticsv1.AgentHello, len(s.Agents))
	copy(out, s.Agents)
	return out
}

func (s *Store) ListAgentHealth() []agenthealth.AgentHealth {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]agenthealth.AgentHealth, 0, len(s.Health))
	for _, health := range s.Health {
		out = append(out, health)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TenantID == out[j].TenantID {
			return out[i].AgentID < out[j].AgentID
		}
		return out[i].TenantID < out[j].TenantID
	})
	return out
}

func (s *Store) GetAgentHealth(tenantID, agentID string) (agenthealth.AgentHealth, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if agentID == "" {
		return agenthealth.AgentHealth{}, false
	}
	if tenantID != "" {
		health, ok := s.Health[agentHealthKey(tenantID, agentID)]
		return health, ok
	}
	var found agenthealth.AgentHealth
	var ok bool
	for _, health := range s.Health {
		if health.AgentID == agentID {
			if ok && found.TenantID != health.TenantID {
				return agenthealth.AgentHealth{}, false
			}
			found = health
			ok = true
		}
	}
	return found, ok
}

func (s *Store) ListEvents(scenario, kind string) []*eventv1.CanonicalEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*eventv1.CanonicalEvent, 0, len(s.Events))
	for _, ev := range s.Events {
		if scenario != "" && ev.GetScenario() != scenario {
			continue
		}
		if kind != "" && kindName(ev.GetKind()) != kind {
			continue
		}
		out = append(out, ev)
	}
	return out
}

func (s *Store) ListSignals(scenario, layer string, terminalOnly bool) []*signalv1.Signal {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*signalv1.Signal, 0, len(s.Signals))
	for _, sig := range s.Signals {
		if scenario != "" && sig.GetScenario() != scenario {
			continue
		}
		if terminalOnly && !sig.GetTerminal() {
			continue
		}
		if layer != "" && layerName(sig.GetWhere()) != layer {
			continue
		}
		out = append(out, sig)
	}
	return out
}

func (s *Store) ListIncidents(scenario string) []*incidentv1.Incident {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*incidentv1.Incident, 0, len(s.Incidents))
	for _, inc := range s.Incidents {
		if scenario != "" && inc.GetScenario() != scenario {
			continue
		}
		out = append(out, inc)
	}
	return out
}

func (s *Store) MetricsSnapshot() Metrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Metrics
}

func (s *Store) DeleteScenario(scenario string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if scenario == "" {
		s.Agents = nil
		s.Events = nil
		s.Signals = nil
		s.Incidents = nil
		s.Health = map[string]agenthealth.AgentHealth{}
		s.Metrics = Metrics{}
		return
	}
	events := s.Events[:0]
	for _, ev := range s.Events {
		if ev.GetScenario() != scenario {
			events = append(events, ev)
		}
	}
	s.Events = events
	signals := s.Signals[:0]
	for _, sig := range s.Signals {
		if sig.GetScenario() != scenario {
			signals = append(signals, sig)
		}
	}
	s.Signals = signals
	incidents := s.Incidents[:0]
	for _, inc := range s.Incidents {
		if inc.GetScenario() != scenario {
			incidents = append(incidents, inc)
		}
	}
	s.Incidents = incidents
}

func (s *Store) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.path == "" {
		return nil
	}
	var state diskState
	state.Metrics = s.Metrics
	mo := protojson.MarshalOptions{UseProtoNames: true}
	for _, agent := range s.Agents {
		raw, err := mo.Marshal(agent)
		if err != nil {
			return err
		}
		state.Agents = append(state.Agents, raw)
	}
	for _, ev := range s.Events {
		raw, err := mo.Marshal(ev)
		if err != nil {
			return err
		}
		state.Events = append(state.Events, raw)
	}
	for _, sig := range s.Signals {
		raw, err := mo.Marshal(sig)
		if err != nil {
			return err
		}
		state.Signals = append(state.Signals, raw)
	}
	for _, inc := range s.Incidents {
		raw, err := mo.Marshal(inc)
		if err != nil {
			return err
		}
		state.Incidents = append(state.Incidents, raw)
	}
	health := make([]agenthealth.AgentHealth, 0, len(s.Health))
	for _, item := range s.Health {
		health = append(health, item)
	}
	sort.Slice(health, func(i, j int) bool {
		if health[i].TenantID == health[j].TenantID {
			return health[i].AgentID < health[j].AgentID
		}
		return health[i].TenantID < health[j].TenantID
	})
	for _, item := range health {
		raw, err := json.Marshal(item)
		if err != nil {
			return err
		}
		state.Health = append(state.Health, raw)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

func agentHealthKey(tenantID, agentID string) string {
	return tenantID + "/" + agentID
}

func kindName(kind eventv1.EventKind) string {
	switch kind {
	case eventv1.EventKind_EVENT_KIND_EXEC:
		return "EXEC"
	case eventv1.EventKind_EVENT_KIND_EXIT:
		return "EXIT"
	case eventv1.EventKind_EVENT_KIND_FORK:
		return "FORK"
	case eventv1.EventKind_EVENT_KIND_OPEN:
		return "OPEN"
	case eventv1.EventKind_EVENT_KIND_WRITE:
		return "WRITE"
	case eventv1.EventKind_EVENT_KIND_CHMOD:
		return "CHMOD"
	case eventv1.EventKind_EVENT_KIND_CONNECT:
		return "CONNECT"
	default:
		return ""
	}
}

func layerName(where signalv1.SignalWhere) string {
	switch where {
	case signalv1.SignalWhere_SIGNAL_WHERE_ENDPOINT:
		return "endpoint"
	case signalv1.SignalWhere_SIGNAL_WHERE_CLOUD:
		return "cloud"
	default:
		return ""
	}
}

func signalKey(sig *signalv1.Signal) string {
	if sig == nil {
		return ""
	}
	parts := []string{
		sig.GetScenario(),
		layerName(sig.GetWhere()),
		sig.GetName(),
		sig.GetLineageId(),
		boolString(sig.GetTerminal()),
	}
	parts = append(parts, sortedStrings(sig.GetEventRefs())...)
	parts = append(parts, sortedStrings(sig.GetSignalRefs())...)
	for _, ent := range sortedEntities(sig.GetEntities()) {
		parts = append(parts, ent)
	}
	return stableKey(parts...)
}

func incidentKey(inc *incidentv1.Incident) string {
	if inc == nil {
		return ""
	}
	parts := []string{
		inc.GetScenario(),
		inc.GetSummary(),
		inc.GetConverge().GetMethod(),
	}
	parts = append(parts, sortedStrings(inc.GetLineageIds())...)
	parts = append(parts, sortedStrings(inc.GetTerminals())...)
	for _, sig := range inc.GetContributingSignals() {
		parts = append(parts, signalKey(sig))
	}
	return stableKey(parts...)
}

func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func sortedEntities(in []*signalv1.EntityRef) []string {
	out := make([]string, 0, len(in))
	for _, ent := range in {
		out = append(out, strings.Join([]string{ent.GetKind(), ent.GetKey(), ent.GetRole()}, "\x00"))
	}
	sort.Strings(out)
	return out
}

func stableKey(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:16])
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
