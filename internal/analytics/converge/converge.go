package converge

import (
	policyv1 "github.com/sysarmor/sysarmor-next-project/api/proto/policy/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
)

type Decision struct {
	Incident bool
	Method   string
	Controls []string
}

func Decide(byName map[string][]*signalv1.Signal, cloud []*signalv1.Signal, policy *policyv1.DetectionPolicy) Decision {
	if policy != nil && policy.GetConverge().GetMode() == "additive_threshold" {
		threshold := policy.GetConverge().GetAdditiveRiskThreshold()
		if threshold == 0 {
			threshold = 100
		}
		return Decision{
			Incident: additiveRisk(byName) >= threshold,
			Method:   "additive_threshold",
			Controls: []string{"additive_threshold"},
		}
	}
	if hasTerminal(byName["reverse_shell_pattern"]) {
		return Decision{Incident: true, Method: "rarity+causal-topk", Controls: []string{"terminal_reverse_shell"}}
	}
	for _, sig := range cloud {
		if sig.GetName() == "dropped_payload_executed_and_connects" && sig.GetCrossLineage() {
			return Decision{Incident: true, Method: "rarity+causal-topk", Controls: []string{"cross_lineage_payload_connect"}}
		}
	}
	return Decision{Method: "rarity+causal-topk"}
}

func hasTerminal(signals []*signalv1.Signal) bool {
	for _, sig := range signals {
		if sig.GetTerminal() {
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
