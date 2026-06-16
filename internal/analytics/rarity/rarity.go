package rarity

import signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"

type Scorer interface {
	Score(signals []*signalv1.Signal) float32
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

func signalRarity(sig *signalv1.Signal) float32 {
	if sig.GetGlobalRarity() == 0 {
		return 1
	}
	return sig.GetGlobalRarity()
}
