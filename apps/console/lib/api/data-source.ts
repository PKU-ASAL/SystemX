export type ManagerDataSource = "api" | "mock";

export function getManagerDataSource(): ManagerDataSource {
  return process.env.NEXT_PUBLIC_MANAGER_DATA_SOURCE === "mock" ? "mock" : "api";
}

export function getManagerApiBaseUrl() {
  return "/api/manager";
}
