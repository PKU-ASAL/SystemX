import type { SessionIdentity } from "../auth/session-identity";

type ProxyDependencies = {
  identity: SessionIdentity | null;
  managerOrigin: string;
  issueToken(identity: SessionIdentity): Promise<string>;
  fetcher: typeof fetch;
};

const ROOTS = new Set(["agents", "incidents", "search", "ui"]);
const METHODS = new Set(["GET", "POST"]);

export async function proxyManagerRequest(
  request: Request,
  path: string[],
  dependencies: ProxyDependencies,
) {
  if (!dependencies.identity) return apiError(401, "unauthorized", "Unauthorized");
  if (!validPath(path)) return apiError(404, "not_found", "Not found");
  if (!METHODS.has(request.method)) return apiError(405, "method_not_allowed", "Method not allowed");

  try {
    const token = await dependencies.issueToken(dependencies.identity);
    const upstream = await dependencies.fetcher(upstreamURL(request, path, dependencies.managerOrigin), {
      method: request.method,
      headers: upstreamHeaders(request, token),
      body: request.method === "POST" ? await request.text() : undefined,
      cache: "no-store",
    });
    return normalizedResponse(upstream);
  } catch {
    return apiError(502, "manager_unavailable", "Manager API is unavailable");
  }
}

function validPath(path: string[]) {
  return path.length > 0
    && ROOTS.has(path[0])
    && path.every((part) => /^[a-z0-9_-]+$/i.test(part));
}

function upstreamURL(request: Request, path: string[], origin: string) {
  const query = new URL(request.url).search;
  return `${origin.replace(/\/+$/, "")}/api/v1/${path.join("/")}${query}`;
}

function upstreamHeaders(request: Request, token: string) {
  const headers: Record<string, string> = { authorization: `Bearer ${token}` };
  const contentType = request.headers.get("content-type");
  if (contentType) headers["content-type"] = contentType;
  return headers;
}

async function normalizedResponse(response: Response) {
  const text = await response.text();
  if (text && !isJSON(text)) return apiError(502, "invalid_response", "Manager API returned an invalid response");
  return new Response(text || null, {
    status: response.status,
    headers: { "content-type": "application/json; charset=utf-8" },
  });
}

function isJSON(value: string) {
  try { JSON.parse(value); return true; } catch { return false; }
}

function apiError(status: number, code: string, message: string) {
  return Response.json({ error: { code, message } }, { status });
}
