package managerapi

import (
	"fmt"
	"net/http"
	"strings"

	policyv1 "github.com/sysarmor/sysarmor-next-project/api/proto/policy/v1"
	ingest "github.com/sysarmor/sysarmor-next-project/internal/analytics/ingest"
)

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
	engine := ingest.NewEngine()
	engine.SetRarityBaseline(s.store.RarityBaselineSnapshot())
	result := engine.AnalyzeWithPolicy(nil, s.store.ListSignals(parseLabelSelector(q["label"]), "endpoint", false), policy)
	writeAnalysisResult(w, result)
}
