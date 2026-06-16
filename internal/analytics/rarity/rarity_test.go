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
