import NextAuth from "next-auth";
import Credentials from "next-auth/providers/credentials";

import {
  credentialVersion,
  loadBootstrapCredentials,
  readProtectedSecret,
  verifyBootstrapCredentials,
} from "@/lib/auth/bootstrap-credentials";
import { LoginLimiter } from "@/lib/auth/login-limiter";
import { bootstrapIdentity, isCurrentIdentity } from "@/lib/auth/session-identity";

const limiter = new LoginLimiter();

export const { auth, handlers, signIn, signOut } = NextAuth({
  secret: readProtectedSecret(process.env.AUTH_SECRET_FILE, "AUTH_SECRET_FILE"),
  pages: { signIn: "/login" },
  session: { strategy: "jwt", maxAge: 8 * 60 * 60 },
  providers: [Credentials({
    credentials: {
      username: { label: "Username", type: "text" },
      password: { label: "Password", type: "password" },
    },
    authorize(credentials, request) {
      const expected = loadBootstrapCredentials();
      const input = {
        username: String(credentials.username ?? ""),
        password: String(credentials.password ?? ""),
      };
      const client = request.headers.get("x-forwarded-for")?.split(",")[0]?.trim() || "unknown";
      if (!limiter.allow(`${client}:${input.username}`)) return null;
      if (!verifyBootstrapCredentials(input, expected)) return null;
      return { id: "bootstrap-admin", ...bootstrapIdentity(credentialVersion(expected)) };
    },
  })],
  callbacks: {
    jwt({ token, user }) {
      if (user) token.identity = {
        subject: user.subject,
        tenantId: user.tenantId,
        roles: user.roles,
        credentialVersion: user.credentialVersion,
      };
      return token;
    },
    session({ session, token }) {
      const currentVersion = credentialVersion(loadBootstrapCredentials());
      session.identity = isCurrentIdentity(token.identity, currentVersion) ? token.identity : null;
      return session;
    },
    authorized({ auth: session, request }) {
      if (request.nextUrl.pathname === "/login") return true;
      return Boolean(session?.identity);
    },
  },
});
