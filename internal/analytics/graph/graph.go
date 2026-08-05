package graph

import (
	"fmt"

	"github.com/sysarmor/sysarmor-next-project/internal/analytics/entity"
	incidentv1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/incident/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/signal/v1"
)

type Graph struct {
	nodes     map[string]*incidentv1.GraphNode
	nodeOrder []string
	edges     map[string]*incidentv1.GraphEdge
	edgeOrder []string
}

func FromSignals(signals []*signalv1.Signal) *Graph {
	g := New()
	for _, sig := range signals {
		g.AddSignal(sig)
	}
	return g
}

func New() *Graph {
	return &Graph{
		nodes: map[string]*incidentv1.GraphNode{},
		edges: map[string]*incidentv1.GraphEdge{},
	}
}

func (g *Graph) AddSignal(sig *signalv1.Signal) {
	if g == nil || sig == nil {
		return
	}
	refs := entity.Unique(sig.GetEntities())
	for _, ref := range refs {
		g.addNode(ref)
	}
	g.addSignalEdges(sig, refs)
}

func (g *Graph) EvidenceSubgraph() *incidentv1.EvidenceSubgraph {
	if g == nil {
		return &incidentv1.EvidenceSubgraph{}
	}
	nodes := make([]*incidentv1.GraphNode, 0, len(g.nodeOrder))
	for _, id := range g.nodeOrder {
		nodes = append(nodes, g.nodes[id])
	}
	edges := make([]*incidentv1.GraphEdge, 0, len(g.edgeOrder))
	for _, id := range g.edgeOrder {
		edges = append(edges, g.edges[id])
	}
	return &incidentv1.EvidenceSubgraph{Nodes: nodes, Edges: edges}
}

func (g *Graph) KHop(seed string, hops int) *incidentv1.EvidenceSubgraph {
	if g == nil || seed == "" || hops < 0 {
		return &incidentv1.EvidenceSubgraph{}
	}
	if _, ok := g.nodes[seed]; !ok {
		return &incidentv1.EvidenceSubgraph{}
	}
	seenNodes := map[string]bool{seed: true}
	seenEdges := map[string]bool{}
	frontier := []string{seed}
	for depth := 0; depth < hops && len(frontier) > 0; depth++ {
		var next []string
		for _, node := range frontier {
			for _, edgeID := range g.edgeOrder {
				edge := g.edges[edgeID]
				if edge.GetFrom() != node && edge.GetTo() != node {
					continue
				}
				seenEdges[edgeID] = true
				other := edge.GetTo()
				if other == node {
					other = edge.GetFrom()
				}
				if !seenNodes[other] {
					seenNodes[other] = true
					next = append(next, other)
				}
			}
		}
		frontier = next
	}
	return g.subgraph(seenNodes, seenEdges)
}

type pathParent struct {
	node string
	edge string
}

func (g *Graph) ShortestPath(from, to string) *incidentv1.EvidenceSubgraph {
	if g == nil || from == "" || to == "" {
		return &incidentv1.EvidenceSubgraph{}
	}
	if _, ok := g.nodes[from]; !ok {
		return &incidentv1.EvidenceSubgraph{}
	}
	if _, ok := g.nodes[to]; !ok {
		return &incidentv1.EvidenceSubgraph{}
	}
	if from == to {
		return g.subgraph(map[string]bool{from: true}, nil)
	}
	parents := map[string]pathParent{}
	seen := map[string]bool{from: true}
	queue := []string{from}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		for _, edgeID := range g.edgeOrder {
			edge := g.edges[edgeID]
			if edge.GetFrom() != node && edge.GetTo() != node {
				continue
			}
			other := edge.GetTo()
			if other == node {
				other = edge.GetFrom()
			}
			if seen[other] {
				continue
			}
			seen[other] = true
			parents[other] = pathParent{node: node, edge: edgeID}
			if other == to {
				return g.pathSubgraph(from, to, parents)
			}
			queue = append(queue, other)
		}
	}
	return &incidentv1.EvidenceSubgraph{}
}

func (g *Graph) addNode(ref *signalv1.EntityRef) {
	if ref == nil || ref.GetKey() == "" {
		return
	}
	if _, ok := g.nodes[ref.GetKey()]; ok {
		return
	}
	g.nodes[ref.GetKey()] = &incidentv1.GraphNode{
		Id:       ref.GetKey(),
		Kind:     ref.GetKind(),
		Label:    ref.GetKey(),
		Entities: []*signalv1.EntityRef{ref},
	}
	g.nodeOrder = append(g.nodeOrder, ref.GetKey())
}

func (g *Graph) addSignalEdges(sig *signalv1.Signal, refs []*signalv1.EntityRef) {
	if len(refs) < 2 {
		return
	}
	subject := subjectEntity(refs)
	if subject != nil {
		for _, ref := range refs {
			if ref.GetKey() == subject.GetKey() {
				continue
			}
			g.addEdge(subject.GetKey(), ref.GetKey(), edgeKind(sig, ref))
		}
		return
	}
	for i := 0; i < len(refs)-1; i++ {
		g.addEdge(refs[i].GetKey(), refs[i+1].GetKey(), edgeKind(sig, refs[i+1]))
	}
}

func (g *Graph) addEdge(from, to, kind string) {
	if from == "" || to == "" || from == to {
		return
	}
	id := fmt.Sprintf("%s:%s->%s", kind, from, to)
	if _, ok := g.edges[id]; ok {
		return
	}
	g.edges[id] = &incidentv1.GraphEdge{Id: id, From: from, To: to, Kind: kind}
	g.edgeOrder = append(g.edgeOrder, id)
}

func (g *Graph) pathSubgraph(from, to string, parents map[string]pathParent) *incidentv1.EvidenceSubgraph {
	seenNodes := map[string]bool{to: true}
	seenEdges := map[string]bool{}
	for cur := to; cur != from; {
		parent, ok := parents[cur]
		if !ok {
			return &incidentv1.EvidenceSubgraph{}
		}
		seenNodes[parent.node] = true
		seenEdges[parent.edge] = true
		cur = parent.node
	}
	return g.subgraph(seenNodes, seenEdges)
}

func (g *Graph) subgraph(seenNodes, seenEdges map[string]bool) *incidentv1.EvidenceSubgraph {
	nodes := make([]*incidentv1.GraphNode, 0, len(seenNodes))
	for _, id := range g.nodeOrder {
		if seenNodes[id] {
			nodes = append(nodes, g.nodes[id])
		}
	}
	edges := make([]*incidentv1.GraphEdge, 0, len(seenEdges))
	for _, id := range g.edgeOrder {
		if seenEdges[id] {
			edges = append(edges, g.edges[id])
		}
	}
	return &incidentv1.EvidenceSubgraph{Nodes: nodes, Edges: edges}
}

func subjectEntity(refs []*signalv1.EntityRef) *signalv1.EntityRef {
	for _, ref := range refs {
		if ref.GetRole() == "subject" {
			return ref
		}
	}
	for _, ref := range refs {
		if ref.GetKind() == "process" {
			return ref
		}
	}
	return nil
}

func edgeKind(sig *signalv1.Signal, to *signalv1.EntityRef) string {
	switch to.GetKind() {
	case "socket":
		return "connect"
	case "file":
		if sig.GetName() == "payload_dropped" {
			return "write"
		}
		return "load"
	case "process":
		return "exec"
	default:
		return "relates_to"
	}
}
