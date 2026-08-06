import { describe, expect, it } from "vitest";

import {
  DEFAULT_MANAGER_TAB,
  getManagerTabById,
  managerTabs,
} from "./navigation";

describe("manager navigation", () => {
  it("defines the manager tabs in the intended order", () => {
    expect(managerTabs.map((tab) => tab.id)).toEqual([
      "overview",
      "deploy",
      "agents",
      "events",
      "incidents",
    ]);
  });

  it("uses overview as the default tab and resolves unknown ids safely", () => {
    expect(DEFAULT_MANAGER_TAB).toBe("overview");
    expect(getManagerTabById("events")?.label).toBe("Events");
    expect(getManagerTabById("missing")).toBeUndefined();
  });
});
