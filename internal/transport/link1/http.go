package link1

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	incidentv1 "github.com/sysarmor/sysarmor-next-project/api/proto/incident/v1"
	policyv1 "github.com/sysarmor/sysarmor-next-project/api/proto/policy/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
	"github.com/sysarmor/sysarmor-next-project/internal/analytics/ingest"
	"github.com/sysarmor/sysarmor-next-project/internal/store"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type Server struct {
	store  *store.Store
	engine *ingest.Engine
}

type UploadResult struct {
	AcceptedEvents  int
	AcceptedSignals int
	CloudSignals    int
	Incidents       int
}

func NewServer(st *store.Store) *Server {
	return &Server{store: st, engine: ingest.NewEngine()}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.health)
	mux.HandleFunc("/api/v1/reset", s.reset)
	mux.HandleFunc("/api/v1/upload", s.upload)
	mux.HandleFunc("/api/v1/recompute", s.recompute)
	mux.HandleFunc("/api/v1/agents", s.agents)
	mux.HandleFunc("/api/v1/agent-health", s.agentHealth)
	mux.HandleFunc("/api/v1/events", s.events)
	mux.HandleFunc("/api/v1/signals", s.signals)
	mux.HandleFunc("/api/v1/incidents", s.incidents)
	mux.HandleFunc("/api/v1/metrics", s.metrics)
	return mux
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"ok": true})
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
		http.Error(w, fmt.Sprintf("save store: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"ok":               true,
		"accepted_events":  result.AcceptedEvents,
		"accepted_signals": result.AcceptedSignals,
		"cloud_signals":    result.CloudSignals,
		"incidents":        result.Incidents,
	})
}

func (s *Server) AcceptUpload(batch *analyticsv1.UploadBatch) (UploadResult, error) {
	s.store.AddAgent(batch.GetAgent())
	touchedScenarios := map[string]bool{}
	for _, ev := range batch.GetEvents() {
		s.store.AddEvent(ev)
		if ev.GetScenario() != "" {
			touchedScenarios[ev.GetScenario()] = true
		}
	}
	for _, sig := range batch.GetSignals() {
		s.store.AddSignal(sig)
		if sig.GetScenario() != "" {
			touchedScenarios[sig.GetScenario()] = true
		}
	}
	start := time.Now()
	cloudSignals, incidents := s.recomputeTouchedScenarios(touchedScenarios)
	convergenceLatency := time.Since(start)
	s.store.RecordUpload(len(batch.GetEvents()), len(batch.GetSignals()), cloudSignals, incidents, convergenceLatency)
	if err := s.store.Save(); err != nil {
		return UploadResult{}, err
	}
	return UploadResult{
		AcceptedEvents:  len(batch.GetEvents()),
		AcceptedSignals: len(batch.GetSignals()),
		CloudSignals:    cloudSignals,
		Incidents:       incidents,
	}, nil
}

func (s *Server) recomputeTouchedScenarios(touchedScenarios map[string]bool) (int, int) {
	totalCloud := 0
	totalIncidents := 0
	if len(touchedScenarios) == 0 {
		return 0, 0
	}
	for scenario := range touchedScenarios {
		events := s.store.ListEvents(scenario, "")
		endpointSignals := s.store.ListSignals(scenario, "endpoint", false)
		analysis := s.engine.Analyze(events, endpointSignals)
		s.store.ReplaceDerivedForScenario(scenario, analysis.CloudSignals, analysis.Incidents)
		totalCloud += len(analysis.CloudSignals)
		totalIncidents += len(analysis.Incidents)
	}
	return totalCloud, totalIncidents
}

func (s *Server) agents(w http.ResponseWriter, _ *http.Request) {
	writeAgentList(w, s.store.ListAgents())
}

func (s *Server) agentHealth(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
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

func (s *Server) metrics(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.store.MetricsSnapshot())
}

func (s *Server) recompute(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	policy := &policyv1.DetectionPolicy{Converge: &policyv1.ConvergeParams{CrossLineage: true}}
	switch q.Get("disable") {
	case "cloud.cross_lineage":
		policy.Converge.CrossLineage = false
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

func writeSignalList(w http.ResponseWriter, signals []*signalv1.Signal) {
	w.Header().Set("Content-Type", "application/json")
	raw := make([]json.RawMessage, 0, len(signals))
	for _, sig := range signals {
		raw = append(raw, mustProtoJSON(sig))
	}
	_ = json.NewEncoder(w).Encode(raw)
}

func writeAgentList(w http.ResponseWriter, agents []*analyticsv1.AgentHello) {
	w.Header().Set("Content-Type", "application/json")
	raw := make([]json.RawMessage, 0, len(agents))
	for _, agent := range agents {
		raw = append(raw, mustProtoJSON(agent))
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
