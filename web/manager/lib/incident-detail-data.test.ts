import { describe, expect, it } from "vitest";

import { getIncidentDetail, getSyscallsForAttackStep, getSyscallsForProvenanceNode } from "./incident-detail-data";

describe("incident detail data", () => {
  it("returns attack chain and provenance graph data for an incident", () => {
    const detail = getIncidentDetail("inc-1027");

    expect(detail?.incidentId).toBe("inc-1027");
    expect(detail?.attackChain.length).toBeGreaterThanOrEqual(4);
    expect(detail?.provenance.nodes.map((node) => node.node_name)).toContain("nginx");
    expect(detail?.provenance.edges.length).toBeGreaterThan(0);
  });

  it("returns undefined for unknown incidents", () => {
    expect(getIncidentDetail("missing")).toBeUndefined();
  });

  it("filters syscall evidence around a provenance node", () => {
    const detail = getIncidentDetail("inc-1027");
    const node = detail?.provenance.nodes.find((item) => item.id === "n-c2");

    expect(detail && node ? getSyscallsForProvenanceNode(detail, node).map((event) => event.syscall) : []).toEqual([
      "connect",
    ]);
  });

  it("matches provenance process and file nodes to their related syscalls", () => {
    const detail = getIncidentDetail("inc-1027");
    const bashNode = detail?.provenance.nodes.find((item) => item.id === "n-bash");
    const webshellNode = detail?.provenance.nodes.find((item) => item.id === "n-webshell");

    expect(detail && bashNode ? getSyscallsForProvenanceNode(detail, bashNode).map((event) => event.syscall) : []).toEqual([
      "execve",
    ]);
    expect(detail && webshellNode ? getSyscallsForProvenanceNode(detail, webshellNode).map((event) => event.syscall) : []).toEqual([
      "write",
    ]);
  });

  it("filters syscall evidence for an attack chain step", () => {
    const detail = getIncidentDetail("inc-1027");
    const step = detail?.attackChain.find((item) => item.id === "step-c2");

    expect(detail && step ? getSyscallsForAttackStep(detail, step).map((event) => event.syscall) : []).toEqual([
      "connect",
    ]);
  });
});
