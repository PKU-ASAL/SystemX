import { createHash, timingSafeEqual } from "node:crypto";
import { readFileSync, statSync } from "node:fs";

export type BootstrapCredentials = {
  username: string;
  password: string;
};

type AuthEnvironment = Record<string, string | undefined>;

const USERNAME_FILE = "SYSARMOR_BOOTSTRAP_ADMIN_USERNAME_FILE";
const PASSWORD_FILE = "SYSARMOR_BOOTSTRAP_ADMIN_PASSWORD_FILE";

export function loadBootstrapCredentials(env: AuthEnvironment = process.env): BootstrapCredentials {
  return {
    username: readProtectedSecret(env[USERNAME_FILE], USERNAME_FILE),
    password: readProtectedSecret(env[PASSWORD_FILE], PASSWORD_FILE),
  };
}

export function verifyBootstrapCredentials(
  input: BootstrapCredentials,
  expected: BootstrapCredentials,
) {
  const usernameMatches = safeEqual(input.username, expected.username);
  const passwordMatches = safeEqual(input.password, expected.password);
  return usernameMatches && passwordMatches;
}

export function credentialVersion(credentials: BootstrapCredentials) {
  return digest(`${credentials.username}\0${credentials.password}`);
}

export function readProtectedSecret(path: string | undefined, name: string) {
  if (!path?.trim()) throw new Error(`${name} is required`);

  const stat = statSync(path);
  if (!stat.isFile()) throw new Error(`${name} must reference a regular file`);
  if ((stat.mode & 0o077) !== 0) throw new Error(`${name} must not be accessible by group or others`);

  const value = readFileSync(path, "utf8").trim();
  if (!value) throw new Error(`${name} must not be empty`);
  return value;
}

function safeEqual(left: string, right: string) {
  return timingSafeEqual(Buffer.from(digest(left), "hex"), Buffer.from(digest(right), "hex"));
}

function digest(value: string) {
  return createHash("sha256").update(value).digest("hex");
}
