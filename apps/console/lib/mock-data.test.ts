import { describe, expect, it } from "vitest";

import {
  attackChain,
  buildEventHistogram,
  buildIncidentHistogram,
  buildIncidentSeverityBuckets,
  createDiscoverRows,
  createEventFields,
  createEventRawDocument,
  createEventSourceChips,
  events,
  filterAgents,
  filterEvents,
  filterIncidents,
  incidents,
  overviewMetrics,
  agents,
} from "./mock-data";

describe("manager mock data", () => {
  it("covers every initial manager domain", () => {
    expect(overviewMetrics.length).toBeGreaterThanOrEqual(4);
    expect(agents.length).toBeGreaterThanOrEqual(3);
    expect(events.length).toBeGreaterThanOrEqual(4);
    expect(incidents.length).toBeGreaterThanOrEqual(2);
    expect(attackChain.length).toBeGreaterThanOrEqual(4);
  });

  it("filters events by free-text query like a search view", () => {
    const results = filterEvents(events, { query: "credential" });

    expect(results.length).toBeGreaterThan(0);
    expect(results.every((event) => event.summary.toLowerCase().includes("credential"))).toBe(true);
  });

  it("filters agents by free text and field query", () => {
    expect(filterAgents(agents, "prod-db").map((agent) => agent.id)).toEqual(["agent-prod-014"]);
    expect(filterAgents(agents, "agent.registered_at:2026-07-07").map((agent) => agent.id)).toEqual([
      "agent-prod-014",
    ]);
    expect(filterAgents(agents, "status:healthy").map((agent) => agent.id)).toEqual([
      "agent-prod-001",
      "agent-lab-003",
    ]);
  });

  it("filters events by selected index and time range", () => {
    const results = filterEvents(events, {
      indexes: ["sysarmor-signals"],
      minutes: 15,
      now: new Date("2026-07-08T21:05:00").getTime(),
    });

    expect(results.map((event) => event.id)).toEqual(["evt-24091", "evt-24065"]);
  });

  it("filters events by absolute timestamp range", () => {
    const results = filterEvents(events, {
      startTime: new Date("2026-07-08T20:50:00").getTime(),
      endTime: new Date("2026-07-08T21:00:00").getTime(),
    });

    expect(results.map((event) => event.id)).toEqual(["evt-24076", "evt-24065"]);
  });

  it("builds hit histogram buckets for the current result set", () => {
    const buckets = buildEventHistogram(events, {
      minutes: 30,
      now: new Date("2026-07-08T21:10:00").getTime(),
      bucketCount: 6,
    });

    expect(buckets).toHaveLength(6);
    expect(buckets.reduce((sum, bucket) => sum + bucket.count, 0)).toBe(events.length);
  });

  it("creates Kibana-like source chips and raw documents for events", () => {
    const event = events[0];
    const chips = createEventSourceChips(event);
    const raw = createEventRawDocument(event);
    const fields = createEventFields(events);

    expect(chips.map((chip) => chip.key)).toContain("event.summary");
    expect(raw._index).toBe(event.index);
    expect(raw.event.kind).toBe("signal");
    expect(fields.selected).toEqual([{ name: "_source", type: "object" }]);
    expect(fields.available.map((field) => field.name)).toContain("host.name");
  });

  it("creates stable discover rows for a virtualized table", () => {
    const rows = createDiscoverRows(events);

    expect(rows).toHaveLength(events.length);
    expect(rows[0]).toMatchObject({
      id: events[0].id,
      time: events[0].timestamp,
      source: expect.arrayContaining([{ key: "host.name", value: events[0].host }]),
    });
    expect(rows[0].raw._id).toBe(events[0].id);
  });

  it("filters incidents by query, severity, and status", () => {
    const results = filterIncidents(incidents, {
      query: "凭据",
      severity: "critical",
      status: "triage",
    });

    expect(results.map((incident) => incident.id)).toEqual(["inc-1019"]);
  });

  it("filters incidents by time range", () => {
    const results = filterIncidents(incidents, {
      minutes: 15,
      now: new Date("2026-07-08T21:10:00").getTime(),
    });

    expect(results.map((incident) => incident.id)).toEqual(["inc-1027"]);
  });

  it("builds incident severity chart buckets", () => {
    const buckets = buildIncidentSeverityBuckets(incidents);

    expect(buckets).toEqual([
      { severity: "critical", count: 2 },
      { severity: "high", count: 0 },
      { severity: "medium", count: 0 },
    ]);
  });

  it("builds incident time histogram with severity stacks", () => {
    const buckets = buildIncidentHistogram(incidents, {
      minutes: 30,
      now: new Date("2026-07-08T21:10:00").getTime(),
      bucketCount: 6,
    });

    expect(buckets).toHaveLength(6);
    expect(buckets.reduce((sum, bucket) => sum + bucket.total, 0)).toBe(incidents.length);
    expect(buckets.some((bucket) => bucket.critical > 0)).toBe(true);
  });
});
