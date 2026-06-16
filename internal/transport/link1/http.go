package link1

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	incidentv1 "github.com/sysarmor/sysarmor-next-project/api/proto/incident/v1"
	policyv1 "github.com/sysarmor/sysarmor-next-project/api/proto/policy/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
	"github.com/sysarmor/sysarmor-next-project/internal/analytics/graph"
	"github.com/sysarmor/sysarmor-next-project/internal/analytics/ingest"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
	responsemodel "github.com/sysarmor/sysarmor-next-project/internal/response"
	"github.com/sysarmor/sysarmor-next-project/internal/store"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type Server struct {
	store     ManagerStore
	engine    *ingest.Engine
	authToken string
}

type ManagerStore interface {
	AckResponse(responsemodel.Ack) (responsemodel.Command, bool)
	AddAgent(*analyticsv1.AgentHello)
	AddEvent(*eventv1.CanonicalEvent) bool
	AddSignal(*signalv1.Signal) bool
	AssignPolicy(policymodel.Assignment) (policymodel.Assignment, bool)
	AttachIncidentEvidence(string, string, *incidentv1.EvidenceSubgraph) (*incidentv1.Incident, bool)
	CreateResponse(responsemodel.Command) responsemodel.Command
	DeleteScenario(string)
	EffectivePolicy(string, string, string, string) (policymodel.Policy, bool)
	EnsureDefaultPolicy(string)
	GetAgentHealth(string, string) (agenthealth.AgentHealth, bool)
	GetIncident(string, string) (*incidentv1.Incident, bool)
	GetPolicy(string, string, uint64) (policymodel.Policy, bool)
	GetSignal(string) (*signalv1.Signal, bool)
	Info() store.Info
	ListAgentHealth() []agenthealth.AgentHealth
	ListAgents() []*analyticsv1.AgentHello
	ListAssignments(string, string) []policymodel.Assignment
	ListEvents(string, string) []*eventv1.CanonicalEvent
	ListIncidents(string) []*incidentv1.Incident
	ListPolicies(string) []policymodel.Policy
	ListResponses(string, string) []responsemodel.AuditRecord
	ListRules(string) []policymodel.RuleContent
	ListSignals(string, string, bool) []*signalv1.Signal
	MergeIncidents(string, string) (*incidentv1.Incident, bool)
	MetricsSnapshot() store.Metrics
	PendingResponses(string, string) []responsemodel.Command
	RecordUpload(int, int, int, int, time.Duration)
	ReplaceDerivedForScenario(string, []*signalv1.Signal, []*incidentv1.Incident)
	Save() error
	UpdateIncidentStatus(string, string, string, string, string) (*incidentv1.Incident, bool)
	UpsertAgentHealth(agenthealth.AgentHealth)
	UpsertPolicy(policymodel.Policy) policymodel.Policy
}

var _ ManagerStore = (*store.Store)(nil)

type UploadResult struct {
	AcceptedEvents  int
	AcceptedSignals int
	CloudSignals    int
	Incidents       int
}

type responseDecisionRequest struct {
	SignalID string              `json:"signal_id"`
	TenantID string              `json:"tenant_id"`
	AgentID  string              `json:"agent_id"`
	Scope    responsemodel.Scope `json:"scope,omitempty"`
	Target   string              `json:"target,omitempty"`
	Actor    string              `json:"actor,omitempty"`
}

type incidentLifecycleRequest struct {
	IncidentID string `json:"incident_id"`
	Scenario   string `json:"scenario"`
	Status     string `json:"status"`
	Reason     string `json:"reason,omitempty"`
	Actor      string `json:"actor,omitempty"`
}

type incidentEvidenceAttachRequest struct {
	IncidentID string          `json:"incident_id"`
	Scenario   string          `json:"scenario"`
	Evidence   json.RawMessage `json:"evidence"`
}

type incidentMergeRequest struct {
	TargetIncidentID string `json:"target_incident_id"`
	SourceIncidentID string `json:"source_incident_id"`
}

type AgentListItem struct {
	AgentID        string                       `json:"agent_id"`
	HostID         string                       `json:"host_id"`
	TenantID       string                       `json:"tenant_id"`
	Version        string                       `json:"version,omitempty"`
	HealthStatus   string                       `json:"health_status,omitempty"`
	Scope          agenthealth.RuntimeScope     `json:"scope,omitempty"`
	Capability     agenthealth.SensorCapability `json:"sensor_capability,omitempty"`
	HealthObserved time.Time                    `json:"health_observed_at,omitempty"`
}

var ErrInvalidUpload = errors.New("invalid upload")

func NewServer(st ManagerStore) *Server {
	st.EnsureDefaultPolicy("default")
	return &Server{store: st, engine: ingest.NewEngine()}
}

func NewServerWithAuth(st ManagerStore, token string) *Server {
	st.EnsureDefaultPolicy("default")
	return &Server{store: st, engine: ingest.NewEngine(), authToken: token}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.health)
	mux.HandleFunc("/api/v1/reset", s.reset)
	mux.HandleFunc("/api/v1/upload", s.upload)
	mux.HandleFunc("/api/v1/recompute", s.recompute)
	mux.HandleFunc("/api/v1/rules", s.rules)
	mux.HandleFunc("/api/v1/policies", s.policies)
	mux.HandleFunc("/api/v1/policy-assignments", s.policyAssignments)
	mux.HandleFunc("/api/v1/effective-policy", s.effectivePolicy)
	mux.HandleFunc("/api/v1/responses", s.responses)
	mux.HandleFunc("/api/v1/response-decisions", s.responseDecisions)
	mux.HandleFunc("/api/v1/response-acks", s.responseAcks)
	mux.HandleFunc("/api/v1/agents", s.agents)
	mux.HandleFunc("/api/v1/agent-health", s.agentHealth)
	mux.HandleFunc("/api/v1/events", s.events)
	mux.HandleFunc("/api/v1/signals", s.signals)
	mux.HandleFunc("/api/v1/incidents", s.incidents)
	mux.HandleFunc("/api/v1/incident-evidence", s.incidentEvidence)
	mux.HandleFunc("/api/v1/incident-lifecycle", s.incidentLifecycle)
	mux.HandleFunc("/api/v1/incident-merge", s.incidentMerge)
	mux.HandleFunc("/api/v1/metrics", s.metrics)
	mux.HandleFunc("/api/v1/store-status", s.storeStatus)
	return mux
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"ok": true, "store": s.store.Info()})
}

func (s *Server) reset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	scenario := r.URL.Query().Get("scenario")
	s.store.DeleteScenario(scenario)
	if err := s.store.Save(); err != nil {
		http.Error(w, fmt.Sprintf("save store: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "scenario": scenario})
}

func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("read body: %v", err), http.StatusBadRequest)
		return
	}
	batch := &analyticsv1.UploadBatch{}
	if err := protojson.Unmarshal(body, batch); err != nil {
		http.Error(w, fmt.Sprintf("decode upload batch: %v", err), http.StatusBadRequest)
		return
	}
	result, err := s.AcceptUpload(batch)
	if err != nil {
		if errors.Is(err, ErrInvalidUpload) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, fmt.Sprintf("save store: %v", err), http.StatusInternalServerError)
		return
	}
	writeProtoJSON(w, &analyticsv1.UploadAck{
		Ok:              true,
		Message:         "accepted",
		AcceptedEvents:  uint64(result.AcceptedEvents),
		AcceptedSignals: uint64(result.AcceptedSignals),
		BatchId:         batch.GetBatchId(),
	})
}

func (s *Server) AcceptUpload(batch *analyticsv1.UploadBatch) (UploadResult, error) {
	if err := validateUploadIdentity(batch); err != nil {
		return UploadResult{}, err
	}
	s.store.AddAgent(batch.GetAgent())
	touchedScenarios := map[string]*analyticsv1.AgentHello{}
	acceptedEvents := 0
	acceptedSignals := 0
	for _, ev := range batch.GetEvents() {
		inserted := s.store.AddEvent(ev)
		if inserted {
			acceptedEvents++
		}
		if inserted && ev.GetScenario() != "" {
			touchedScenarios[ev.GetScenario()] = batch.GetAgent()
		}
	}
	for _, sig := range batch.GetSignals() {
		inserted := s.store.AddSignal(sig)
		if inserted {
			acceptedSignals++
		}
		if inserted && sig.GetScenario() != "" {
			touchedScenarios[sig.GetScenario()] = batch.GetAgent()
		}
	}
	start := time.Now()
	cloudSignals, incidents := s.recomputeTouchedScenarios(touchedScenarios)
	convergenceLatency := time.Since(start)
	s.store.RecordUpload(acceptedEvents, acceptedSignals, cloudSignals, incidents, convergenceLatency)
	if err := s.store.Save(); err != nil {
		return UploadResult{}, err
	}
	return UploadResult{
		AcceptedEvents:  acceptedEvents,
		AcceptedSignals: acceptedSignals,
		CloudSignals:    cloudSignals,
		Incidents:       incidents,
	}, nil
}

func validateUploadIdentity(batch *analyticsv1.UploadBatch) error {
	if batch == nil || batch.GetAgent() == nil {
		return fmt.Errorf("%w: agent identity is required", ErrInvalidUpload)
	}
	agent := batch.GetAgent()
	missing := []string{}
	if agent.GetAgentId() == "" {
		missing = append(missing, "agent_id")
	}
	if agent.GetHostId() == "" {
		missing = append(missing, "host_id")
	}
	if agent.GetTenantId() == "" {
		missing = append(missing, "tenant_id")
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: agent identity missing %s", ErrInvalidUpload, strings.Join(missing, ", "))
	}
	return nil
}

func (s *Server) recomputeTouchedScenarios(touchedScenarios map[string]*analyticsv1.AgentHello) (int, int) {
	totalCloud := 0
	totalIncidents := 0
	if len(touchedScenarios) == 0 {
		return 0, 0
	}
	for scenario, agent := range touchedScenarios {
		events := s.store.ListEvents(scenario, "")
		endpointSignals := s.store.ListSignals(scenario, "endpoint", false)
		policy := s.effectiveDetectionPolicyForAgent(agent)
		analysis := s.engine.AnalyzeWithPolicy(events, endpointSignals, policy)
		s.store.ReplaceDerivedForScenario(scenario, analysis.CloudSignals, analysis.Incidents)
		totalCloud += len(analysis.CloudSignals)
		totalIncidents += len(analysis.Incidents)
	}
	return totalCloud, totalIncidents
}

func (s *Server) effectiveDetectionPolicyForAgent(agent *analyticsv1.AgentHello) *policyv1.DetectionPolicy {
	if agent == nil {
		policy, _ := s.store.EffectivePolicy("default", "", "", "")
		return policy.DetectionPolicy()
	}
	var scope agenthealth.RuntimeScope
	if health, ok := s.store.GetAgentHealth(agent.GetTenantId(), agent.GetAgentId()); ok {
		scope = health.Scope
	}
	policy, _ := s.store.EffectivePolicy(agent.GetTenantId(), agent.GetAgentId(), scope.Type, scope.Selector)
	return policy.DetectionPolicy()
}

func (s *Server) agents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	tenantID := q.Get("tenant_id")
	scopeType := q.Get("scope_type")
	scopeSelector := q.Get("scope_selector")
	healthStatus := q.Get("health_status")
	agents := s.store.ListAgents()
	out := make([]AgentListItem, 0, len(agents))
	for _, agent := range agents {
		if tenantID != "" && agent.GetTenantId() != tenantID {
			continue
		}
		item := AgentListItem{
			AgentID:  agent.GetAgentId(),
			HostID:   agent.GetHostId(),
			TenantID: agent.GetTenantId(),
			Version:  agent.GetVersion(),
		}
		if health, ok := s.store.GetAgentHealth(agent.GetTenantId(), agent.GetAgentId()); ok {
			item.HealthStatus = health.Status
			item.Scope = health.Scope
			item.Capability = health.Capability
			item.HealthObserved = health.ObservedAt
		}
		if scopeType != "" && item.Scope.Type != scopeType {
			continue
		}
		if scopeSelector != "" && item.Scope.Selector != scopeSelector {
			continue
		}
		if healthStatus != "" && item.HealthStatus != healthStatus {
			continue
		}
		out = append(out, item)
	}
	writeJSON(w, out)
}

func (s *Server) agentHealth(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		if !s.authorized(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var health agenthealth.AgentHealth
		if err := json.NewDecoder(r.Body).Decode(&health); err != nil {
			http.Error(w, fmt.Sprintf("decode agent health: %v", err), http.StatusBadRequest)
			return
		}
		if health.AgentID == "" {
			http.Error(w, "agent_id is required", http.StatusBadRequest)
			return
		}
		s.store.UpsertAgentHealth(health)
		if err := s.store.Save(); err != nil {
			http.Error(w, fmt.Sprintf("save store: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	case http.MethodGet:
		q := r.URL.Query()
		agentID := q.Get("agent_id")
		if agentID == "" {
			writeJSON(w, s.store.ListAgentHealth())
			return
		}
		health, ok := s.store.GetAgentHealth(q.Get("tenant_id"), agentID)
		if !ok {
			http.Error(w, "agent health not found", http.StatusNotFound)
			return
		}
		writeJSON(w, health)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) authorized(r *http.Request) bool {
	if s.authToken == "" {
		return true
	}
	if r.Header.Get("X-SysArmor-Agent-Token") == s.authToken {
		return true
	}
	if r.Header.Get("Authorization") == "Bearer "+s.authToken {
		return true
	}
	return false
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	writeEventList(w, s.store.ListEvents(q.Get("scenario"), q.Get("kind")))
}

func (s *Server) signals(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	signals := s.store.ListSignals(q.Get("scenario"), q.Get("layer"), q.Get("terminal") == "true")
	writeSignalList(w, signals)
}

func (s *Server) incidents(w http.ResponseWriter, r *http.Request) {
	incidents := s.store.ListIncidents(r.URL.Query().Get("scenario"))
	writeIncidentList(w, incidents)
}

func (s *Server) incidentEvidence(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.attachIncidentEvidence(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	inc, ok := s.store.GetIncident(q.Get("incident_id"), q.Get("scenario"))
	if !ok {
		http.Error(w, "incident not found", http.StatusNotFound)
		return
	}
	if q.Get("path_from") != "" || q.Get("path_to") != "" {
		if q.Get("path_from") == "" || q.Get("path_to") == "" {
			http.Error(w, "path_from and path_to are required together", http.StatusBadRequest)
			return
		}
		writeProtoJSON(w, graph.FromSignals(inc.GetContributingSignals()).ShortestPath(q.Get("path_from"), q.Get("path_to")))
		return
	}
	if q.Get("seed") != "" {
		writeProtoJSON(w, graph.FromSignals(inc.GetContributingSignals()).KHop(q.Get("seed"), int(parseUint(q.Get("hops")))))
		return
	}
	if inc.GetEvidence() == nil {
		writeProtoJSON(w, &incidentv1.EvidenceSubgraph{})
		return
	}
	writeProtoJSON(w, inc.GetEvidence())
}

func (s *Server) attachIncidentEvidence(w http.ResponseWriter, r *http.Request) {
	var req incidentEvidenceAttachRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode incident evidence: %v", err), http.StatusBadRequest)
		return
	}
	if req.IncidentID == "" && req.Scenario == "" {
		http.Error(w, "incident_id or scenario is required", http.StatusBadRequest)
		return
	}
	if len(req.Evidence) == 0 {
		http.Error(w, "evidence is required", http.StatusBadRequest)
		return
	}
	evidence := &incidentv1.EvidenceSubgraph{}
	if err := protojson.Unmarshal(req.Evidence, evidence); err != nil {
		http.Error(w, fmt.Sprintf("decode evidence: %v", err), http.StatusBadRequest)
		return
	}
	inc, ok := s.store.AttachIncidentEvidence(req.IncidentID, req.Scenario, evidence)
	if !ok {
		http.Error(w, "incident not found", http.StatusNotFound)
		return
	}
	if err := s.store.Save(); err != nil {
		http.Error(w, fmt.Sprintf("save store: %v", err), http.StatusInternalServerError)
		return
	}
	writeProtoJSON(w, inc)
}

func (s *Server) incidentLifecycle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req incidentLifecycleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode incident lifecycle: %v", err), http.StatusBadRequest)
		return
	}
	if req.IncidentID == "" && req.Scenario == "" {
		http.Error(w, "incident_id or scenario is required", http.StatusBadRequest)
		return
	}
	inc, ok := s.store.UpdateIncidentStatus(req.IncidentID, req.Scenario, req.Status, req.Reason, req.Actor)
	if !ok {
		http.Error(w, "incident not found or status invalid", http.StatusNotFound)
		return
	}
	if err := s.store.Save(); err != nil {
		http.Error(w, fmt.Sprintf("save store: %v", err), http.StatusInternalServerError)
		return
	}
	writeProtoJSON(w, inc)
}

func (s *Server) incidentMerge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req incidentMergeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode incident merge: %v", err), http.StatusBadRequest)
		return
	}
	if req.TargetIncidentID == "" || req.SourceIncidentID == "" {
		http.Error(w, "target_incident_id and source_incident_id are required", http.StatusBadRequest)
		return
	}
	inc, ok := s.store.MergeIncidents(req.TargetIncidentID, req.SourceIncidentID)
	if !ok {
		http.Error(w, "incident not found or merge invalid", http.StatusNotFound)
		return
	}
	if err := s.store.Save(); err != nil {
		http.Error(w, fmt.Sprintf("save store: %v", err), http.StatusInternalServerError)
		return
	}
	writeProtoJSON(w, inc)
}

func (s *Server) metrics(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.store.MetricsSnapshot())
}

func (s *Server) storeStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, s.store.Info())
}

func (s *Server) rules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, s.store.ListRules(r.URL.Query().Get("where")))
}

func (s *Server) policies(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		if policyID := q.Get("policy_id"); policyID != "" {
			version := parseUint(q.Get("version"))
			policy, ok := s.store.GetPolicy(q.Get("tenant_id"), policyID, version)
			if !ok {
				http.Error(w, "policy not found", http.StatusNotFound)
				return
			}
			writeJSON(w, policy)
			return
		}
		writeJSON(w, s.store.ListPolicies(q.Get("tenant_id")))
	case http.MethodPost:
		var policy policymodel.Policy
		if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
			http.Error(w, fmt.Sprintf("decode policy: %v", err), http.StatusBadRequest)
			return
		}
		if policy.PolicyID == "" {
			http.Error(w, "policy_id is required", http.StatusBadRequest)
			return
		}
		policy = s.store.UpsertPolicy(policy)
		if err := s.store.Save(); err != nil {
			http.Error(w, fmt.Sprintf("save store: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, policy)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) policyAssignments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		writeJSON(w, s.store.ListAssignments(q.Get("tenant_id"), q.Get("agent_id")))
	case http.MethodPost:
		var assignment policymodel.Assignment
		if err := json.NewDecoder(r.Body).Decode(&assignment); err != nil {
			http.Error(w, fmt.Sprintf("decode assignment: %v", err), http.StatusBadRequest)
			return
		}
		saved, ok := s.store.AssignPolicy(assignment)
		if !ok {
			http.Error(w, "policy not found or assignment invalid", http.StatusBadRequest)
			return
		}
		if err := s.store.Save(); err != nil {
			http.Error(w, fmt.Sprintf("save store: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, saved)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) effectivePolicy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	policy, ok := s.store.EffectivePolicy(q.Get("tenant_id"), q.Get("agent_id"), q.Get("scope_type"), q.Get("scope_selector"))
	if !ok {
		http.Error(w, "effective policy not found", http.StatusNotFound)
		return
	}
	writeJSON(w, policy)
}

func (s *Server) responses(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		if q.Get("pending") == "true" {
			writeJSON(w, s.store.PendingResponses(q.Get("tenant_id"), q.Get("agent_id")))
			return
		}
		writeJSON(w, s.store.ListResponses(q.Get("tenant_id"), q.Get("agent_id")))
	case http.MethodPost:
		var cmd responsemodel.Command
		if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
			http.Error(w, fmt.Sprintf("decode response command: %v", err), http.StatusBadRequest)
			return
		}
		s.createResponse(w, cmd)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) responseDecisions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req responseDecisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode response decision: %v", err), http.StatusBadRequest)
		return
	}
	if req.SignalID == "" {
		http.Error(w, "signal_id is required", http.StatusBadRequest)
		return
	}
	if req.AgentID == "" {
		http.Error(w, "agent_id is required", http.StatusBadRequest)
		return
	}
	sig, ok := s.store.GetSignal(req.SignalID)
	if !ok {
		http.Error(w, "signal not found", http.StatusNotFound)
		return
	}
	intent := sig.GetResponseIntent()
	if intent == nil || intent.GetResponseIntent() == "" {
		http.Error(w, "signal response intent not found", http.StatusBadRequest)
		return
	}
	action := intent.GetRecommendedAction()
	if action == "" {
		action = intent.GetResponseIntent()
	}
	target := req.Target
	if target == "" {
		target = signalResponseTarget(sig)
	}
	reason := fmt.Sprintf("signal=%s name=%s response_intent=%s confidence=%d", sig.GetId(), sig.GetName(), intent.GetResponseIntent(), intent.GetConfidence())
	if intent.GetReason() != "" {
		reason += " reason=" + intent.GetReason()
	}
	cmd := responsemodel.Command{
		ResponseID: "resp-" + sig.GetId(),
		TenantID:   req.TenantID,
		AgentID:    req.AgentID,
		SignalID:   sig.GetId(),
		Scenario:   sig.GetScenario(),
		Scope:      req.Scope,
		Action:     action,
		Mode:       responsemodel.DefaultMode,
		Target:     target,
		Reason:     reason,
		Actor:      req.Actor,
	}
	s.createResponse(w, cmd)
}

func (s *Server) createResponse(w http.ResponseWriter, cmd responsemodel.Command) {
	if cmd.AgentID == "" {
		http.Error(w, "agent_id is required", http.StatusBadRequest)
		return
	}
	if cmd.TenantID == "" {
		cmd.TenantID = "default"
	}
	if health, ok := s.store.GetAgentHealth(cmd.TenantID, cmd.AgentID); ok {
		if cmd.Scope.Type == "" && cmd.Scope.Selector == "" {
			cmd.Scope = responsemodel.Scope{Type: health.Scope.Type, Selector: health.Scope.Selector}
		}
		if decision := responsemodel.ScopeDecision(cmd.Scope, responsemodel.Scope{Type: health.Scope.Type, Selector: health.Scope.Selector}, true); !decision.Allowed {
			s.denyResponse(w, cmd, decision)
			return
		}
	} else if cmd.Scope.Type != "" || cmd.Scope.Selector != "" {
		s.denyResponse(w, cmd, responsemodel.Decision{Allowed: false, Reason: "agent runtime scope is required for scoped response command"})
		return
	}
	if cmd.PolicyID == "" {
		policy, _ := s.store.EffectivePolicy(cmd.TenantID, cmd.AgentID, cmd.Scope.Type, cmd.Scope.Selector)
		cmd.PolicyID = policy.PolicyID
		cmd.PolicyVersion = policy.Version
	}
	if decision := responsemodel.ValidateCommand(cmd); !decision.Allowed {
		s.denyResponse(w, cmd, decision)
		return
	}
	cmd = s.store.CreateResponse(cmd)
	if err := s.store.Save(); err != nil {
		http.Error(w, fmt.Sprintf("save store: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, cmd)
}

func (s *Server) denyResponse(w http.ResponseWriter, cmd responsemodel.Command, decision responsemodel.Decision) {
	cmd = responsemodel.NormalizeCommand(cmd)
	cmd.Status = "denied"
	if cmd.Reason == "" {
		cmd.Reason = decision.Reason
	} else {
		cmd.Reason = cmd.Reason + "; denied: " + decision.Reason
	}
	cmd = s.store.CreateResponse(cmd)
	if err := s.store.Save(); err != nil {
		http.Error(w, fmt.Sprintf("save store: %v", err), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusForbidden)
	writeJSON(w, responsemodel.AuditRecord{Command: cmd})
}

func signalResponseTarget(sig *signalv1.Signal) string {
	for _, entity := range sig.GetEntities() {
		if entity.GetKind() == "process" && entity.GetKey() != "" {
			return entity.GetKey()
		}
	}
	for _, entity := range sig.GetEntities() {
		if entity.GetKey() != "" {
			return entity.GetKey()
		}
	}
	return ""
}

func (s *Server) responseAcks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var ack responsemodel.Ack
	if err := json.NewDecoder(r.Body).Decode(&ack); err != nil {
		http.Error(w, fmt.Sprintf("decode response ack: %v", err), http.StatusBadRequest)
		return
	}
	if ack.AgentID == "" {
		http.Error(w, "agent_id is required", http.StatusBadRequest)
		return
	}
	if _, ok := s.store.AckResponse(ack); !ok {
		http.Error(w, "response command not found", http.StatusNotFound)
		return
	}
	if err := s.store.Save(); err != nil {
		http.Error(w, fmt.Sprintf("save store: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) recompute(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	effective, _ := s.store.EffectivePolicy(q.Get("tenant_id"), q.Get("agent_id"), q.Get("scope_type"), q.Get("scope_selector"))
	policy := effective.DetectionPolicy()
	if policy.Converge == nil {
		policy.Converge = &policyv1.ConvergeParams{CrossLineage: true}
	}
	switch q.Get("disable") {
	case "cloud.cross_lineage":
		policy.Converge.CrossLineage = false
	case "":
	default:
		if strings.HasPrefix(q.Get("disable"), "cloud.rule:") {
			disabled := strings.TrimPrefix(q.Get("disable"), "cloud.rule:")
			policy.CloudRules = removeString(policy.CloudRules, disabled)
			break
		}
		http.Error(w, fmt.Sprintf("unknown disable %q", q.Get("disable")), http.StatusBadRequest)
		return
	}
	switch q.Get("mode") {
	case "additive_threshold":
		policy.Converge.Mode = "additive_threshold"
		policy.Converge.AdditiveRiskThreshold = 100
	case "", "rarity_structural":
	default:
		http.Error(w, fmt.Sprintf("unknown converge mode %q", q.Get("mode")), http.StatusBadRequest)
		return
	}
	result := s.engine.AnalyzeWithPolicy(nil, s.store.ListSignals(q.Get("scenario"), "endpoint", false), policy)
	writeAnalysisResult(w, result)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func parseUint(raw string) uint64 {
	if raw == "" {
		return 0
	}
	v, _ := strconv.ParseUint(raw, 10, 64)
	return v
}

func removeString(in []string, value string) []string {
	out := make([]string, 0, len(in))
	for _, item := range in {
		if item != value {
			out = append(out, item)
		}
	}
	return out
}

func writeProtoJSON(w http.ResponseWriter, msg proto.Message) {
	w.Header().Set("Content-Type", "application/json")
	data, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(msg)
	if err != nil {
		http.Error(w, fmt.Sprintf("encode proto json: %v", err), http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(append(data, '\n'))
}

func writeSignalList(w http.ResponseWriter, signals []*signalv1.Signal) {
	w.Header().Set("Content-Type", "application/json")
	raw := make([]json.RawMessage, 0, len(signals))
	for _, sig := range signals {
		raw = append(raw, mustProtoJSON(sig))
	}
	_ = json.NewEncoder(w).Encode(raw)
}

func writeEventList(w http.ResponseWriter, events []*eventv1.CanonicalEvent) {
	w.Header().Set("Content-Type", "application/json")
	raw := make([]json.RawMessage, 0, len(events))
	for _, ev := range events {
		raw = append(raw, mustProtoJSON(ev))
	}
	_ = json.NewEncoder(w).Encode(raw)
}

func writeIncidentList(w http.ResponseWriter, incidents []*incidentv1.Incident) {
	w.Header().Set("Content-Type", "application/json")
	raw := make([]json.RawMessage, 0, len(incidents))
	for _, inc := range incidents {
		raw = append(raw, mustProtoJSON(inc))
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"incidents": raw})
}

func writeAnalysisResult(w http.ResponseWriter, result ingest.Result) {
	w.Header().Set("Content-Type", "application/json")
	cloud := make([]json.RawMessage, 0, len(result.CloudSignals))
	for _, sig := range result.CloudSignals {
		cloud = append(cloud, mustProtoJSON(sig))
	}
	incidents := make([]json.RawMessage, 0, len(result.Incidents))
	for _, inc := range result.Incidents {
		incidents = append(incidents, mustProtoJSON(inc))
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"cloud_signals": cloud, "incidents": incidents})
}

func mustProtoJSON(msg proto.Message) json.RawMessage {
	data, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(msg)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return data
}
