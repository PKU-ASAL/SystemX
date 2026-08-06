import "server-only";

import { readProtectedSecret } from "./bootstrap-credentials";
import type { ManagerTokenConfig } from "./manager-token";

export function loadManagerTokenConfig(
  env: Record<string, string | undefined> = process.env,
): ManagerTokenConfig {
  const issuer = env.SYSARMOR_MANAGER_JWT_ISSUER?.trim();
  const audience = env.SYSARMOR_MANAGER_JWT_AUDIENCE?.trim();
  if (!issuer) throw new Error("SYSARMOR_MANAGER_JWT_ISSUER is required");
  if (!audience) throw new Error("SYSARMOR_MANAGER_JWT_AUDIENCE is required");

  return {
    privateKeyPem: readProtectedSecret(
      env.SYSARMOR_BFF_JWT_PRIVATE_KEY_FILE,
      "SYSARMOR_BFF_JWT_PRIVATE_KEY_FILE",
    ),
    issuer,
    audience,
  };
}
