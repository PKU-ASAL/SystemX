import { auth } from "@/auth";
import { loadManagerTokenConfig } from "@/lib/auth/manager-token-config";
import { issueManagerToken } from "@/lib/auth/manager-token";
import { getManagerApiOrigin } from "@/lib/api/manager-origin";
import { proxyManagerRequest } from "@/lib/api/manager-proxy";

type RouteContext = { params: Promise<{ path: string[] }> };

async function handle(request: Request, context: RouteContext) {
  const [session, { path }] = await Promise.all([auth(), context.params]);

  return proxyManagerRequest(request, path, {
    identity: session?.identity ?? null,
    managerOrigin: getManagerApiOrigin(),
    issueToken: (identity) => issueManagerToken(identity, loadManagerTokenConfig()),
    fetcher: fetch,
  });
}

export const GET = handle;
export const POST = handle;
