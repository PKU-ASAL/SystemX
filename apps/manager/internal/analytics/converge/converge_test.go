package converge

import (
	"testing"

	policyv1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/policy/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/signal/v1"
)

func TestDecideTerminalReverseShell(t *testing.T) {
	decision := Decide(map[string][]*signalv1.Signal{
		"reverse_shell_pattern": []*signalv1.Signal{{Terminal: true}},
	}, nil, nil)
	if !decision.Incident || decision.Method != "rarity+causal-topk" {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestDecideCrossLineageCloudSignal(t *testing.T) {
	decision := Decide(nil, []*signalv1.Signal{{
		Name:         "dropped_payload_executed_and_connects",
		CrossLineage: true,
	}}, nil)
	if !decision.Incident {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestDecideAdditiveThreshold(t *testing.T) {
	decision := Decide(map[string][]*signalv1.Signal{
		"download_by_lolbin": []*signalv1.Signal{{BaseRisk: 50}, {BaseRisk: 50}},
	}, nil, &policyv1.DetectionPolicy{Converge: &policyv1.ConvergeParams{Mode: "additive_threshold", AdditiveRiskThreshold: 100}})
	if !decision.Incident || decision.Method != "additive_threshold" {
		t.Fatalf("decision = %+v", decision)
	}
}
