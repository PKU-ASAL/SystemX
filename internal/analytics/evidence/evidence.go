package evidence

import (
	incidentv1 "github.com/sysarmor/sysarmor-next-project/api/proto/incident/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/analytics/entity"
)

func FromSignals(signals []*signalv1.Signal) *incidentv1.EvidenceSubgraph {
	seenNodes := map[string]bool{}
	nodes := make([]*incidentv1.GraphNode, 0)
	for _, sig := range signals {
		for _, ent := range entity.Unique(sig.GetEntities()) {
			if seenNodes[ent.GetKey()] {
				continue
			}
			seenNodes[ent.GetKey()] = true
			nodes = append(nodes, &incidentv1.GraphNode{
				Id:       ent.GetKey(),
				Kind:     ent.GetKind(),
				Label:    ent.GetKey(),
				Entities: []*signalv1.EntityRef{ent},
			})
		}
	}
	return &incidentv1.EvidenceSubgraph{Nodes: nodes}
}
