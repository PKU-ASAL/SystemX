package correlate

import (
	"github.com/sysarmor/sysarmor-next-project/apps/manager/internal/analytics/entity"
	eventv1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/event/v1"
	policyv1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/policy/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/signal/v1"
)

type View struct {
	ByName map[string][]*signalv1.Signal
	Labels map[string]string
}

func Build(events []*eventv1.CanonicalEvent, signals []*signalv1.Signal, policy *policyv1.DetectionPolicy) View {
	view := View{ByName: map[string][]*signalv1.Signal{}, Labels: commonLabels(events, signals)}
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

func commonLabels(events []*eventv1.CanonicalEvent, signals []*signalv1.Signal) map[string]string {
	var common map[string]string
	seen := false
	merge := func(labels map[string]string) {
		if len(labels) == 0 {
			return
		}
		if !seen {
			common = cloneLabels(labels)
			seen = true
			return
		}
		for key, value := range common {
			if labels[key] != value {
				delete(common, key)
			}
		}
	}
	for _, ev := range events {
		merge(ev.GetLabels())
	}
	for _, sig := range signals {
		merge(sig.GetLabels())
	}
	return common
}

func cloneLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	out := make(map[string]string, len(labels))
	for key, value := range labels {
		out[key] = value
	}
	return out
}
