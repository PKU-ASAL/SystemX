import { chmodSync, mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { describe, expect, test } from "vitest";

import {
  credentialVersion,
  loadBootstrapCredentials,
  verifyBootstrapCredentials,
} from "./bootstrap-credentials";

function secretFile(name: string, value: string, mode = 0o600) {
  const directory = mkdtempSync(join(tmpdir(), "sysarmor-auth-"));
  const path = join(directory, name);
  writeFileSync(path, value, { mode });
  chmodSync(path, mode);
  return path;
}

describe("bootstrap credentials", () => {
  test("loads trimmed credentials from protected files", () => {
    const credentials = loadBootstrapCredentials({
      SYSARMOR_BOOTSTRAP_ADMIN_USERNAME_FILE: secretFile("username", " admin\n"),
      SYSARMOR_BOOTSTRAP_ADMIN_PASSWORD_FILE: secretFile("password", " correct horse \n"),
    });

    expect(credentials).toEqual({ username: "admin", password: "correct horse" });
  });

  test.each([
    ["missing path", {}],
    ["empty username", {
      SYSARMOR_BOOTSTRAP_ADMIN_USERNAME_FILE: secretFile("username", "\n"),
      SYSARMOR_BOOTSTRAP_ADMIN_PASSWORD_FILE: secretFile("password", "secret"),
    }],
    ["permissive password", {
      SYSARMOR_BOOTSTRAP_ADMIN_USERNAME_FILE: secretFile("username", "admin"),
      SYSARMOR_BOOTSTRAP_ADMIN_PASSWORD_FILE: secretFile("password", "secret", 0o640),
    }],
  ])("rejects %s", (_name, env) => {
    expect(() => loadBootstrapCredentials(env)).toThrow();
  });

  test("accepts only the complete credential pair", () => {
    const expected = { username: "admin", password: "secret" };

    expect(verifyBootstrapCredentials({ username: "admin", password: "secret" }, expected)).toBe(true);
    expect(verifyBootstrapCredentials({ username: "other", password: "secret" }, expected)).toBe(false);
    expect(verifyBootstrapCredentials({ username: "admin", password: "wrong" }, expected)).toBe(false);
  });

  test("produces a stable non-secret credential version", () => {
    const credentials = { username: "admin", password: "secret" };

    expect(credentialVersion(credentials)).toBe(credentialVersion(credentials));
    expect(credentialVersion(credentials)).not.toContain("secret");
    expect(credentialVersion({ ...credentials, password: "rotated" })).not.toBe(credentialVersion(credentials));
  });
});
