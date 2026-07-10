import { describe, expect, it } from "vitest";

import { mapOverviewSummaryToMetrics } from "./overview-data";

describe("overview data mapping", () => {
  it("maps manager overview summary into metric cards", () => {
    const metrics = mapOverviewSummaryToMetrics({
      generated_at: "2026-07-10T04:00:00Z",
      agents: { total: 3, online: 1, degraded: 1, offline: 1 },
      telemetry: { events_24h: 12, signals_24h: 5 },
      incidents: { open: 2, critical: 1, high: 1, medium: 1 },
      store: { backend: "postgres", postgres_schema_version: 1 },
    });

    expect(metrics).toEqual([
      { label: "Online agents", value: "1/3", trend: "1 degraded · 1 offline" },
      { label: "Open events", value: "12", trend: "last 24h" },
      { label: "Signals", value: "5", trend: "last 24h" },
      { label: "Incidents", value: "2", trend: "1 critical · 1 high" },
    ]);
  });
});
