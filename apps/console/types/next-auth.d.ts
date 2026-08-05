import "next-auth";
import "next-auth/jwt";

import type { SessionIdentity } from "@/lib/auth/session-identity";

declare module "next-auth" {
  interface Session {
    identity: SessionIdentity | null;
  }

  // NextAuth requires interface merging for module augmentation.
  // eslint-disable-next-line @typescript-eslint/no-empty-object-type
  interface User extends SessionIdentity {}
}

declare module "next-auth/jwt" {
  interface JWT {
    identity?: SessionIdentity;
  }
}
