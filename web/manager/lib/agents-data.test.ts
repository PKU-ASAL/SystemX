import { describe, expect, it } from "vitest";

import { mapAgentListItemToRecord } from "./agents-data";

describe("agents data mapping", () => {
  it("maps manager API agents into table records", () => {
    expect(
      mapAgentListItemToRecord({
        agent_id: "agent-prod-001",
        host_id: "prod-api-01",
        version: "0.8.0",
        health_status: "ok",
        health_observed: "2026-07-10T10:12:00Z",
      }),
    ).toEqual({
      id: "agent-prod-001",
      host: "prod-api-01",
      version: "0.8.0",
      status: "healthy",
      policy: "-",
      registeredAt: "-",
      lastSeen: "2026-07-10T10:12:00Z",
    });
  });

  it("uses safe display fallbacks for partial API agents", () => {
    expect(
      mapAgentListItemToRecord({
        agent_id: "agent-lab-001",
        health_status: "missing",
      }),
    ).toMatchObject({
      host: "-",
      version: "-",
      status: "offline",
      policy: "-",
      registeredAt: "-",
      lastSeen: "-",
    });
  });
});
