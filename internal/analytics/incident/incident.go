package incident

import (
	"fmt"

	"github.com/sysarmor/sysarmor-next-project/internal/analytics/converge"
	"github.com/sysarmor/sysarmor-next-project/internal/analytics/evidence"
	"github.com/sysarmor/sysarmor-next-project/internal/analytics/rarity"
	incidentv1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/incident/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/signal/v1"
)

type Builder struct {
	nextID uint64
	Scorer rarity.Scorer
}

func NewBuilder() *Builder {
	return &Builder{Scorer: rarity.CountScorer{}}
}

func (b *Builder) Build(signals []*signalv1.Signal, decision converge.Decision) *incidentv1.Incident {
	if b.Scorer == nil {
		b.Scorer = rarity.CountScorer{}
	}
	b.nextID++
	contributing := contributingSignals(signals)
	return &incidentv1.Incident{
		Id:                  fmt.Sprintf("inc-%020d", b.nextID),
		Labels:              commonLabels(contributing),
		Summary:             "SysArmor detected a causal attack chain",
		Severity:            80,
		LineageIds:          lineageIDs(contributing),
		Terminals:           terminalEntities(contributing),
		Evidence:            evidence.FromSignals(contributing),
		Converge:            &incidentv1.ConvergeTrace{Method: decision.Method, Score: b.Scorer.Score(contributing), Controls: decision.Controls},
		ContributingSignals: contributing,
	}
}

func contributingSignals(signals []*signalv1.Signal) []*signalv1.Signal {
	out := make([]*signalv1.Signal, 0, len(signals))
	for _, sig := range signals {
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

func commonLabels(signals []*signalv1.Signal) map[string]string {
	var common map[string]string
	for i, sig := range signals {
		labels := sig.GetLabels()
		if i == 0 {
			common = cloneLabels(labels)
			continue
		}
		for key, value := range common {
			if labels[key] != value {
				delete(common, key)
			}
		}
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
