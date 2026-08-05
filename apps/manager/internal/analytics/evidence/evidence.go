package evidence

import (
	"github.com/sysarmor/sysarmor-next-project/apps/manager/internal/analytics/graph"
	incidentv1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/incident/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/signal/v1"
)

func FromSignals(signals []*signalv1.Signal) *incidentv1.EvidenceSubgraph {
	return graph.FromSignals(signals).EvidenceSubgraph()
}
