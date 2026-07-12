import { exportPKCS8, generateKeyPair, importSPKI, jwtVerify } from "jose";
import { expect, test } from "vitest";

import { issueManagerToken } from "./manager-token";
import { bootstrapIdentity } from "./session-identity";

test("issues a five-minute RS256 manager token from trusted identity", async () => {
  const { privateKey, publicKey } = await generateKeyPair("RS256", { extractable: true });
  const privateKeyPem = await exportPKCS8(privateKey);
  const publicKeyPem = await crypto.subtle.exportKey("spki", publicKey);
  const now = new Date("2026-07-12T10:00:00Z");

  const raw = await issueManagerToken(bootstrapIdentity("version"), {
    privateKeyPem,
    issuer: "sysarmor-bff",
    audience: "sysarmor-manager",
  }, now);
  const key = await importSPKI(toPEM(publicKeyPem), "RS256");
  const { payload, protectedHeader } = await jwtVerify(raw, key, {
    issuer: "sysarmor-bff",
    audience: "sysarmor-manager",
    currentDate: now,
  });

  expect(protectedHeader.alg).toBe("RS256");
  expect(payload.sub).toBe("bootstrap-admin");
  expect(payload.tenant_id).toBe("default");
  expect(payload.roles).toEqual(["admin"]);
  expect(Number(payload.exp) - Number(payload.iat)).toBe(300);
});

test("rejects invalid private key material", async () => {
  await expect(issueManagerToken(bootstrapIdentity("version"), {
    privateKeyPem: "not-a-key",
    issuer: "sysarmor-bff",
    audience: "sysarmor-manager",
  })).rejects.toThrow();
});

function toPEM(value: ArrayBuffer) {
  const base64 = Buffer.from(value).toString("base64").match(/.{1,64}/g)?.join("\n");
  return `-----BEGIN PUBLIC KEY-----\n${base64}\n-----END PUBLIC KEY-----`;
}
