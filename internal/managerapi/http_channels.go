package managerapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sysarmor/sysarmor-next-project/internal/store"
)

func (s *Server) channels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		writeJSON(w, map[string]any{"channels": s.store.ListChannels(q.Get("tenant_id"))})
	case http.MethodPost:
		if !s.requireOperator(w, r, "admin") {
			return
		}
		var req channelRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode channel: %v", err), http.StatusBadRequest)
			return
		}
		tenantID := strings.TrimSpace(req.TenantID)
		if tenantID == "" {
			tenantID = "default"
		}
		if strings.TrimSpace(req.Channel) == "" || strings.TrimSpace(req.ArtifactID) == "" {
			http.Error(w, "channel and artifact_id are required", http.StatusBadRequest)
			return
		}
		artifact, ok := s.store.GetArtifact(tenantID, req.ArtifactID)
		if !ok || artifact.Status != "active" {
			http.Error(w, "active artifact not found", http.StatusBadRequest)
			return
		}
		now := time.Now().UTC()
		channel := s.store.UpsertChannel(store.ArtifactChannel{
			TenantID:   tenantID,
			Channel:    req.Channel,
			ArtifactID: artifact.ArtifactID,
			UpdatedAt:  now,
			CreatedBy:  s.actorFromRequest(r, req.Actor),
		})
		if channel.Channel == "" {
			http.Error(w, "upsert channel failed", http.StatusBadRequest)
			return
		}
		if err := s.store.Save(); err != nil {
			http.Error(w, fmt.Sprintf("save store: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"channel": channel, "artifact": artifact})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
