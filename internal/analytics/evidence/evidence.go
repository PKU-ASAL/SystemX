package evidence

import (
	incidentv1 "github.com/sysarmor/sysarmor-next-project/api/proto/incident/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/analytics/graph"
)

func FromSignals(signals []*signalv1.Signal) *incidentv1.EvidenceSubgraph {
	return graph.FromSignals(signals).EvidenceSubgraph()
}
