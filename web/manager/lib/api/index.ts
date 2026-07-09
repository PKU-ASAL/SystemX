import { createManagerApiClient } from "./client";
import { getManagerApiBaseUrl } from "./data-source";

export { ManagerApiError, createManagerApiClient } from "./client";
export { getManagerApiBaseUrl, getManagerDataSource } from "./data-source";
export * from "./types";

export function createDefaultManagerApiClient() {
  return createManagerApiClient({ baseUrl: getManagerApiBaseUrl() });
}
