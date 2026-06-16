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

func TestBaselineObserveUpdatesWorkloadAndGlobalCounts(t *testing.T) {
	var baseline Baseline
	baseline.Observe([]*signalv1.Signal{{
		Name: "download_by_lolbin",
		Entities: []*signalv1.EntityRef{{
			Kind: "container",
			Key:  "checkout-api",
		}},
	}})
	if got := baseline.Count("container:checkout-api", "download_by_lolbin"); got != 1 {
		t.Fatalf("workload count = %d, want 1", got)
	}
	if got := baseline.Count("global", "download_by_lolbin"); got != 1 {
		t.Fatalf("global count = %d, want 1", got)
	}
}

func TestBaselineSnapshotIsDeepCopy(t *testing.T) {
	baseline := Baseline{WorkloadCounts: map[string]map[string]uint64{
		"host:node-a": {"reverse_shell_pattern": 2},
	}}
	snapshot := baseline.Snapshot()
	snapshot.Add("host:node-a", "reverse_shell_pattern", 3)
	if got := baseline.Count("host:node-a", "reverse_shell_pattern"); got != 2 {
		t.Fatalf("baseline changed through snapshot, count = %d", got)
	}
	if got := snapshot.Count("host:node-a", "reverse_shell_pattern"); got != 5 {
		t.Fatalf("snapshot count = %d, want 5", got)
	}
}

func TestBaselineMergeAddsCounts(t *testing.T) {
	baseline := Baseline{WorkloadCounts: map[string]map[string]uint64{
		"global": {"payload_dropped": 1},
	}}
	baseline.Merge(Baseline{WorkloadCounts: map[string]map[string]uint64{
		"global":              {"payload_dropped": 2},
		"pod:checkout-api-01": {"payload_dropped": 4},
	}})
	if got := baseline.Count("global", "payload_dropped"); got != 3 {
		t.Fatalf("global count = %d, want 3", got)
	}
	if got := baseline.Count("pod:checkout-api-01", "payload_dropped"); got != 4 {
		t.Fatalf("pod count = %d, want 4", got)
	}
}
