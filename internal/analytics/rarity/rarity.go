package rarity

import "strings"

import signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"

type Scorer interface {
	Score(signals []*signalv1.Signal) float32
}

type Baseline struct {
	WorkloadCounts map[string]map[string]uint64
}

type RiskScorer struct{}

func (RiskScorer) Score(signals []*signalv1.Signal) float32 {
	var score float32
	for _, sig := range signals {
		score += float32(sig.GetBaseRisk()) * signalRarity(sig)
	}
	return score
}

type CountScorer struct{}

func (CountScorer) Score(signals []*signalv1.Signal) float32 {
	counts := map[string]uint32{}
	var score float32
	for _, sig := range signals {
		key := sig.GetName()
		if key == "" {
			key = sig.GetId()
		}
		counts[key]++
		score += float32(sig.GetBaseRisk()) * signalRarity(sig) / float32(counts[key])
	}
	return score
}

type WorkloadBaselineScorer struct {
	Baseline Baseline
}

func (s WorkloadBaselineScorer) Score(signals []*signalv1.Signal) float32 {
	var score float32
	for _, sig := range signals {
		score += float32(sig.GetBaseRisk()) * signalRarity(sig) * s.workloadRarity(sig)
	}
	return score
}

func (s WorkloadBaselineScorer) workloadRarity(sig *signalv1.Signal) float32 {
	count := s.Baseline.Count(workloadKey(sig), signalKey(sig))
	if count == 0 {
		return 1
	}
	return 1 / float32(count+1)
}

func (b Baseline) Count(workload, signal string) uint64 {
	if b.WorkloadCounts == nil {
		return 0
	}
	if signals := b.WorkloadCounts[workload]; signals != nil {
		if count := signals[signal]; count > 0 {
			return count
		}
	}
	if signals := b.WorkloadCounts["global"]; signals != nil {
		return signals[signal]
	}
	return 0
}

func signalRarity(sig *signalv1.Signal) float32 {
	if sig.GetGlobalRarity() == 0 {
		return 1
	}
	return sig.GetGlobalRarity()
}

func signalKey(sig *signalv1.Signal) string {
	key := strings.TrimSpace(sig.GetName())
	if key == "" {
		key = strings.TrimSpace(sig.GetId())
	}
	return key
}

func workloadKey(sig *signalv1.Signal) string {
	for _, kind := range []string{"pod", "container", "host", "user"} {
		for _, ent := range sig.GetEntities() {
			if ent.GetKind() == kind && ent.GetKey() != "" {
				return kind + ":" + ent.GetKey()
			}
		}
	}
	if sig.GetScenario() != "" {
		return "scenario:" + sig.GetScenario()
	}
	return "global"
}
