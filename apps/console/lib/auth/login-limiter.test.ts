import { expect, test } from "vitest";

import { LoginLimiter } from "./login-limiter";

test("limits attempts per key within a fixed window", () => {
  const limiter = new LoginLimiter(5, 60_000, 1024);

  for (let attempt = 0; attempt < 5; attempt += 1) {
    expect(limiter.allow("client:admin", 1_000)).toBe(true);
  }
  expect(limiter.allow("client:admin", 1_000)).toBe(false);
  expect(limiter.allow("other:admin", 1_000)).toBe(true);
  expect(limiter.allow("client:admin", 61_001)).toBe(true);
});

test("bounds retained client keys", () => {
  const limiter = new LoginLimiter(1, 60_000, 2);

  expect(limiter.allow("first", 1_000)).toBe(true);
  expect(limiter.allow("second", 1_000)).toBe(true);
  expect(limiter.allow("third", 1_000)).toBe(true);
  expect(limiter.allow("first", 1_000)).toBe(true);
});
