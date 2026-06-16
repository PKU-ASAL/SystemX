package incident

import (
	"testing"

	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/analytics/converge"
)

func TestBuilderCreatesIncidentWithEvidenceAndStatus(t *testing.T) {
	builder := NewBuilder()
	inc := builder.Build("scenario-a", []*signalv1.Signal{
		{
			Name:         "reverse_shell_pattern",
			BaseRisk:     80,
			GlobalRarity: 1,
			LineageId:    "lin-a",
			Terminal:     true,
			Scenario:     "scenario-a",
			Entities: []*signalv1.EntityRef{
				{Kind: "process", Key: "process:p-bash", Role: "subject"},
				{Kind: "socket", Key: "socket:10.66.0.99:443", Role: "object"},
			},
		},
	}, converge.Decision{Incident: true, Method: "rarity+causal-topk", Controls: []string{"terminal_reverse_shell"}})
	if inc.GetStatus() != "open" {
		t.Fatalf("status = %q, want open", inc.GetStatus())
	}
	if inc.GetConverge().GetScore() != 80 || inc.GetConverge().GetMethod() != "rarity+causal-topk" {
		t.Fatalf("converge = %+v", inc.GetConverge())
	}
	if len(inc.GetEvidence().GetEdges()) == 0 {
		t.Fatalf("evidence edges missing: %+v", inc.GetEvidence())
	}
	if len(inc.GetTerminals()) != 1 || inc.GetTerminals()[0] != "process:p-bash" {
		t.Fatalf("terminals = %v", inc.GetTerminals())
	}
}
