import { expect, test } from "vitest";

import { bootstrapIdentity, isCurrentIdentity } from "./session-identity";

test("maps bootstrap credentials to the only interactive identity", () => {
  expect(bootstrapIdentity("version-1")).toEqual({
    subject: "bootstrap-admin",
    tenantId: "default",
    roles: ["admin"],
    credentialVersion: "version-1",
  });
});

test("accepts only a complete identity with the current credential version", () => {
  const identity = bootstrapIdentity("version-1");

  expect(isCurrentIdentity(identity, "version-1")).toBe(true);
  expect(isCurrentIdentity(identity, "version-2")).toBe(false);
  expect(isCurrentIdentity({ ...identity, roles: ["viewer"] }, "version-1")).toBe(false);
  expect(isCurrentIdentity(null, "version-1")).toBe(false);
});
