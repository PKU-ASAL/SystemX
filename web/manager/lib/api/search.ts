import type { ManagerApiClient, ManagerApiRequestOptions } from "./client";
import type {
  SearchFieldsResponse,
  TelemetrySearchRequest,
  TelemetrySearchResponse,
} from "./types";

export function getSearchFields(
  client: ManagerApiClient,
  indexPattern: string,
  options: { signal?: AbortSignal } = {},
) {
  return client.get<SearchFieldsResponse>("/search/fields", {
    query: { index: indexPattern },
    signal: options.signal,
  } satisfies ManagerApiRequestOptions);
}

export function searchTelemetry(
  client: ManagerApiClient,
  request: TelemetrySearchRequest,
  options: { signal?: AbortSignal } = {},
) {
  return client.post<TelemetrySearchResponse>("/search", request, {
    signal: options.signal,
  });
}
