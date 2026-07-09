import { useMemo, useRef, useState } from "react";
import {
  flexRender,
  getCoreRowModel,
  getSortedRowModel,
  useReactTable,
  type Cell,
  type ColumnDef,
  type SortingState,
} from "@tanstack/react-table";

import { GitBranchIcon, NetworkIcon, SearchIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  buildIncidentSeverityBuckets,
  filterIncidents,
  incidents,
  type IncidentRecord,
  type IncidentSeverityBucket,
} from "@/lib/mock-data";
import { cn } from "@/lib/utils";

const severityClass = {
  critical: "border-destructive/40 bg-destructive/10 text-destructive",
  high: "text-status-high",
  medium: "text-status-medium",
} as const;

export function IncidentsPage() {
  const [query, setQuery] = useState("");
  const [severity, setSeverity] = useState<IncidentRecord["severity"] | "all">("all");
  const [status, setStatus] = useState<IncidentRecord["status"] | "all">("all");
  const filteredIncidents = useMemo(
    () => filterIncidents(incidents, { query, severity, status }),
    [query, severity, status],
  );
  const severityBuckets = useMemo(
    () => buildIncidentSeverityBuckets(filteredIncidents),
    [filteredIncidents],
  );

  return (
    <section className="flex h-full min-h-0 flex-col bg-bg">
      <header className="shrink-0 border-b bg-muted/10 px-6 py-5">
        <h1 className="text-xl font-semibold">威胁管理</h1>
      </header>
      <IncidentWorkbench
        query={query}
        severity={severity}
        status={status}
        rows={filteredIncidents}
        severityBuckets={severityBuckets}
        onQueryChange={setQuery}
        onSeverityChange={setSeverity}
        onStatusChange={setStatus}
      />
    </section>
  );
}

function IncidentWorkbench({
  query,
  severity,
  status,
  rows,
  severityBuckets,
  onQueryChange,
  onSeverityChange,
  onStatusChange,
}: {
  query: string;
  severity: IncidentRecord["severity"] | "all";
  status: IncidentRecord["status"] | "all";
  rows: IncidentRecord[];
  severityBuckets: IncidentSeverityBucket[];
  onQueryChange: (value: string) => void;
  onSeverityChange: (value: IncidentRecord["severity"] | "all") => void;
  onStatusChange: (value: IncidentRecord["status"] | "all") => void;
}) {
  return (
    <>
      <div className="flex shrink-0 flex-col gap-3 border-b bg-muted/10 p-4">
        <IncidentFilterBar
          query={query}
          severity={severity}
          status={status}
          onQueryChange={onQueryChange}
          onSeverityChange={onSeverityChange}
          onStatusChange={onStatusChange}
        />
        <IncidentChart rows={rows} buckets={severityBuckets} />
      </div>
      <IncidentTable rows={rows} />
    </>
  );
}

function IncidentFilterBar({
  query,
  severity,
  status,
  onQueryChange,
  onSeverityChange,
  onStatusChange,
}: {
  query: string;
  severity: IncidentRecord["severity"] | "all";
  status: IncidentRecord["status"] | "all";
  onQueryChange: (value: string) => void;
  onSeverityChange: (value: IncidentRecord["severity"] | "all") => void;
  onStatusChange: (value: IncidentRecord["status"] | "all") => void;
}) {
  return (
    <div className="flex min-h-11 items-stretch overflow-hidden rounded-lg border bg-bg max-lg:flex-wrap max-lg:overflow-visible max-lg:border-0 max-lg:bg-transparent">
      <div className="relative min-w-72 flex-1 max-lg:min-w-full max-lg:rounded-lg max-lg:border">
        <SearchIcon className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-fg" />
        <Input
          className="h-11 rounded-none border-0 bg-transparent pl-9 font-mono focus-visible:ring-0 max-lg:rounded-lg"
          value={query}
          onChange={(event) => onQueryChange(event.target.value)}
          placeholder="chainId, host, root cause"
          aria-label="Incident query"
        />
      </div>
      <InlineSelect
        ariaLabel="Incident severity"
        label="Severity"
        value={severity}
        options={[
          { value: "all", label: "All" },
          { value: "critical", label: "Critical" },
          { value: "high", label: "High" },
          { value: "medium", label: "Medium" },
        ]}
        onChange={(value) => onSeverityChange(value as IncidentRecord["severity"] | "all")}
      />
      <InlineSelect
        ariaLabel="Incident status"
        label="Status"
        value={status}
        options={[
          { value: "all", label: "All" },
          { value: "active", label: "Active" },
          { value: "triage", label: "Triage" },
          { value: "contained", label: "Contained" },
        ]}
        onChange={(value) => onStatusChange(value as IncidentRecord["status"] | "all")}
      />
    </div>
  );
}

function InlineSelect({
  ariaLabel,
  label,
  value,
  options,
  onChange,
}: {
  ariaLabel: string;
  label: string;
  value: string;
  options: Array<{ value: string; label: string }>;
  onChange: (value: string) => void;
}) {
  return (
    <label className="flex w-[220px] min-w-0 items-center gap-2 border-l bg-bg px-3 max-lg:w-full max-lg:rounded-lg max-lg:border">
      <span className="shrink-0 text-xs font-medium text-muted-fg">{label}</span>
      <select
        aria-label={ariaLabel}
        className="h-11 min-w-0 flex-1 bg-transparent text-sm font-medium outline-none"
        value={value}
        onChange={(event) => onChange(event.target.value)}
      >
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
    </label>
  );
}

function IncidentChart({
  rows,
  buckets,
}: {
  rows: IncidentRecord[];
  buckets: IncidentSeverityBucket[];
}) {
  const maxCount = Math.max(...buckets.map((bucket) => bucket.count), 1);
  const totalAlerts = rows.reduce((sum, incident) => sum + incident.alertCount, 0);
  const hostCount = new Set(rows.flatMap((incident) => incident.hosts)).size;

  return (
    <div className="grid gap-3 lg:grid-cols-[260px_260px_minmax(0,1fr)]">
      <MetricTile label="Incidents" value={String(rows.length)} />
      <MetricTile label="Alerts" value={String(totalAlerts)} detail={`${hostCount} hosts`} />
      <div className="rounded-lg border bg-bg p-3">
        <div className="mb-3 flex items-center justify-between">
          <div className="text-sm font-semibold">Severity distribution</div>
          <Badge className="bg-bg text-muted-fg">filtered</Badge>
        </div>
        <div className="flex h-16 items-end gap-2">
          {buckets.map((bucket) => (
            <div key={bucket.severity} className="flex min-w-0 flex-1 flex-col items-center gap-1">
              <div
                aria-label={`${bucket.severity}: ${bucket.count}`}
                className={cn(
                  "w-full rounded-t",
                  bucket.severity === "critical" && "bg-destructive/75",
                  bucket.severity === "high" && "bg-warning/75",
                  bucket.severity === "medium" && "bg-success/75",
                )}
                style={{ height: `${Math.max((bucket.count / maxCount) * 48, bucket.count ? 6 : 2)}px` }}
              />
              <span className="max-w-full truncate text-[10px] uppercase text-muted-fg">
                {bucket.severity}
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function MetricTile({
  label,
  value,
  detail,
}: {
  label: string;
  value: string;
  detail?: string;
}) {
  return (
    <div className="rounded-lg border bg-bg p-3">
      <div className="text-xs font-semibold uppercase tracking-wide text-muted-fg">{label}</div>
      <div className="mt-2 text-2xl font-semibold tabular-nums">{value}</div>
      {detail && <div className="mt-1 text-xs text-muted-fg">{detail}</div>}
    </div>
  );
}

function IncidentTable({ rows }: { rows: IncidentRecord[] }) {
  const [sorting, setSorting] = useState<SortingState>([{ id: "alertCount", desc: true }]);
  const parentRef = useRef<HTMLDivElement>(null);
  const columns = useMemo<ColumnDef<IncidentRecord>[]>(
    () => [
      {
        id: "chain",
        header: "攻击链",
        accessorFn: (incident) => incident.chainId,
        cell: ({ row }) => (
          <div>
            <div className="font-mono text-sm font-semibold">{row.original.chainId}</div>
            <div className="mt-1 text-xs text-muted-fg">{row.original.title}</div>
          </div>
        ),
      },
      {
        id: "killChain",
        header: "Kill Chain",
        enableSorting: false,
        cell: ({ row }) => (
          <KillChainDots
            detected={row.original.detectedStages}
            total={row.original.totalStages}
          />
        ),
      },
      {
        id: "actions",
        header: "操作",
        enableSorting: false,
        cell: ({ row }) => <IncidentActions incident={row.original} />,
      },
      {
        id: "hosts",
        header: "涉及主机",
        accessorFn: (incident) => incident.hosts.join(","),
        cell: ({ row }) => <HostChips hosts={row.original.hosts} />,
      },
      {
        accessorKey: "alertCount",
        header: "告警数",
        cell: ({ row }) => (
          <span className="font-semibold tabular-nums">{row.original.alertCount}</span>
        ),
      },
      {
        accessorKey: "severity",
        header: "严重度",
        cell: ({ row }) => (
          <Badge className={cn("rounded-full px-3", severityClass[row.original.severity])}>
            {row.original.severity}
          </Badge>
        ),
      },
    ],
    [],
  );
  // TanStack Table intentionally returns method-heavy state objects; keep it outside React Compiler memoization.
  // eslint-disable-next-line react-hooks/incompatible-library
  const table = useReactTable({
    data: rows,
    columns,
    state: { sorting },
    onSortingChange: setSorting,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getRowId: (row) => row.id,
  });

  return (
    <div className="min-h-0 flex-1 overflow-hidden bg-bg">
      <Table
        containerClassName="h-full overflow-auto"
        containerRef={parentRef}
        className="min-w-[1040px]"
      >
        <TableHeader className="sticky top-0 z-10 bg-muted/10">
          {table.getHeaderGroups().map((headerGroup) => (
            <TableRow key={headerGroup.id}>
              {headerGroup.headers.map((header) => (
                <TableHead key={header.id}>
                  {header.isPlaceholder ? null : (
                    <button
                      className={cn(
                        "flex items-center gap-1 text-left",
                        header.column.getCanSort() && "cursor-pointer hover:text-fg",
                      )}
                      type="button"
                      onClick={header.column.getToggleSortingHandler()}
                    >
                      {flexRender(header.column.columnDef.header, header.getContext())}
                      {header.column.getIsSorted() === "asc" && <span>↑</span>}
                      {header.column.getIsSorted() === "desc" && <span>↓</span>}
                    </button>
                  )}
                </TableHead>
              ))}
            </TableRow>
          ))}
        </TableHeader>
        <TableBody>
          {table.getRowModel().rows.map((row) => (
            <IncidentRow
              key={row.id}
              cells={row.getVisibleCells()}
            />
          ))}
        </TableBody>
      </Table>
    </div>
  );
}

function IncidentRow({ cells }: { cells: Cell<IncidentRecord, unknown>[] }) {
  return (
    <TableRow className="hover:bg-muted/40">
      {cells.map((cell) => (
        <TableCell key={cell.id}>
          {flexRender(cell.column.columnDef.cell, cell.getContext())}
        </TableCell>
      ))}
    </TableRow>
  );
}

function IncidentActions({ incident }: { incident: IncidentRecord }) {
  return (
    <div className="flex items-center gap-2">
      <Button aria-label={`查看 ${incident.chainId} 攻击链`} intent="plain" size="xs">
        <GitBranchIcon />
        攻击链
      </Button>
      <Button aria-label={`查看 ${incident.chainId} 溯源图`} intent="plain" size="xs">
        <NetworkIcon />
        溯源图
      </Button>
    </div>
  );
}

function HostChips({ hosts }: { hosts: string[] }) {
  return (
    <div className="flex flex-wrap gap-2">
      {hosts.map((host) => (
        <span key={host} className="rounded border bg-bg px-2 py-1 font-mono text-xs text-muted-fg">
          {host}
        </span>
      ))}
    </div>
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
