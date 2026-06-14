package ingest

import (
	"fmt"

	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	incidentv1 "github.com/sysarmor/sysarmor-next-project/api/proto/incident/v1"
	policyv1 "github.com/sysarmor/sysarmor-next-project/api/proto/policy/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/analytics/entity"
	"github.com/sysarmor/sysarmor-next-project/internal/analytics/evidence"
)

type Engine struct {
	nextSignalID   uint64
	nextIncidentID uint64
}

type Result struct {
	CloudSignals []*signalv1.Signal
	Incidents    []*incidentv1.Incident
}

func NewEngine() *Engine {
	return &Engine{}
}

func (e *Engine) Analyze(events []*eventv1.CanonicalEvent, signals []*signalv1.Signal) Result {
	return e.AnalyzeWithPolicy(events, signals, nil)
}

func (e *Engine) AnalyzeWithPolicy(events []*eventv1.CanonicalEvent, signals []*signalv1.Signal, policy *policyv1.DetectionPolicy) Result {
	byName := map[string][]*signalv1.Signal{}
	for _, sig := range signals {
		byName[sig.GetName()] = append(byName[sig.GetName()], sig)
	}
	scenario := firstScenario(signals)
	if scenario == "" {
		scenario = firstEventScenario(events)
	}

	var result Result
	crossLineageEnabled := policy == nil || policy.GetConverge() == nil || policy.GetConverge().GetCrossLineage()
	if policy != nil && policy.GetConverge() != nil && policy.GetConverge().GetMode() == "" {
		crossLineageEnabled = policy.GetConverge().GetCrossLineage()
	}

	if has(byName, "payload_dropped") && (hasTerminal(byName["reverse_shell_pattern"]) || (crossLineageEnabled && has(byName, "suspicious_exec_connect"))) {
		cs := e.cloudSignal("dropped_payload_executed_and_connects", scenario, 80, collectEntities(byName, "payload_dropped", "reverse_shell_pattern", "suspicious_exec_connect")...)
		cs.CrossLineage = has(byName, "suspicious_exec_connect") && !hasTerminal(byName["reverse_shell_pattern"])
		result.CloudSignals = append(result.CloudSignals, cs)
	}
	if has(byName, "web_runtime_spawns_shell") && hasTerminal(byName["reverse_shell_pattern"]) {
		result.CloudSignals = append(result.CloudSignals, e.cloudSignal("web_shell_chain", scenario, 85, collectEntities(byName, "web_runtime_spawns_shell", "reverse_shell_pattern")...))
	}

	allSignals := append([]*signalv1.Signal{}, signals...)
	allSignals = append(allSignals, result.CloudSignals...)
	if shouldIncident(byName, result.CloudSignals, policy) {
		result.Incidents = append(result.Incidents, e.incident(scenario, allSignals))
	}
	return result
}

func shouldIncident(byName map[string][]*signalv1.Signal, cloud []*signalv1.Signal, policy *policyv1.DetectionPolicy) bool {
	if policy != nil && policy.GetConverge().GetMode() == "additive_threshold" {
		threshold := policy.GetConverge().GetAdditiveRiskThreshold()
		if threshold == 0 {
			threshold = 100
		}
		return additiveRisk(byName) >= threshold
	}
	if hasTerminal(byName["reverse_shell_pattern"]) {
		return true
	}
	for _, sig := range cloud {
		if sig.GetName() == "dropped_payload_executed_and_connects" && sig.GetCrossLineage() {
			return true
		}
	}
	return false
}

func additiveRisk(byName map[string][]*signalv1.Signal) uint32 {
	var total uint32
	for _, signals := range byName {
		for _, sig := range signals {
			total += sig.GetBaseRisk()
		}
	}
	return total
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
		Entities:     uniqueEntities(entities),
		Scenario:     scenario,
	}
}

func (e *Engine) incident(scenario string, signals []*signalv1.Signal) *incidentv1.Incident {
	e.nextIncidentID++
	lineages := map[string]bool{}
	var lineageList []string
	var terminals []string
	var contributing []*signalv1.Signal
	score := float32(0)
	for _, sig := range signals {
		if sig.GetScenario() != "" && scenario != "" && sig.GetScenario() != scenario {
			continue
		}
		contributing = append(contributing, sig)
		score += float32(sig.GetBaseRisk()) * max(sig.GetGlobalRarity(), 1)
		if sig.GetLineageId() != "" && !lineages[sig.GetLineageId()] {
			lineages[sig.GetLineageId()] = true
			lineageList = append(lineageList, sig.GetLineageId())
		}
		if sig.GetTerminal() {
			for _, ent := range sig.GetEntities() {
				if ent.GetKind() == "process" {
					terminals = append(terminals, ent.GetKey())
				}
			}
		}
	}
	return &incidentv1.Incident{
		Id:                  fmt.Sprintf("inc-%020d", e.nextIncidentID),
		Scenario:            scenario,
		Summary:             "SysArmor detected a causal attack chain",
		Severity:            80,
		LineageIds:          lineageList,
		Terminals:           terminals,
		Evidence:            evidence.FromSignals(contributing),
		Converge:            &incidentv1.ConvergeTrace{Method: "rarity+causal-topk", Score: score},
		ContributingSignals: contributing,
	}
}

func has(byName map[string][]*signalv1.Signal, name string) bool {
	return len(byName[name]) > 0
}

func hasTerminal(signals []*signalv1.Signal) bool {
	for _, sig := range signals {
		if sig.GetTerminal() {
			return true
		}
	}
	return false
}

func collectEntities(byName map[string][]*signalv1.Signal, names ...string) []*signalv1.EntityRef {
	var out []*signalv1.EntityRef
	for _, name := range names {
		for _, sig := range byName[name] {
			out = append(out, sig.GetEntities()...)
		}
	}
	return uniqueEntities(out)
}

func uniqueEntities(in []*signalv1.EntityRef) []*signalv1.EntityRef {
	return entity.Unique(in)
}

func firstScenario(signals []*signalv1.Signal) string {
	for _, sig := range signals {
		if sig.GetScenario() != "" {
			return sig.GetScenario()
		}
	}
	return ""
}

func firstEventScenario(events []*eventv1.CanonicalEvent) string {
	for _, ev := range events {
		if ev.GetScenario() != "" {
			return ev.GetScenario()
		}
	}
	return ""
}

func max(v, fallback float32) float32 {
	if v == 0 {
		return fallback
	}
	return v
}
