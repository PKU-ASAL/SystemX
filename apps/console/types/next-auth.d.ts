import "next-auth";
import "next-auth/jwt";

import type { SessionIdentity } from "@/lib/auth/session-identity";

declare module "next-auth" {
  interface Session {
    identity: SessionIdentity | null;
  }

  interface User extends SessionIdentity {}
}

declare module "next-auth/jwt" {
  interface JWT {
    identity?: SessionIdentity;
  }
}
