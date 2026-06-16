package ingest

import (
	"fmt"

	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	incidentv1 "github.com/sysarmor/sysarmor-next-project/api/proto/incident/v1"
	policyv1 "github.com/sysarmor/sysarmor-next-project/api/proto/policy/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/analytics/converge"
	"github.com/sysarmor/sysarmor-next-project/internal/analytics/entity"
	incidentbuilder "github.com/sysarmor/sysarmor-next-project/internal/analytics/incident"
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

func (e *Engine) Analyze(events []*eventv1.CanonicalEvent, signals []*signalv1.Signal) Result {
	return e.AnalyzeWithPolicy(events, signals, nil)
}

func (e *Engine) AnalyzeWithPolicy(events []*eventv1.CanonicalEvent, signals []*signalv1.Signal, policy *policyv1.DetectionPolicy) Result {
	byName := map[string][]*signalv1.Signal{}
	for _, sig := range signals {
		if !endpointRuleEnabled(policy, sig.GetName()) {
			continue
		}
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

	if cloudRuleEnabled(policy, "dropped_payload_executed_and_connects") && has(byName, "payload_dropped") && (hasTerminal(byName["reverse_shell_pattern"]) || (crossLineageEnabled && has(byName, "suspicious_exec_connect"))) {
		cs := e.cloudSignal("dropped_payload_executed_and_connects", scenario, 80, collectEntities(byName, "payload_dropped", "reverse_shell_pattern", "suspicious_exec_connect")...)
		cs.CrossLineage = has(byName, "suspicious_exec_connect") && !hasTerminal(byName["reverse_shell_pattern"])
		result.CloudSignals = append(result.CloudSignals, cs)
	}
	if cloudRuleEnabled(policy, "web_shell_chain") && has(byName, "web_runtime_spawns_shell") && hasTerminal(byName["reverse_shell_pattern"]) {
		result.CloudSignals = append(result.CloudSignals, e.cloudSignal("web_shell_chain", scenario, 85, collectEntities(byName, "web_runtime_spawns_shell", "reverse_shell_pattern")...))
	}

	allSignals := append([]*signalv1.Signal{}, signals...)
	allSignals = append(allSignals, result.CloudSignals...)
	decision := converge.Decide(byName, result.CloudSignals, policy)
	if decision.Incident {
		result.Incidents = append(result.Incidents, e.incidents.Build(scenario, allSignals, decision))
	}
	return result
}

func endpointRuleEnabled(policy *policyv1.DetectionPolicy, name string) bool {
	if policy == nil || len(policy.GetEndpointRules()) == 0 {
		return true
	}
	for _, rule := range policy.GetEndpointRules() {
		if rule == name {
			return true
		}
	}
	return false
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
		Entities:     uniqueEntities(entities),
		Scenario:     scenario,
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
