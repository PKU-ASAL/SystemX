import type { ManagerApiClient, ManagerApiRequestOptions } from "./client";
import type {
  TelemetryHistogramRequest,
  TelemetryHistogramResponse,
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

export function searchTelemetryHistogram(
  client: ManagerApiClient,
  request: TelemetryHistogramRequest,
  options: { signal?: AbortSignal } = {},
) {
  return client.post<TelemetryHistogramResponse>("/search/histogram", request, {
    signal: options.signal,
  });
}
