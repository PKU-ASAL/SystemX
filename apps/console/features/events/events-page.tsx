"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import {
  flexRender,
  getCoreRowModel,
  getSortedRowModel,
  useReactTable,
  type Cell,
  type ColumnDef,
  type SortingState,
} from "@tanstack/react-table";
import { useVirtualizer } from "@tanstack/react-virtual";
import {
  ChevronRightIcon,
  ClockIcon,
  RefreshCwIcon,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { FieldMetadataPopover } from "@/components/field-metadata-popover";
import { Input } from "@/components/ui/input";
import { QueryAutocompleteInput } from "@/components/query-autocomplete-input";
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
  type EventDiscoverRow,
  type EventHistogramBucket,
  type SecurityEventIndex,
} from "@/lib/mock-data";
import { createDefaultManagerApiClient, getManagerDataSource, type ManagerTimeRange } from "@/lib/api";
import {
  loadEventDiscoverData,
  mapSearchFieldsToUiFields,
} from "@/lib/events-data";
import { getSearchFieldsForIndexPattern, type SearchField } from "@/lib/opensearch-fields";
import { cn } from "@/lib/utils";

type DiscoverIndexPattern = "events-*,signals-*" | "events-*" | "signals-*";
type DiscoverTimeRange =
  | { mode: "quick"; label: string; minutes: number }
  | { mode: "absolute"; label: string; start: string; end: string };

const indexOptions: DiscoverIndexPattern[] = ["events-*,signals-*", "events-*", "signals-*"];
const quickTimeRanges: Array<Extract<DiscoverTimeRange, { mode: "quick" }>> = [
  { mode: "quick", label: "Last 15 minutes", minutes: 15 },
  { mode: "quick", label: "Last 30 minutes", minutes: 30 },
  { mode: "quick", label: "Last 1 hour", minutes: 60 },
  { mode: "quick", label: "Last 24 hours", minutes: 1440 },
];
const referenceNow = new Date("2026-07-08T21:10:00").getTime();
const managerApiClient = createDefaultManagerApiClient();
const managerDataSource = getManagerDataSource();

export function EventsPage() {
  const [indexPattern, setIndexPattern] = useState<DiscoverIndexPattern>("events-*,signals-*");
  const [query, setQuery] = useState("credential");
  const [timeRange, setTimeRange] = useState<DiscoverTimeRange>(quickTimeRanges[1]);
  const [refresh, setRefresh] = useState({ paused: true, intervalSeconds: 10 });
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  const [rows, setRows] = useState<EventDiscoverRow[]>([]);
  const [histogram, setHistogram] = useState<EventHistogramBucket[]>([]);
  const [apiFields, setApiFields] = useState<SearchField[] | null>(null);
  const [totalHits, setTotalHits] = useState(0);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);
  const selectedIndexes = useMemo(() => resolveIndexPattern(indexPattern), [indexPattern]);
  const fallbackFields = useMemo(() => getSearchFieldsForIndexPattern(indexPattern), [indexPattern]);
  const searchFields = apiFields ?? fallbackFields;

  useEffect(() => {
    const controller = new AbortController();

    loadEventDiscoverData({
      client: managerApiClient,
      dataSource: managerDataSource,
      indexPattern,
      indexes: selectedIndexes,
      query,
      time: resolveManagerTimeRange(timeRange, managerDataSource),
      bucketCount: 12,
      signal: controller.signal,
    })
      .then((data) => {
        setRows(data.rows);
        setHistogram(data.histogram);
        setApiFields(mapSearchFieldsToUiFields(data.fields));
        setTotalHits(data.total);
      })
      .catch((nextError: unknown) => {
        if (controller.signal.aborted) return;
        setRows([]);
        setHistogram([]);
        setTotalHits(0);
        setError(nextError instanceof Error ? nextError.message : "Failed to load events");
      })
      .finally(() => {
        if (!controller.signal.aborted) {
          setIsLoading(false);
        }
      });

    return () => controller.abort();
  }, [indexPattern, query, selectedIndexes, timeRange, reloadKey]);

  function beginEventLoad() {
    setIsLoading(true);
    setError(null);
  }

  function changeIndexPattern(value: string) {
    beginEventLoad();
    setIndexPattern(value as DiscoverIndexPattern);
  }

  function changeQuery(value: string) {
    beginEventLoad();
    setQuery(value);
  }

  function changeTimeRange(value: DiscoverTimeRange) {
    beginEventLoad();
    setTimeRange(value);
  }

  function reloadEvents() {
    beginEventLoad();
    setReloadKey((value) => value + 1);
  }

  return (
    <section className="flex h-full min-h-0 flex-col overflow-hidden bg-bg">
      <div className="flex shrink-0 flex-col gap-3 border-b bg-muted/10 p-4">
        <DiscoverQueryBar
          count={totalHits}
          indexPattern={indexPattern}
          query={query}
          timeRange={timeRange}
          refresh={refresh}
          searchFields={searchFields}
          onIndexPatternChange={changeIndexPattern}
          onQueryChange={changeQuery}
          onRefreshChange={setRefresh}
          onTimeRangeChange={changeTimeRange}
          onRun={reloadEvents}
        />
        <DiscoverHistogram buckets={histogram} count={totalHits} />
      </div>
      <DiscoverTable
        rows={rows}
        isLoading={isLoading}
        error={error}
        expanded={expanded}
        onToggle={(id) => setExpanded((current) => ({ ...current, [id]: !current[id] }))}
      />
    </section>
  );
}

function DiscoverQueryBar({
  count,
  indexPattern,
  query,
  timeRange,
  refresh,
  searchFields,
  onIndexPatternChange,
  onQueryChange,
  onRefreshChange,
  onTimeRangeChange,
  onRun,
}: {
  count: number;
  indexPattern: DiscoverIndexPattern;
  query: string;
  timeRange: DiscoverTimeRange;
  refresh: { paused: boolean; intervalSeconds: number };
  searchFields: SearchField[];
  onIndexPatternChange: (value: string) => void;
  onQueryChange: (value: string) => void;
  onRefreshChange: (value: { paused: boolean; intervalSeconds: number }) => void;
  onTimeRangeChange: (value: DiscoverTimeRange) => void;
  onRun: () => void;
}) {
  return (
    <SearchToolbar
      controls={
        <>
          <InlineSelect
            ariaLabel="Index pattern"
            label="Index"
            value={indexPattern}
            options={indexOptions.map((option) => ({ value: option, label: option }))}
            onChange={onIndexPatternChange}
          />
          <QueryAutocompleteInput
            query={query}
            fields={searchFields}
            placeholder="agent.id: node-a and event.kind: signal"
            ariaLabel="KQL query"
            onQueryChange={onQueryChange}
          />
          <SuperDatePicker
            value={timeRange}
            refresh={refresh}
            onChange={onTimeRangeChange}
            onRefreshChange={onRefreshChange}
          />
          <Button className={searchToolbarRunButtonClass} size="md" onPress={onRun}>
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

function SuperDatePicker({
  value,
  refresh,
  onChange,
  onRefreshChange,
}: {
  value: DiscoverTimeRange;
  refresh: { paused: boolean; intervalSeconds: number };
  onChange: (value: DiscoverTimeRange) => void;
  onRefreshChange: (value: { paused: boolean; intervalSeconds: number }) => void;
}) {
  const absoluteValue = value.mode === "absolute" ? value : {
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
      <Button className={searchToolbarPlainButtonClass} intent="plain">
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

function DiscoverHistogram({
  buckets,
  count,
}: {
  buckets: EventHistogramBucket[];
  count: number;
}) {
  const maxCount = Math.max(...buckets.map((bucket) => bucket.count), 1);

  return (
    <div className="rounded-lg border bg-bg p-3">
      <div className="mb-2 flex flex-wrap items-center justify-between gap-3">
        <div className="text-lg font-semibold">{count.toLocaleString()} hits</div>
        <Badge className="bg-bg text-muted-fg">@timestamp per bucket</Badge>
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
              <div
                aria-label={`${bucket.label}: ${bucket.count} hits`}
                className="w-full rounded-t bg-primary/70"
                style={{
                  height: `${Math.max((bucket.count / maxCount) * 78, bucket.count ? 4 : 1)}px`,
                }}
              />
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

function DiscoverTable({
  rows,
  isLoading,
  error,
  expanded,
  onToggle,
}: {
  rows: EventDiscoverRow[];
  isLoading: boolean;
  error: string | null;
  expanded: Record<string, boolean>;
  onToggle: (id: string) => void;
}) {
  const [sorting, setSorting] = useState<SortingState>([{ id: "time", desc: true }]);
  const parentRef = useRef<HTMLDivElement>(null);
  const columns = useMemo<ColumnDef<EventDiscoverRow>[]>(
    () => [
      {
        id: "expander",
        header: "",
        size: 44,
        enableSorting: false,
        cell: ({ row }) => (
          <Button
            intent="plain"
            size="sq-xs"
            aria-label={`${expanded[row.original.id] ? "Collapse" : "Expand"} ${row.original.id}`}
            onPress={() => onToggle(row.original.id)}
          >
            <ChevronRightIcon
              className={cn("transition-transform", expanded[row.original.id] && "rotate-90")}
            />
          </Button>
        ),
      },
      {
        accessorKey: "time",
        header: "Time",
        size: 196,
        cell: ({ row }) => (
          <span className="font-mono text-xs text-muted-fg">{row.original.time}</span>
        ),
      },
      {
        id: "source",
        header: "_source",
        enableSorting: false,
        cell: ({ row }) => <SourceSummary row={row.original} />,
      },
    ],
    [expanded, onToggle],
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
  const tableRows = table.getRowModel().rows;
  const virtualizer = useVirtualizer({
    count: tableRows.length,
    getScrollElement: () => parentRef.current,
    estimateSize: (index) => (expanded[tableRows[index]?.original.id ?? ""] ? 250 : 54),
    overscan: 8,
  });
  const virtualRows = virtualizer.getVirtualItems();
  const paddingTop = virtualRows.length > 0 ? virtualRows[0].start : 0;
  const paddingBottom =
    virtualRows.length > 0
      ? virtualizer.getTotalSize() - virtualRows[virtualRows.length - 1].end
      : 0;

  return (
    <div className="min-h-0 flex-1 overflow-hidden bg-bg">
      <Table containerClassName="h-full overflow-auto" containerRef={parentRef}>
        <TableHeader className="sticky top-0 z-10 bg-muted/10">
          {table.getHeaderGroups().map((headerGroup) => (
            <TableRow key={headerGroup.id}>
              {headerGroup.headers.map((header) => (
                <TableHead
                  key={header.id}
                  className={cn(
                    header.column.id === "time" && "w-48 font-mono text-xs",
                    header.column.id === "expander" && "w-10",
                  )}
                >
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
          {paddingTop > 0 && (
            <TableRow>
              <TableCell colSpan={columns.length} style={{ height: `${paddingTop}px` }} />
            </TableRow>
          )}
          {isLoading || error || rows.length === 0 ? (
            <TableRow>
              <TableCell colSpan={columns.length} className="h-32 text-center text-sm text-muted-fg">
                {isLoading ? "Loading events..." : error ? error : "No events match the current query."}
              </TableCell>
            </TableRow>
          ) : (
            virtualRows.map((virtualRow) => {
              const row = tableRows[virtualRow.index];
              return (
                <ExpandableDocumentRow
                  key={row.id}
                  row={row.original}
                  cells={row.getVisibleCells()}
                  expanded={Boolean(expanded[row.original.id])}
                  onToggle={() => onToggle(row.original.id)}
                />
              );
            })
          )}
          {paddingBottom > 0 && (
            <TableRow>
              <TableCell colSpan={columns.length} style={{ height: `${paddingBottom}px` }} />
            </TableRow>
          )}
        </TableBody>
      </Table>
    </div>
  );
}

function ExpandableDocumentRow({
  row,
  cells,
  expanded,
  onToggle,
}: {
  row: EventDiscoverRow;
  cells: Cell<EventDiscoverRow, unknown>[];
  expanded: boolean;
  onToggle: () => void;
}) {
  return (
    <>
      <TableRow className="cursor-pointer" aria-expanded={expanded} onClick={onToggle}>
        {cells.map((cell) => (
          <TableCell key={cell.id}>
            {flexRender(cell.column.columnDef.cell, cell.getContext())}
          </TableCell>
        ))}
      </TableRow>
      {expanded && (
        <TableRow>
          <TableCell colSpan={3} className="bg-muted/30 p-0">
            <ExpandedDocument row={row} />
          </TableCell>
        </TableRow>
      )}
    </>
  );
}

function SourceSummary({ row }: { row: EventDiscoverRow }) {
  return (
    <div className="flex flex-wrap gap-1.5 py-1">
      {row.source.map((chip, index) => (
        <span
          key={`${chip.key}-${index}`}
          className="inline-flex min-w-0 items-center gap-1 rounded-md border bg-bg px-1.5 py-0.5 font-mono text-xs"
        >
          <span className="font-semibold text-muted-fg">{chip.key}:</span>
          <span className="max-w-80 truncate">{chip.value}</span>
        </span>
      ))}
    </div>
  );
}

function ExpandedDocument({ row }: { row: EventDiscoverRow }) {
  const raw = row.raw;
  return (
    <div className="grid gap-4 p-4 lg:grid-cols-[minmax(0,1fr)_420px]">
      <Card className="rounded-lg bg-bg">
        <CardContent className="flex flex-col gap-3 py-4">
          <div className="flex flex-wrap gap-2">
            <Badge>_id:{raw._id}</Badge>
            <Badge>host:{raw.host.name}</Badge>
            <Badge>{raw.event.kind}</Badge>
            <Badge>{raw.event.severity}</Badge>
          </div>
          <div>
            <div className="text-sm font-medium">{raw.event.tactic}</div>
            <p className="mt-1 text-sm text-muted-fg">{raw.event.summary}</p>
          </div>
        </CardContent>
      </Card>
      <pre className="max-h-80 overflow-auto rounded-lg border bg-bg p-4 font-mono text-xs text-muted-fg">
        {JSON.stringify(raw, null, 2)}
      </pre>
    </div>
  );
}

function resolveIndexPattern(pattern: DiscoverIndexPattern): SecurityEventIndex[] {
  if (pattern === "events-*") return ["sysarmor-events"];
  if (pattern === "signals-*") return ["sysarmor-signals"];
  return ["sysarmor-events", "sysarmor-signals"];
}

function resolveManagerTimeRange(range: DiscoverTimeRange, dataSource: "api" | "mock"): ManagerTimeRange {
  if (range.mode === "quick") {
    const now = dataSource === "mock" ? referenceNow : Date.now();

    return {
      field: "@timestamp",
      from: new Date(now - range.minutes * 60 * 1000).toISOString(),
      to: new Date(now).toISOString(),
    };
  }

  return {
    field: "@timestamp",
    from: new Date(range.start).toISOString(),
    to: new Date(range.end).toISOString(),
  };
}

function formatDateTimeLabel(value: string) {
  return value.replace("T", " ");
}
