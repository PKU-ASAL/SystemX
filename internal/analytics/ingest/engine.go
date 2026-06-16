package ingest

import (
	"fmt"

	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	incidentv1 "github.com/sysarmor/sysarmor-next-project/api/proto/incident/v1"
	policyv1 "github.com/sysarmor/sysarmor-next-project/api/proto/policy/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/analytics/converge"
	"github.com/sysarmor/sysarmor-next-project/internal/analytics/correlate"
	"github.com/sysarmor/sysarmor-next-project/internal/analytics/entity"
	incidentbuilder "github.com/sysarmor/sysarmor-next-project/internal/analytics/incident"
	"github.com/sysarmor/sysarmor-next-project/internal/analytics/rarity"
)

type Engine struct {
	nextSignalID uint64
	incidents    *incidentbuilder.Builder
}

type Result struct {
	CloudSignals []*signalv1.Signal
	Incidents    []*incidentv1.Incident
}

func NewEngine() *Engine {
	return &Engine{incidents: incidentbuilder.NewBuilder()}
}

func (e *Engine) SetRarityBaseline(baseline rarity.Baseline) {
	e.incidents.Scorer = rarity.WorkloadBaselineScorer{Baseline: baseline.Snapshot()}
}

func (e *Engine) Analyze(events []*eventv1.CanonicalEvent, signals []*signalv1.Signal) Result {
	return e.AnalyzeWithPolicy(events, signals, nil)
}

func (e *Engine) AnalyzeWithPolicy(events []*eventv1.CanonicalEvent, signals []*signalv1.Signal, policy *policyv1.DetectionPolicy) Result {
	view := correlate.Build(events, signals, policy)

	var result Result
	crossLineageEnabled := policy == nil || policy.GetConverge() == nil || policy.GetConverge().GetCrossLineage()
	if policy != nil && policy.GetConverge() != nil && policy.GetConverge().GetMode() == "" {
		crossLineageEnabled = policy.GetConverge().GetCrossLineage()
	}

	if cloudRuleEnabled(policy, "dropped_payload_executed_and_connects") && view.Has("payload_dropped") && (view.HasTerminal("reverse_shell_pattern") || (crossLineageEnabled && view.Has("suspicious_exec_connect"))) {
		cs := e.cloudSignal("dropped_payload_executed_and_connects", view.Scenario, 80, view.CollectEntities("payload_dropped", "reverse_shell_pattern", "suspicious_exec_connect")...)
		cs.CrossLineage = view.Has("suspicious_exec_connect") && !view.HasTerminal("reverse_shell_pattern")
		result.CloudSignals = append(result.CloudSignals, cs)
	}
	if cloudRuleEnabled(policy, "web_shell_chain") && view.Has("web_runtime_spawns_shell") && view.HasTerminal("reverse_shell_pattern") {
		result.CloudSignals = append(result.CloudSignals, e.cloudSignal("web_shell_chain", view.Scenario, 85, view.CollectEntities("web_runtime_spawns_shell", "reverse_shell_pattern")...))
	}

	allSignals := append([]*signalv1.Signal{}, signals...)
	allSignals = append(allSignals, result.CloudSignals...)
	decision := converge.Decide(view.ByName, result.CloudSignals, policy)
	if decision.Incident {
		result.Incidents = append(result.Incidents, e.incidents.Build(view.Scenario, allSignals, decision))
	}
	return result
}

func cloudRuleEnabled(policy *policyv1.DetectionPolicy, name string) bool {
	if policy == nil || len(policy.GetCloudRules()) == 0 {
		return true
	}
	for _, rule := range policy.GetCloudRules() {
		if rule == name {
			return true
		}
	}
	return false
}

func (e *Engine) cloudSignal(name, scenario string, risk uint32, entities ...*signalv1.EntityRef) *signalv1.Signal {
	e.nextSignalID++
	return &signalv1.Signal{
		Id:           fmt.Sprintf("cloud-sig-%020d", e.nextSignalID),
		Name:         name,
		Where:        signalv1.SignalWhere_SIGNAL_WHERE_CLOUD,
		BaseRisk:     risk,
		LocalRarity:  1,
		GlobalRarity: 1,
		Entities:     entity.Unique(entities),
		Scenario:     scenario,
	}
}
