package rarity

import (
	"testing"

	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
)

func TestCountScorerKeepsUniqueSignalRisk(t *testing.T) {
	got := CountScorer{}.Score([]*signalv1.Signal{
		{Name: "reverse_shell_pattern", BaseRisk: 80, GlobalRarity: 1},
	})
	if got != 80 {
		t.Fatalf("score = %f, want 80", got)
	}
}

func TestCountScorerDownweightsRepeatedSignalNames(t *testing.T) {
	got := CountScorer{}.Score([]*signalv1.Signal{
		{Name: "download_by_lolbin", BaseRisk: 50, GlobalRarity: 1},
		{Name: "download_by_lolbin", BaseRisk: 50, GlobalRarity: 1},
	})
	if got != 75 {
		t.Fatalf("score = %f, want 75", got)
	}
}

func TestWorkloadBaselineScorerDownweightsCommonWorkloadSignal(t *testing.T) {
	scorer := WorkloadBaselineScorer{Baseline: Baseline{WorkloadCounts: map[string]map[string]uint64{
		"container:checkout-api": {"download_by_lolbin": 4},
	}}}
	got := scorer.Score([]*signalv1.Signal{{
		Name:         "download_by_lolbin",
		BaseRisk:     50,
		GlobalRarity: 1,
		Entities: []*signalv1.EntityRef{{
			Kind: "container",
			Key:  "checkout-api",
		}},
	}})
	if got != 10 {
		t.Fatalf("score = %f, want 10", got)
	}
}

func TestWorkloadBaselineScorerFallsBackToGlobal(t *testing.T) {
	scorer := WorkloadBaselineScorer{Baseline: Baseline{WorkloadCounts: map[string]map[string]uint64{
		"global": {"reverse_shell_pattern": 1},
	}}}
	got := scorer.Score([]*signalv1.Signal{{Name: "reverse_shell_pattern", BaseRisk: 80, GlobalRarity: 1}})
	if got != 40 {
		t.Fatalf("score = %f, want 40", got)
	}
}

func TestWorkloadBaselineScorerKeepsUnknownWorkloadRare(t *testing.T) {
	scorer := WorkloadBaselineScorer{Baseline: Baseline{WorkloadCounts: map[string]map[string]uint64{
		"container:checkout-api": {"download_by_lolbin": 4},
	}}}
	got := scorer.Score([]*signalv1.Signal{{
		Name:         "reverse_shell_pattern",
		BaseRisk:     80,
		GlobalRarity: 1,
		Entities: []*signalv1.EntityRef{{
			Kind: "container",
			Key:  "checkout-api",
		}},
	}})
	if got != 80 {
		t.Fatalf("score = %f, want 80", got)
	}
}
