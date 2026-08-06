import { expect, test, vi } from "vitest";

import { proxyManagerRequest } from "./manager-proxy";
import { bootstrapIdentity } from "../auth/session-identity";

const identity = bootstrapIdentity("version");

test("rejects unauthenticated requests with a JSON envelope", async () => {
  const response = await proxyManagerRequest(
    new Request("http://ui/api/manager/status"),
    ["status"],
    dependencies(null),
  );

  expect(response.status).toBe(401);
  expect(await response.json()).toEqual({ error: { code: "unauthorized", message: "Unauthorized" } });
});

test("forwards an authenticated allowlisted request with only the internal token", async () => {
  const fetcher = vi.fn(async () => Response.json({ ok: true }));
  const request = new Request("http://ui/api/manager/agents?limit=10", {
    headers: { authorization: "Bearer browser-token", cookie: "session=secret" },
  });

  const response = await proxyManagerRequest(request, ["agents"], dependencies(identity, fetcher));

  expect(await response.json()).toEqual({ ok: true });
  expect(fetcher).toHaveBeenCalledWith("http://manager:9443/api/v1/agents?limit=10", expect.objectContaining({
    headers: expect.objectContaining({ authorization: "Bearer internal-token" }),
  }));
  const headers = fetcher.mock.calls[0][1]?.headers as Record<string, string>;
  expect(headers.cookie).toBeUndefined();
});

test.each([
  [["..", "healthz"], "GET", 404],
  [["unknown"], "GET", 404],
  [["agents"], "DELETE", 405],
])("rejects unsupported path or method", async (path, method, status) => {
  const response = await proxyManagerRequest(
    new Request("http://ui/api/manager/test", { method }),
    path,
    dependencies(identity),
  );
  expect(response.status).toBe(status);
});

function dependencies(sessionIdentity: typeof identity | null, fetcher = vi.fn()) {
  return {
    identity: sessionIdentity,
    managerOrigin: "http://manager:9443",
    issueToken: vi.fn(async () => "internal-token"),
    fetcher,
  };
}
