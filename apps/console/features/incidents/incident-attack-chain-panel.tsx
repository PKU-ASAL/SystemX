"use client";

import { useMemo, useState } from "react";
import {
  Background,
  Controls,
  Handle,
  MarkerType,
  Position,
  ReactFlow,
  type Edge,
  type Node,
  type NodeTypes,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";

import { Badge } from "@/components/ui/badge";
import type { AttackChainStep, IncidentDetail } from "@/lib/incident-detail-data";
import { getSyscallsForAttackStep } from "@/lib/incident-detail-data";
import { cn } from "@/lib/utils";

import { IncidentEvidencePanel } from "./incident-evidence-panel";

const mutedEdgeColor = "color-mix(in oklab, var(--color-muted-fg) 45%, transparent)";

const tacticClass: Record<string, { accent: string; header: string; text: string; dot: string }> = {
  "Initial Access": {
    accent: "border-l-destructive",
    header: "bg-destructive/10",
    text: "text-destructive",
    dot: "bg-destructive",
  },
  Execution: {
    accent: "border-l-warning",
    header: "bg-warning/10",
    text: "text-warning-fg",
    dot: "bg-warning",
  },
  "Command and Control": {
    accent: "border-l-primary",
    header: "bg-primary/10",
    text: "text-fg",
    dot: "bg-primary",
  },
  "Credential Access": {
    accent: "border-l-success",
    header: "bg-success/10",
    text: "text-success-fg",
    dot: "bg-success",
  },
  "Lateral Movement": {
    accent: "border-l-ring",
    header: "bg-secondary",
    text: "text-fg",
    dot: "bg-ring",
  },
  Discovery: {
    accent: "border-l-muted-fg",
    header: "bg-muted/40",
    text: "text-muted-fg",
    dot: "bg-muted-fg",
  },
};

const defaultTactic = {
  accent: "border-l-muted-fg",
  header: "bg-muted/40",
  text: "text-muted-fg",
  dot: "bg-muted-fg",
};

type ActionNodeData = AttackChainStep & {
  selected: boolean;
};

function ActionNode({ data }: { data: ActionNodeData }) {
  const tactic = tacticClass[data.tactic] ?? defaultTactic;

  return (
    <div
      className={cn(
        "min-w-[260px] max-w-[290px] cursor-pointer overflow-hidden rounded-sm border border-l-4 bg-bg shadow-md transition-all",
        tactic.accent,
        data.selected ? "ring-2 ring-primary ring-offset-1 ring-offset-bg shadow-lg" : "hover:border-muted-fg/40 hover:shadow-lg",
      )}
    >
      <Handle type="target" id="left" position={Position.Left} className="!size-2 !border-0 !bg-muted-fg" />
      <Handle type="source" id="right" position={Position.Right} className="!size-2 !border-0 !bg-muted-fg" />
      <div className={cn("border-b px-2.5 py-1.5", data.selected ? "bg-primary/10" : tactic.header)}>
        <div className="flex items-center justify-between gap-2">
          <span className={cn("text-[9px] font-bold uppercase tracking-widest", data.selected ? "text-fg" : tactic.text)}>
            {data.tactic}
          </span>
          <span className="font-mono text-[9px] text-muted-fg">{data.techniqueId}</span>
        </div>
      </div>
      <div className="px-3 pt-2.5 pb-1">
        <div className="text-[13px] font-bold leading-snug">{data.technique}</div>
        <div className="mt-1 grid grid-cols-[1fr_auto_1fr] items-center gap-2 font-mono text-[10px]">
          <span className="truncate rounded border bg-muted/30 px-1.5 py-1">{data.source}</span>
          <span className="text-muted-fg">→</span>
          <span className="truncate rounded border bg-muted/30 px-1.5 py-1">{data.target}</span>
        </div>
      </div>
      <div className="px-3 pb-2">
        <p className="line-clamp-2 text-[10px] leading-relaxed text-muted-fg">{data.evidence}</p>
      </div>
      <div className="flex items-center justify-between border-t bg-muted/20 px-3 py-1.5">
        <span className="flex items-center gap-1.5 font-mono text-[10px] text-muted-fg">
          <span className={cn("size-1.5 rounded-full", tactic.dot)} />
          action {data.order}
        </span>
        <Badge className={cn("border text-[9px]", data.detected ? "text-success" : "text-muted-fg")}>
          {data.detected ? "detected" : "missed"}
        </Badge>
      </div>
    </div>
  );
}

const nodeTypes: NodeTypes = { action: ActionNode as NodeTypes["action"] };

function buildActionChain(steps: AttackChainStep[], selectedStepId: string | null) {
  const sorted = [...steps].sort((a, b) => a.order - b.order);
  const nodes: Node<ActionNodeData>[] = sorted.map((step, index) => ({
    id: step.id,
    type: "action",
    position: { x: index * 330, y: 0 },
    data: { ...step, selected: step.id === selectedStepId },
  }));
  const edges: Edge[] = sorted.slice(1).map((step, index) => {
    const previous = sorted[index];

    return {
      id: `edge-${previous.id}-${step.id}`,
      source: previous.id,
      sourceHandle: "right",
      target: step.id,
      targetHandle: "left",
      type: "smoothstep",
      style: { stroke: mutedEdgeColor, strokeWidth: 1.5 },
      markerEnd: { type: MarkerType.ArrowClosed, width: 11, height: 11, color: mutedEdgeColor },
    };
  });

  return { nodes, edges };
}

export function IncidentAttackChainPanel({ detail }: { detail: IncidentDetail }) {
  const [selectedStepId, setSelectedStepId] = useState<string | null>(null);
  const selectedStep = detail.attackChain.find((step) => step.id === selectedStepId);
  const { nodes, edges } = useMemo(() => buildActionChain(detail.attackChain, selectedStepId), [detail.attackChain, selectedStepId]);
  const events = selectedStep ? getSyscallsForAttackStep(detail, selectedStep) : detail.evidence;

  return (
    <div className="flex min-h-0 flex-1 overflow-hidden max-xl:flex-col">
      <div className="min-w-0 flex-1">
        <ReactFlow
          nodes={nodes}
          edges={edges}
          nodeTypes={nodeTypes}
          fitView
          fitViewOptions={{ padding: 0.18, minZoom: 0.35 }}
          minZoom={0.12}
          maxZoom={2}
          proOptions={{ hideAttribution: true }}
          className="bg-bg"
          onNodeClick={(_, node) => setSelectedStepId((current) => (current === node.id ? null : node.id))}
          onPaneClick={() => setSelectedStepId(null)}
        >
          <Background gap={24} size={1} color="color-mix(in oklab, var(--color-muted-fg) 8%, transparent)" />
          <Controls className="!border-border !bg-bg !shadow-sm [&>button]:!border-border [&>button]:!bg-bg [&>button]:!text-fg" />
        </ReactFlow>
      </div>
      <IncidentEvidencePanel
        events={events}
        totalLabel={detail.chainId}
        selectedLabel={selectedStep ? `${selectedStep.tactic} · ${selectedStep.techniqueId}` : undefined}
        onClear={() => setSelectedStepId(null)}
      />
    </div>
  );
}
