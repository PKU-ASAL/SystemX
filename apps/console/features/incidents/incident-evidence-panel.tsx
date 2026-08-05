"use client";

import { XIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { IncidentDetail } from "@/lib/incident-detail-data";
import { cn } from "@/lib/utils";

type EvidenceEvent = IncidentDetail["evidence"][number];

const categoryClass = {
  network: "border-primary/30 bg-primary/10 text-fg",
  file: "border-warning/40 bg-warning/10 text-warning-fg",
  process: "border-success/40 bg-success/10 text-success-fg",
} as const;

export function IncidentEvidencePanel({
  events,
  totalLabel,
  selectedLabel,
  onClear,
}: {
  events: EvidenceEvent[];
  totalLabel: string;
  selectedLabel?: string;
  onClear?: () => void;
}) {
  return (
    <aside className="flex min-h-0 w-[420px] shrink-0 flex-col overflow-hidden border-l bg-bg max-xl:w-full max-xl:border-l-0 max-xl:border-t">
      <div className="flex shrink-0 items-start justify-between gap-3 border-b px-3 py-2.5">
        <div className="min-w-0">
          <div className="truncate text-[11px] font-semibold uppercase tracking-wider text-muted-fg">
            {selectedLabel ?? "Raw syscalls"}
          </div>
          <div className="mt-0.5 text-[10px] text-muted-fg">
            {totalLabel} · {events.length} rows
          </div>
        </div>
        {selectedLabel && onClear ? (
          <Button intent="plain" size="sq-xs" aria-label="Show all events" onPress={onClear}>
            <XIcon />
          </Button>
        ) : null}
      </div>
      <div className="min-h-0 flex-1 overflow-auto">
        {events.length === 0 ? (
          <div className="flex h-full items-center justify-center px-4 text-center text-xs text-muted-fg">
            No matching syscall events.
          </div>
        ) : (
          <Table>
            <TableHeader className="sticky top-0 z-10 bg-bg">
              <TableRow>
                <TableHead className="h-7 py-1 text-[10px]">Time</TableHead>
                <TableHead className="h-7 py-1 text-[10px]">Process</TableHead>
                <TableHead className="h-7 py-1 text-[10px]">Call</TableHead>
                <TableHead className="h-7 py-1 text-[10px]">Args</TableHead>
                <TableHead className="h-7 py-1 text-[10px]">Type</TableHead>
                <TableHead className="h-7 py-1 text-[10px]">Result</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {events.map((event) => (
                <TableRow key={`${event.time}-${event.process}-${event.syscall}-${event.args}`} className="hover:bg-muted/30">
                  <TableCell className="whitespace-nowrap py-1 font-mono text-[10px] text-muted-fg">
                    {event.time}
                  </TableCell>
                  <TableCell className="py-1 font-mono text-[10px]">{event.process}</TableCell>
                  <TableCell className="py-1">
                    <Badge className="bg-bg text-[10px] text-muted-fg">{event.syscall}</Badge>
                  </TableCell>
                  <TableCell className="max-w-[160px] py-1">
                    <span className="block truncate font-mono text-[9px] text-muted-fg">{event.args}</span>
                  </TableCell>
                  <TableCell className="py-1">
                    <Badge className={cn("border text-[9px]", categoryClass[event.category])}>{event.category}</Badge>
                  </TableCell>
                  <TableCell className="py-1 font-mono text-[10px] text-muted-fg">{event.result}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </div>
    </aside>
  );
}
