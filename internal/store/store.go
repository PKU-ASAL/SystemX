package store

import (
	"context"
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

	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	incidentv1 "github.com/sysarmor/sysarmor-next-project/api/proto/incident/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
	"github.com/sysarmor/sysarmor-next-project/internal/analytics/rarity"
	controlmodel "github.com/sysarmor/sysarmor-next-project/internal/controlmodel"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
	responsemodel "github.com/sysarmor/sysarmor-next-project/internal/response"
	"github.com/sysarmor/sysarmor-next-project/internal/store/migrations"
	"google.golang.org/protobuf/encoding/protojson"
)

const FileStoreStateVersion = 1

type Store struct {
	mu              sync.RWMutex
	durableMu       sync.Mutex
	path            string
	backendInfo     *Info
	backend         Backend
	baseCtx         context.Context
	Agents          []AgentIdentity
	Events          []*eventv1.CanonicalEvent
	Signals         []*signalv1.Signal
	Incidents       []*incidentv1.Incident
	Health          map[string]agenthealth.AgentHealth
	Rules           []policymodel.RuleContent
	Policies        []policymodel.Policy
	Assignments     []policymodel.Assignment
	PolicyAudits    []policymodel.AuditRecord
	Responses       []responsemodel.Command
	ResponseAcks    []responsemodel.Ack
	Pullbacks       []controlmodel.EvidencePullbackRequest
	ControlCommands []controlmodel.ControlCommand
	AgentSessions   []AgentSession
	Enrollments     []Enrollment
	Artifacts       []Artifact
	Channels        []ArtifactChannel
	Certificates    []AgentCertificate
	Metrics         Metrics
	RarityBaseline  rarity.Baseline
}

type LabelSelector map[string]string

func LabelsMatch(labels map[string]string, selector LabelSelector) bool {
	return labelSelectorMatches(labels, selector)
}

type Info struct {
	Backend          string `json:"backend"`
	Path             string `json:"path,omitempty"`
	StateVersion     int    `json:"state_version"`
	MigrationVersion int    `json:"migration_version"`
	PostgresSchema   int    `json:"postgres_schema_version"`
}

type Metrics struct {
	DataBatchesAppended       uint64  `json:"data_batches_appended"`
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

type AgentSession struct {
	SessionID         string    `json:"session_id"`
	TenantID          string    `json:"tenant_id"`
	AgentID           string    `json:"agent_id"`
	StartedAt         time.Time `json:"started_at"`
	LastSeenAt        time.Time `json:"last_seen_at"`
	LastDataSeenAt    time.Time `json:"last_data_seen_at,omitempty"`
	LastControlSeenAt time.Time `json:"last_control_seen_at,omitempty"`
	ClosedAt          time.Time `json:"closed_at,omitempty"`
	LastAckCursor     string    `json:"last_ack_cursor,omitempty"`
	DataTransport     string    `json:"data_transport,omitempty"`
	ControlTransport  string    `json:"control_transport,omitempty"`
	Status            string    `json:"status,omitempty"`
}

type Enrollment struct {
	EnrollmentID          string            `json:"enrollment_id"`
	TenantID              string            `json:"tenant_id"`
	AgentID               string            `json:"agent_id,omitempty"`
	HostID                string            `json:"host_id,omitempty"`
	TokenHash             string            `json:"token_hash,omitempty"`
	TokenPreview          string            `json:"token_preview,omitempty"`
	BootstrapTokenHash    string            `json:"bootstrap_token_hash,omitempty"`
	BootstrapTokenPreview string            `json:"bootstrap_token_preview,omitempty"`
	BootstrapFetchedAt    time.Time         `json:"bootstrap_fetched_at,omitempty"`
	GatewayAddr           string            `json:"gateway_addr"`
	GatewaySNI            string            `json:"gateway_sni,omitempty"`
	Profile               string            `json:"profile,omitempty"`
	Channel               string            `json:"channel,omitempty"`
	ArtifactID            string            `json:"artifact_id,omitempty"`
	ArtifactSHA256        string            `json:"artifact_sha256,omitempty"`
	ArtifactURL           string            `json:"artifact_url,omitempty"`
	Labels                map[string]string `json:"labels,omitempty"`
	Status                string            `json:"status"`
	CreatedAt             time.Time         `json:"created_at"`
	ExpiresAt             time.Time         `json:"expires_at"`
	CreatedBy             string            `json:"created_by,omitempty"`
	UsedAt                time.Time         `json:"used_at,omitempty"`
	IssuedKeySHA256       string            `json:"issued_key_sha256,omitempty"`
	IssuedCertificatePEM  string            `json:"issued_certificate_pem,omitempty"`
	IssuedCAPEM           string            `json:"issued_ca_pem,omitempty"`
	IssuedSerialNumber    string            `json:"issued_serial_number,omitempty"`
	IssuedNotAfter        time.Time         `json:"issued_not_after,omitempty"`
	IssuedAt              time.Time         `json:"issued_at,omitempty"`
}

type ArtifactChannel struct {
	TenantID   string    `json:"tenant_id"`
	Channel    string    `json:"channel"`
	ArtifactID string    `json:"artifact_id"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	CreatedBy  string    `json:"created_by,omitempty"`
}

type Artifact struct {
	ArtifactID  string            `json:"artifact_id"`
	TenantID    string            `json:"tenant_id"`
	Name        string            `json:"name"`
	Kind        string            `json:"kind"`
	Version     string            `json:"version"`
	OS          string            `json:"os,omitempty"`
	Arch        string            `json:"arch,omitempty"`
	SHA256      string            `json:"sha256"`
	SizeBytes   int64             `json:"size_bytes"`
	Status      string            `json:"status"`
	StoragePath string            `json:"storage_path,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	CreatedBy   string            `json:"created_by,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type AgentCertificate struct {
	TenantID          string    `json:"tenant_id"`
	AgentID           string    `json:"agent_id"`
	EnrollmentID      string    `json:"enrollment_id,omitempty"`
	SerialNumber      string    `json:"serial_number"`
	Subject           string    `json:"subject,omitempty"`
	NotBefore         time.Time `json:"not_before"`
	NotAfter          time.Time `json:"not_after"`
	CreatedAt         time.Time `json:"created_at"`
	RevokedAt         time.Time `json:"revoked_at,omitempty"`
	RevocationReceipt string    `json:"revocation_receipt,omitempty"`
	CertificatePEM    string    `json:"certificate_pem,omitempty"`
}

type State struct {
	Agents          []json.RawMessage                      `json:"agents"`
	Events          []json.RawMessage                      `json:"events"`
	Signals         []json.RawMessage                      `json:"signals"`
	Incidents       []json.RawMessage                      `json:"incidents"`
	Health          []json.RawMessage                      `json:"health"`
	Rules           []policymodel.RuleContent              `json:"rules"`
	Policies        []policymodel.Policy                   `json:"policies"`
	Assignments     []policymodel.Assignment               `json:"assignments"`
	PolicyAudits    []policymodel.AuditRecord              `json:"policy_audits"`
	Responses       []responsemodel.Command                `json:"responses"`
	ResponseAcks    []responsemodel.Ack                    `json:"response_acks"`
	Pullbacks       []controlmodel.EvidencePullbackRequest `json:"evidence_pullbacks"`
	ControlCommands []controlmodel.ControlCommand          `json:"control_commands,omitempty"`
	AgentSessions   []AgentSession                         `json:"agent_sessions"`
	Enrollments     []Enrollment                           `json:"enrollments,omitempty"`
	Artifacts       []Artifact                             `json:"artifacts,omitempty"`
	Channels        []ArtifactChannel                      `json:"channels,omitempty"`
	Certificates    []AgentCertificate                     `json:"certificates,omitempty"`
	Metrics         Metrics                                `json:"metrics"`
	RarityBaseline  rarity.Baseline                        `json:"rarity_baseline,omitempty"`
}

func Open(path string) (*Store, error) {
	s := &Store{path: path, baseCtx: context.Background(), Health: map[string]agenthealth.AgentHealth{}}
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
		var msg AgentIdentity
		if err := json.Unmarshal(raw, &msg); err != nil {
			return err
		}
		msg = msg.Normalized()
		if msg.Valid() {
			s.Agents = append(s.Agents, msg)
		}
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
	s.ControlCommands = state.ControlCommands
	s.AgentSessions = state.AgentSessions
	s.Enrollments = state.Enrollments
	s.Artifacts = state.Artifacts
	s.Channels = state.Channels
	s.Certificates = state.Certificates
	s.Metrics = state.Metrics
	s.RarityBaseline = state.RarityBaseline.Snapshot()
	return nil
}

// AttachBackend binds a durable persistence Backend to the store. The supplied
// context becomes the base context for all backend operations, so backend work
// is cancelled when this context is done (e.g. on server shutdown). Passing a
// nil context falls back to context.Background().
func (s *Store) AttachBackend(ctx context.Context, backend Backend, info Info) {
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backendInfo = &info
	s.backend = backend
	s.baseCtx = ctx
}

func (s *Store) backendCtx() (Backend, context.Context) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.backend, ctxOrBackground(s.baseCtx)
}

func ctxOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
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

func (s *Store) AddAgent(agent AgentIdentity) {
	agent = agent.Normalized()
	if !agent.Valid() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.Agents {
		if existing.TenantID == agent.TenantID && existing.AgentID == agent.AgentID {
			if agent.HostID == "" {
				agent.HostID = existing.HostID
			}
			if agent.Version == "" {
				agent.Version = existing.Version
			}
			if agent.AuthType == "" {
				agent.AuthType = existing.AuthType
			}
			if agent.CertIdentity == "" {
				agent.CertIdentity = existing.CertIdentity
			}
			s.Agents[i] = agent
			return
		}
	}
	s.Agents = append(s.Agents, agent)
}

func (s *Store) BindAgentIdentity(agent AgentIdentity) error {
	agent = agent.Normalized()
	if !agent.Valid() {
		return fmt.Errorf("agent_id is required")
	}
	if agent.AuthType == "" || agent.CertIdentity == "" {
		return fmt.Errorf("auth_type and cert_identity are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.Agents {
		if existing.TenantID != agent.TenantID || existing.AgentID != agent.AgentID {
			continue
		}
		if existing.CertIdentity != "" && existing.CertIdentity != agent.CertIdentity {
			return fmt.Errorf("agent identity binding mismatch: registered=%s presented=%s", existing.CertIdentity, agent.CertIdentity)
		}
		if existing.AuthType != "" && existing.AuthType != agent.AuthType {
			return fmt.Errorf("agent auth binding mismatch: registered=%s presented=%s", existing.AuthType, agent.AuthType)
		}
		if agent.HostID == "" {
			agent.HostID = existing.HostID
		}
		if agent.Version == "" {
			agent.Version = existing.Version
		}
		s.Agents[i] = agent
		return nil
	}
	s.Agents = append(s.Agents, agent)
	return nil
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
	for i, policy := range s.Policies {
		if policy.TenantID == tenantID && policy.PolicyID == policymodel.DefaultPolicyID && policy.Version == policymodel.DefaultPolicyVersion {
			if !managerDefaultPolicyUsable(policy) {
				s.Policies[i] = policymodel.Normalize(policymodel.ManagerDefaultPolicy(tenantID))
			}
			return
		}
	}
	s.Policies = append(s.Policies, policymodel.Normalize(policymodel.ManagerDefaultPolicy(tenantID)))
}

func (s *Store) EnsureDefaultPolicyWithError(tenantID string) error {
	s.EnsureDefaultPolicy(tenantID)
	s.durableMu.Lock()
	defer s.durableMu.Unlock()
	backend, ctx := s.backendCtx()
	if backend == nil {
		return nil
	}
	existing, ok, err := backend.GetPolicy(ctxOrBackground(ctx), tenantID, policymodel.DefaultPolicyID, policymodel.DefaultPolicyVersion)
	if err != nil {
		return fmt.Errorf("read default policy: %w", err)
	}
	if ok && managerDefaultPolicyUsable(existing) {
		return nil
	}
	policy := policymodel.Normalize(policymodel.ManagerDefaultPolicy(tenantID))
	if err := backend.WritePolicy(ctxOrBackground(ctx), policy); err != nil {
		return fmt.Errorf("persist default policy: %w", err)
	}
	return nil
}

func managerDefaultPolicyUsable(policy policymodel.Policy) bool {
	if !policy.Published || policy.Detection == nil {
		return false
	}
	for _, ruleset := range policy.Detection.RuleSets {
		if ruleset.Ref == policymodel.DefaultRuleSetRef {
			return true
		}
	}
	return false
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
	out, _ := s.UpsertPolicyWithError(policy)
	return out
}

func (s *Store) UpsertPolicyWithError(policy policymodel.Policy) (policymodel.Policy, error) {
	if policy.PolicyID == "" {
		return policymodel.Policy{}, nil
	}
	policy = policymodel.Normalize(policy)
	s.durableMu.Lock()
	defer s.durableMu.Unlock()
	s.mu.Lock()
	policies, policy := upsertPolicySnapshot(s.Policies, policy)
	backend, ctx := s.backend, s.baseCtx
	if backend == nil {
		oldPolicies := s.Policies
		s.Policies = policies
		if err := s.persistFileLocked(); err != nil {
			s.Policies = oldPolicies
			s.mu.Unlock()
			return policymodel.Policy{}, fmt.Errorf("write policy: %w", err)
		}
		s.mu.Unlock()
		return policy, nil
	}
	s.mu.Unlock()
	if err := backend.WritePolicy(ctxOrBackground(ctx), policy); err != nil {
		return policymodel.Policy{}, fmt.Errorf("write policy: %w", err)
	}
	s.mu.Lock()
	s.Policies = policies
	s.mu.Unlock()
	return policy, nil
}

func upsertPolicySnapshot(policies []policymodel.Policy, policy policymodel.Policy) ([]policymodel.Policy, policymodel.Policy) {
	out := append([]policymodel.Policy(nil), policies...)
	for i, existing := range out {
		if existing.TenantID == policy.TenantID && existing.PolicyID == policy.PolicyID && existing.Version == policy.Version {
			if !existing.CreatedAt.IsZero() {
				policy.CreatedAt = existing.CreatedAt
			}
			out[i] = policy
			return out, policy
		}
	}
	return append(out, policy), policy
}

func (s *Store) RecordPolicyAudit(record policymodel.AuditRecord) policymodel.AuditRecord {
	record = normalizePolicyAudit(record)
	s.mu.Lock()
	s.PolicyAudits = append(s.PolicyAudits, record)
	backend, ctx := s.backend, s.baseCtx
	s.mu.Unlock()
	if backend != nil {
		_ = backend.WritePolicyAudit(ctxOrBackground(ctx), record)
	}
	return record
}

func normalizePolicyAudit(record policymodel.AuditRecord) policymodel.AuditRecord {
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
	return record
}

func (s *Store) ListPolicyAudits(tenantID, policyID string) []policymodel.AuditRecord {
	if backend, ctx := s.backendCtx(); backend != nil {
		if audits, err := backend.ListPolicyAudits(ctx, tenantID, policyID); err == nil {
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

func (s *Store) ListPolicyAuditsWithError(tenantID, policyID string) ([]policymodel.AuditRecord, error) {
	if backend, ctx := s.backendCtx(); backend != nil {
		audits, err := backend.ListPolicyAudits(ctx, tenantID, policyID)
		if err != nil {
			return nil, fmt.Errorf("list policy audits: %w", err)
		}
		return audits, nil
	}
	return s.ListPolicyAudits(tenantID, policyID), nil
}

func (s *Store) ListPolicies(tenantID string) []policymodel.Policy {
	if backend, ctx := s.backendCtx(); backend != nil {
		if policies, err := backend.ListPolicies(ctx, tenantID); err == nil {
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

func (s *Store) ListPoliciesWithError(tenantID string) ([]policymodel.Policy, error) {
	if backend, ctx := s.backendCtx(); backend != nil {
		policies, err := backend.ListPolicies(ctx, tenantID)
		if err != nil {
			return nil, fmt.Errorf("list policies: %w", err)
		}
		return policies, nil
	}
	return s.ListPolicies(tenantID), nil
}

func (s *Store) GetPolicy(tenantID, policyID string, version uint64) (policymodel.Policy, bool) {
	if backend, ctx := s.backendCtx(); backend != nil {
		if policy, ok, err := backend.GetPolicy(ctx, tenantID, policyID, version); err == nil && ok {
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

func (s *Store) GetPolicyWithError(tenantID, policyID string, version uint64) (policymodel.Policy, bool, error) {
	if backend, ctx := s.backendCtx(); backend != nil {
		policy, ok, err := backend.GetPolicy(ctx, tenantID, policyID, version)
		if err != nil {
			return policymodel.Policy{}, false, fmt.Errorf("get policy: %w", err)
		}
		return policy, ok, nil
	}
	policy, ok := s.GetPolicy(tenantID, policyID, version)
	return policy, ok, nil
}

func (s *Store) PublishPolicy(tenantID, policyID string, version uint64, published bool) (policymodel.Policy, bool, error) {
	if tenantID == "" {
		tenantID = "default"
	}
	if policyID == "" {
		return policymodel.Policy{}, false, nil
	}
	s.durableMu.Lock()
	defer s.durableMu.Unlock()
	if err := s.loadPolicyForCommit(tenantID, policyID, version); err != nil {
		return policymodel.Policy{}, false, err
	}
	s.mu.RLock()
	index := policyIndex(s.Policies, tenantID, policyID, version)
	if index < 0 {
		s.mu.RUnlock()
		return policymodel.Policy{}, false, nil
	}
	policy := s.Policies[index]
	backend, ctx := s.backend, s.baseCtx
	s.mu.RUnlock()
	policy.Published, policy.UpdatedAt = published, time.Now().UTC()
	if backend != nil {
		if err := backend.WritePolicy(ctxOrBackground(ctx), policy); err != nil {
			return policymodel.Policy{}, false, fmt.Errorf("write policy publication: %w", err)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	oldPolicies := s.Policies
	s.Policies, _ = upsertPolicySnapshot(s.Policies, policy)
	if err := s.persistFileLocked(); err != nil {
		s.Policies = oldPolicies
		return policymodel.Policy{}, false, fmt.Errorf("write policy publication: %w", err)
	}
	return policy, true, nil
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

func (s *Store) AssignPolicy(assignment policymodel.Assignment) (policymodel.Assignment, bool, error) {
	if assignment.PolicyID == "" {
		return policymodel.Assignment{}, false, nil
	}
	if assignment.TenantID == "" {
		assignment.TenantID = "default"
	}
	s.durableMu.Lock()
	defer s.durableMu.Unlock()
	if err := s.loadAssignmentState(assignment); err != nil {
		return policymodel.Assignment{}, false, err
	}
	s.mu.RLock()
	policies := append([]policymodel.Policy(nil), s.Policies...)
	assignments := append([]policymodel.Assignment(nil), s.Assignments...)
	backend, ctx := s.backend, s.baseCtx
	s.mu.RUnlock()
	assignment, _, ok := prepareAssignment(policies, assignments, assignment)
	if !ok {
		return policymodel.Assignment{}, false, nil
	}
	if backend != nil {
		if err := backend.WriteAssignment(ctxOrBackground(ctx), assignment); err != nil {
			return policymodel.Assignment{}, false, fmt.Errorf("write policy assignment: %w", err)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	oldAssignments := s.Assignments
	s.Assignments = upsertAssignmentSnapshot(s.Assignments, assignment)
	if err := s.persistFileLocked(); err != nil {
		s.Assignments = oldAssignments
		return policymodel.Assignment{}, false, fmt.Errorf("write policy assignment: %w", err)
	}
	return assignment, true, nil
}

func (s *Store) ListAssignments(tenantID, agentID string) []policymodel.Assignment {
	if backend, ctx := s.backendCtx(); backend != nil {
		if assignments, err := backend.ListAssignments(ctx, tenantID, agentID); err == nil {
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

func (s *Store) ListAssignmentsWithError(tenantID, agentID string) ([]policymodel.Assignment, error) {
	if backend, ctx := s.backendCtx(); backend != nil {
		assignments, err := backend.ListAssignments(ctx, tenantID, agentID)
		if err != nil {
			return nil, fmt.Errorf("list policy assignments: %w", err)
		}
		return assignments, nil
	}
	return s.ListAssignments(tenantID, agentID), nil
}

func (s *Store) EffectivePolicy(tenantID, agentID, scopeType, scopeSelector string) (policymodel.Policy, bool) {
	if backend, ctx := s.backendCtx(); backend != nil {
		if policy, ok, err := backend.EffectivePolicy(ctx, tenantID, agentID, scopeType, scopeSelector); err == nil && ok {
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
	return policymodel.ManagerDefaultPolicy(tenantID), true
}

func (s *Store) EffectivePolicyWithError(tenantID, agentID, scopeType, scopeSelector string) (policymodel.Policy, bool, error) {
	if backend, ctx := s.backendCtx(); backend != nil {
		policy, ok, err := backend.EffectivePolicy(ctx, tenantID, agentID, scopeType, scopeSelector)
		if err != nil {
			return policymodel.Policy{}, false, fmt.Errorf("read effective policy: %w", err)
		}
		return policy, ok, nil
	}
	policy, ok := s.EffectivePolicy(tenantID, agentID, scopeType, scopeSelector)
	return policy, ok, nil
}

func (s *Store) CreateResponse(cmd responsemodel.Command) (responsemodel.Command, error) {
	cmd = responsemodel.NormalizeCommand(cmd)
	s.durableMu.Lock()
	defer s.durableMu.Unlock()
	s.mu.RLock()
	backend, ctx := s.backend, s.baseCtx
	if backend == nil {
		for _, existing := range s.Responses {
			if existing.TenantID == cmd.TenantID && existing.ResponseID == cmd.ResponseID {
				s.mu.RUnlock()
				return existing, nil
			}
		}
	}
	s.mu.RUnlock()
	if backend != nil {
		out, authoritative, err := s.createResponseInBackend(backend, ctxOrBackground(ctx), cmd)
		if err != nil {
			return responsemodel.Command{}, err
		}
		if authoritative {
			return out, nil
		}
	}
	s.mu.Lock()
	oldResponses := s.Responses
	s.Responses = upsertResponseSnapshot(append([]responsemodel.Command(nil), s.Responses...), cmd)
	if err := s.persistFileLocked(); err != nil {
		s.Responses = oldResponses
		s.mu.Unlock()
		return responsemodel.Command{}, fmt.Errorf("write response: %w", err)
	}
	s.mu.Unlock()
	return cmd, nil
}

func (s *Store) createResponseInBackend(backend Backend, ctx context.Context, cmd responsemodel.Command) (responsemodel.Command, bool, error) {
	records, err := backend.ListResponses(ctx, cmd.TenantID, "")
	if err != nil {
		return responsemodel.Command{}, false, fmt.Errorf("find existing response: %w", err)
	}
	if existing, ok := findResponseAudit(records, cmd.TenantID, cmd.ResponseID); ok {
		s.mu.Lock()
		s.cacheResponseAuditLocked(existing)
		s.mu.Unlock()
		return existing.Command, true, nil
	}
	created, err := backend.CreateResponse(ctx, cmd)
	if err != nil {
		return responsemodel.Command{}, false, fmt.Errorf("write response: %w", err)
	}
	if created {
		return cmd, false, nil
	}
	records, err = backend.ListResponses(ctx, cmd.TenantID, "")
	if err != nil {
		return responsemodel.Command{}, false, fmt.Errorf("reload conflicting response: %w", err)
	}
	existing, ok := findResponseAudit(records, cmd.TenantID, cmd.ResponseID)
	if !ok {
		return responsemodel.Command{}, false, fmt.Errorf("reload conflicting response: response not found")
	}
	s.mu.Lock()
	s.cacheResponseAuditLocked(existing)
	s.mu.Unlock()
	return existing.Command, true, nil
}

func findResponseAudit(records []responsemodel.AuditRecord, tenantID, responseID string) (responsemodel.AuditRecord, bool) {
	for _, record := range records {
		if record.Command.TenantID == tenantID && record.Command.ResponseID == responseID {
			return record, true
		}
	}
	return responsemodel.AuditRecord{}, false
}

func (s *Store) cacheResponseAuditLocked(record responsemodel.AuditRecord) {
	s.Responses = upsertResponseSnapshot(s.Responses, record.Command)
	if record.Ack != nil {
		s.ResponseAcks = upsertResponseAckSnapshot(s.ResponseAcks, *record.Ack)
	}
}

func upsertResponseSnapshot(responses []responsemodel.Command, command responsemodel.Command) []responsemodel.Command {
	for i, existing := range responses {
		if existing.TenantID == command.TenantID && existing.ResponseID == command.ResponseID {
			responses[i] = command
			return responses
		}
	}
	return append(responses, command)
}

func upsertResponseAckSnapshot(acks []responsemodel.Ack, ack responsemodel.Ack) []responsemodel.Ack {
	for i, existing := range acks {
		if existing.TenantID == ack.TenantID && existing.ResponseID == ack.ResponseID {
			acks[i] = ack
			return acks
		}
	}
	return append(acks, ack)
}

func (s *Store) ListResponses(tenantID, agentID string) []responsemodel.AuditRecord {
	if backend, ctx := s.backendCtx(); backend != nil {
		if records, err := backend.ListResponses(ctx, tenantID, agentID); err == nil {
			return records
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	acks := map[string]responsemodel.Ack{}
	for _, ack := range s.ResponseAcks {
		acks[responseKey(ack.TenantID, ack.ResponseID)] = ack
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
		if ack, ok := acks[responseKey(cmd.TenantID, cmd.ResponseID)]; ok {
			record.Ack = &ack
		}
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Command.CreatedAt.Before(out[j].Command.CreatedAt)
	})
	return out
}

func (s *Store) ListResponsesWithError(tenantID, agentID string) ([]responsemodel.AuditRecord, error) {
	if backend, ctx := s.backendCtx(); backend != nil {
		records, err := backend.ListResponses(ctx, tenantID, agentID)
		if err != nil {
			return nil, fmt.Errorf("list responses: %w", err)
		}
		return records, nil
	}
	return s.ListResponses(tenantID, agentID), nil
}

func (s *Store) PendingResponses(tenantID, agentID string) []responsemodel.Command {
	if backend, ctx := s.backendCtx(); backend != nil {
		if records, err := backend.ListResponses(ctx, tenantID, agentID); err == nil {
			return pendingResponsesFromAudit(records)
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]responsemodel.Command, 0, len(s.Responses))
	acked := map[string]bool{}
	for _, ack := range s.ResponseAcks {
		acked[responseKey(ack.TenantID, ack.ResponseID)] = true
	}
	for _, cmd := range s.Responses {
		if tenantID != "" && cmd.TenantID != tenantID {
			continue
		}
		if agentID != "" && cmd.AgentID != agentID {
			continue
		}
		if cmd.Status != "pending" || acked[responseKey(cmd.TenantID, cmd.ResponseID)] {
			continue
		}
		out = append(out, cmd)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

func (s *Store) PendingResponsesWithError(tenantID, agentID string) ([]responsemodel.Command, error) {
	if backend, ctx := s.backendCtx(); backend != nil {
		records, err := backend.ListResponses(ctx, tenantID, agentID)
		if err != nil {
			return nil, fmt.Errorf("list pending responses: %w", err)
		}
		return pendingResponsesFromAudit(records), nil
	}
	return s.PendingResponses(tenantID, agentID), nil
}

func pendingResponsesFromAudit(records []responsemodel.AuditRecord) []responsemodel.Command {
	out := make([]responsemodel.Command, 0, len(records))
	for _, record := range records {
		if record.Command.Status == "pending" && record.Ack == nil {
			out = append(out, record.Command)
		}
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
		backend, ctx := s.backend, s.baseCtx
		s.mu.Unlock()
		if backend != nil {
			_ = backend.WriteResponse(ctxOrBackground(ctx), cmd, nil)
		}
		return cmd, true
	}
	s.mu.Unlock()
	return responsemodel.Command{}, false
}

func (s *Store) AckResponse(ack responsemodel.Ack) (responsemodel.Command, bool, error) {
	if ack.ResponseID == "" {
		return responsemodel.Command{}, false, nil
	}
	if ack.ObservedAt.IsZero() {
		ack.ObservedAt = time.Now().UTC()
	}
	if ack.TenantID == "" {
		ack.TenantID = "default"
	}
	if ack.AgentID == "" {
		return responsemodel.Command{}, false, nil
	}
	s.durableMu.Lock()
	defer s.durableMu.Unlock()
	s.mu.RLock()
	var command responsemodel.Command
	var ok bool
	for _, cmd := range s.Responses {
		if cmd.ResponseID == ack.ResponseID && cmd.TenantID == ack.TenantID && cmd.AgentID == ack.AgentID {
			command = cmd
			ok = true
			break
		}
	}
	backend, ctx := s.backend, s.baseCtx
	s.mu.RUnlock()
	if !ok {
		if backend != nil {
			records, err := backend.ListResponses(ctxOrBackground(ctx), ack.TenantID, ack.AgentID)
			if err != nil {
				return responsemodel.Command{}, false, fmt.Errorf("find response for ack: %w", err)
			}
			if record, found := findResponseAudit(records, ack.TenantID, ack.ResponseID); found {
				command, ok = record.Command, record.Command.AgentID == ack.AgentID
			}
		}
		if !ok {
			return responsemodel.Command{}, false, nil
		}
	}
	command.Status = "acked"
	command.UpdatedAt = ack.ObservedAt
	if backend != nil {
		if err := backend.WriteResponse(ctxOrBackground(ctx), command, &ack); err != nil {
			return responsemodel.Command{}, false, fmt.Errorf("write response ack: %w", err)
		}
	}
	s.mu.Lock()
	oldResponses, oldAcks := s.Responses, s.ResponseAcks
	s.Responses = upsertResponseSnapshot(append([]responsemodel.Command(nil), s.Responses...), command)
	s.ResponseAcks = upsertResponseAckSnapshot(append([]responsemodel.Ack(nil), s.ResponseAcks...), ack)
	if err := s.persistFileLocked(); err != nil {
		s.Responses, s.ResponseAcks = oldResponses, oldAcks
		s.mu.Unlock()
		return responsemodel.Command{}, false, fmt.Errorf("write response ack: %w", err)
	}
	s.mu.Unlock()
	return command, true, nil
}

func responseKey(tenantID, responseID string) string {
	return tenantID + "/" + responseID
}

func (s *Store) CreateEvidencePullback(req controlmodel.EvidencePullbackRequest) controlmodel.EvidencePullbackRequest {
	req = controlmodel.NormalizeEvidencePullback(req)
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

func (s *Store) ListEvidencePullbacks(tenantID, agentID string) []controlmodel.EvidencePullbackRequest {
	if backend, ctx := s.backendCtx(); backend != nil {
		if pullbacks, err := backend.ListEvidencePullbacks(ctx, tenantID, agentID); err == nil {
			return pullbacks
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]controlmodel.EvidencePullbackRequest, 0, len(s.Pullbacks))
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

func (s *Store) ListEvidencePullbacksWithError(tenantID, agentID string) ([]controlmodel.EvidencePullbackRequest, error) {
	if backend, ctx := s.backendCtx(); backend != nil {
		pullbacks, err := backend.ListEvidencePullbacks(ctx, tenantID, agentID)
		if err != nil {
			return nil, fmt.Errorf("list evidence pullbacks: %w", err)
		}
		return pullbacks, nil
	}
	return s.ListEvidencePullbacks(tenantID, agentID), nil
}

func (s *Store) GetEvidencePullback(requestID, tenantID, agentID string) (controlmodel.EvidencePullbackRequest, bool) {
	if requestID == "" {
		return controlmodel.EvidencePullbackRequest{}, false
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
	return controlmodel.EvidencePullbackRequest{}, false
}

func (s *Store) GetEvidencePullbackWithError(requestID, tenantID, agentID string) (controlmodel.EvidencePullbackRequest, bool, error) {
	if requestID == "" {
		return controlmodel.EvidencePullbackRequest{}, false, nil
	}
	if backend, ctx := s.backendCtx(); backend != nil {
		pullbacks, err := backend.ListEvidencePullbacks(ctx, tenantID, agentID)
		if err != nil {
			return controlmodel.EvidencePullbackRequest{}, false, fmt.Errorf("get evidence pullback: %w", err)
		}
		for _, req := range pullbacks {
			if req.RequestID == requestID {
				return req, true, nil
			}
		}
		return controlmodel.EvidencePullbackRequest{}, false, nil
	}
	req, ok := s.GetEvidencePullback(requestID, tenantID, agentID)
	return req, ok, nil
}

func (s *Store) PendingEvidencePullbacks(tenantID, agentID string) []controlmodel.EvidencePullbackRequest {
	all := s.ListEvidencePullbacks(tenantID, agentID)
	out := make([]controlmodel.EvidencePullbackRequest, 0, len(all))
	for _, req := range all {
		if req.Status == controlmodel.EvidencePullbackStatusPending {
			out = append(out, req)
		}
	}
	return out
}

func (s *Store) PendingEvidencePullbacksWithError(tenantID, agentID string) ([]controlmodel.EvidencePullbackRequest, error) {
	all, err := s.ListEvidencePullbacksWithError(tenantID, agentID)
	if err != nil {
		return nil, err
	}
	out := make([]controlmodel.EvidencePullbackRequest, 0, len(all))
	for _, req := range all {
		if req.Status == controlmodel.EvidencePullbackStatusPending {
			out = append(out, req)
		}
	}
	return out, nil
}

func (s *Store) CompleteEvidencePullback(result controlmodel.EvidencePullbackResult) (controlmodel.EvidencePullbackRequest, bool) {
	if result.RequestID == "" {
		return controlmodel.EvidencePullbackRequest{}, false
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
			req.Status = controlmodel.EvidencePullbackStatusCompleted
		} else {
			req.Status = controlmodel.EvidencePullbackStatusFailed
		}
		s.Pullbacks[i] = req
		return req, true
	}
	return controlmodel.EvidencePullbackRequest{}, false
}

func (s *Store) CreateControlCommand(cmd controlmodel.ControlCommand) (controlmodel.ControlCommand, error) {
	cmd = controlmodel.NormalizeControlCommand(cmd)
	s.durableMu.Lock()
	defer s.durableMu.Unlock()
	s.mu.RLock()
	backend, ctx := s.backend, s.baseCtx
	if backend == nil {
		for _, existing := range s.ControlCommands {
			if existing.CommandID == cmd.CommandID && existing.TenantID == cmd.TenantID {
				s.mu.RUnlock()
				return existing, nil
			}
		}
	}
	s.mu.RUnlock()
	if backend != nil {
		out, authoritative, err := s.createControlCommandInBackend(backend, ctxOrBackground(ctx), cmd)
		if err != nil {
			return controlmodel.ControlCommand{}, err
		}
		if authoritative {
			return out, nil
		}
	}
	s.mu.Lock()
	oldCommands := s.ControlCommands
	s.ControlCommands = upsertControlCommandSnapshot(append([]controlmodel.ControlCommand(nil), s.ControlCommands...), cmd)
	if err := s.persistFileLocked(); err != nil {
		s.ControlCommands = oldCommands
		s.mu.Unlock()
		return controlmodel.ControlCommand{}, fmt.Errorf("write control command: %w", err)
	}
	s.mu.Unlock()
	return cmd, nil
}

func (s *Store) createControlCommandInBackend(backend Backend, ctx context.Context, cmd controlmodel.ControlCommand) (controlmodel.ControlCommand, bool, error) {
	commands, err := backend.ListControlCommands(ctx, cmd.TenantID, "", "")
	if err != nil {
		return controlmodel.ControlCommand{}, false, fmt.Errorf("find existing control command: %w", err)
	}
	if existing, ok := findControlCommandByTenant(commands, cmd.TenantID, cmd.CommandID); ok {
		s.cacheControlCommand(existing)
		return existing, true, nil
	}
	created, err := backend.CreateControlCommand(ctx, cmd)
	if err != nil {
		return controlmodel.ControlCommand{}, false, fmt.Errorf("write control command: %w", err)
	}
	if created {
		return cmd, false, nil
	}
	commands, err = backend.ListControlCommands(ctx, cmd.TenantID, "", "")
	if err != nil {
		return controlmodel.ControlCommand{}, false, fmt.Errorf("reload conflicting control command: %w", err)
	}
	existing, ok := findControlCommandByTenant(commands, cmd.TenantID, cmd.CommandID)
	if !ok {
		return controlmodel.ControlCommand{}, false, fmt.Errorf("reload conflicting control command: command not found")
	}
	s.cacheControlCommand(existing)
	return existing, true, nil
}

func (s *Store) cacheControlCommand(cmd controlmodel.ControlCommand) {
	s.mu.Lock()
	s.ControlCommands = upsertControlCommandSnapshot(s.ControlCommands, cmd)
	s.mu.Unlock()
}

func (s *Store) ListControlCommands(tenantID, agentID, commandType string) []controlmodel.ControlCommand {
	if backend, ctx := s.backendCtx(); backend != nil {
		if commands, err := backend.ListControlCommands(ctx, tenantID, agentID, commandType); err == nil {
			return commands
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]controlmodel.ControlCommand, 0, len(s.ControlCommands))
	for _, cmd := range s.ControlCommands {
		if tenantID != "" && cmd.TenantID != tenantID {
			continue
		}
		if agentID != "" && cmd.AgentID != agentID {
			continue
		}
		if commandType != "" && cmd.Type != commandType {
			continue
		}
		out = append(out, cmd)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

func (s *Store) ListControlCommandsWithError(tenantID, agentID, commandType string) ([]controlmodel.ControlCommand, error) {
	if backend, ctx := s.backendCtx(); backend != nil {
		commands, err := backend.ListControlCommands(ctx, tenantID, agentID, commandType)
		if err != nil {
			return nil, fmt.Errorf("list control commands: %w", err)
		}
		return commands, nil
	}
	return s.ListControlCommands(tenantID, agentID, commandType), nil
}

func (s *Store) PendingControlCommands(tenantID, agentID string) []controlmodel.ControlCommand {
	all := s.ListControlCommands(tenantID, agentID, "")
	out := make([]controlmodel.ControlCommand, 0, len(all))
	for _, cmd := range all {
		if controlmodel.ControlCommandTerminalStatus(cmd.Status) {
			continue
		}
		out = append(out, cmd)
	}
	return out
}

func (s *Store) PendingControlCommandsWithError(tenantID, agentID string) ([]controlmodel.ControlCommand, error) {
	all, err := s.ListControlCommandsWithError(tenantID, agentID, "")
	if err != nil {
		return nil, err
	}
	out := make([]controlmodel.ControlCommand, 0, len(all))
	for _, cmd := range all {
		if !controlmodel.ControlCommandTerminalStatus(cmd.Status) {
			out = append(out, cmd)
		}
	}
	return out, nil
}

func (s *Store) MarkControlCommandSent(commandID, tenantID, agentID string, sentAt time.Time) (controlmodel.ControlCommand, bool) {
	if commandID == "" {
		return controlmodel.ControlCommand{}, false
	}
	if sentAt.IsZero() {
		sentAt = time.Now().UTC()
	}
	s.mu.Lock()
	for i, cmd := range s.ControlCommands {
		if cmd.CommandID != commandID {
			continue
		}
		if tenantID != "" && cmd.TenantID != tenantID {
			continue
		}
		if agentID != "" && cmd.AgentID != agentID {
			continue
		}
		if !controlmodel.ControlCommandTerminalStatus(cmd.Status) {
			cmd.Status = controlmodel.ControlCommandStatusSent
		}
		cmd.SentAt = sentAt
		cmd.LastSentAt = sentAt
		cmd.AttemptCount++
		cmd.UpdatedAt = sentAt
		s.ControlCommands[i] = cmd
		s.mu.Unlock()
		return cmd, true
	}
	s.mu.Unlock()
	for _, cmd := range s.ListControlCommands(tenantID, agentID, "") {
		if cmd.CommandID != commandID {
			continue
		}
		if !controlmodel.ControlCommandTerminalStatus(cmd.Status) {
			cmd.Status = controlmodel.ControlCommandStatusSent
		}
		cmd.SentAt = sentAt
		cmd.LastSentAt = sentAt
		cmd.AttemptCount++
		cmd.UpdatedAt = sentAt
		s.mu.Lock()
		s.ControlCommands = upsertControlCommandSnapshot(s.ControlCommands, cmd)
		s.mu.Unlock()
		return cmd, true
	}
	return controlmodel.ControlCommand{}, false
}

func (s *Store) CancelControlCommand(commandID, tenantID, agentID, actor, reason string) (controlmodel.ControlCommand, bool) {
	if commandID == "" {
		return controlmodel.ControlCommand{}, false
	}
	now := time.Now().UTC()
	return s.updateControlCommand(commandID, tenantID, agentID, func(cmd controlmodel.ControlCommand) controlmodel.ControlCommand {
		if controlmodel.ControlCommandTerminalStatus(cmd.Status) {
			return cmd
		}
		cmd.Status = controlmodel.ControlCommandStatusCanceled
		cmd.CanceledAt = now
		cmd.UpdatedAt = now
		if actor != "" {
			cmd.Actor = actor
		}
		if reason != "" {
			cmd.Error = reason
		}
		return cmd
	})
}

func (s *Store) RetryControlCommand(commandID, tenantID, agentID, actor, reason string) (controlmodel.ControlCommand, bool) {
	if commandID == "" {
		return controlmodel.ControlCommand{}, false
	}
	now := time.Now().UTC()
	return s.updateControlCommand(commandID, tenantID, agentID, func(cmd controlmodel.ControlCommand) controlmodel.ControlCommand {
		if cmd.Status == controlmodel.ControlCommandStatusApplied {
			return cmd
		}
		cmd.Status = controlmodel.ControlCommandStatusPending
		cmd.UpdatedAt = now
		cmd.AckedAt = time.Time{}
		cmd.CanceledAt = time.Time{}
		cmd.ExpiredAt = time.Time{}
		cmd.AckStatus = ""
		cmd.AckMessage = ""
		cmd.AckPolicyID = ""
		cmd.AckPolicyVer = 0
		cmd.AckReportJSON = ""
		cmd.Error = ""
		if actor != "" {
			cmd.Actor = actor
		}
		if reason != "" {
			cmd.Reason = reason
		}
		return cmd
	})
}

func (s *Store) ExpireControlCommand(commandID, tenantID, agentID, reason string) (controlmodel.ControlCommand, bool) {
	if commandID == "" {
		return controlmodel.ControlCommand{}, false
	}
	now := time.Now().UTC()
	return s.updateControlCommand(commandID, tenantID, agentID, func(cmd controlmodel.ControlCommand) controlmodel.ControlCommand {
		if controlmodel.ControlCommandTerminalStatus(cmd.Status) {
			return cmd
		}
		cmd.Status = controlmodel.ControlCommandStatusExpired
		cmd.ExpiredAt = now
		cmd.UpdatedAt = now
		if reason != "" {
			cmd.Error = reason
		}
		return cmd
	})
}

func (s *Store) updateControlCommand(commandID, tenantID, agentID string, update func(controlmodel.ControlCommand) controlmodel.ControlCommand) (controlmodel.ControlCommand, bool) {
	s.mu.Lock()
	for i, cmd := range s.ControlCommands {
		if cmd.CommandID != commandID {
			continue
		}
		if tenantID != "" && cmd.TenantID != tenantID {
			continue
		}
		if agentID != "" && cmd.AgentID != agentID {
			continue
		}
		cmd = update(cmd)
		s.ControlCommands[i] = cmd
		s.mu.Unlock()
		return cmd, true
	}
	s.mu.Unlock()
	for _, cmd := range s.ListControlCommands(tenantID, agentID, "") {
		if cmd.CommandID != commandID {
			continue
		}
		cmd = update(cmd)
		s.mu.Lock()
		s.ControlCommands = upsertControlCommandSnapshot(s.ControlCommands, cmd)
		s.mu.Unlock()
		return cmd, true
	}
	return controlmodel.ControlCommand{}, false
}

func (s *Store) AckControlCommand(ack controlmodel.ControlCommandAck) (controlmodel.ControlCommand, bool, error) {
	if ack.CommandID == "" {
		return controlmodel.ControlCommand{}, false, nil
	}
	if ack.ObservedAt.IsZero() {
		ack.ObservedAt = time.Now().UTC()
	}
	if ack.TenantID == "" {
		ack.TenantID = "default"
	}
	if ack.AgentID == "" {
		return controlmodel.ControlCommand{}, false, nil
	}
	s.durableMu.Lock()
	defer s.durableMu.Unlock()
	s.mu.RLock()
	cmd, ok := findControlCommand(s.ControlCommands, ack.TenantID, ack.AgentID, ack.CommandID)
	backend, ctx := s.backend, s.baseCtx
	s.mu.RUnlock()
	if !ok && backend != nil {
		commands, err := backend.ListControlCommands(ctxOrBackground(ctx), ack.TenantID, ack.AgentID, "")
		if err != nil {
			return controlmodel.ControlCommand{}, false, fmt.Errorf("find control command for ack: %w", err)
		}
		cmd, ok = findControlCommand(commands, ack.TenantID, ack.AgentID, ack.CommandID)
	}
	if !ok {
		return controlmodel.ControlCommand{}, false, nil
	}
	cmd = applyControlCommandAck(cmd, ack)
	if backend != nil {
		if err := backend.WriteControlCommand(ctxOrBackground(ctx), cmd); err != nil {
			return controlmodel.ControlCommand{}, false, fmt.Errorf("write control command ack: %w", err)
		}
	}
	s.mu.Lock()
	oldCommands := s.ControlCommands
	s.ControlCommands = upsertControlCommandSnapshot(append([]controlmodel.ControlCommand(nil), s.ControlCommands...), cmd)
	if err := s.persistFileLocked(); err != nil {
		s.ControlCommands = oldCommands
		s.mu.Unlock()
		return controlmodel.ControlCommand{}, false, fmt.Errorf("write control command ack: %w", err)
	}
	s.mu.Unlock()
	return cmd, true, nil
}

func findControlCommand(commands []controlmodel.ControlCommand, tenantID, agentID, commandID string) (controlmodel.ControlCommand, bool) {
	for _, cmd := range commands {
		if cmd.CommandID == commandID && cmd.TenantID == tenantID && cmd.AgentID == agentID {
			return cmd, true
		}
	}
	return controlmodel.ControlCommand{}, false
}

func findControlCommandByTenant(commands []controlmodel.ControlCommand, tenantID, commandID string) (controlmodel.ControlCommand, bool) {
	for _, cmd := range commands {
		if cmd.CommandID == commandID && cmd.TenantID == tenantID {
			return cmd, true
		}
	}
	return controlmodel.ControlCommand{}, false
}

func applyControlCommandAck(cmd controlmodel.ControlCommand, ack controlmodel.ControlCommandAck) controlmodel.ControlCommand {
	status := ack.Status
	if status == "" {
		status = controlmodel.ControlCommandStatusApplied
	}
	switch status {
	case controlmodel.ControlCommandStatusApplied, "accepted", "validated", "degraded":
		cmd.Status = controlmodel.ControlCommandStatusApplied
	case controlmodel.ControlCommandStatusRejected:
		cmd.Status = controlmodel.ControlCommandStatusRejected
		cmd.Error = ack.Message
	case controlmodel.ControlCommandStatusFailed:
		cmd.Status = controlmodel.ControlCommandStatusFailed
		cmd.Error = ack.Message
	default:
		cmd.Status = status
	}
	cmd.AckStatus = ack.Status
	cmd.AckMessage = ack.Message
	cmd.AckPolicyID = ack.PolicyID
	cmd.AckPolicyVer = ack.PolicyVersion
	cmd.AckReportJSON = ack.ReportJSON
	cmd.AckedAt = ack.ObservedAt
	cmd.UpdatedAt = ack.ObservedAt
	return cmd
}

func upsertControlCommandSnapshot(commands []controlmodel.ControlCommand, cmd controlmodel.ControlCommand) []controlmodel.ControlCommand {
	for i, existing := range commands {
		if existing.CommandID == cmd.CommandID && existing.TenantID == cmd.TenantID {
			commands[i] = cmd
			return commands
		}
	}
	return append(commands, cmd)
}

func (s *Store) ReplaceDerivedForLabels(labels LabelSelector, cloudSignals []*signalv1.Signal, incidents []*incidentv1.Incident) {
	s.mu.Lock()
	defer s.mu.Unlock()
	signals := s.Signals[:0]
	for _, sig := range s.Signals {
		if labelSelectorMatches(sig.GetLabels(), labels) && layerName(sig.GetWhere()) == "cloud" {
			continue
		}
		signals = append(signals, sig)
	}
	s.Signals = signals
	keptIncidents := s.Incidents[:0]
	for _, inc := range s.Incidents {
		if labelSelectorMatches(inc.GetLabels(), labels) {
			continue
		}
		keptIncidents = append(keptIncidents, inc)
	}
	s.Incidents = keptIncidents
	s.Signals = append(s.Signals, cloudSignals...)
	s.Incidents = append(s.Incidents, incidents...)
}

func (s *Store) RecordDataBatchIngest(events, endpointSignals, cloudSignals, incidents int, convergenceLatency time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	latencyMs := uint64(convergenceLatency.Milliseconds())
	s.Metrics.DataBatchesAppended++
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
	s.Metrics.AverageConvergenceLatency = float64(s.Metrics.TotalConvergenceLatencyMs) / float64(s.Metrics.DataBatchesAppended)
}

func (s *Store) RecordDataBatchAppend(agent AgentIdentity, batchID, transport string, observedAt time.Time) AgentSession {
	agent = agent.Normalized()
	if !agent.Valid() {
		return AgentSession{}
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	sessionID := agentSessionID(agent.TenantID, agent.AgentID)
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.AgentSessions {
		if existing.SessionID == sessionID {
			existing.LastSeenAt = observedAt
			existing.LastDataSeenAt = observedAt
			if batchID != "" {
				existing.LastAckCursor = batchID
			}
			if transport != "" {
				existing.DataTransport = transport
			}
			existing.Status = "active"
			existing.ClosedAt = time.Time{}
			s.AgentSessions[i] = existing
			return existing
		}
	}
	session := AgentSession{
		SessionID:      sessionID,
		TenantID:       agent.TenantID,
		AgentID:        agent.AgentID,
		StartedAt:      observedAt,
		LastSeenAt:     observedAt,
		LastDataSeenAt: observedAt,
		LastAckCursor:  batchID,
		DataTransport:  transport,
		Status:         "active",
	}
	s.AgentSessions = append(s.AgentSessions, session)
	return session
}

func (s *Store) RecordControlSessionOpen(tenantID, agentID, transport string, observedAt time.Time) AgentSession {
	return s.updateAgentSession(tenantID, agentID, transport, "", "open", observedAt, false)
}

func (s *Store) RecordAgentSessionSeen(tenantID, agentID string, observedAt time.Time) AgentSession {
	return s.updateAgentSession(tenantID, agentID, "", "", "", observedAt, false)
}

func (s *Store) CloseAgentSession(tenantID, agentID string, observedAt time.Time) AgentSession {
	return s.updateAgentSession(tenantID, agentID, "", "", "closed", observedAt, true)
}

func (s *Store) updateAgentSession(tenantID, agentID, transport, cursor, status string, observedAt time.Time, closeSession bool) AgentSession {
	tenantID = strings.TrimSpace(tenantID)
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return AgentSession{}
	}
	if tenantID == "" {
		tenantID = "default"
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	sessionID := agentSessionID(tenantID, agentID)
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, session := range s.AgentSessions {
		if session.SessionID != sessionID {
			continue
		}
		session.LastSeenAt = observedAt
		if transport != "" {
			session.ControlTransport = transport
			session.LastControlSeenAt = observedAt
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
		s.AgentSessions[i] = session
		return session
	}
	session := AgentSession{
		SessionID:        sessionID,
		TenantID:         tenantID,
		AgentID:          agentID,
		StartedAt:        observedAt,
		LastSeenAt:       observedAt,
		LastAckCursor:    cursor,
		ControlTransport: transport,
		Status:           status,
	}
	if transport != "" {
		session.LastControlSeenAt = observedAt
	}
	if session.Status == "" {
		session.Status = "active"
	}
	if closeSession {
		session.ClosedAt = observedAt
	}
	s.AgentSessions = append(s.AgentSessions, session)
	return session
}

func (s *Store) ListAgents() []AgentIdentity {
	if backend, ctx := s.backendCtx(); backend != nil {
		if agents, err := backend.ListAgents(ctx); err == nil {
			return agents
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]AgentIdentity, len(s.Agents))
	copy(out, s.Agents)
	return out
}

func (s *Store) ListAgentsWithError() ([]AgentIdentity, error) {
	if backend, ctx := s.backendCtx(); backend != nil {
		agents, err := backend.ListAgents(ctx)
		if err != nil {
			return nil, fmt.Errorf("list agents: %w", err)
		}
		return agents, nil
	}
	return s.ListAgents(), nil
}

func (s *Store) ListAgentSessions(tenantID, agentID string) []AgentSession {
	if backend, ctx := s.backendCtx(); backend != nil {
		if sessions, err := backend.ListAgentSessions(ctx, tenantID, agentID); err == nil {
			return sessions
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]AgentSession, 0, len(s.AgentSessions))
	for _, session := range s.AgentSessions {
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

func (s *Store) ListAgentSessionsWithError(tenantID, agentID string) ([]AgentSession, error) {
	if backend, ctx := s.backendCtx(); backend != nil {
		sessions, err := backend.ListAgentSessions(ctx, tenantID, agentID)
		if err != nil {
			return nil, fmt.Errorf("list agent sessions: %w", err)
		}
		return sessions, nil
	}
	return s.ListAgentSessions(tenantID, agentID), nil
}

func (s *Store) CreateEnrollment(enrollment Enrollment) Enrollment {
	enrollment = normalizeEnrollment(enrollment)
	if enrollment.EnrollmentID == "" || enrollment.TokenHash == "" {
		return Enrollment{}
	}
	now := time.Now().UTC()
	if enrollment.CreatedAt.IsZero() {
		enrollment.CreatedAt = now
	}
	if enrollment.Status == "" {
		enrollment.Status = "active"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.Enrollments {
		if existing.TenantID == enrollment.TenantID && existing.EnrollmentID == enrollment.EnrollmentID {
			s.Enrollments[i] = enrollment
			return cloneEnrollment(enrollment)
		}
	}
	s.Enrollments = append(s.Enrollments, enrollment)
	return cloneEnrollment(enrollment)
}

func (s *Store) ListEnrollments(tenantID, status string) []Enrollment {
	if backend, ctx := s.backendCtx(); backend != nil {
		if enrollments, err := backend.ListEnrollments(ctx, tenantID, status); err == nil {
			return enrollments
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Enrollment, 0, len(s.Enrollments))
	for _, enrollment := range s.Enrollments {
		if tenantID != "" && enrollment.TenantID != tenantID {
			continue
		}
		if status != "" && enrollment.Status != status {
			continue
		}
		out = append(out, cloneEnrollment(enrollment))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TenantID == out[j].TenantID {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].TenantID < out[j].TenantID
	})
	return out
}

func (s *Store) ListEnrollmentsWithError(tenantID, status string) ([]Enrollment, error) {
	if backend, ctx := s.backendCtx(); backend != nil {
		enrollments, err := backend.ListEnrollments(ctx, tenantID, status)
		if err != nil {
			return nil, fmt.Errorf("list enrollments: %w", err)
		}
		return enrollments, nil
	}
	return s.ListEnrollments(tenantID, status), nil
}

func (s *Store) GetEnrollmentByTokenHash(tokenHash string) (Enrollment, bool) {
	tokenHash = strings.TrimSpace(tokenHash)
	if tokenHash == "" {
		return Enrollment{}, false
	}
	if backend, ctx := s.backendCtx(); backend != nil {
		if enrollment, ok, err := backend.GetEnrollmentByTokenHash(ctx, tokenHash); err == nil {
			return enrollment, ok
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, enrollment := range s.Enrollments {
		if enrollment.TokenHash == tokenHash {
			return cloneEnrollment(enrollment), true
		}
	}
	return Enrollment{}, false
}

func (s *Store) GetEnrollmentByTokenHashWithError(tokenHash string) (Enrollment, bool, error) {
	tokenHash = strings.TrimSpace(tokenHash)
	if tokenHash == "" {
		return Enrollment{}, false, nil
	}
	if backend, ctx := s.backendCtx(); backend != nil {
		enrollment, ok, err := backend.GetEnrollmentByTokenHash(ctx, tokenHash)
		if err != nil {
			return Enrollment{}, false, fmt.Errorf("get enrollment by token: %w", err)
		}
		return enrollment, ok, nil
	}
	enrollment, ok := s.GetEnrollmentByTokenHash(tokenHash)
	return enrollment, ok, nil
}

func (s *Store) MarkEnrollmentUsed(tokenHash string, usedAt time.Time) (Enrollment, bool) {
	tokenHash = strings.TrimSpace(tokenHash)
	if tokenHash == "" {
		return Enrollment{}, false
	}
	if usedAt.IsZero() {
		usedAt = time.Now().UTC()
	}
	if backend, ctx := s.backendCtx(); backend != nil {
		enrollment, ok, err := backend.GetEnrollmentByTokenHash(ctx, tokenHash)
		if err == nil && ok && enrollment.Status == "active" {
			enrollment.Status = "used"
			enrollment.UsedAt = usedAt
			if err := backend.WriteEnrollment(ctx, enrollment); err == nil {
				s.replaceEnrollmentInMemory(enrollment)
				return cloneEnrollment(enrollment), true
			}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, enrollment := range s.Enrollments {
		if enrollment.TokenHash != tokenHash {
			continue
		}
		if enrollment.Status != "active" {
			return Enrollment{}, false
		}
		enrollment.Status = "used"
		enrollment.UsedAt = usedAt
		s.Enrollments[i] = enrollment
		return cloneEnrollment(enrollment), true
	}
	return Enrollment{}, false
}

func (s *Store) replaceEnrollmentInMemory(enrollment Enrollment) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.Enrollments {
		if existing.TenantID == enrollment.TenantID && existing.EnrollmentID == enrollment.EnrollmentID {
			s.Enrollments[i] = enrollment
			return
		}
	}
	s.Enrollments = append(s.Enrollments, enrollment)
}

func (s *Store) UpsertArtifact(artifact Artifact) Artifact {
	artifact = normalizeArtifact(artifact)
	if artifact.ArtifactID == "" || artifact.Name == "" || artifact.Kind == "" || artifact.Version == "" || artifact.SHA256 == "" {
		return Artifact{}
	}
	now := time.Now().UTC()
	if artifact.UpdatedAt.IsZero() {
		artifact.UpdatedAt = now
	}
	if artifact.Status == "" {
		artifact.Status = "draft"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.Artifacts {
		if existing.TenantID == artifact.TenantID && existing.ArtifactID == artifact.ArtifactID {
			if artifact.CreatedAt.IsZero() {
				artifact.CreatedAt = existing.CreatedAt
			}
			if artifact.CreatedAt.IsZero() {
				artifact.CreatedAt = now
			}
			s.Artifacts[i] = artifact
			return cloneArtifact(artifact)
		}
	}
	if artifact.CreatedAt.IsZero() {
		artifact.CreatedAt = now
	}
	s.Artifacts = append(s.Artifacts, artifact)
	return cloneArtifact(artifact)
}

func (s *Store) ListArtifacts(tenantID, kind, status string) []Artifact {
	if backend, ctx := s.backendCtx(); backend != nil {
		if artifacts, err := backend.ListArtifacts(ctx, tenantID, kind, status); err == nil {
			return artifacts
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Artifact, 0, len(s.Artifacts))
	for _, artifact := range s.Artifacts {
		if tenantID != "" && artifact.TenantID != tenantID {
			continue
		}
		if kind != "" && artifact.Kind != kind {
			continue
		}
		if status != "" && artifact.Status != status {
			continue
		}
		out = append(out, cloneArtifact(artifact))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TenantID == out[j].TenantID {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].TenantID < out[j].TenantID
	})
	return out
}

func (s *Store) ListArtifactsWithError(tenantID, kind, status string) ([]Artifact, error) {
	if backend, ctx := s.backendCtx(); backend != nil {
		artifacts, err := backend.ListArtifacts(ctx, tenantID, kind, status)
		if err != nil {
			return nil, fmt.Errorf("list artifacts: %w", err)
		}
		return artifacts, nil
	}
	return s.ListArtifacts(tenantID, kind, status), nil
}

func (s *Store) GetArtifact(tenantID, artifactID string) (Artifact, bool) {
	tenantID = strings.TrimSpace(tenantID)
	artifactID = strings.TrimSpace(artifactID)
	if artifactID == "" {
		return Artifact{}, false
	}
	if tenantID == "" {
		tenantID = "default"
	}
	if backend, ctx := s.backendCtx(); backend != nil {
		if artifact, ok, err := backend.GetArtifact(ctx, tenantID, artifactID); err == nil {
			return artifact, ok
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, artifact := range s.Artifacts {
		if artifact.TenantID == tenantID && artifact.ArtifactID == artifactID {
			return cloneArtifact(artifact), true
		}
	}
	return Artifact{}, false
}

func (s *Store) GetArtifactWithError(tenantID, artifactID string) (Artifact, bool, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		tenantID = "default"
	}
	artifactID = strings.TrimSpace(artifactID)
	if artifactID == "" {
		return Artifact{}, false, nil
	}
	if backend, ctx := s.backendCtx(); backend != nil {
		artifact, ok, err := backend.GetArtifact(ctx, tenantID, artifactID)
		if err != nil {
			return Artifact{}, false, fmt.Errorf("get artifact: %w", err)
		}
		return artifact, ok, nil
	}
	artifact, ok := s.GetArtifact(tenantID, artifactID)
	return artifact, ok, nil
}

func normalizeEnrollment(enrollment Enrollment) Enrollment {
	enrollment.EnrollmentID = strings.TrimSpace(enrollment.EnrollmentID)
	enrollment.TenantID = strings.TrimSpace(enrollment.TenantID)
	if enrollment.TenantID == "" {
		enrollment.TenantID = "default"
	}
	enrollment.AgentID = strings.TrimSpace(enrollment.AgentID)
	enrollment.HostID = strings.TrimSpace(enrollment.HostID)
	enrollment.TokenHash = strings.TrimSpace(enrollment.TokenHash)
	enrollment.TokenPreview = strings.TrimSpace(enrollment.TokenPreview)
	enrollment.GatewayAddr = strings.TrimSpace(enrollment.GatewayAddr)
	enrollment.GatewaySNI = strings.TrimSpace(enrollment.GatewaySNI)
	enrollment.Profile = strings.TrimSpace(enrollment.Profile)
	enrollment.Channel = strings.TrimSpace(enrollment.Channel)
	enrollment.ArtifactID = strings.TrimSpace(enrollment.ArtifactID)
	enrollment.ArtifactSHA256 = strings.TrimSpace(enrollment.ArtifactSHA256)
	enrollment.ArtifactURL = strings.TrimSpace(enrollment.ArtifactURL)
	enrollment.Status = strings.TrimSpace(enrollment.Status)
	enrollment.CreatedBy = strings.TrimSpace(enrollment.CreatedBy)
	enrollment.Labels = cloneStringMap(enrollment.Labels)
	return enrollment
}

func normalizeArtifact(artifact Artifact) Artifact {
	artifact.ArtifactID = strings.TrimSpace(artifact.ArtifactID)
	artifact.TenantID = strings.TrimSpace(artifact.TenantID)
	if artifact.TenantID == "" {
		artifact.TenantID = "default"
	}
	artifact.Name = strings.TrimSpace(artifact.Name)
	artifact.Kind = strings.TrimSpace(artifact.Kind)
	artifact.Version = strings.TrimSpace(artifact.Version)
	artifact.OS = strings.TrimSpace(artifact.OS)
	artifact.Arch = strings.TrimSpace(artifact.Arch)
	artifact.SHA256 = strings.TrimSpace(artifact.SHA256)
	artifact.Status = strings.TrimSpace(artifact.Status)
	artifact.StoragePath = strings.TrimSpace(artifact.StoragePath)
	artifact.CreatedBy = strings.TrimSpace(artifact.CreatedBy)
	artifact.Metadata = cloneStringMap(artifact.Metadata)
	return artifact
}

func cloneArtifact(artifact Artifact) Artifact {
	artifact.Metadata = cloneStringMap(artifact.Metadata)
	return artifact
}

func cloneEnrollment(enrollment Enrollment) Enrollment {
	enrollment.Labels = cloneStringMap(enrollment.Labels)
	return enrollment
}

func (s *Store) UpsertChannel(channel ArtifactChannel) ArtifactChannel {
	channel = normalizeChannel(channel)
	if channel.Channel == "" || channel.ArtifactID == "" {
		return ArtifactChannel{}
	}
	now := time.Now().UTC()
	if channel.UpdatedAt.IsZero() {
		channel.UpdatedAt = now
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.Channels {
		if existing.TenantID == channel.TenantID && existing.Channel == channel.Channel {
			if channel.CreatedAt.IsZero() {
				channel.CreatedAt = existing.CreatedAt
			}
			if channel.CreatedAt.IsZero() {
				channel.CreatedAt = now
			}
			s.Channels[i] = channel
			return channel
		}
	}
	if channel.CreatedAt.IsZero() {
		channel.CreatedAt = now
	}
	s.Channels = append(s.Channels, channel)
	return channel
}

func (s *Store) ListChannels(tenantID string) []ArtifactChannel {
	if backend, ctx := s.backendCtx(); backend != nil {
		if channels, err := backend.ListChannels(ctx, tenantID); err == nil {
			return channels
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ArtifactChannel, 0, len(s.Channels))
	for _, channel := range s.Channels {
		if tenantID != "" && channel.TenantID != tenantID {
			continue
		}
		out = append(out, channel)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TenantID == out[j].TenantID {
			return out[i].Channel < out[j].Channel
		}
		return out[i].TenantID < out[j].TenantID
	})
	return out
}

func (s *Store) ListChannelsWithError(tenantID string) ([]ArtifactChannel, error) {
	if backend, ctx := s.backendCtx(); backend != nil {
		channels, err := backend.ListChannels(ctx, tenantID)
		if err != nil {
			return nil, fmt.Errorf("list channels: %w", err)
		}
		return channels, nil
	}
	return s.ListChannels(tenantID), nil
}

func (s *Store) GetChannel(tenantID, channelName string) (ArtifactChannel, bool) {
	tenantID = strings.TrimSpace(tenantID)
	channelName = strings.TrimSpace(channelName)
	if tenantID == "" {
		tenantID = "default"
	}
	if channelName == "" {
		return ArtifactChannel{}, false
	}
	if backend, ctx := s.backendCtx(); backend != nil {
		if channel, ok, err := backend.GetChannel(ctx, tenantID, channelName); err == nil {
			return channel, ok
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, channel := range s.Channels {
		if channel.TenantID == tenantID && channel.Channel == channelName {
			return channel, true
		}
	}
	return ArtifactChannel{}, false
}

func (s *Store) GetChannelWithError(tenantID, channelName string) (ArtifactChannel, bool, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		tenantID = "default"
	}
	channelName = strings.TrimSpace(channelName)
	if channelName == "" {
		return ArtifactChannel{}, false, nil
	}
	if backend, ctx := s.backendCtx(); backend != nil {
		channel, ok, err := backend.GetChannel(ctx, tenantID, channelName)
		if err != nil {
			return ArtifactChannel{}, false, fmt.Errorf("get channel: %w", err)
		}
		return channel, ok, nil
	}
	channel, ok := s.GetChannel(tenantID, channelName)
	return channel, ok, nil
}

func (s *Store) RecordAgentCertificate(cert AgentCertificate) AgentCertificate {
	cert = normalizeAgentCertificate(cert)
	if cert.TenantID == "" || cert.AgentID == "" || cert.SerialNumber == "" {
		return AgentCertificate{}
	}
	now := time.Now().UTC()
	if cert.CreatedAt.IsZero() {
		cert.CreatedAt = now
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.Certificates {
		if existing.TenantID == cert.TenantID && existing.SerialNumber == cert.SerialNumber {
			s.Certificates[i] = cert
			return cert
		}
	}
	s.Certificates = append(s.Certificates, cert)
	return cert
}

func (s *Store) RevokeAgentCertificate(tenantID, agentID, enrollmentID, serial string, revokedAt time.Time) (AgentCertificate, bool, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		tenantID = "default"
	}
	agentID, enrollmentID, serial = strings.TrimSpace(agentID), strings.TrimSpace(enrollmentID), strings.TrimSpace(serial)
	if agentID == "" || enrollmentID == "" || serial == "" {
		return AgentCertificate{}, false, fmt.Errorf("certificate revocation identity is incomplete")
	}
	if revokedAt.IsZero() {
		revokedAt = time.Now().UTC()
	}
	receipt := certificateRevocationReceipt(tenantID, agentID, enrollmentID, serial)
	s.durableMu.Lock()
	defer s.durableMu.Unlock()
	if backend, ctx := s.backendCtx(); backend != nil {
		cert, ok, err := backend.RevokeAgentCertificate(ctx, tenantID, agentID, enrollmentID, serial, revokedAt, receipt)
		if err != nil || !ok {
			return AgentCertificate{}, ok, err
		}
		s.mu.Lock()
		s.Certificates = upsertAgentCertificateSnapshot(s.Certificates, cert)
		s.mu.Unlock()
		return cert, true, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, cert := range s.Certificates {
		if cert.TenantID != tenantID || cert.SerialNumber != serial {
			continue
		}
		if cert.AgentID != agentID || cert.EnrollmentID != enrollmentID {
			return AgentCertificate{}, false, fmt.Errorf("%w: certificate identity mismatch", ErrConflict)
		}
		changed := false
		if cert.RevokedAt.IsZero() {
			cert.RevokedAt, cert.RevocationReceipt = revokedAt.UTC(), receipt
			changed = true
		} else if cert.RevocationReceipt == "" {
			cert.RevocationReceipt = receipt
			changed = true
		}
		if changed {
			s.Certificates[i] = cert
			if err := s.persistFileLocked(); err != nil {
				return AgentCertificate{}, false, fmt.Errorf("persist certificate revocation: %w", err)
			}
		}
		return cert, true, nil
	}
	return AgentCertificate{}, false, nil
}

func (s *Store) GetAgentCertificateWithError(tenantID, serial string) (AgentCertificate, bool, error) {
	tenantID, serial = strings.TrimSpace(tenantID), strings.TrimSpace(serial)
	if tenantID == "" {
		tenantID = "default"
	}
	if backend, ctx := s.backendCtx(); backend != nil {
		cert, ok, err := backend.GetAgentCertificate(ctx, tenantID, serial)
		if err != nil {
			return AgentCertificate{}, false, fmt.Errorf("get agent certificate: %w", err)
		}
		return cert, ok, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, cert := range s.Certificates {
		if cert.TenantID == tenantID && cert.SerialNumber == serial {
			return cert, true, nil
		}
	}
	return AgentCertificate{}, false, nil
}

func certificateRevocationReceipt(tenantID, agentID, enrollmentID, serial string) string {
	sum := sha256.Sum256([]byte(tenantID + "\x00" + agentID + "\x00" + enrollmentID + "\x00" + serial))
	return "revoke-" + hex.EncodeToString(sum[:16])
}

func upsertAgentCertificateSnapshot(certs []AgentCertificate, cert AgentCertificate) []AgentCertificate {
	out := append([]AgentCertificate(nil), certs...)
	for i, existing := range out {
		if existing.TenantID == cert.TenantID && existing.SerialNumber == cert.SerialNumber {
			out[i] = cert
			return out
		}
	}
	return append(out, cert)
}

func normalizeChannel(channel ArtifactChannel) ArtifactChannel {
	channel.TenantID = strings.TrimSpace(channel.TenantID)
	if channel.TenantID == "" {
		channel.TenantID = "default"
	}
	channel.Channel = strings.TrimSpace(channel.Channel)
	channel.ArtifactID = strings.TrimSpace(channel.ArtifactID)
	channel.CreatedBy = strings.TrimSpace(channel.CreatedBy)
	return channel
}

func normalizeAgentCertificate(cert AgentCertificate) AgentCertificate {
	cert.TenantID = strings.TrimSpace(cert.TenantID)
	if cert.TenantID == "" {
		cert.TenantID = "default"
	}
	cert.AgentID = strings.TrimSpace(cert.AgentID)
	cert.EnrollmentID = strings.TrimSpace(cert.EnrollmentID)
	cert.SerialNumber = strings.TrimSpace(cert.SerialNumber)
	cert.Subject = strings.TrimSpace(cert.Subject)
	return cert
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (s *Store) ListAgentHealth() []agenthealth.AgentHealth {
	if backend, ctx := s.backendCtx(); backend != nil {
		if health, err := backend.ListAgentHealth(ctx); err == nil {
			return health
		}
	}
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

func (s *Store) ListAgentHealthWithError() ([]agenthealth.AgentHealth, error) {
	if backend, ctx := s.backendCtx(); backend != nil {
		health, err := backend.ListAgentHealth(ctx)
		if err != nil {
			return nil, fmt.Errorf("list agent health: %w", err)
		}
		return health, nil
	}
	return s.ListAgentHealth(), nil
}

func (s *Store) GetAgentHealth(tenantID, agentID string) (agenthealth.AgentHealth, bool) {
	if backend, ctx := s.backendCtx(); backend != nil {
		if health, ok, err := backend.GetAgentHealth(ctx, tenantID, agentID); err == nil {
			return health, ok
		}
	}
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

func (s *Store) GetAgentHealthWithError(tenantID, agentID string) (agenthealth.AgentHealth, bool, error) {
	if backend, ctx := s.backendCtx(); backend != nil {
		health, ok, err := backend.GetAgentHealth(ctx, tenantID, agentID)
		if err != nil {
			return agenthealth.AgentHealth{}, false, fmt.Errorf("get agent health: %w", err)
		}
		return health, ok, nil
	}
	health, ok := s.GetAgentHealth(tenantID, agentID)
	return health, ok, nil
}

// ListEvents reads from the in-process working set only. Telemetry is not
// persisted in the relational backend; durable event retention/search lives in
// the index tier (OpenSearch).
func (s *Store) ListEvents(labels LabelSelector, behavior string) []*eventv1.CanonicalEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*eventv1.CanonicalEvent, 0, len(s.Events))
	for _, ev := range s.Events {
		if !labelSelectorMatches(ev.GetLabels(), labels) {
			continue
		}
		if behavior != "" && ev.GetBehavior() != behavior {
			continue
		}
		out = append(out, ev)
	}
	return out
}

// ListSignals reads from the in-process working set only. Like events, signals
// are telemetry and are not persisted in the relational backend; durable
// retention/search lives in the index tier (OpenSearch).
func (s *Store) ListSignals(labels LabelSelector, layer string, terminalOnly bool) []*signalv1.Signal {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*signalv1.Signal, 0, len(s.Signals))
	for _, sig := range s.Signals {
		if !labelSelectorMatches(sig.GetLabels(), labels) {
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

func (s *Store) ListIncidents(labels LabelSelector) []*incidentv1.Incident {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*incidentv1.Incident, 0, len(s.Incidents))
	for _, inc := range s.Incidents {
		if !labelSelectorMatches(inc.GetLabels(), labels) {
			continue
		}
		out = append(out, inc)
	}
	return out
}

func (s *Store) GetIncident(id string, labels LabelSelector) (*incidentv1.Incident, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, inc := range s.Incidents {
		if id != "" && inc.GetId() != id {
			continue
		}
		if !labelSelectorMatches(inc.GetLabels(), labels) {
			continue
		}
		return inc, true
	}
	return nil, false
}

func (s *Store) AttachIncidentEvidence(id string, labels LabelSelector, evidence *incidentv1.EvidenceSubgraph) (*incidentv1.Incident, bool) {
	if (id == "" && len(labels) == 0) || evidence == nil {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, inc := range s.Incidents {
		if id != "" && inc.GetId() != id {
			continue
		}
		if !labelSelectorMatches(inc.GetLabels(), labels) {
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
	backend := s.backend
	ctx := ctxOrBackground(s.baseCtx)
	s.mu.RUnlock()
	if metricsBackend, ok := backend.(MetricsBackend); ok {
		if metrics, err := metricsBackend.LoadMetrics(ctx); err == nil {
			return metrics
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Metrics
}

func (s *Store) SaveMetrics() error {
	s.mu.RLock()
	metrics := s.Metrics
	backend := s.backend
	ctx := ctxOrBackground(s.baseCtx)
	s.mu.RUnlock()
	if metricsBackend, ok := backend.(MetricsBackend); ok {
		return metricsBackend.SaveMetrics(ctx, metrics)
	}
	return nil
}

func (s *Store) ResetMetrics() error {
	s.mu.Lock()
	s.Metrics = Metrics{}
	backend := s.backend
	ctx := ctxOrBackground(s.baseCtx)
	s.mu.Unlock()
	if metricsBackend, ok := backend.(MetricsBackend); ok {
		return metricsBackend.ResetMetrics(ctx)
	}
	return nil
}

func (s *Store) DeleteByLabels(labels LabelSelector) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(labels) == 0 {
		s.Agents = nil
		s.Events = nil
		s.Signals = nil
		s.Incidents = nil
		s.Health = map[string]agenthealth.AgentHealth{}
		s.AgentSessions = nil
		s.Metrics = Metrics{}
		return
	}
	events := s.Events[:0]
	for _, ev := range s.Events {
		if !labelSelectorMatches(ev.GetLabels(), labels) {
			events = append(events, ev)
		}
	}
	s.Events = events
	signals := s.Signals[:0]
	for _, sig := range s.Signals {
		if !labelSelectorMatches(sig.GetLabels(), labels) {
			signals = append(signals, sig)
		}
	}
	s.Signals = signals
	incidents := s.Incidents[:0]
	for _, inc := range s.Incidents {
		if !labelSelectorMatches(inc.GetLabels(), labels) {
			incidents = append(incidents, inc)
		}
	}
	s.Incidents = incidents
}

func (s *Store) Save() error {
	s.durableMu.Lock()
	defer s.durableMu.Unlock()
	s.mu.RLock()
	state, err := s.exportStateLocked()
	path := s.path
	backend := s.backend
	ctx := ctxOrBackground(s.baseCtx)
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	if backend != nil {
		return backend.SaveState(ctx, state)
	}
	if path == "" {
		return nil
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data)
}

func (s *Store) persistFileLocked() error {
	if s.backend != nil || s.path == "" {
		return nil
	}
	state, err := s.exportStateLocked()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.path, data)
}

// writeFileAtomic writes data to a temporary file in the destination directory
// and renames it into place, so a crash mid-write cannot leave a partially
// written or corrupt state file.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".store-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
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
	for _, enrollment := range s.Enrollments {
		state.Enrollments = append(state.Enrollments, cloneEnrollment(enrollment))
	}
	for _, artifact := range s.Artifacts {
		state.Artifacts = append(state.Artifacts, cloneArtifact(artifact))
	}
	state.Channels = append([]ArtifactChannel(nil), s.Channels...)
	state.Certificates = append([]AgentCertificate(nil), s.Certificates...)
	for _, agent := range s.Agents {
		raw, err := json.Marshal(agent)
		if err != nil {
			return State{}, err
		}
		state.Agents = append(state.Agents, raw)
	}
	mo := protojson.MarshalOptions{UseProtoNames: true}
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
	state.Pullbacks = append([]controlmodel.EvidencePullbackRequest(nil), s.Pullbacks...)
	state.ControlCommands = append([]controlmodel.ControlCommand(nil), s.ControlCommands...)
	state.AgentSessions = append([]AgentSession(nil), s.AgentSessions...)
	return state, nil
}

func agentHealthKey(tenantID, agentID string) string {
	return tenantID + "/" + agentID
}

func agentSessionID(tenantID, agentID string) string {
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
		labelKey(sig.GetLabels()),
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
		labelKey(inc.GetLabels()),
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

func sortedEntities(in []*signalv1.EntityRef) []string {
	out := make([]string, 0, len(in))
	for _, ent := range in {
		out = append(out, strings.Join([]string{ent.GetKind(), ent.GetKey(), ent.GetRole()}, "\x00"))
	}
	sort.Strings(out)
	return out
}

func labelSelectorMatches(labels map[string]string, selector LabelSelector) bool {
	if len(selector) == 0 {
		return true
	}
	for key, want := range selector {
		if key == "" {
			continue
		}
		if labels[key] != want {
			return false
		}
	}
	return true
}

func labelKey(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+labels[key])
	}
	return strings.Join(parts, "\x00")
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
