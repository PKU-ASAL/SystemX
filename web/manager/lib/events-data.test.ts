import { describe, expect, it, vi } from "vitest";

import type { ManagerApiClient } from "./api/client";
import type { SearchFieldsResponse, TelemetryHistogramResponse, TelemetrySearchResponse } from "./api/types";
import {
  loadEventDiscoverData,
  mapSearchFieldsToUiFields,
  mapTelemetryRowToDiscoverRow,
} from "./events-data";

describe("events data mapping", () => {
  it("maps manager telemetry rows into discover table rows", () => {
    expect(
      mapTelemetryRowToDiscoverRow({
        index: "sysarmor-signals",
        id: "sig-a",
        timestamp: "2026-07-08T21:06:18Z",
        severity: "critical",
        host: "prod-api-01",
        summary: "credential access",
        tactic: "CredentialAccess",
        source: {
          "event.kind": "signal",
          "host.name": "prod-api-01",
          "event.summary": "credential access",
        },
        raw: {
          id: "sig-a",
          "@timestamp": "2026-07-08T21:06:18Z",
        },
      }),
    ).toMatchObject({
      id: "sig-a",
      time: "2026-07-08T21:06:18Z",
      severity: "critical",
      event: {
        index: "sysarmor-signals",
        host: "prod-api-01",
        summary: "credential access",
        tactic: "CredentialAccess",
      },
      source: expect.arrayContaining([{ key: "event.kind", value: "signal" }]),
      raw: expect.objectContaining({ _id: "sig-a" }),
    });
  });

  it("loads fields, rows and histogram from the manager API", async () => {
    const client = {
      get: vi.fn(async () => ({
        indexes: ["sysarmor-events", "sysarmor-signals"],
        fields: [{ name: "host.name", type: "keyword", searchable: true, aggregatable: true }],
      } satisfies SearchFieldsResponse)),
      post: vi
        .fn()
        .mockResolvedValueOnce({
          total: 1,
          total_relation: "eq",
          rows: [
            {
              index: "sysarmor-events",
              id: "evt-a",
              timestamp: "2026-07-08T21:04:18Z",
              host: "prod-api-01",
              summary: "process execution",
            },
          ],
        } satisfies TelemetrySearchResponse)
        .mockResolvedValueOnce({
          buckets: [
            {
              start: "2026-07-08T21:00:00Z",
              end: "2026-07-08T21:05:00Z",
              total: 1,
              events: 1,
              signals: 0,
            },
          ],
        } satisfies TelemetryHistogramResponse),
    } satisfies ManagerApiClient;

    const data = await loadEventDiscoverData({
      client,
      dataSource: "api",
      indexPattern: "events-*,signals-*",
      indexes: ["sysarmor-events", "sysarmor-signals"],
      query: "host.name:prod-api-01",
      time: { field: "@timestamp", from: "2026-07-08T21:00:00Z", to: "2026-07-08T21:10:00Z" },
      bucketCount: 12,
    });

    expect(mapSearchFieldsToUiFields(data.fields).map((field) => field.name)).toEqual(["host.name"]);
    expect(data.rows).toHaveLength(1);
    expect(data.histogram).toEqual([
      expect.objectContaining({ count: 1, start: new Date("2026-07-08T21:00:00Z").getTime() }),
    ]);
    expect(client.get).toHaveBeenCalledWith("/search/fields", expect.objectContaining({ query: { index: "events-*,signals-*" } }));
    expect(client.post).toHaveBeenNthCalledWith(
      1,
      "/search",
      expect.objectContaining({ query: "host.name:prod-api-01", limit: 500 }),
      expect.any(Object),
    );
    expect(client.post).toHaveBeenNthCalledWith(
      2,
      "/search/histogram",
      expect.objectContaining({ bucket_count: 12 }),
      expect.any(Object),
    );
  });
});
