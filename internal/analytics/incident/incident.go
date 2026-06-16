package incident

import (
	"fmt"

	incidentv1 "github.com/sysarmor/sysarmor-next-project/api/proto/incident/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/analytics/converge"
	"github.com/sysarmor/sysarmor-next-project/internal/analytics/evidence"
	"github.com/sysarmor/sysarmor-next-project/internal/analytics/rarity"
)

type Builder struct {
	nextID uint64
	Scorer rarity.Scorer
}

func NewBuilder() *Builder {
	return &Builder{Scorer: rarity.RiskScorer{}}
}

func (b *Builder) Build(scenario string, signals []*signalv1.Signal, decision converge.Decision) *incidentv1.Incident {
	if b.Scorer == nil {
		b.Scorer = rarity.RiskScorer{}
	}
	b.nextID++
	contributing := contributingSignals(scenario, signals)
	return &incidentv1.Incident{
		Id:                  fmt.Sprintf("inc-%020d", b.nextID),
		Scenario:            scenario,
		Summary:             "SysArmor detected a causal attack chain",
		Severity:            80,
		LineageIds:          lineageIDs(contributing),
		Terminals:           terminalEntities(contributing),
		Evidence:            evidence.FromSignals(contributing),
		Converge:            &incidentv1.ConvergeTrace{Method: decision.Method, Score: b.Scorer.Score(contributing), Controls: decision.Controls},
		ContributingSignals: contributing,
		Status:              "open",
	}
}

func contributingSignals(scenario string, signals []*signalv1.Signal) []*signalv1.Signal {
	out := make([]*signalv1.Signal, 0, len(signals))
	for _, sig := range signals {
		if sig.GetScenario() != "" && scenario != "" && sig.GetScenario() != scenario {
			continue
		}
		out = append(out, sig)
	}
	return out
}

func lineageIDs(signals []*signalv1.Signal) []string {
	seen := map[string]bool{}
	var out []string
	for _, sig := range signals {
		if sig.GetLineageId() != "" && !seen[sig.GetLineageId()] {
			seen[sig.GetLineageId()] = true
			out = append(out, sig.GetLineageId())
		}
	}
	return out
}

func terminalEntities(signals []*signalv1.Signal) []string {
	var out []string
	for _, sig := range signals {
		if !sig.GetTerminal() {
			continue
		}
		for _, ent := range sig.GetEntities() {
			if ent.GetKind() == "process" {
				out = append(out, ent.GetKey())
			}
		}
	}
	return out
}
