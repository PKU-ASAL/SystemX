import type { ManagerApiClient, ManagerApiRequestOptions } from "./client";
import type {
  IncidentDetailResponse,
  IncidentSearchRequest,
  IncidentSearchResponse,
} from "./types";

export function searchIncidents(
  client: ManagerApiClient,
  request: IncidentSearchRequest,
  options: { signal?: AbortSignal } = {},
) {
  return client.post<IncidentSearchResponse>("/incidents/search", request, {
    signal: options.signal,
  });
}

export function getIncidentDetail(
  client: ManagerApiClient,
  incidentId: string,
  options: { signal?: AbortSignal } = {},
) {
  return client.get<IncidentDetailResponse>(`/incidents/${encodeURIComponent(incidentId)}`, {
    signal: options.signal,
  } satisfies ManagerApiRequestOptions);
}
