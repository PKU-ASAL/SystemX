package managerapi

import (
	"encoding/json"
	"fmt"
	"net/http"

	incidentv1 "github.com/sysarmor/sysarmor-next-project/api/proto/incident/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/analytics/graph"
	platformopensearch "github.com/sysarmor/sysarmor-next-project/internal/platform/opensearch"
	"github.com/sysarmor/sysarmor-next-project/internal/store"
	"google.golang.org/protobuf/encoding/protojson"
)

func (s *Server) incidents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	labels := parseLabelSelector(q["label"])
	limit := parseUint(q.Get("limit"))
	offset := parseUint(q.Get("offset"))
	if s.searcher != nil {
		raw, err := s.searchTelemetry(r.Context(), platformopensearch.SearchRequest{
			Index:  "sysarmor-incidents",
			Size:   searchLimit(limit),
			Offset: int(offset),
			Labels: labels,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("query incidents: %v", err), http.StatusBadGateway)
			return
		}
		raw = filterRawTelemetry(raw, labels, nil)
		writeRawList(w, raw)
		return
	}
	incidents := s.store.ListIncidents(labels)
	writeIncidentList(w, pageSlice(incidents, limit, offset))
}

func (s *Server) incidentEvidence(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		if !s.requireOperator(w, r, "incident_admin") {
			return
		}
		s.attachIncidentEvidence(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	inc, ok := s.store.GetIncident(q.Get("incident_id"), parseLabelSelector(q["label"]))
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
	labels := store.LabelSelector(req.Labels)
	if req.IncidentID == "" && len(labels) == 0 {
		http.Error(w, "incident_id or labels is required", http.StatusBadRequest)
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
	inc, ok := s.store.AttachIncidentEvidence(req.IncidentID, labels, evidence)
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
	if !s.requireOperator(w, r, "incident_admin") {
		return
	}
	var req incidentLifecycleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode incident lifecycle: %v", err), http.StatusBadRequest)
		return
	}
	labels := store.LabelSelector(req.Labels)
	if req.IncidentID == "" && len(labels) == 0 {
		http.Error(w, "incident_id or labels is required", http.StatusBadRequest)
		return
	}
	inc, ok := s.store.UpdateIncidentStatus(req.IncidentID, labels, req.Status, req.Reason, s.actorFromRequest(r, req.Actor))
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
	if !s.requireOperator(w, r, "incident_admin") {
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
