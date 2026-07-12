import { importPKCS8, SignJWT } from "jose";

import type { SessionIdentity } from "./session-identity";

export type ManagerTokenConfig = {
  privateKeyPem: string;
  issuer: string;
  audience: string;
};

export async function issueManagerToken(
  identity: SessionIdentity,
  config: ManagerTokenConfig,
  now = new Date(),
) {
  const key = await importPKCS8(config.privateKeyPem, "RS256");
  const issuedAt = Math.floor(now.getTime() / 1000);

  return new SignJWT({ tenant_id: identity.tenantId, roles: identity.roles })
    .setProtectedHeader({ alg: "RS256", typ: "JWT" })
    .setSubject(identity.subject)
    .setIssuer(config.issuer)
    .setAudience(config.audience)
    .setIssuedAt(issuedAt)
    .setExpirationTime(issuedAt + 300)
    .sign(key);
}
