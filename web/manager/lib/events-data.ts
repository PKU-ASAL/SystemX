import { getSearchFields, searchTelemetry, searchTelemetryHistogram } from "./api/search";
import type { ManagerApiClient } from "./api/client";
import type {
  ManagerTimeRange,
  SearchFieldCapability,
  TelemetryHistogramBucket,
  TelemetrySearchRow,
} from "./api/types";
import {
  buildEventHistogram,
  createDiscoverRows,
  events as mockEvents,
  filterEvents,
  type EventDiscoverRow,
  type EventHistogramBucket,
  type SecurityEvent,
  type SecurityEventIndex,
} from "./mock-data";
import { getSearchFieldsForIndexPattern, type SearchField } from "./opensearch-fields";

export type LoadEventDiscoverDataOptions = {
  client: ManagerApiClient;
  dataSource: "api" | "mock";
  indexPattern: string;
  indexes: SecurityEventIndex[];
  query: string;
  time: ManagerTimeRange;
  bucketCount: number;
  signal?: AbortSignal;
};

export type EventDiscoverData = {
  total: number;
  fields: SearchFieldCapability[];
  rows: EventDiscoverRow[];
  histogram: EventHistogramBucket[];
};

export async function loadEventDiscoverData({
  client,
  dataSource,
  indexPattern,
  indexes,
  query,
  time,
  bucketCount,
  signal,
}: LoadEventDiscoverDataOptions): Promise<EventDiscoverData> {
  if (dataSource === "mock") {
    return loadMockEventDiscoverData({ indexes, query, time, bucketCount });
  }

  const [fields, search, histogram] = await Promise.all([
    getSearchFields(client, indexPattern, { signal }),
    searchTelemetry(client, { indexes, query, time, limit: 500, offset: 0 }, { signal }),
    searchTelemetryHistogram(client, { indexes, query, time, bucket_count: bucketCount }, { signal }),
  ]);

  return {
    total: search.total,
    fields: fields.fields,
    rows: search.rows.map(mapTelemetryRowToDiscoverRow),
    histogram: histogram.buckets.map(mapTelemetryHistogramBucket),
  };
}

export function mapTelemetryRowToDiscoverRow(row: TelemetrySearchRow): EventDiscoverRow {
  const event: SecurityEvent = {
    id: row.id,
    timestamp: row.timestamp ?? "-",
    index: normalizeEventIndex(row.index),
    severity: normalizeSeverity(row.severity),
    host: row.host ?? "-",
    summary: row.summary ?? "-",
    tactic: row.tactic ?? "-",
  };
  const source = Object.entries(row.source ?? {
    "event.kind": event.index === "sysarmor-signals" ? "signal" : "event",
    "host.name": event.host,
    "event.summary": event.summary,
    "event.tactic": event.tactic,
    "event.severity": event.severity,
  }).map(([key, value]) => ({ key, value: String(value ?? "") }));

  return {
    id: row.id,
    time: event.timestamp,
    severity: event.severity,
    event,
    source,
    raw: {
      ...(isRecord(row.raw) ? row.raw : {}),
      _index: event.index,
      _id: row.id,
      "@timestamp": event.timestamp,
      host: { name: event.host },
      event: {
        kind: event.index === "sysarmor-signals" ? "signal" : "event",
        severity: event.severity,
        summary: event.summary,
        tactic: event.tactic,
      },
    },
  };
}

export function mapSearchFieldsToUiFields(fields: SearchFieldCapability[]): SearchField[] {
  return fields.map((field) => ({
    name: field.name,
    type: field.type,
    searchable: field.searchable,
    aggregatable: field.aggregatable,
    conflict: false,
  }));
}

function loadMockEventDiscoverData({
  indexes,
  query,
  time,
  bucketCount,
}: Pick<LoadEventDiscoverDataOptions, "indexes" | "query" | "time" | "bucketCount">): EventDiscoverData {
  const timeFilter = timeRangeToMockFilter(time);
  const histogram = buildEventHistogram(mockEvents, {
    query,
    indexes,
    ...timeFilter,
    bucketCount,
  });
  const filtered = filterEvents(mockEvents, { query, indexes, ...timeFilter });
  const rows = createDiscoverRows(filtered);

  return {
    total: rows.length,
    fields: getSearchFieldsForIndexPattern("events-*,signals-*"),
    rows,
    histogram,
  };
}

function mapTelemetryHistogramBucket(bucket: TelemetryHistogramBucket): EventHistogramBucket {
  const start = new Date(bucket.start).getTime();
  const end = new Date(bucket.end).getTime();

  return {
    start,
    end,
    label: new Date(start).toLocaleTimeString("zh-CN", {
      hour: "2-digit",
      minute: "2-digit",
    }),
    count: bucket.total,
  };
}

function timeRangeToMockFilter(time: ManagerTimeRange) {
  return {
    startTime: time.from ? new Date(time.from).getTime() : undefined,
    endTime: time.to ? new Date(time.to).getTime() : undefined,
  };
}

function normalizeEventIndex(index: string): SecurityEventIndex {
  return index === "sysarmor-signals" ? "sysarmor-signals" : "sysarmor-events";
}

function normalizeSeverity(severity?: string): SecurityEvent["severity"] {
  if (severity === "critical" || severity === "high" || severity === "medium") {
    return severity;
  }
  return "info";
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}
