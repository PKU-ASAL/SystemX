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

import { ClockIcon, GitBranchIcon, NetworkIcon, SearchIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Popover, PopoverContent } from "@/components/ui/popover";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  buildIncidentHistogram,
  filterIncidents,
  incidents,
  type IncidentHistogramBucket,
  type IncidentRecord,
} from "@/lib/mock-data";
import { cn } from "@/lib/utils";

type IncidentIndexPattern = "incidents-*" | "incident-*";
type IncidentTimeRange =
  | { mode: "quick"; label: string; minutes: number }
  | { mode: "absolute"; label: string; start: string; end: string };

const incidentIndexOptions: IncidentIndexPattern[] = ["incidents-*", "incident-*"];
const quickTimeRanges: Array<Extract<IncidentTimeRange, { mode: "quick" }>> = [
  { mode: "quick", label: "Last 15 minutes", minutes: 15 },
  { mode: "quick", label: "Last 30 minutes", minutes: 30 },
  { mode: "quick", label: "Last 1 hour", minutes: 60 },
  { mode: "quick", label: "Last 24 hours", minutes: 1440 },
];
const referenceNow = new Date("2026-07-08T21:10:00").getTime();

const severityClass = {
  critical: "border-destructive/40 bg-destructive/10 text-destructive",
  high: "text-status-high",
  medium: "text-status-medium",
} as const;

export function IncidentsPage() {
  const [indexPattern, setIndexPattern] = useState<IncidentIndexPattern>("incidents-*");
  const [query, setQuery] = useState("");
  const [timeRange, setTimeRange] = useState<IncidentTimeRange>(quickTimeRanges[1]);
  const [refresh, setRefresh] = useState({ paused: true, intervalSeconds: 10 });
  const [severity, setSeverity] = useState<IncidentRecord["severity"] | "all">("all");
  const [status, setStatus] = useState<IncidentRecord["status"] | "all">("all");
  const timeFilter = useMemo(() => resolveTimeFilter(timeRange), [timeRange]);
  const filteredIncidents = useMemo(
    () => filterIncidents(incidents, { query, severity, status, ...timeFilter, now: referenceNow }),
    [query, severity, status, timeFilter],
  );
  const histogram = useMemo(
    () =>
      buildIncidentHistogram(incidents, {
        query,
        severity,
        status,
        ...timeFilter,
        now: referenceNow,
        bucketCount: 12,
      }),
    [query, severity, status, timeFilter],
  );

  return (
    <section className="flex h-full min-h-0 flex-col overflow-hidden bg-bg">
      <IncidentWorkbench
        indexPattern={indexPattern}
        query={query}
        timeRange={timeRange}
        refresh={refresh}
        severity={severity}
        status={status}
        rows={filteredIncidents}
        histogram={histogram}
        onIndexPatternChange={(value) => setIndexPattern(value as IncidentIndexPattern)}
        onQueryChange={setQuery}
        onRefreshChange={setRefresh}
        onTimeRangeChange={setTimeRange}
        onSeverityChange={setSeverity}
        onStatusChange={setStatus}
      />
    </section>
  );
}

function IncidentWorkbench({
  indexPattern,
  query,
  timeRange,
  refresh,
  severity,
  status,
  rows,
  histogram,
  onIndexPatternChange,
  onQueryChange,
  onRefreshChange,
  onTimeRangeChange,
  onSeverityChange,
  onStatusChange,
}: {
  indexPattern: IncidentIndexPattern;
  query: string;
  timeRange: IncidentTimeRange;
  refresh: { paused: boolean; intervalSeconds: number };
  severity: IncidentRecord["severity"] | "all";
  status: IncidentRecord["status"] | "all";
  rows: IncidentRecord[];
  histogram: IncidentHistogramBucket[];
  onIndexPatternChange: (value: string) => void;
  onQueryChange: (value: string) => void;
  onRefreshChange: (value: { paused: boolean; intervalSeconds: number }) => void;
  onTimeRangeChange: (value: IncidentTimeRange) => void;
  onSeverityChange: (value: IncidentRecord["severity"] | "all") => void;
  onStatusChange: (value: IncidentRecord["status"] | "all") => void;
}) {
  return (
    <>
      <div className="flex shrink-0 flex-col gap-3 border-b bg-muted/10 p-4">
        <IncidentFilterBar
          count={rows.length}
          indexPattern={indexPattern}
          query={query}
          timeRange={timeRange}
          refresh={refresh}
          severity={severity}
          status={status}
          onIndexPatternChange={onIndexPatternChange}
          onQueryChange={onQueryChange}
          onRefreshChange={onRefreshChange}
          onTimeRangeChange={onTimeRangeChange}
          onSeverityChange={onSeverityChange}
          onStatusChange={onStatusChange}
        />
        <IncidentHistogram buckets={histogram} />
      </div>
      <IncidentTable rows={rows} />
    </>
  );
}

function IncidentFilterBar({
  count,
  indexPattern,
  query,
  timeRange,
  refresh,
  severity,
  status,
  onIndexPatternChange,
  onQueryChange,
  onRefreshChange,
  onTimeRangeChange,
  onSeverityChange,
  onStatusChange,
}: {
  count: number;
  indexPattern: IncidentIndexPattern;
  query: string;
  timeRange: IncidentTimeRange;
  refresh: { paused: boolean; intervalSeconds: number };
  severity: IncidentRecord["severity"] | "all";
  status: IncidentRecord["status"] | "all";
  onIndexPatternChange: (value: string) => void;
  onQueryChange: (value: string) => void;
  onRefreshChange: (value: { paused: boolean; intervalSeconds: number }) => void;
  onTimeRangeChange: (value: IncidentTimeRange) => void;
  onSeverityChange: (value: IncidentRecord["severity"] | "all") => void;
  onStatusChange: (value: IncidentRecord["status"] | "all") => void;
}) {
  return (
    <div className="flex flex-col gap-3">
      <div className="flex min-h-11 items-stretch overflow-hidden rounded-lg border bg-bg max-lg:flex-wrap max-lg:overflow-visible max-lg:border-0 max-lg:bg-transparent">
        <InlineSelect
          ariaLabel="Incident index pattern"
          label="Index"
          value={indexPattern}
          options={incidentIndexOptions.map((option) => ({ value: option, label: option }))}
          onChange={onIndexPatternChange}
        />
        <div className="relative min-w-72 flex-1 max-lg:min-w-full max-lg:rounded-lg max-lg:border">
          <SearchIcon className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-fg" />
          <Input
            className="h-11 rounded-none border-0 bg-transparent pl-9 font-mono focus-visible:ring-0 max-lg:rounded-lg"
            value={query}
            onChange={(event) => onQueryChange(event.target.value)}
            placeholder="incident.chain_id: threat-chain-001 or host.name: oa-web"
            aria-label="Incident index query"
          />
        </div>
        <Button className="h-11 rounded-none border-0 px-4 max-lg:w-full max-lg:rounded-lg" size="md">
          Run
        </Button>
        <SuperDatePicker
          value={timeRange}
          refresh={refresh}
          onChange={onTimeRangeChange}
          onRefreshChange={onRefreshChange}
        />
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <Badge>{count} hits</Badge>
        <CompactSelect
          ariaLabel="Incident severity"
          label="severity"
          value={severity}
          options={[
            { value: "all", label: "all" },
            { value: "critical", label: "critical" },
            { value: "high", label: "high" },
            { value: "medium", label: "medium" },
          ]}
          onChange={(value) => onSeverityChange(value as IncidentRecord["severity"] | "all")}
        />
        <CompactSelect
          ariaLabel="Incident status"
          label="status"
          value={status}
          options={[
            { value: "all", label: "all" },
            { value: "active", label: "active" },
            { value: "triage", label: "triage" },
            { value: "contained", label: "contained" },
          ]}
          onChange={(value) => onStatusChange(value as IncidentRecord["status"] | "all")}
        />
        <Badge className="bg-bg text-muted-fg">incident.chain_id</Badge>
        <Badge className="bg-bg text-muted-fg">host.name</Badge>
        <Badge className="bg-bg text-muted-fg">incident.status</Badge>
      </div>
    </div>
  );
}

function SuperDatePicker({
  value,
  refresh,
  onChange,
  onRefreshChange,
}: {
  value: IncidentTimeRange;
  refresh: { paused: boolean; intervalSeconds: number };
  onChange: (value: IncidentTimeRange) => void;
  onRefreshChange: (value: { paused: boolean; intervalSeconds: number }) => void;
}) {
  const absoluteValue =
    value.mode === "absolute"
      ? value
      : {
          mode: "absolute" as const,
          label: "Absolute",
          start: "2026-07-08T20:40",
          end: "2026-07-08T21:10",
        };
  const [draft, setDraft] = useState(absoluteValue);

  function applyAbsoluteRange() {
    onChange({
      ...draft,
      label: `${formatDateTimeLabel(draft.start)} - ${formatDateTimeLabel(draft.end)}`,
    });
  }

  return (
    <Popover>
      <Button
        className="h-11 rounded-none border-0 border-l px-3 max-lg:w-full max-lg:rounded-lg max-lg:border"
        intent="plain"
      >
        <ClockIcon />
        <span className="min-w-0 truncate">{value.label}</span>
      </Button>
      <PopoverContent className="max-w-none [--trigger-width:22rem]" placement="bottom end">
        <div className="grid w-[560px] grid-cols-[220px_minmax(0,1fr)] gap-0 max-sm:w-[calc(100vw-2rem)] max-sm:grid-cols-1">
          <div className="border-r p-3 max-sm:border-r-0 max-sm:border-b">
            <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-fg">
              Quick select
            </div>
            <div className="flex flex-col gap-1">
              {quickTimeRanges.map((range) => (
                <Button
                  key={range.label}
                  className="justify-start"
                  intent={value.mode === "quick" && value.minutes === range.minutes ? "primary" : "plain"}
                  size="sm"
                  onPress={() => onChange(range)}
                >
                  {range.label}
                </Button>
              ))}
            </div>
            <div className="mt-4 rounded-lg border bg-muted/20 p-3">
              <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-fg">
                Refresh every
              </div>
              <div className="flex items-center gap-2">
                <Button
                  intent={refresh.paused ? "primary" : "outline"}
                  size="sm"
                  onPress={() => onRefreshChange({ ...refresh, paused: !refresh.paused })}
                >
                  {refresh.paused ? "Paused" : "Live"}
                </Button>
                <select
                  aria-label="Refresh interval"
                  className="h-9 min-w-0 flex-1 rounded-md border bg-bg px-2 text-sm outline-none"
                  value={refresh.intervalSeconds}
                  onChange={(event) =>
                    onRefreshChange({ ...refresh, intervalSeconds: Number(event.target.value) })
                  }
                >
                  <option value={5}>5s</option>
                  <option value={10}>10s</option>
                  <option value={30}>30s</option>
                  <option value={60}>1m</option>
                </select>
              </div>
            </div>
          </div>
          <div className="p-3">
            <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-fg">
              Absolute range
            </div>
            <div className="flex flex-col gap-3">
              <label className="flex flex-col gap-1 text-sm">
                Start date
                <Input
                  type="datetime-local"
                  value={draft.start}
                  onChange={(event) => setDraft((current) => ({ ...current, start: event.target.value }))}
                />
              </label>
              <label className="flex flex-col gap-1 text-sm">
                End date
                <Input
                  type="datetime-local"
                  value={draft.end}
                  onChange={(event) => setDraft((current) => ({ ...current, end: event.target.value }))}
                />
              </label>
              <Button className="self-start" size="sm" onPress={applyAbsoluteRange}>
                Apply time range
              </Button>
            </div>
          </div>
        </div>
      </PopoverContent>
    </Popover>
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

function CompactSelect({
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
    <label className="inline-flex h-7 items-center gap-1 rounded-md border bg-bg px-2 font-mono text-xs text-muted-fg">
      <span>{label}:</span>
      <select
        aria-label={ariaLabel}
        className="bg-transparent font-mono text-xs text-fg outline-none"
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

function IncidentHistogram({
  buckets,
}: {
  buckets: IncidentHistogramBucket[];
}) {
  const maxCount = Math.max(...buckets.map((bucket) => bucket.total), 1);
  const total = buckets.reduce((sum, bucket) => sum + bucket.total, 0);

  return (
    <div className="rounded-lg border bg-bg p-3">
      <div className="mb-2 flex flex-wrap items-center justify-between gap-3">
        <div className="text-lg font-semibold">{total.toLocaleString()} hits</div>
        <div className="flex flex-wrap items-center gap-2">
          <LegendSwatch className="bg-destructive/75" label="critical" />
          <LegendSwatch className="bg-warning/75" label="high" />
          <LegendSwatch className="bg-success/75" label="medium" />
          <Badge className="bg-bg text-muted-fg">@timestamp per bucket</Badge>
        </div>
      </div>
      <div className="grid h-28 grid-cols-[32px_1fr] gap-2">
        <div className="flex flex-col justify-between text-right font-mono text-[10px] text-muted-fg">
          <span>{maxCount}</span>
          <span>{Math.floor(maxCount / 2)}</span>
          <span>0</span>
        </div>
        <div className="flex items-end gap-1 border-l border-b px-2 pb-1">
          {buckets.map((bucket) => (
            <div key={bucket.start} className="flex min-w-0 flex-1 flex-col items-center gap-1">
              <div className="flex w-full flex-col justify-end overflow-hidden rounded-t bg-muted/40">
                <StackedBarSegment
                  count={bucket.critical}
                  maxCount={maxCount}
                  className="bg-destructive/75"
                />
                <StackedBarSegment count={bucket.high} maxCount={maxCount} className="bg-warning/75" />
                <StackedBarSegment count={bucket.medium} maxCount={maxCount} className="bg-success/75" />
                {!bucket.total && <div className="h-px w-full bg-muted-fg/20" />}
              </div>
              <span className="max-w-full truncate font-mono text-[9px] text-muted-fg">
                {bucket.label}
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function StackedBarSegment({
  count,
  maxCount,
  className,
}: {
  count: number;
  maxCount: number;
  className: string;
}) {
  if (!count) return null;

  return (
    <div
      className={cn("w-full", className)}
      style={{ height: `${Math.max((count / maxCount) * 78, 4)}px` }}
    />
  );
}

function LegendSwatch({
  label,
  className,
}: {
  label: string;
  className: string;
}) {
  return (
    <span className="inline-flex items-center gap-1 text-xs text-muted-fg">
      <span className={cn("size-2 rounded-sm", className)} />
      {label}
    </span>
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

function resolveTimeFilter(range: IncidentTimeRange) {
  if (range.mode === "quick") {
    return { minutes: range.minutes };
  }

  return {
    startTime: new Date(range.start).getTime(),
    endTime: new Date(range.end).getTime(),
  };
}

function formatDateTimeLabel(value: string) {
  return value.replace("T", " ");
}
