import { describe, expect, it, vi } from "vitest";

import { ManagerApiError, createManagerApiClient } from "./client";

describe("manager api client", () => {
  it("joins the configured base URL with request paths", async () => {
    const fetcher = vi.fn(async () => jsonResponse({ ok: true }));
    const client = createManagerApiClient({ baseUrl: "https://manager.example/api/v1", fetcher });

    await client.get("/agents", { query: { tenant_id: "default", limit: 50 } });

    expect(fetcher).toHaveBeenCalledWith(
      "https://manager.example/api/v1/agents?tenant_id=default&limit=50",
      expect.objectContaining({ method: "GET" }),
    );
  });

  it("sends JSON POST requests and passes abort signals through", async () => {
    const fetcher = vi.fn(async () => jsonResponse({ rows: [] }));
    const client = createManagerApiClient({ baseUrl: "/api/v1", fetcher });
    const controller = new AbortController();

    await client.post("/search", { indexes: ["sysarmor-events"] }, { signal: controller.signal });

    expect(fetcher).toHaveBeenCalledWith(
      "/api/v1/search",
      expect.objectContaining({
        body: JSON.stringify({ indexes: ["sysarmor-events"] }),
        headers: { "Content-Type": "application/json" },
        method: "POST",
        signal: controller.signal,
      }),
    );
  });

  it("throws a typed API error from the manager error envelope", async () => {
    const fetcher = vi.fn(async () =>
      jsonResponse(
        {
          error: {
            code: "invalid_query",
            message: "query field is not supported",
            details: { field: "process.parent.pid" },
          },
        },
        { status: 400 },
      ),
    );
    const client = createManagerApiClient({ baseUrl: "/api/v1", fetcher });

    await expect(client.get("/search")).rejects.toMatchObject({
      code: "invalid_query",
      message: "query field is not supported",
      status: 400,
      details: { field: "process.parent.pid" },
    });
    await expect(client.get("/search")).rejects.toBeInstanceOf(ManagerApiError);
  });
});

function jsonResponse(body: unknown, init: { status?: number } = {}) {
  return new Response(JSON.stringify(body), {
    status: init.status ?? 200,
    headers: { "Content-Type": "application/json" },
  });
}
