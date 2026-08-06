export type SessionIdentity = {
  subject: "bootstrap-admin";
  tenantId: "default";
  roles: ["admin"];
  credentialVersion: string;
};

export function bootstrapIdentity(credentialVersion: string): SessionIdentity {
  return {
    subject: "bootstrap-admin",
    tenantId: "default",
    roles: ["admin"],
    credentialVersion,
  };
}

export function isCurrentIdentity(
  value: unknown,
  credentialVersion: string,
): value is SessionIdentity {
  if (!value || typeof value !== "object") return false;

  const identity = value as Partial<SessionIdentity>;
  return identity.subject === "bootstrap-admin"
    && identity.tenantId === "default"
    && identity.credentialVersion === credentialVersion
    && Array.isArray(identity.roles)
    && identity.roles.length === 1
    && identity.roles[0] === "admin";
}
