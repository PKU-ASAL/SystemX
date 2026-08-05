import { describe, expect, it, vi } from "vitest";

import { createManagerApiClient } from "./client";
import { listAgents } from "./agents";
import { getOverview } from "./overview";
import { getIncidentDetail, searchIncidents } from "./incidents";
import { getSearchFields, searchTelemetry, searchTelemetryHistogram } from "./search";

describe("manager api services", () => {
  it("loads agents through the existing manager agents endpoint", async () => {
    const fetcher = vi.fn(async () => jsonResponse([{ agent_id: "agent-prod-001" }]));
    const client = createManagerApiClient({ baseUrl: "/api/v1", fetcher });

    const agents = await listAgents(client, { tenantId: "default", limit: 25 });

    expect(agents).toEqual([{ agent_id: "agent-prod-001" }]);
    expect(fetcher).toHaveBeenCalledWith(
      "/api/v1/agents?tenant_id=default&limit=25",
      expect.objectContaining({ method: "GET" }),
    );
  });

  it("loads overview from the UI overview endpoint", async () => {
    const fetcher = vi.fn(async () =>
      jsonResponse({
        generated_at: "2026-07-09T15:00:00Z",
        agents: { total: 1, online: 1, degraded: 0, offline: 0 },
        telemetry: { events_24h: 2, signals_24h: 3 },
        incidents: { open: 4, critical: 1, high: 2, medium: 1 },
        store: { backend: "postgres" },
      }),
    );
    const client = createManagerApiClient({ baseUrl: "/api/v1", fetcher });

    const overview = await getOverview(client);

    expect(overview.agents.online).toBe(1);
    expect(fetcher).toHaveBeenCalledWith("/api/v1/ui/overview", expect.objectContaining({ method: "GET" }));
  });

  it("sends telemetry search and field metadata requests", async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ indexes: ["sysarmor-events"], fields: [] }))
      .mockResolvedValueOnce(jsonResponse({ total: 0, total_relation: "eq", rows: [] }))
      .mockResolvedValueOnce(jsonResponse({ buckets: [] }));
    const client = createManagerApiClient({ baseUrl: "/api/v1", fetcher });

    await getSearchFields(client, "events-*,signals-*");
    await searchTelemetry(client, {
      indexes: ["sysarmor-events", "sysarmor-signals"],
      query: "host.name: prod-api-01",
      limit: 100,
      offset: 0,
    });
    await searchTelemetryHistogram(client, {
      indexes: ["sysarmor-events", "sysarmor-signals"],
      query: "host.name: prod-api-01",
      bucket_count: 12,
    });

    expect(fetcher).toHaveBeenNthCalledWith(
      1,
      "/api/v1/search/fields?index=events-*%2Csignals-*",
      expect.objectContaining({ method: "GET" }),
    );
    expect(fetcher).toHaveBeenNthCalledWith(
      2,
      "/api/v1/search",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({
          indexes: ["sysarmor-events", "sysarmor-signals"],
          query: "host.name: prod-api-01",
          limit: 100,
          offset: 0,
        }),
      }),
    );
    expect(fetcher).toHaveBeenNthCalledWith(
      3,
      "/api/v1/search/histogram",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({
          indexes: ["sysarmor-events", "sysarmor-signals"],
          query: "host.name: prod-api-01",
          bucket_count: 12,
        }),
      }),
    );
  });

  it("sends incident search and detail requests", async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ total: 0, rows: [], histogram: [] }))
      .mockResolvedValueOnce(jsonResponse({ incident: { incident_id: "inc-1027" }, attack_chain: [], provenance: { nodes: [], edges: [] }, evidence: [] }));
    const client = createManagerApiClient({ baseUrl: "/api/v1", fetcher });

    await searchIncidents(client, { query: "severity: critical", limit: 50 });
    await getIncidentDetail(client, "inc-1027");

    expect(fetcher).toHaveBeenNthCalledWith(
      1,
      "/api/v1/incidents/search",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ query: "severity: critical", limit: 50 }),
      }),
    );
    expect(fetcher).toHaveBeenNthCalledWith(
      2,
      "/api/v1/incidents/inc-1027",
      expect.objectContaining({ method: "GET" }),
    );
  });
});

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}
