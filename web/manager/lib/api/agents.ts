import type { ManagerApiClient, ManagerApiRequestOptions } from "./client";
import type { AgentListItem } from "./types";

export type ListAgentsOptions = {
  tenantId?: string;
  scopeType?: string;
  scopeSelector?: string;
  healthStatus?: string;
  limit?: number;
  offset?: number;
  signal?: AbortSignal;
};

export function listAgents(client: ManagerApiClient, options: ListAgentsOptions = {}) {
  return client.get<AgentListItem[]>("/agents", {
    query: {
      tenant_id: options.tenantId,
      scope_type: options.scopeType,
      scope_selector: options.scopeSelector,
      health_status: options.healthStatus,
      limit: options.limit,
      offset: options.offset,
    },
    signal: options.signal,
  } satisfies ManagerApiRequestOptions);
}
