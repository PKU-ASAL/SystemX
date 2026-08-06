import type { ManagerApiClient, ManagerApiRequestOptions } from "./client";
import type {
  DeployAgentCommandRequest,
  DeployAgentCommandResponse,
  DeployOptionsResponse,
} from "./types";

export function getDeployOptions(
  client: ManagerApiClient,
  options: { tenantId?: string; signal?: AbortSignal } = {},
) {
  return client.get<DeployOptionsResponse>("/ui/deploy/options", {
    query: { tenant_id: options.tenantId ?? "default" },
    signal: options.signal,
  } satisfies ManagerApiRequestOptions);
}

export function generateAgentInstallCommand(
  client: ManagerApiClient,
  request: DeployAgentCommandRequest,
  options: { signal?: AbortSignal } = {},
) {
  return client.post<DeployAgentCommandResponse>("/ui/deploy/agent-command", request, {
    signal: options.signal,
  });
}
