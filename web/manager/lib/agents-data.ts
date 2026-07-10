import { listAgents } from "./api/agents";
import type { ManagerApiClient } from "./api/client";
import type { AgentListItem } from "./api/types";
import { agents as mockAgents, type AgentRecord } from "./mock-data";

export type LoadAgentsOptions = {
  client: ManagerApiClient;
  dataSource: "api" | "mock";
  signal?: AbortSignal;
};

export async function loadAgentRecords({ client, dataSource, signal }: LoadAgentsOptions) {
  if (dataSource === "mock") {
    return mockAgents;
  }

  const items = await listAgents(client, { signal });

  return items.map(mapAgentListItemToRecord);
}

export function mapAgentListItemToRecord(item: AgentListItem): AgentRecord {
  return {
    id: displayValue(item.agent_id),
    host: displayValue(item.host_id),
    version: displayValue(item.version),
    status: mapHealthStatus(item.health_status),
    policy: "-",
    registeredAt: "-",
    lastSeen: displayValue(item.health_observed),
  };
}

function mapHealthStatus(status?: string): AgentRecord["status"] {
  if (status === "ok" || status === "healthy") return "healthy";
  if (status === "degraded") return "degraded";

  return "offline";
}

function displayValue(value?: string) {
  return value && value.trim() !== "" ? value : "-";
}
