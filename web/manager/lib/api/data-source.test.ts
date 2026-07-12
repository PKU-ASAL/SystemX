import { afterEach, describe, expect, it, vi } from "vitest";

import { getManagerApiBaseUrl, getManagerDataSource } from "./data-source";

describe("manager api data source configuration", () => {
  afterEach(() => {
    vi.unstubAllEnvs();
  });

  it("always uses the same-origin authenticated BFF", () => {
    vi.stubEnv("NEXT_PUBLIC_MANAGER_DATA_SOURCE", "");
    vi.stubEnv("NEXT_PUBLIC_MANAGER_API_BASE", "");

    expect(getManagerDataSource()).toBe("api");
    expect(getManagerApiBaseUrl()).toBe("/api/manager");
  });

  it("accepts mock mode for local UI fixture work", () => {
    vi.stubEnv("NEXT_PUBLIC_MANAGER_DATA_SOURCE", "mock");
    vi.stubEnv("NEXT_PUBLIC_MANAGER_API_BASE", "http://127.0.0.1:9443/api/v1/");

    expect(getManagerDataSource()).toBe("mock");
    expect(getManagerApiBaseUrl()).toBe("/api/manager");
  });

  it("falls back to API mode for unknown values", () => {
    vi.stubEnv("NEXT_PUBLIC_MANAGER_DATA_SOURCE", "fixture");

    expect(getManagerDataSource()).toBe("api");
  });
});
