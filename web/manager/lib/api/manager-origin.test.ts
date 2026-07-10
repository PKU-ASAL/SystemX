import { describe, expect, it } from "vitest";

import { getManagerApiOrigin } from "./manager-origin";

describe("manager api origin", () => {
  it("defaults to the compose-exposed manager port", () => {
    expect(getManagerApiOrigin({})).toBe("http://127.0.0.1:19443");
  });

  it("uses MANAGER_API_ORIGIN when provided", () => {
    expect(getManagerApiOrigin({ MANAGER_API_ORIGIN: "http://manager.local:9443/" })).toBe(
      "http://manager.local:9443",
    );
  });
});
