package correlate

import (
	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	policyv1 "github.com/sysarmor/sysarmor-next-project/api/proto/policy/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/analytics/entity"
)

type View struct {
	ByName   map[string][]*signalv1.Signal
	Scenario string
}

func Build(events []*eventv1.CanonicalEvent, signals []*signalv1.Signal, policy *policyv1.DetectionPolicy) View {
	view := View{ByName: map[string][]*signalv1.Signal{}, Scenario: firstScenario(signals)}
	if view.Scenario == "" {
		view.Scenario = firstEventScenario(events)
	}
	for _, sig := range signals {
		if !EndpointRuleEnabled(policy, sig.GetName()) {
			continue
		}
		view.ByName[sig.GetName()] = append(view.ByName[sig.GetName()], sig)
	}
	return view
}

func (v View) Has(name string) bool {
	return len(v.ByName[name]) > 0
}

func (v View) HasTerminal(name string) bool {
	for _, sig := range v.ByName[name] {
		if sig.GetTerminal() {
			return true
		}
	}
	return false
}

func (v View) CollectEntities(names ...string) []*signalv1.EntityRef {
	var out []*signalv1.EntityRef
	for _, name := range names {
		for _, sig := range v.ByName[name] {
			out = append(out, sig.GetEntities()...)
		}
	}
	return entity.Unique(out)
}

func EndpointRuleEnabled(policy *policyv1.DetectionPolicy, name string) bool {
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
