package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	gatewaymodel "github.com/sysarmor/sysarmor-next-project/internal/agentgateway/model"
	"github.com/sysarmor/sysarmor-next-project/internal/analytics/rarity"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
	responsemodel "github.com/sysarmor/sysarmor-next-project/internal/response"
	"github.com/sysarmor/sysarmor-next-project/internal/store/migrations"
	"google.golang.org/protobuf/encoding/protojson"
)

const FileStoreStateVersion = 1

type Store struct {
	mu                   sync.RWMutex
	path                 string
	backendInfo          *Info
	saveState            func(State) error
	listEvents           func(scenario, behavior string) ([]*eventv1.CanonicalEvent, error)
	listSignals          func(scenario, layer string, terminalOnly bool) ([]*signalv1.Signal, error)
	listIncidents        func(scenario string) ([]*incidentv1.Incident, error)
	listResponses        func(tenantID, agentID string) ([]responsemodel.AuditRecord, error)
	listPolicies         func(tenantID string) ([]policymodel.Policy, error)
	listAssignments      func(tenantID, agentID string) ([]policymodel.Assignment, error)
	listPolicyAudits     func(tenantID, policyID string) ([]policymodel.AuditRecord, error)
	getPolicy            func(tenantID, policyID string, version uint64) (policymodel.Policy, bool, error)
	effectivePolicy      func(tenantID, agentID, scopeType, scopeSelector string) (policymodel.Policy, bool, error)
	writeResponse        func(responsemodel.Command, *responsemodel.Ack) error
	writePolicy          func(policymodel.Policy) error
	writeAssignment      func(policymodel.Assignment) error
	writePolicyAudit     func(policymodel.AuditRecord) error
	Agents               []*analyticsv1.AgentHello
	Events               []*eventv1.CanonicalEvent
	Signals              []*signalv1.Signal
	Incidents            []*incidentv1.Incident
	Health               map[string]agenthealth.AgentHealth
	Rules                []policymodel.RuleContent
	Policies             []policymodel.Policy
	Assignments          []policymodel.Assignment
	PolicyAudits         []policymodel.AuditRecord
	Responses            []responsemodel.Command
	ResponseAcks         []responsemodel.Ack
	Pullbacks            []gatewaymodel.EvidencePullbackRequest
	AgentGatewaySessions []AgentGatewaySession
	OperatorRoles        []OperatorRoleBinding
	Metrics              Metrics
	RarityBaseline       rarity.Baseline
}

type Info struct {
	Backend          string `json:"backend"`
	Path             string `json:"path,omitempty"`
	StateVersion     int    `json:"state_version"`
	MigrationVersion int    `json:"migration_version"`
	PostgresSchema   int    `json:"postgres_schema_version"`
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

type AgentGatewaySession struct {
	SessionID     string    `json:"session_id"`
	TenantID      string    `json:"tenant_id"`
	AgentID       string    `json:"agent_id"`
	StartedAt     time.Time `json:"started_at"`
	LastSeenAt    time.Time `json:"last_seen_at"`
	ClosedAt      time.Time `json:"closed_at,omitempty"`
	LastAckCursor string    `json:"last_ack_cursor,omitempty"`
	Transport     string    `json:"transport,omitempty"`
	Status        string    `json:"status,omitempty"`
}

type OperatorRoleBinding struct {
	Actor     string    `json:"actor"`
	Roles     []string  `json:"roles"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type State struct {
	Agents               []json.RawMessage                      `json:"agents"`
	Events               []json.RawMessage                      `json:"events"`
	Signals              []json.RawMessage                      `json:"signals"`
	Incidents            []json.RawMessage                      `json:"incidents"`
	Health               []json.RawMessage                      `json:"health"`
	Rules                []policymodel.RuleContent              `json:"rules"`
	Policies             []policymodel.Policy                   `json:"policies"`
	Assignments          []policymodel.Assignment               `json:"assignments"`
	PolicyAudits         []policymodel.AuditRecord              `json:"policy_audits"`
	Responses            []responsemodel.Command                `json:"responses"`
	ResponseAcks         []responsemodel.Ack                    `json:"response_acks"`
	Pullbacks            []gatewaymodel.EvidencePullbackRequest `json:"evidence_pullbacks"`
	AgentGatewaySessions []AgentGatewaySession                  `json:"agent_gateway_sessions"`
	OperatorRoles        []OperatorRoleBinding                  `json:"operator_role_bindings,omitempty"`
	Metrics              Metrics                                `json:"metrics"`
	RarityBaseline       rarity.Baseline                        `json:"rarity_baseline,omitempty"`
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
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	if err := s.ImportState(state); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) ImportState(state State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Agents = nil
	s.Events = nil
	s.Signals = nil
	s.Incidents = nil
	s.Health = map[string]agenthealth.AgentHealth{}
	for _, raw := range state.Agents {
		msg := &analyticsv1.AgentHello{}
		if err := protojson.Unmarshal(raw, msg); err != nil {
			return err
		}
		s.Agents = append(s.Agents, msg)
	}
	for _, raw := range state.Events {
		msg := &eventv1.CanonicalEvent{}
		if err := protojson.Unmarshal(raw, msg); err != nil {
			return err
		}
		s.Events = append(s.Events, msg)
	}
	for _, raw := range state.Signals {
		msg := &signalv1.Signal{}
		if err := protojson.Unmarshal(raw, msg); err != nil {
			return err
		}
		s.Signals = append(s.Signals, msg)
	}
	for _, raw := range state.Incidents {
		msg := &incidentv1.Incident{}
		if err := protojson.Unmarshal(raw, msg); err != nil {
			return err
		}
		s.Incidents = append(s.Incidents, msg)
	}
	for _, raw := range state.Health {
		var msg agenthealth.AgentHealth
		if err := json.Unmarshal(raw, &msg); err != nil {
			return err
		}
		s.Health[agentHealthKey(msg.TenantID, msg.AgentID)] = msg
	}
	s.Rules = state.Rules
	s.Policies = state.Policies
	s.Assignments = state.Assignments
	s.PolicyAudits = state.PolicyAudits
	s.Responses = state.Responses
	s.ResponseAcks = state.ResponseAcks
	s.Pullbacks = state.Pullbacks
	s.AgentGatewaySessions = state.AgentGatewaySessions
	s.OperatorRoles = state.OperatorRoles
	s.Metrics = state.Metrics
	s.RarityBaseline = state.RarityBaseline.Snapshot()
	return nil
}

func (s *Store) ConfigureBackend(info Info, saveState func(State) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backendInfo = &info
	s.saveState = saveState
}

func (s *Store) ConfigureQueryHooks(
	listEvents func(string, string) ([]*eventv1.CanonicalEvent, error),
	listSignals func(string, string, bool) ([]*signalv1.Signal, error),
	listIncidents func(string) ([]*incidentv1.Incident, error),
	listResponses func(string, string) ([]responsemodel.AuditRecord, error),
	listPolicies func(string) ([]policymodel.Policy, error),
	listAssignments func(string, string) ([]policymodel.Assignment, error),
	listPolicyAudits func(string, string) ([]policymodel.AuditRecord, error),
	getPolicy func(string, string, uint64) (policymodel.Policy, bool, error),
	effectivePolicy func(string, string, string, string) (policymodel.Policy, bool, error),
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listEvents = listEvents
	s.listSignals = listSignals
	s.listIncidents = listIncidents
	s.listResponses = listResponses
	s.listPolicies = listPolicies
	s.listAssignments = listAssignments
	s.listPolicyAudits = listPolicyAudits
	s.getPolicy = getPolicy
	s.effectivePolicy = effectivePolicy
}

func (s *Store) ConfigureWriteHooks(
	writeResponse func(responsemodel.Command, *responsemodel.Ack) error,
	writePolicy func(policymodel.Policy) error,
	writeAssignment func(policymodel.Assignment) error,
	writePolicyAudit func(policymodel.AuditRecord) error,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writeResponse = writeResponse
	s.writePolicy = writePolicy
	s.writeAssignment = writeAssignment
	s.writePolicyAudit = writePolicyAudit
}

func (s *Store) Info() Info {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.backendInfo != nil {
		return *s.backendInfo
	}
	backend := "file"
	if s.path == "" {
		backend = "memory"
	}
	return Info{
		Backend:          backend,
		Path:             s.path,
		StateVersion:     FileStoreStateVersion,
		MigrationVersion: FileStoreStateVersion,
		PostgresSchema:   migrations.PostgresVersion,
	}
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
	defaultIncidentStatus(inc)
	s.mu.Lock()
	defer s.mu.Unlock()
	key := incidentKey(inc)
	for i, existing := range s.Incidents {
		if key != "" && incidentKey(existing) == key {
			if existing.GetStatus() != "" {
				inc.Status = existing.GetStatus()
				inc.StatusReason = existing.GetStatusReason()
				inc.StatusActor = existing.GetStatusActor()
			}
			inc.Evidence = mergeEvidence(inc.GetEvidence(), existing.GetEvidence())
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

func (s *Store) EnsureDefaultPolicy(tenantID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.Rules) == 0 {
		s.Rules = policymodel.DefaultRules()
	}
	for _, policy := range s.Policies {
		if policy.TenantID == tenantID && policy.PolicyID == policymodel.DefaultPolicyID && policy.Version == policymodel.DefaultPolicyVersion {
			return
		}
	}
	s.Policies = append(s.Policies, policymodel.Normalize(policymodel.DefaultPolicy(tenantID)))
}

func (s *Store) UpsertRule(rule policymodel.RuleContent) {
	if rule.RuleID == "" {
		return
	}
	if rule.Version == 0 {
		rule.Version = 1
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.Rules {
		if existing.RuleID == rule.RuleID && existing.Version == rule.Version {
			s.Rules[i] = rule
			return
		}
	}
	s.Rules = append(s.Rules, rule)
}

func (s *Store) ListRules(where string) []policymodel.RuleContent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]policymodel.RuleContent, 0, len(s.Rules))
	for _, rule := range s.Rules {
		if where != "" && rule.Where != where {
			continue
		}
		out = append(out, rule)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Where == out[j].Where {
			if out[i].RuleID == out[j].RuleID {
				return out[i].Version < out[j].Version
			}
			return out[i].RuleID < out[j].RuleID
		}
		return out[i].Where < out[j].Where
	})
	return out
}

func (s *Store) UpsertPolicy(policy policymodel.Policy) policymodel.Policy {
	if policy.PolicyID == "" {
		return policymodel.Policy{}
	}
	policy = policymodel.Normalize(policy)
	s.mu.Lock()
	for i, existing := range s.Policies {
		if existing.TenantID == policy.TenantID && existing.PolicyID == policy.PolicyID && existing.Version == policy.Version {
			if existing.CreatedAt.IsZero() {
				existing.CreatedAt = policy.CreatedAt
			}
			policy.CreatedAt = existing.CreatedAt
			s.Policies[i] = policy
			writePolicy := s.writePolicy
			s.mu.Unlock()
			if writePolicy != nil {
				_ = writePolicy(policy)
			}
			return policy
		}
	}
	s.Policies = append(s.Policies, policy)
	writePolicy := s.writePolicy
	s.mu.Unlock()
	if writePolicy != nil {
		_ = writePolicy(policy)
	}
	return policy
}

func (s *Store) RecordPolicyAudit(record policymodel.AuditRecord) policymodel.AuditRecord {
	if record.TenantID == "" {
		record.TenantID = "default"
	}
	if record.Status == "" {
		record.Status = "ok"
	}
	now := time.Now().UTC()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	if record.AuditID == "" {
		record.AuditID = fmt.Sprintf("policy-audit-%d", now.UnixNano())
	}
	s.mu.Lock()
	s.PolicyAudits = append(s.PolicyAudits, record)
	writePolicyAudit := s.writePolicyAudit
	s.mu.Unlock()
	if writePolicyAudit != nil {
		_ = writePolicyAudit(record)
	}
	return record
}

func (s *Store) ListPolicyAudits(tenantID, policyID string) []policymodel.AuditRecord {
	s.mu.RLock()
	listPolicyAudits := s.listPolicyAudits
	s.mu.RUnlock()
	if listPolicyAudits != nil {
		audits, err := listPolicyAudits(tenantID, policyID)
		if err == nil && len(audits) > 0 {
			return audits
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]policymodel.AuditRecord, 0, len(s.PolicyAudits))
	for _, record := range s.PolicyAudits {
		if tenantID != "" && record.TenantID != tenantID {
			continue
		}
		if policyID != "" && record.PolicyID != policyID {
			continue
		}
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

func (s *Store) ListPolicies(tenantID string) []policymodel.Policy {
	s.mu.RLock()
	listPolicies := s.listPolicies
	s.mu.RUnlock()
	if listPolicies != nil {
		policies, err := listPolicies(tenantID)
		if err == nil && len(policies) > 0 {
			return policies
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]policymodel.Policy, 0, len(s.Policies))
	for _, policy := range s.Policies {
		if tenantID != "" && policy.TenantID != tenantID {
			continue
		}
		out = append(out, policy)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TenantID == out[j].TenantID {
			if out[i].PolicyID == out[j].PolicyID {
				return out[i].Version < out[j].Version
			}
			return out[i].PolicyID < out[j].PolicyID
		}
		return out[i].TenantID < out[j].TenantID
	})
	return out
}

func (s *Store) GetPolicy(tenantID, policyID string, version uint64) (policymodel.Policy, bool) {
	s.mu.RLock()
	getPolicy := s.getPolicy
	s.mu.RUnlock()
	if getPolicy != nil {
		policy, ok, err := getPolicy(tenantID, policyID, version)
		if err == nil && ok {
			return policy, true
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var latest policymodel.Policy
	var ok bool
	for _, policy := range s.Policies {
		if tenantID != "" && policy.TenantID != tenantID {
			continue
		}
		if policy.PolicyID != policyID {
			continue
		}
		if version != 0 && policy.Version != version {
			continue
		}
		if version != 0 {
			return policy, true
		}
		if !ok || policy.Version > latest.Version {
			latest = policy
			ok = true
		}
	}
	return latest, ok
}

func (s *Store) PublishPolicy(tenantID, policyID string, version uint64, published bool) (policymodel.Policy, bool) {
	if tenantID == "" {
		tenantID = "default"
	}
	if policyID == "" {
		return policymodel.Policy{}, false
	}
	s.mu.Lock()
	for i, policy := range s.Policies {
		if policy.TenantID != tenantID || policy.PolicyID != policyID {
			continue
		}
		if version != 0 && policy.Version != version {
			continue
		}
		policy.Published = published
		policy.UpdatedAt = time.Now().UTC()
		s.Policies[i] = policy
		writePolicy := s.writePolicy
		s.mu.Unlock()
		if writePolicy != nil {
			_ = writePolicy(policy)
		}
		return policy, true
	}
	s.mu.Unlock()
	return policymodel.Policy{}, false
}

func (s *Store) publishedPolicy(tenantID, policyID string, version uint64) (policymodel.Policy, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var latest policymodel.Policy
	var ok bool
	for _, policy := range s.Policies {
		if tenantID != "" && policy.TenantID != tenantID {
			continue
		}
		if policy.PolicyID != policyID || !policy.Published {
			continue
		}
		if version != 0 && policy.Version != version {
			continue
		}
		if version != 0 {
			return policy, true
		}
		if !ok || policy.Version > latest.Version {
			latest = policy
			ok = true
		}
	}
	return latest, ok
}

func (s *Store) AssignPolicy(assignment policymodel.Assignment) (policymodel.Assignment, bool) {
	if assignment.PolicyID == "" {
		return policymodel.Assignment{}, false
	}
	if assignment.TenantID == "" {
		assignment.TenantID = "default"
	}
	policy, ok := s.publishedPolicy(assignment.TenantID, assignment.PolicyID, assignment.PolicyVersion)
	if !ok {
		return policymodel.Assignment{}, false
	}
	assignment.PolicyVersion = policy.Version
	if assignment.AssignmentID == "" {
		assignment.AssignmentID = assignmentKey(assignment)
	}
	now := time.Now().UTC()
	if assignment.CreatedAt.IsZero() {
		assignment.CreatedAt = now
	}
	assignment.UpdatedAt = now
	s.mu.Lock()
	for i, existing := range s.Assignments {
		if existing.AssignmentID == assignment.AssignmentID {
			assignment.CreatedAt = existing.CreatedAt
			s.Assignments[i] = assignment
			writeAssignment := s.writeAssignment
			s.mu.Unlock()
			if writeAssignment != nil {
				_ = writeAssignment(assignment)
			}
			return assignment, true
		}
		if sameAssignmentTarget(existing, assignment) {
			assignment.CreatedAt = existing.CreatedAt
			s.Assignments[i] = assignment
			writeAssignment := s.writeAssignment
			s.mu.Unlock()
			if writeAssignment != nil {
				_ = writeAssignment(assignment)
			}
			return assignment, true
		}
	}
	s.Assignments = append(s.Assignments, assignment)
	writeAssignment := s.writeAssignment
	s.mu.Unlock()
	if writeAssignment != nil {
		_ = writeAssignment(assignment)
	}
	return assignment, true
}

func (s *Store) ListAssignments(tenantID, agentID string) []policymodel.Assignment {
	s.mu.RLock()
	listAssignments := s.listAssignments
	s.mu.RUnlock()
	if listAssignments != nil {
		assignments, err := listAssignments(tenantID, agentID)
		if err == nil && len(assignments) > 0 {
			return assignments
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]policymodel.Assignment, 0, len(s.Assignments))
	for _, assignment := range s.Assignments {
		if tenantID != "" && assignment.TenantID != tenantID {
			continue
		}
		if agentID != "" && assignment.AgentID != agentID {
			continue
		}
		out = append(out, assignment)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TenantID == out[j].TenantID {
			return out[i].AssignmentID < out[j].AssignmentID
		}
		return out[i].TenantID < out[j].TenantID
	})
	return out
}

func (s *Store) EffectivePolicy(tenantID, agentID, scopeType, scopeSelector string) (policymodel.Policy, bool) {
	s.mu.RLock()
	effectivePolicy := s.effectivePolicy
	s.mu.RUnlock()
	if effectivePolicy != nil {
		policy, ok, err := effectivePolicy(tenantID, agentID, scopeType, scopeSelector)
		if err == nil && ok {
			return policy, true
		}
	}
	s.mu.RLock()
	assignments := append([]policymodel.Assignment(nil), s.Assignments...)
	s.mu.RUnlock()
	var best policymodel.Assignment
	bestRank := -1
	for _, assignment := range assignments {
		if tenantID != "" && assignment.TenantID != tenantID {
			continue
		}
		rank := AssignmentRank(assignment, agentID, scopeType, scopeSelector)
		if rank > bestRank {
			best = assignment
			bestRank = rank
		}
	}
	if bestRank >= 0 {
		return s.publishedPolicy(best.TenantID, best.PolicyID, best.PolicyVersion)
	}
	if tenantID == "" {
		tenantID = "default"
	}
	if policy, ok := s.publishedPolicy(tenantID, policymodel.DefaultPolicyID, 0); ok {
		return policy, true
	}
	return policymodel.DefaultPolicy(tenantID), true
}

func (s *Store) CreateResponse(cmd responsemodel.Command) responsemodel.Command {
	cmd = responsemodel.NormalizeCommand(cmd)
	s.mu.Lock()
	for i, existing := range s.Responses {
		if existing.ResponseID == cmd.ResponseID {
			cmd.CreatedAt = existing.CreatedAt
			s.Responses[i] = cmd
			writeResponse := s.writeResponse
			s.mu.Unlock()
			if writeResponse != nil {
				_ = writeResponse(cmd, nil)
			}
			return cmd
		}
	}
	s.Responses = append(s.Responses, cmd)
	writeResponse := s.writeResponse
	s.mu.Unlock()
	if writeResponse != nil {
		_ = writeResponse(cmd, nil)
	}
	return cmd
}

func (s *Store) ListResponses(tenantID, agentID string) []responsemodel.AuditRecord {
	s.mu.RLock()
	listResponses := s.listResponses
	s.mu.RUnlock()
	if listResponses != nil {
		records, err := listResponses(tenantID, agentID)
		if err == nil && len(records) > 0 {
			return records
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	acks := map[string]responsemodel.Ack{}
	for _, ack := range s.ResponseAcks {
		acks[ack.ResponseID] = ack
	}
	out := make([]responsemodel.AuditRecord, 0, len(s.Responses))
	for _, cmd := range s.Responses {
		if tenantID != "" && cmd.TenantID != tenantID {
			continue
		}
		if agentID != "" && cmd.AgentID != agentID {
			continue
		}
		record := responsemodel.AuditRecord{Command: cmd}
		if ack, ok := acks[cmd.ResponseID]; ok {
			record.Ack = &ack
		}
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Command.CreatedAt.Before(out[j].Command.CreatedAt)
	})
	return out
}

func (s *Store) PendingResponses(tenantID, agentID string) []responsemodel.Command {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]responsemodel.Command, 0, len(s.Responses))
	acked := map[string]bool{}
	for _, ack := range s.ResponseAcks {
		acked[ack.ResponseID] = true
	}
	for _, cmd := range s.Responses {
		if tenantID != "" && cmd.TenantID != tenantID {
			continue
		}
		if agentID != "" && cmd.AgentID != agentID {
			continue
		}
		if cmd.Status != "pending" || acked[cmd.ResponseID] {
			continue
		}
		out = append(out, cmd)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

func (s *Store) ApproveResponse(tenantID, agentID, responseID string, approved bool, actor, role, reason string) (responsemodel.Command, bool) {
	if responseID == "" {
		return responsemodel.Command{}, false
	}
	if tenantID == "" {
		tenantID = "default"
	}
	now := time.Now().UTC()
	s.mu.Lock()
	for i, cmd := range s.Responses {
		if cmd.ResponseID != responseID {
			continue
		}
		if tenantID != "" && cmd.TenantID != tenantID {
			continue
		}
		if agentID != "" && cmd.AgentID != agentID {
			continue
		}
		if !cmd.ApprovalRequired || cmd.Status != "pending_approval" || (cmd.ApprovalStatus != "required" && cmd.ApprovalStatus != "partial") {
			s.mu.Unlock()
			return responsemodel.Command{}, false
		}
		if !responsemodel.ApprovalRoleAllowed(cmd, role) {
			s.mu.Unlock()
			return responsemodel.Command{}, false
		}
		approval := responsemodel.Approval{
			Actor:      actor,
			Role:       role,
			Approved:   approved,
			Reason:     reason,
			ObservedAt: now,
		}
		cmd.Approvals = append(cmd.Approvals, approval)
		if approved && responsemodel.ApprovalCount(cmd) >= responsemodel.ApprovalThreshold(cmd) {
			cmd.Status = "pending"
			cmd.ApprovalStatus = "approved"
			cmd.ApprovedBy = actor
			cmd.ApprovedAt = now
		} else if approved {
			cmd.ApprovalStatus = "partial"
		} else {
			cmd.Status = "denied"
			cmd.ApprovalStatus = "rejected"
			cmd.ApprovedBy = actor
			cmd.ApprovedAt = now
		}
		cmd.UpdatedAt = now
		if reason != "" {
			if cmd.Reason == "" {
				cmd.Reason = reason
			} else {
				cmd.Reason = cmd.Reason + "; approval: " + reason
			}
		}
		s.Responses[i] = cmd
		writeResponse := s.writeResponse
		s.mu.Unlock()
		if writeResponse != nil {
			_ = writeResponse(cmd, nil)
		}
		return cmd, true
	}
	s.mu.Unlock()
	return responsemodel.Command{}, false
}

func (s *Store) AckResponse(ack responsemodel.Ack) (responsemodel.Command, bool) {
	if ack.ResponseID == "" {
		return responsemodel.Command{}, false
	}
	if ack.ObservedAt.IsZero() {
		ack.ObservedAt = time.Now().UTC()
	}
	s.mu.Lock()
	var command responsemodel.Command
	var ok bool
	for i, cmd := range s.Responses {
		if cmd.ResponseID == ack.ResponseID {
			cmd.Status = "acked"
			cmd.UpdatedAt = ack.ObservedAt
			s.Responses[i] = cmd
			command = cmd
			ok = true
			break
		}
	}
	if !ok {
		s.mu.Unlock()
		return responsemodel.Command{}, false
	}
	for i, existing := range s.ResponseAcks {
		if existing.ResponseID == ack.ResponseID {
			s.ResponseAcks[i] = ack
			writeResponse := s.writeResponse
			s.mu.Unlock()
			if writeResponse != nil {
				_ = writeResponse(command, &ack)
			}
			return command, true
		}
	}
	s.ResponseAcks = append(s.ResponseAcks, ack)
	writeResponse := s.writeResponse
	s.mu.Unlock()
	if writeResponse != nil {
		_ = writeResponse(command, &ack)
	}
	return command, true
}

func (s *Store) CreateEvidencePullback(req gatewaymodel.EvidencePullbackRequest) gatewaymodel.EvidencePullbackRequest {
	req = gatewaymodel.NormalizeEvidencePullback(req)
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.Pullbacks {
		if existing.RequestID == req.RequestID {
			req.CreatedAt = existing.CreatedAt
			s.Pullbacks[i] = req
			return req
		}
	}
	s.Pullbacks = append(s.Pullbacks, req)
	return req
}

func (s *Store) ListEvidencePullbacks(tenantID, agentID string) []gatewaymodel.EvidencePullbackRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]gatewaymodel.EvidencePullbackRequest, 0, len(s.Pullbacks))
	for _, req := range s.Pullbacks {
		if tenantID != "" && req.TenantID != tenantID {
			continue
		}
		if agentID != "" && req.AgentID != agentID {
			continue
		}
		out = append(out, req)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

func (s *Store) GetEvidencePullback(requestID, tenantID, agentID string) (gatewaymodel.EvidencePullbackRequest, bool) {
	if requestID == "" {
		return gatewaymodel.EvidencePullbackRequest{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, req := range s.Pullbacks {
		if req.RequestID != requestID {
			continue
		}
		if tenantID != "" && req.TenantID != tenantID {
			continue
		}
		if agentID != "" && req.AgentID != agentID {
			continue
		}
		return req, true
	}
	return gatewaymodel.EvidencePullbackRequest{}, false
}

func (s *Store) PendingEvidencePullbacks(tenantID, agentID string) []gatewaymodel.EvidencePullbackRequest {
	all := s.ListEvidencePullbacks(tenantID, agentID)
	out := make([]gatewaymodel.EvidencePullbackRequest, 0, len(all))
	for _, req := range all {
		if req.Status == gatewaymodel.EvidencePullbackStatusPending {
			out = append(out, req)
		}
	}
	return out
}

func (s *Store) CompleteEvidencePullback(result gatewaymodel.EvidencePullbackResult) (gatewaymodel.EvidencePullbackRequest, bool) {
	if result.RequestID == "" {
		return gatewaymodel.EvidencePullbackRequest{}, false
	}
	if result.ObservedAt.IsZero() {
		result.ObservedAt = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, req := range s.Pullbacks {
		if req.RequestID != result.RequestID {
			continue
		}
		if result.TenantID != "" && req.TenantID != result.TenantID {
			continue
		}
		if result.AgentID != "" && req.AgentID != result.AgentID {
			continue
		}
		req.ResultOK = result.OK
		req.Result = result.Message
		req.UpdatedAt = result.ObservedAt
		req.CompletedAt = result.ObservedAt
		if result.OK {
			req.Status = gatewaymodel.EvidencePullbackStatusCompleted
		} else {
			req.Status = gatewaymodel.EvidencePullbackStatusFailed
		}
		s.Pullbacks[i] = req
		return req, true
	}
	return gatewaymodel.EvidencePullbackRequest{}, false
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
	statusByKey := map[string]*incidentv1.Incident{}
	for _, inc := range s.Incidents {
		if inc.GetScenario() == scenario {
			if key := incidentKey(inc); key != "" && inc.GetStatus() != "" {
				statusByKey[key] = inc
			}
			continue
		}
		keptIncidents = append(keptIncidents, inc)
	}
	s.Incidents = keptIncidents
	for _, inc := range incidents {
		defaultIncidentStatus(inc)
		if existing := statusByKey[incidentKey(inc)]; existing != nil {
			inc.Status = existing.GetStatus()
			inc.StatusReason = existing.GetStatusReason()
			inc.StatusActor = existing.GetStatusActor()
			inc.Evidence = mergeEvidence(inc.GetEvidence(), existing.GetEvidence())
		}
	}
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

func (s *Store) RecordAgentGatewayUpload(agent *analyticsv1.AgentHello, batchID, transport string, observedAt time.Time) AgentGatewaySession {
	if agent == nil || agent.GetAgentId() == "" {
		return AgentGatewaySession{}
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	sessionID := agentgatewaySessionID(agent.GetTenantId(), agent.GetAgentId())
	session := AgentGatewaySession{
		SessionID:     sessionID,
		TenantID:      agent.GetTenantId(),
		AgentID:       agent.GetAgentId(),
		StartedAt:     observedAt,
		LastSeenAt:    observedAt,
		LastAckCursor: batchID,
		Transport:     transport,
		Status:        "active",
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.AgentGatewaySessions {
		if existing.SessionID == sessionID {
			session.StartedAt = existing.StartedAt
			if session.LastAckCursor == "" {
				session.LastAckCursor = existing.LastAckCursor
			}
			s.AgentGatewaySessions[i] = session
			return session
		}
	}
	s.AgentGatewaySessions = append(s.AgentGatewaySessions, session)
	return session
}

func (s *Store) RecordAgentGatewayStreamOpen(tenantID, agentID, transport string, observedAt time.Time) AgentGatewaySession {
	return s.updateAgentGatewaySession(tenantID, agentID, transport, "", "open", observedAt, false)
}

func (s *Store) RecordAgentGatewaySessionSeen(tenantID, agentID string, observedAt time.Time) AgentGatewaySession {
	return s.updateAgentGatewaySession(tenantID, agentID, "", "", "", observedAt, false)
}

func (s *Store) CloseAgentGatewaySession(tenantID, agentID string, observedAt time.Time) AgentGatewaySession {
	return s.updateAgentGatewaySession(tenantID, agentID, "", "", "closed", observedAt, true)
}

func (s *Store) updateAgentGatewaySession(tenantID, agentID, transport, cursor, status string, observedAt time.Time, closeSession bool) AgentGatewaySession {
	tenantID = strings.TrimSpace(tenantID)
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return AgentGatewaySession{}
	}
	if tenantID == "" {
		tenantID = "default"
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	sessionID := agentgatewaySessionID(tenantID, agentID)
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, session := range s.AgentGatewaySessions {
		if session.SessionID != sessionID {
			continue
		}
		session.LastSeenAt = observedAt
		if transport != "" {
			session.Transport = transport
		}
		if cursor != "" {
			session.LastAckCursor = cursor
		}
		if status != "" {
			session.Status = status
		}
		if closeSession {
			session.ClosedAt = observedAt
		} else if status == "open" {
			session.ClosedAt = time.Time{}
		}
		s.AgentGatewaySessions[i] = session
		return session
	}
	session := AgentGatewaySession{
		SessionID:     sessionID,
		TenantID:      tenantID,
		AgentID:       agentID,
		StartedAt:     observedAt,
		LastSeenAt:    observedAt,
		LastAckCursor: cursor,
		Transport:     transport,
		Status:        status,
	}
	if session.Status == "" {
		session.Status = "active"
	}
	if closeSession {
		session.ClosedAt = observedAt
	}
	s.AgentGatewaySessions = append(s.AgentGatewaySessions, session)
	return session
}

func (s *Store) ListAgents() []*analyticsv1.AgentHello {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*analyticsv1.AgentHello, len(s.Agents))
	copy(out, s.Agents)
	return out
}

func (s *Store) ListAgentGatewaySessions(tenantID, agentID string) []AgentGatewaySession {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]AgentGatewaySession, 0, len(s.AgentGatewaySessions))
	for _, session := range s.AgentGatewaySessions {
		if tenantID != "" && session.TenantID != tenantID {
			continue
		}
		if agentID != "" && session.AgentID != agentID {
			continue
		}
		out = append(out, session)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TenantID == out[j].TenantID {
			return out[i].AgentID < out[j].AgentID
		}
		return out[i].TenantID < out[j].TenantID
	})
	return out
}

func (s *Store) UpsertOperatorRoleBinding(binding OperatorRoleBinding) OperatorRoleBinding {
	binding.Actor = strings.TrimSpace(binding.Actor)
	if binding.Actor == "" {
		return OperatorRoleBinding{}
	}
	roles := normalizeRoles(binding.Roles)
	now := time.Now().UTC()
	binding.Roles = roles
	binding.UpdatedAt = now
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.OperatorRoles {
		if existing.Actor == binding.Actor {
			binding.CreatedAt = existing.CreatedAt
			if binding.CreatedAt.IsZero() {
				binding.CreatedAt = now
			}
			s.OperatorRoles[i] = binding
			return binding
		}
	}
	binding.CreatedAt = now
	s.OperatorRoles = append(s.OperatorRoles, binding)
	return binding
}

func (s *Store) ListOperatorRoleBindings(actor string) []OperatorRoleBinding {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]OperatorRoleBinding, 0, len(s.OperatorRoles))
	for _, binding := range s.OperatorRoles {
		if actor != "" && binding.Actor != actor {
			continue
		}
		binding.Roles = append([]string(nil), binding.Roles...)
		out = append(out, binding)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Actor < out[j].Actor })
	return out
}

func (s *Store) OperatorRolesForActor(actor string) ([]string, bool) {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, binding := range s.OperatorRoles {
		if binding.Actor == actor {
			return append([]string(nil), binding.Roles...), true
		}
	}
	return nil, false
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

func (s *Store) ListEvents(scenario, behavior string) []*eventv1.CanonicalEvent {
	s.mu.RLock()
	listEvents := s.listEvents
	s.mu.RUnlock()
	if listEvents != nil {
		events, err := listEvents(scenario, behavior)
		if err == nil && len(events) > 0 {
			return events
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*eventv1.CanonicalEvent, 0, len(s.Events))
	for _, ev := range s.Events {
		if scenario != "" && ev.GetScenario() != scenario {
			continue
		}
		if behavior != "" && ev.GetBehavior() != behavior {
			continue
		}
		out = append(out, ev)
	}
	return out
}

func (s *Store) ListSignals(scenario, layer string, terminalOnly bool) []*signalv1.Signal {
	s.mu.RLock()
	listSignals := s.listSignals
	s.mu.RUnlock()
	if listSignals != nil {
		signals, err := listSignals(scenario, layer, terminalOnly)
		if err == nil && len(signals) > 0 {
			return signals
		}
	}
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

func (s *Store) GetSignal(id string) (*signalv1.Signal, bool) {
	if id == "" {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, sig := range s.Signals {
		if sig.GetId() == id {
			return sig, true
		}
	}
	return nil, false
}

func (s *Store) ListIncidents(scenario string) []*incidentv1.Incident {
	s.mu.RLock()
	listIncidents := s.listIncidents
	s.mu.RUnlock()
	if listIncidents != nil {
		incidents, err := listIncidents(scenario)
		if err == nil && len(incidents) > 0 {
			return incidents
		}
	}
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

func (s *Store) GetIncident(id, scenario string) (*incidentv1.Incident, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, inc := range s.Incidents {
		if id != "" && inc.GetId() != id {
			continue
		}
		if scenario != "" && inc.GetScenario() != scenario {
			continue
		}
		return inc, true
	}
	return nil, false
}

func (s *Store) UpdateIncidentStatus(id, scenario, status, reason, actor string) (*incidentv1.Incident, bool) {
	if id == "" && scenario == "" {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, inc := range s.Incidents {
		if id != "" && inc.GetId() != id {
			continue
		}
		if scenario != "" && inc.GetScenario() != scenario {
			continue
		}
		switch status {
		case "", "open":
			inc.Status = "open"
		case "closed", "suppressed":
			inc.Status = status
		default:
			return nil, false
		}
		inc.StatusReason = reason
		inc.StatusActor = actor
		return inc, true
	}
	return nil, false
}

func (s *Store) AttachIncidentEvidence(id, scenario string, evidence *incidentv1.EvidenceSubgraph) (*incidentv1.Incident, bool) {
	if (id == "" && scenario == "") || evidence == nil {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, inc := range s.Incidents {
		if id != "" && inc.GetId() != id {
			continue
		}
		if scenario != "" && inc.GetScenario() != scenario {
			continue
		}
		inc.Evidence = mergeEvidence(inc.GetEvidence(), evidence)
		return inc, true
	}
	return nil, false
}

func (s *Store) MergeIncidents(targetID, sourceID string) (*incidentv1.Incident, bool) {
	if targetID == "" || sourceID == "" || targetID == sourceID {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var target *incidentv1.Incident
	var source *incidentv1.Incident
	sourceIndex := -1
	for i, inc := range s.Incidents {
		switch inc.GetId() {
		case targetID:
			target = inc
		case sourceID:
			source = inc
			sourceIndex = i
		}
	}
	if target == nil || source == nil {
		return nil, false
	}
	if source.GetSeverity() > target.GetSeverity() {
		target.Severity = source.GetSeverity()
	}
	target.Mitre = mergeStrings(target.GetMitre(), source.GetMitre())
	target.LineageIds = mergeStrings(target.GetLineageIds(), source.GetLineageIds())
	target.Terminals = mergeStrings(target.GetTerminals(), source.GetTerminals())
	target.Evidence = mergeEvidence(target.GetEvidence(), source.GetEvidence())
	target.ContributingSignals = mergeSignals(target.GetContributingSignals(), source.GetContributingSignals())
	s.Incidents = append(s.Incidents[:sourceIndex], s.Incidents[sourceIndex+1:]...)
	return target, true
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
		s.AgentGatewaySessions = nil
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
	state, err := s.exportStateLocked()
	path := s.path
	saveState := s.saveState
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	if saveState != nil {
		return saveState(state)
	}
	if path == "" {
		return nil
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (s *Store) ExportState() (State, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.exportStateLocked()
}

func (s *Store) RarityBaselineSnapshot() rarity.Baseline {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.RarityBaseline.Snapshot()
}

func (s *Store) ObserveRaritySignals(signals []*signalv1.Signal) rarity.Baseline {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.RarityBaseline.Observe(signals)
	return s.RarityBaseline.Snapshot()
}

func (s *Store) exportStateLocked() (State, error) {
	var state State
	state.Metrics = s.Metrics
	state.RarityBaseline = s.RarityBaseline.Snapshot()
	state.OperatorRoles = append([]OperatorRoleBinding(nil), s.OperatorRoles...)
	mo := protojson.MarshalOptions{UseProtoNames: true}
	for _, agent := range s.Agents {
		raw, err := mo.Marshal(agent)
		if err != nil {
			return State{}, err
		}
		state.Agents = append(state.Agents, raw)
	}
	for _, ev := range s.Events {
		raw, err := mo.Marshal(ev)
		if err != nil {
			return State{}, err
		}
		state.Events = append(state.Events, raw)
	}
	for _, sig := range s.Signals {
		raw, err := mo.Marshal(sig)
		if err != nil {
			return State{}, err
		}
		state.Signals = append(state.Signals, raw)
	}
	for _, inc := range s.Incidents {
		raw, err := mo.Marshal(inc)
		if err != nil {
			return State{}, err
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
			return State{}, err
		}
		state.Health = append(state.Health, raw)
	}
	state.Rules = append([]policymodel.RuleContent(nil), s.Rules...)
	state.Policies = append([]policymodel.Policy(nil), s.Policies...)
	state.Assignments = append([]policymodel.Assignment(nil), s.Assignments...)
	state.PolicyAudits = append([]policymodel.AuditRecord(nil), s.PolicyAudits...)
	state.Responses = append([]responsemodel.Command(nil), s.Responses...)
	state.ResponseAcks = append([]responsemodel.Ack(nil), s.ResponseAcks...)
	state.Pullbacks = append([]gatewaymodel.EvidencePullbackRequest(nil), s.Pullbacks...)
	state.AgentGatewaySessions = append([]AgentGatewaySession(nil), s.AgentGatewaySessions...)
	return state, nil
}

func agentHealthKey(tenantID, agentID string) string {
	return tenantID + "/" + agentID
}

func agentgatewaySessionID(tenantID, agentID string) string {
	return stableKey(tenantID, agentID)
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

func SignalLayerName(where signalv1.SignalWhere) string {
	return layerName(where)
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

func SignalProjectionKey(sig *signalv1.Signal) string {
	return signalKey(sig)
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

func IncidentProjectionKey(inc *incidentv1.Incident) string {
	return incidentKey(inc)
}

func defaultIncidentStatus(inc *incidentv1.Incident) {
	if inc != nil && inc.GetStatus() == "" {
		inc.Status = "open"
	}
}

func mergeEvidence(base, extra *incidentv1.EvidenceSubgraph) *incidentv1.EvidenceSubgraph {
	if base == nil && extra == nil {
		return nil
	}
	out := &incidentv1.EvidenceSubgraph{}
	seenNodes := map[string]bool{}
	seenEdges := map[string]bool{}
	appendNode := func(node *incidentv1.GraphNode) {
		if node == nil {
			return
		}
		key := graphNodeKey(node)
		if key == "" || seenNodes[key] {
			return
		}
		seenNodes[key] = true
		out.Nodes = append(out.Nodes, node)
	}
	appendEdge := func(edge *incidentv1.GraphEdge) {
		if edge == nil {
			return
		}
		key := graphEdgeKey(edge)
		if key == "" || seenEdges[key] {
			return
		}
		seenEdges[key] = true
		out.Edges = append(out.Edges, edge)
	}
	for _, node := range base.GetNodes() {
		appendNode(node)
	}
	for _, node := range extra.GetNodes() {
		appendNode(node)
	}
	for _, edge := range base.GetEdges() {
		appendEdge(edge)
	}
	for _, edge := range extra.GetEdges() {
		appendEdge(edge)
	}
	return out
}

func graphNodeKey(node *incidentv1.GraphNode) string {
	if node.GetId() != "" {
		return node.GetId()
	}
	return strings.Join([]string{node.GetKind(), node.GetLabel()}, "\x00")
}

func graphEdgeKey(edge *incidentv1.GraphEdge) string {
	if edge.GetId() != "" {
		return edge.GetId()
	}
	return strings.Join([]string{edge.GetFrom(), edge.GetTo(), edge.GetKind()}, "\x00")
}

func mergeStrings(base, extra []string) []string {
	out := append([]string(nil), base...)
	seen := map[string]bool{}
	for _, item := range out {
		seen[item] = true
	}
	for _, item := range extra {
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func mergeSignals(base, extra []*signalv1.Signal) []*signalv1.Signal {
	out := append([]*signalv1.Signal(nil), base...)
	seen := map[string]bool{}
	for _, sig := range out {
		key := signalKey(sig)
		if key != "" {
			seen[key] = true
		}
	}
	for _, sig := range extra {
		key := signalKey(sig)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, sig)
	}
	return out
}

func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func normalizeRoles(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, role := range in {
		role = strings.TrimSpace(role)
		if role == "" || seen[role] {
			continue
		}
		seen[role] = true
		out = append(out, role)
	}
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

func assignmentKey(assignment policymodel.Assignment) string {
	parts := []string{
		assignment.TenantID,
		assignment.AgentID,
		assignment.Scope.Type,
		assignment.Scope.Selector,
		assignment.PolicyID,
	}
	return stableKey(parts...)
}

func sameAssignmentTarget(a, b policymodel.Assignment) bool {
	return a.TenantID == b.TenantID &&
		a.AgentID == b.AgentID &&
		a.Scope.Type == b.Scope.Type &&
		a.Scope.Selector == b.Scope.Selector
}

func AssignmentRank(assignment policymodel.Assignment, agentID, scopeType, scopeSelector string) int {
	if assignment.AgentID != "" {
		if agentID == "" || assignment.AgentID != agentID {
			return -1
		}
		return 30
	}
	if assignment.Scope.Type != "" {
		if assignment.Scope.Type != scopeType {
			return -1
		}
		if assignment.Scope.Selector != "" && assignment.Scope.Selector != scopeSelector {
			return -1
		}
		if assignment.Scope.Selector != "" {
			return 20
		}
		return 10
	}
	return 0
}
