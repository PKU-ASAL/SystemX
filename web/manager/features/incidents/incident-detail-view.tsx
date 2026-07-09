"use client";

import { ChevronLeftIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import type { IncidentDetail } from "@/lib/incident-detail-data";
import type { IncidentRecord } from "@/lib/mock-data";

import { IncidentAttackChainPanel } from "./incident-attack-chain-panel";
import { IncidentProvenancePanel } from "./incident-provenance-panel";

export type IncidentDetailMode = "attack-chain" | "provenance";

export function IncidentDetailView({
  incident,
  detail,
  mode,
  onBack,
}: {
  incident: IncidentRecord;
  detail: IncidentDetail;
  mode: IncidentDetailMode;
  onBack: () => void;
}) {
  return (
    <section className="flex h-full min-h-0 flex-col overflow-hidden bg-bg">
      <header className="flex shrink-0 flex-wrap items-center gap-2 border-b bg-muted/10 p-4">
        <Button intent="plain" size="sm" onPress={onBack}>
          <ChevronLeftIcon />
          Incidents
        </Button>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-mono text-sm font-semibold">{incident.chainId}</span>
            <Badge>{incident.severity}</Badge>
            <Badge className="bg-bg text-muted-fg">{incident.status}</Badge>
          </div>
          <p className="mt-1 truncate text-sm text-muted-fg">{detail.summary}</p>
        </div>
      </header>
      {mode === "attack-chain" ? (
        <IncidentAttackChainPanel detail={detail} />
      ) : (
        <IncidentProvenancePanel detail={detail} />
      )}
    </section>
  );
}
