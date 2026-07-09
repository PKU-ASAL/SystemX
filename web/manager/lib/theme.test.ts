import { describe, expect, it } from "vitest";

import {
  getNextThemeMode,
  isThemeMode,
  resolveThemeMode,
  THEME_MODES,
} from "./theme";

describe("theme mode", () => {
  it("accepts only supported theme modes", () => {
    expect(THEME_MODES).toEqual(["light", "dark", "system"]);
    expect(isThemeMode("light")).toBe(true);
    expect(isThemeMode("dark")).toBe(true);
    expect(isThemeMode("system")).toBe(true);
    expect(isThemeMode("auto")).toBe(false);
  });

  it("cycles light, dark, and system from an icon button", () => {
    expect(getNextThemeMode("light")).toBe("dark");
    expect(getNextThemeMode("dark")).toBe("system");
    expect(getNextThemeMode("system")).toBe("light");
  });

  it("resolves system mode to the actual color scheme", () => {
    expect(resolveThemeMode("light", true)).toBe("light");
    expect(resolveThemeMode("dark", false)).toBe("dark");
    expect(resolveThemeMode("system", true)).toBe("dark");
    expect(resolveThemeMode("system", false)).toBe("light");
  });
});
