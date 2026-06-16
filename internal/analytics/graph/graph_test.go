package graph

import (
	"testing"

	incidentv1 "github.com/sysarmor/sysarmor-next-project/api/proto/incident/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
)

func TestFromSignalsBuildsEvidenceGraphEdges(t *testing.T) {
	got := FromSignals([]*signalv1.Signal{
		sig("payload_dropped", file("/var/lib/app/plugins/helper")),
		sig("suspicious_exec_connect", file("/var/lib/app/plugins/helper"), socket("10.66.0.99:443")),
	}).EvidenceSubgraph()
	if len(got.GetNodes()) != 2 {
		t.Fatalf("nodes = %d, want 2: %#v", len(got.GetNodes()), got.GetNodes())
	}
	if !hasNode(got.GetNodes(), "file:/var/lib/app/plugins/helper") || !hasNode(got.GetNodes(), "socket:10.66.0.99:443") {
		t.Fatalf("missing expected nodes: %#v", got.GetNodes())
	}
	if !hasEdge(got.GetEdges(), "file:/var/lib/app/plugins/helper", "socket:10.66.0.99:443", "connect") {
		t.Fatalf("missing file->socket connect edge: %#v", got.GetEdges())
	}
}

func TestFromSignalsUsesSubjectEdges(t *testing.T) {
	got := FromSignals([]*signalv1.Signal{
		sig("reverse_shell_pattern", process("p-bash"), socket("10.66.0.99:443")),
	}).EvidenceSubgraph()
	if !hasEdge(got.GetEdges(), "process:p-bash", "socket:10.66.0.99:443", "connect") {
		t.Fatalf("missing process->socket connect edge: %#v", got.GetEdges())
	}
}

func TestShortestPathReturnsPathSubgraph(t *testing.T) {
	g := FromSignals([]*signalv1.Signal{
		sig("payload_dropped", process("p-curl"), file("/dev/shm/x.sh")),
		sig("reverse_shell_pattern", process("p-curl"), socket("10.66.0.99:443")),
	})
	got := g.ShortestPath("file:/dev/shm/x.sh", "socket:10.66.0.99:443")
	if !hasNode(got.GetNodes(), "file:/dev/shm/x.sh") || !hasNode(got.GetNodes(), "process:p-curl") || !hasNode(got.GetNodes(), "socket:10.66.0.99:443") {
		t.Fatalf("path nodes missing: %#v", got.GetNodes())
	}
	if len(got.GetEdges()) != 2 {
		t.Fatalf("path edges = %d, want 2: %#v", len(got.GetEdges()), got.GetEdges())
	}
}

func TestKHopReturnsNeighborhood(t *testing.T) {
	g := FromSignals([]*signalv1.Signal{
		sig("payload_dropped", process("p-curl"), file("/dev/shm/x.sh")),
		sig("reverse_shell_pattern", process("p-curl"), socket("10.66.0.99:443")),
	})
	got := g.KHop("process:p-curl", 1)
	if !hasNode(got.GetNodes(), "file:/dev/shm/x.sh") || !hasNode(got.GetNodes(), "socket:10.66.0.99:443") {
		t.Fatalf("neighborhood nodes missing: %#v", got.GetNodes())
	}
	if len(got.GetEdges()) != 2 {
		t.Fatalf("neighborhood edges = %d, want 2: %#v", len(got.GetEdges()), got.GetEdges())
	}
}

func sig(name string, entities ...*signalv1.EntityRef) *signalv1.Signal {
	return &signalv1.Signal{Name: name, Entities: entities}
}

func process(key string) *signalv1.EntityRef {
	return &signalv1.EntityRef{Kind: "process", Key: key, Role: "subject"}
}

func file(key string) *signalv1.EntityRef {
	return &signalv1.EntityRef{Kind: "file", Key: key, Role: "object"}
}

func socket(key string) *signalv1.EntityRef {
	return &signalv1.EntityRef{Kind: "socket", Key: key, Role: "object"}
}

func hasNode(nodes []*incidentv1.GraphNode, id string) bool {
	for _, node := range nodes {
		if node.GetId() == id {
			return true
		}
	}
	return false
}

func hasEdge(edges []*incidentv1.GraphEdge, from, to, kind string) bool {
	for _, edge := range edges {
		if edge.GetFrom() == from && edge.GetTo() == to && edge.GetKind() == kind {
			return true
		}
	}
	return false
}
