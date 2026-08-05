const defaultManagerApiOrigin = "http://127.0.0.1:19443";

export function getManagerApiOrigin(env: Record<string, string | undefined> = process.env) {
  return trimTrailingSlash(env.MANAGER_API_ORIGIN || defaultManagerApiOrigin);
}

function trimTrailingSlash(value: string) {
  return value.replace(/\/+$/, "");
}
