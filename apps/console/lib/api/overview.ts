import type { ManagerApiClient, ManagerApiRequestOptions } from "./client";
import type { OverviewSummary } from "./types";

export function getOverview(client: ManagerApiClient, options: { signal?: AbortSignal } = {}) {
  return client.get<OverviewSummary>("/ui/overview", {
    signal: options.signal,
  } satisfies ManagerApiRequestOptions);
}
