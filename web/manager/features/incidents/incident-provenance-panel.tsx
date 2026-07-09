"use client";

import { useCallback, useState } from "react";
import dagre from "@dagrejs/dagre";
import {
  Background,
  BaseEdge,
  Controls,
  EdgeLabelRenderer,
  Handle,
  Position,
  ReactFlow,
  getStraightPath,
  type Edge,
  type EdgeProps,
  type Node,
  type NodeTypes,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";

import type { IncidentDetail, ProvenanceNode } from "@/lib/incident-detail-data";
import { getSyscallsForProvenanceNode } from "@/lib/incident-detail-data";
import { cn } from "@/lib/utils";

import { IncidentEvidencePanel } from "./incident-evidence-panel";

type ProvenanceNodeData = {
  label: string;
  nodeType: string;
  nodeDesc: string;
  nodeScore: number;
  stageLevel?: string;
  nodeVariant?: string;
  revealAtSec?: number;
  selected?: boolean;
  rawNode: ProvenanceNode;
};

type ProvenanceEdgeData = {
  technique?: string;
  tactic?: string;
};

const variantClass = {
  tp: {
    border: "border-orange-500",
    bg: "bg-orange-500/15",
    label: "text-orange-300",
    glow: "shadow-[0_0_8px_rgba(249,115,22,0.4)]",
  },
  propagated: {
    border: "border-slate-500",
    bg: "bg-slate-500/10",
    label: "text-slate-300",
    glow: "",
  },
  fp: {
    border: "border-sky-700/60",
    bg: "bg-sky-900/10",
    label: "text-sky-400/70",
    glow: "",
  },
} as const;

const stageClass = {
  L1: "border-red-500/30 bg-red-500/10 text-red-400",
  L2: "border-orange-500/30 bg-orange-500/10 text-orange-400",
  L3: "border-yellow-500/30 bg-yellow-500/10 text-yellow-400",
  L4: "border-purple-500/30 bg-purple-500/10 text-purple-400",
} as const;

const tacticColor: Record<string, string> = {
  InitialAccess: "text-red-400",
  Execution: "text-orange-400",
  Persistence: "text-amber-400",
  CredentialAccess: "text-rose-400",
  LateralMovement: "text-purple-400",
  Discovery: "text-sky-400",
  C2: "text-pink-400",
};

function ProvenanceNodeCard({ data }: { data: ProvenanceNodeData }) {
  const variant = (data.nodeVariant ?? "propagated") as keyof typeof variantClass;
  const stage = data.stageLevel as keyof typeof stageClass | undefined;
  const styles = variantClass[variant] ?? variantClass.propagated;
  const icon = data.nodeType === "2" ? "⬡" : data.nodeType === "3" ? "▤" : "⚙";

  return (
    <div
      className={cn(
        "min-w-[130px] max-w-[180px] cursor-pointer rounded-xl border px-2.5 py-2 text-[11px] leading-tight transition-all",
        styles.border,
        styles.bg,
        styles.glow,
        data.selected && "scale-105 ring-2 ring-primary/50 ring-offset-1 ring-offset-bg",
      )}
    >
      <Handle type="target" position={Position.Top} className="!size-1 !border-0 !bg-transparent" />
      <div className="mb-1 flex items-center gap-1.5">
        <span className={cn("shrink-0 text-[13px]", styles.label)}>{icon}</span>
        <span className={cn("truncate font-mono font-semibold", styles.label)}>{data.label}</span>
        {stage ? (
          <span className={cn("ml-auto shrink-0 rounded border px-1 text-[9px] font-medium", stageClass[stage])}>
            {stage}
          </span>
        ) : null}
      </div>
      <div className="truncate text-[9px] text-muted-fg">{data.nodeDesc}</div>
      {data.nodeScore >= 70 ? (
        <div className={cn("mt-0.5 font-mono text-[9px]", variant === "tp" ? "text-orange-400" : "text-muted-fg")}>
          risk:{data.nodeScore}
        </div>
      ) : null}
      <Handle type="source" position={Position.Bottom} className="!size-1 !border-0 !bg-transparent" />
    </div>
  );
}

function TechniqueEdge({ id, sourceX, sourceY, targetX, targetY, data }: EdgeProps<Edge<ProvenanceEdgeData>>) {
  const [edgePath, labelX, labelY] = getStraightPath({ sourceX, sourceY, targetX, targetY });
  const technique = data?.technique;
  const color = data?.tactic ? tacticColor[data.tactic] ?? "text-muted-fg" : "text-muted-fg";

  return (
    <>
      <BaseEdge id={id} path={edgePath} style={{ stroke: "rgba(249,115,22,0.35)", strokeWidth: 1.5 }} />
      {technique ? (
        <EdgeLabelRenderer>
          <div
            className="nodrag nopan absolute"
            style={{ transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px)`, pointerEvents: "none" }}
          >
            <span className={cn("rounded border border-border bg-bg/90 px-1 py-0.5 font-mono text-[8px]", color)}>
              {technique}
            </span>
          </div>
        </EdgeLabelRenderer>
      ) : null}
    </>
  );
}

const nodeTypes: NodeTypes = { prov: ProvenanceNodeCard as NodeTypes["prov"] };
const edgeTypes = { technique: TechniqueEdge };

function layoutGraph(nodes: Node<ProvenanceNodeData>[], edges: Edge<ProvenanceEdgeData>[]) {
  const graph = new dagre.graphlib.Graph();
  graph.setDefaultEdgeLabel(() => ({}));
  graph.setGraph({ rankdir: "TB", nodesep: 60, ranksep: 80 });
  nodes.forEach((node) => graph.setNode(node.id, { width: 180, height: 68 }));
  edges.forEach((edge) => graph.setEdge(edge.source, edge.target));
  dagre.layout(graph);

  return nodes.map((node) => {
    const position = graph.node(node.id);

    return { ...node, position: { x: position.x - 90, y: position.y - 34 } };
  });
}

function GraphLegend() {
  return (
    <div className="pointer-events-none absolute right-3 top-3 z-10 flex flex-col gap-2">
      <div className="space-y-1.5 rounded-lg border border-border bg-bg/90 px-3 py-2 text-[10px] text-muted-fg shadow-sm">
        <div className="font-medium text-fg">Node Types</div>
        <LegendItem className="border-orange-500 bg-orange-500/15" label="confirmed malicious (TP)" />
        <LegendItem className="border-slate-500 bg-slate-500/10" label="propagated context" />
        <LegendItem className="border-sky-700/60 bg-sky-900/10" label="normal context (FP)" />
      </div>
      <div className="space-y-1 rounded-lg border border-border bg-bg/90 px-3 py-2 text-[10px] text-muted-fg shadow-sm">
        <div className="font-medium text-fg">Attack Stages</div>
        {(["L1", "L2", "L3", "L4"] as const).map((stage) => (
          <div key={stage} className="flex items-center gap-1.5">
            <span className={cn("rounded border px-1 font-mono text-[9px]", stageClass[stage])}>{stage}</span>
          </div>
        ))}
      </div>
      <div className="rounded-lg border border-border bg-bg/90 px-3 py-2 text-[10px] text-muted-fg shadow-sm">
        Click a node to filter syscalls
      </div>
    </div>
  );
}

function LegendItem({ className, label }: { className: string; label: string }) {
  return (
    <span className="flex items-center gap-2">
      <span className={cn("size-3 rounded border", className)} />
      {label}
    </span>
  );
}

function buildProvenanceGraph(detail: IncidentDetail, selectedNodeId: string | null) {
  const nodes: Node<ProvenanceNodeData>[] = detail.provenance.nodes.map((node) => ({
    id: node.id,
    type: "prov",
    position: { x: 0, y: 0 },
    data: {
      label: node.node_name ?? node.name,
      nodeType: node.node_type ?? "1",
      nodeDesc: node.node_desc ?? node.description,
      nodeScore: node.node_score ?? node.score,
      stageLevel: node.stageLevel,
      nodeVariant: node.nodeVariant,
      revealAtSec: node.revealAtSec,
      selected: selectedNodeId === node.id,
      rawNode: node,
    },
  }));
  const nodeIds = new Set(nodes.map((node) => node.id));
  const edges: Edge<ProvenanceEdgeData>[] = detail.provenance.edges
    .filter((edge) => nodeIds.has(edge.source) && nodeIds.has(edge.target))
    .map((edge, index) => ({
      id: `edge-${edge.source}-${edge.target}-${index}`,
      source: edge.source,
      target: edge.target,
      type: "technique",
      data: { technique: edge.technique, tactic: edge.tactic },
      markerEnd: { type: "arrowclosed", color: "rgba(249,115,22,0.6)" },
    }));

  return { nodes: layoutGraph(nodes, edges), edges };
}

export function IncidentProvenancePanel({ detail }: { detail: IncidentDetail }) {
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const selectedNode = detail.provenance.nodes.find((node) => node.id === selectedNodeId);
  const { nodes, edges } = buildProvenanceGraph(detail, selectedNodeId);
  const events = selectedNode ? getSyscallsForProvenanceNode(detail, selectedNode) : detail.evidence;

  const handleNodeClick = useCallback((_: React.MouseEvent, node: Node<ProvenanceNodeData>) => {
    setSelectedNodeId((current) => (current === node.id ? null : node.id));
  }, []);

  return (
    <div className="flex min-h-0 flex-1 overflow-hidden max-xl:flex-col">
      <div className="min-w-0 flex-1 bg-bg">
        <div className="relative h-full w-full">
          <ReactFlow
            nodes={nodes}
            edges={edges}
            nodeTypes={nodeTypes}
            edgeTypes={edgeTypes}
            fitView
            fitViewOptions={{ padding: 0.2 }}
            minZoom={0.2}
            maxZoom={2.5}
            proOptions={{ hideAttribution: true }}
            className="bg-bg"
            onNodeClick={handleNodeClick}
            onPaneClick={() => setSelectedNodeId(null)}
          >
            <Background gap={24} size={1} color="color-mix(in oklab, var(--color-warning) 18%, transparent)" />
            <Controls className="!border-border !bg-bg !shadow-sm [&>button]:!border-border [&>button]:!bg-bg [&>button]:!text-fg" />
          </ReactFlow>
          <GraphLegend />
        </div>
      </div>
      <IncidentEvidencePanel
        events={events}
        totalLabel={detail.chainId}
        selectedLabel={selectedNode ? `${selectedNode.node_name ?? selectedNode.name} (${selectedNode.stageLevel ?? selectedNode.stage})` : undefined}
        onClear={() => setSelectedNodeId(null)}
      />
    </div>
  );
}
