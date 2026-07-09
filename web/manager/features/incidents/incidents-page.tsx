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

import {
  ClockIcon,
  GitBranchIcon,
  NetworkIcon,
  RefreshCwIcon,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { FieldMetadataPopover } from "@/components/field-metadata-popover";
import { Input } from "@/components/ui/input";
import { QueryAutocompleteInput } from "@/components/query-autocomplete-input";
import {
  IncidentDetailView,
  type IncidentDetailMode,
} from "@/features/incidents/incident-detail-view";
import {
  SearchToolbar,
  searchToolbarInlineSelectClass,
  searchToolbarPlainButtonClass,
  searchToolbarRunButtonClass,
} from "@/components/search-toolbar";
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
import { getIncidentDetail } from "@/lib/incident-detail-data";
import type { SearchField } from "@/lib/opensearch-fields";
import { getSearchFieldsForIndexPattern } from "@/lib/opensearch-fields";
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
  const [detailSelection, setDetailSelection] = useState<{
    incident: IncidentRecord;
    mode: IncidentDetailMode;
  } | null>(null);
  const searchFields = useMemo(() => getSearchFieldsForIndexPattern(indexPattern), [indexPattern]);
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
  const selectedDetail = detailSelection ? getIncidentDetail(detailSelection.incident.id) : undefined;

  if (detailSelection && selectedDetail) {
    return (
      <IncidentDetailView
        incident={detailSelection.incident}
        detail={selectedDetail}
        mode={detailSelection.mode}
        onBack={() => setDetailSelection(null)}
      />
    );
  }

  return (
    <section className="flex h-full min-h-0 flex-col overflow-hidden bg-bg">
      <IncidentWorkbench
        indexPattern={indexPattern}
        query={query}
        timeRange={timeRange}
        refresh={refresh}
        severity={severity}
        status={status}
        searchFields={searchFields}
        rows={filteredIncidents}
        histogram={histogram}
        onOpenDetail={(incident, mode) => setDetailSelection({ incident, mode })}
        onIndexPatternChange={(value) => setIndexPattern(value as IncidentIndexPattern)}
        onQueryChange={(value) =>
          applyIncidentQueryInput(value, {
            setQuery,
            setSeverity,
            setStatus,
          })
        }
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
  searchFields,
  rows,
  histogram,
  onIndexPatternChange,
  onQueryChange,
  onRefreshChange,
  onTimeRangeChange,
  onSeverityChange,
  onStatusChange,
  onOpenDetail,
}: {
  indexPattern: IncidentIndexPattern;
  query: string;
  timeRange: IncidentTimeRange;
  refresh: { paused: boolean; intervalSeconds: number };
  severity: IncidentRecord["severity"] | "all";
  status: IncidentRecord["status"] | "all";
  searchFields: SearchField[];
  rows: IncidentRecord[];
  histogram: IncidentHistogramBucket[];
  onIndexPatternChange: (value: string) => void;
  onQueryChange: (value: string) => void;
  onRefreshChange: (value: { paused: boolean; intervalSeconds: number }) => void;
  onTimeRangeChange: (value: IncidentTimeRange) => void;
  onSeverityChange: (value: IncidentRecord["severity"] | "all") => void;
  onStatusChange: (value: IncidentRecord["status"] | "all") => void;
  onOpenDetail: (incident: IncidentRecord, mode: IncidentDetailMode) => void;
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
          searchFields={searchFields}
          onIndexPatternChange={onIndexPatternChange}
          onQueryChange={onQueryChange}
          onRefreshChange={onRefreshChange}
          onTimeRangeChange={onTimeRangeChange}
          onSeverityChange={onSeverityChange}
          onStatusChange={onStatusChange}
        />
        <IncidentHistogram buckets={histogram} />
      </div>
      <IncidentTable rows={rows} onOpenDetail={onOpenDetail} />
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
  searchFields,
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
  searchFields: SearchField[];
  onIndexPatternChange: (value: string) => void;
  onQueryChange: (value: string) => void;
  onRefreshChange: (value: { paused: boolean; intervalSeconds: number }) => void;
  onTimeRangeChange: (value: IncidentTimeRange) => void;
  onSeverityChange: (value: IncidentRecord["severity"] | "all") => void;
  onStatusChange: (value: IncidentRecord["status"] | "all") => void;
}) {
  return (
    <SearchToolbar
      controls={
        <>
          <InlineSelect
            ariaLabel="Incident index pattern"
            label="Index"
            value={indexPattern}
            options={incidentIndexOptions.map((option) => ({ value: option, label: option }))}
            onChange={onIndexPatternChange}
          />
          <IncidentQueryInput
            query={query}
            severity={severity}
            status={status}
            searchFields={searchFields}
            onQueryChange={onQueryChange}
            onSeverityChange={onSeverityChange}
            onStatusChange={onStatusChange}
          />
          <SuperDatePicker
            value={timeRange}
            refresh={refresh}
            onChange={onTimeRangeChange}
            onRefreshChange={onRefreshChange}
          />
          <Button className={searchToolbarRunButtonClass} size="md">
            <RefreshCwIcon />
            Run
          </Button>
        </>
      }
      meta={
        <>
          <Badge>{count} hits</Badge>
          <FieldMetadataPopover fields={searchFields} />
        </>
      }
    />
  );
}

function IncidentQueryInput({
  query,
  severity,
  status,
  searchFields,
  onQueryChange,
  onSeverityChange,
  onStatusChange,
}: {
  query: string;
  severity: IncidentRecord["severity"] | "all";
  status: IncidentRecord["status"] | "all";
  searchFields: SearchField[];
  onQueryChange: (value: string) => void;
  onSeverityChange: (value: IncidentRecord["severity"] | "all") => void;
  onStatusChange: (value: IncidentRecord["status"] | "all") => void;
}) {
  const tokens: Array<{ key: string; value: string; onRemove: () => void }> = [];

  if (severity !== "all") {
    tokens.push({ key: "severity", value: severity, onRemove: () => onSeverityChange("all") });
  }
  if (status !== "all") {
    tokens.push({ key: "status", value: status, onRemove: () => onStatusChange("all") });
  }

  return (
    <QueryAutocompleteInput
      query={query}
      fields={searchFields}
      tokens={tokens}
      placeholder="incident.chain_id: threat-chain-001 or severity:critical"
      ariaLabel="Incident index query"
      onQueryChange={onQueryChange}
    />
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
        className={searchToolbarPlainButtonClass}
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
    <label className={searchToolbarInlineSelectClass}>
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

function IncidentTable({
  rows,
  onOpenDetail,
}: {
  rows: IncidentRecord[];
  onOpenDetail: (incident: IncidentRecord, mode: IncidentDetailMode) => void;
}) {
  const [sorting, setSorting] = useState<SortingState>([{ id: "alertCount", desc: true }]);
  const parentRef = useRef<HTMLDivElement>(null);
  const columns = useMemo<ColumnDef<IncidentRecord>[]>(
    () => [
      {
        id: "chain",
        header: "Attack Chain",
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
        header: "Actions",
        enableSorting: false,
        cell: ({ row }) => <IncidentActions incident={row.original} onOpenDetail={onOpenDetail} />,
      },
      {
        id: "hosts",
        header: "Hosts",
        accessorFn: (incident) => incident.hosts.join(","),
        cell: ({ row }) => <HostChips hosts={row.original.hosts} />,
      },
      {
        accessorKey: "alertCount",
        header: "Alerts",
        cell: ({ row }) => (
          <span className="font-semibold tabular-nums">{row.original.alertCount}</span>
        ),
      },
      {
        accessorKey: "severity",
        header: "Severity",
        cell: ({ row }) => (
          <Badge className={cn("rounded-full px-3", severityClass[row.original.severity])}>
            {row.original.severity}
          </Badge>
        ),
      },
    ],
    [onOpenDetail],
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

function IncidentActions({
  incident,
  onOpenDetail,
}: {
  incident: IncidentRecord;
  onOpenDetail: (incident: IncidentRecord, mode: IncidentDetailMode) => void;
}) {
  return (
    <div className="flex items-center gap-2">
      <Button
        aria-label={`View ${incident.chainId} attack chain`}
        intent="plain"
        size="xs"
        onPress={() => onOpenDetail(incident, "attack-chain")}
      >
        <GitBranchIcon />
        Attack Chain
      </Button>
      <Button
        aria-label={`View ${incident.chainId} provenance graph`}
        intent="plain"
        size="xs"
        onPress={() => onOpenDetail(incident, "provenance")}
      >
        <NetworkIcon />
        Provenance
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

function applyIncidentQueryInput(
  value: string,
  setters: {
    setQuery: (value: string) => void;
    setSeverity: (value: IncidentRecord["severity"] | "all") => void;
    setStatus: (value: IncidentRecord["status"] | "all") => void;
  },
) {
  const tokens = value.split(/\s+/).filter(Boolean);
  const freeText: string[] = [];

  for (const token of tokens) {
    const [key, rawValue] = token.split(":");
    const normalizedKey = key?.toLowerCase();
    const normalizedValue = rawValue?.toLowerCase();

    if (normalizedKey === "severity" && isIncidentSeverity(normalizedValue)) {
      setters.setSeverity(normalizedValue);
      continue;
    }

    if (normalizedKey === "status" && isIncidentStatus(normalizedValue)) {
      setters.setStatus(normalizedValue);
      continue;
    }

    if (isIncidentFreeTextField(normalizedKey) && rawValue) {
      freeText.push(rawValue);
      continue;
    }

    freeText.push(token);
  }

  setters.setQuery(freeText.join(" "));
}

function isIncidentSeverity(value: string | undefined): value is IncidentRecord["severity"] {
  return value === "critical" || value === "high" || value === "medium";
}

function isIncidentStatus(value: string | undefined): value is IncidentRecord["status"] {
  return value === "active" || value === "triage" || value === "contained";
}

function isIncidentFreeTextField(value: string | undefined) {
  return value === "incident.chain_id" || value === "host.name" || value === "root_cause";
}
