import { GitBranchIcon, NetworkIcon } from "lucide-react";

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
import { incidents, type IncidentRecord } from "@/lib/mock-data";
import { cn } from "@/lib/utils";

const severityClass = {
  critical: "border-destructive/40 bg-destructive/10 text-destructive",
  high: "text-status-high",
  medium: "text-status-medium",
} as const;

export function IncidentsPage() {
  return (
    <section className="flex h-full min-h-0 flex-col bg-bg">
      <header className="shrink-0 border-b px-6 py-5">
        <h1 className="text-xl font-semibold">威胁管理</h1>
      </header>
      <div className="min-h-0 flex-1 overflow-hidden">
        <Table containerClassName="h-full" className="min-w-[1040px]">
          <TableHeader className="sticky top-0 z-10 bg-bg">
            <TableRow>
              <TableHead>攻击链</TableHead>
              <TableHead>Kill Chain</TableHead>
              <TableHead>操作</TableHead>
              <TableHead>涉及主机</TableHead>
              <TableHead>告警数</TableHead>
              <TableHead>严重度</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {incidents.map((incident) => (
              <IncidentRow key={incident.id} incident={incident} />
            ))}
          </TableBody>
        </Table>
      </div>
    </section>
  );
}

function IncidentRow({ incident }: { incident: IncidentRecord }) {
  return (
    <TableRow className="hover:bg-muted/40">
      <TableCell>
        <div className="font-mono text-sm font-semibold">{incident.chainId}</div>
        <div className="mt-1 text-xs text-muted-fg">{incident.title}</div>
      </TableCell>
      <TableCell>
        <KillChainDots detected={incident.detectedStages} total={incident.totalStages} />
      </TableCell>
      <TableCell>
        <div className="flex items-center gap-2">
          <Button intent="plain" size="xs">
            <GitBranchIcon />
            攻击链
          </Button>
          <Button intent="plain" size="xs">
            <NetworkIcon />
            溯源图
          </Button>
        </div>
      </TableCell>
      <TableCell>
        <div className="flex flex-wrap gap-2">
          {incident.hosts.map((host) => (
            <span key={host} className="rounded bg-muted px-2 py-1 font-mono text-xs text-muted-fg">
              {host}
            </span>
          ))}
        </div>
      </TableCell>
      <TableCell className="font-semibold tabular-nums">{incident.alertCount}</TableCell>
      <TableCell>
        <Badge className={cn("rounded-full px-3", severityClass[incident.severity])}>
          {incident.severity}
        </Badge>
      </TableCell>
    </TableRow>
  );
}

function KillChainDots({ detected, total }: { detected: number; total: number }) {
  return (
    <div className="flex items-center gap-1">
      {Array.from({ length: total }).map((_, index) => {
        const active = index < detected;

        return (
          <span
            // Static index sequence renders a visual phase strip; there is no item id.
            key={index}
            className={cn("size-2.5 rounded-full", active ? "bg-success" : "bg-muted-fg/50")}
          />
        );
      })}
      <span className="ml-2 text-xs text-muted-fg">
        {detected}/{total}
      </span>
    </div>
  );
}
