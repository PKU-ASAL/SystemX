export type Fetcher = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>;

export type ManagerApiQuery = Record<string, string | number | boolean | null | undefined>;

export type ManagerApiClient = {
  get<T>(path: string, options?: ManagerApiRequestOptions): Promise<T>;
  post<T>(path: string, body?: unknown, options?: ManagerApiRequestOptions): Promise<T>;
};

export type ManagerApiRequestOptions = {
  query?: ManagerApiQuery;
  signal?: AbortSignal;
};

export type ManagerApiErrorEnvelope = {
  error?: {
    code?: string;
    message?: string;
    details?: unknown;
  };
};

export class ManagerApiError extends Error {
  readonly code: string;
  readonly status: number;
  readonly details: unknown;

  constructor({ code, message, status, details }: { code: string; message: string; status: number; details?: unknown }) {
    super(message);
    this.name = "ManagerApiError";
    this.code = code;
    this.status = status;
    this.details = details;
  }
}

export function createManagerApiClient({
  baseUrl,
  fetcher = fetch,
}: {
  baseUrl: string;
  fetcher?: Fetcher;
}): ManagerApiClient {
  const normalizedBaseUrl = trimTrailingSlash(baseUrl);

  return {
    get: <T>(path: string, options?: ManagerApiRequestOptions) =>
      request<T>(fetcher, buildURL(normalizedBaseUrl, path, options?.query), {
        method: "GET",
        signal: options?.signal,
      }),
    post: <T>(path: string, body?: unknown, options?: ManagerApiRequestOptions) =>
      request<T>(fetcher, buildURL(normalizedBaseUrl, path, options?.query), {
        body: JSON.stringify(body ?? {}),
        headers: { "Content-Type": "application/json" },
        method: "POST",
        signal: options?.signal,
      }),
  };
}

async function request<T>(fetcher: Fetcher, url: string, init: RequestInit): Promise<T> {
  const response = await fetcher(url, init);
  const body = await readJSON(response).catch(() => {
    throw new ManagerApiError({
      code: "invalid_response",
      message: "Manager API returned non-JSON response",
      status: response.status,
    });
  });

  if (!response.ok) {
    const envelope = body as ManagerApiErrorEnvelope;
    throw new ManagerApiError({
      code: envelope.error?.code ?? "request_failed",
      message: envelope.error?.message ?? response.statusText,
      status: response.status,
      details: envelope.error?.details,
    });
  }

  return body as T;
}

async function readJSON(response: Response) {
  if (response.status === 204) return undefined;
  const text = await response.text();

  if (text === "") return undefined;

  return JSON.parse(text) as unknown;
}

function buildURL(baseUrl: string, path: string, query?: ManagerApiQuery) {
  const url = `${baseUrl}/${path.replace(/^\/+/, "")}`;
  const params = new URLSearchParams();

  for (const [key, value] of Object.entries(query ?? {})) {
    if (value !== undefined && value !== null && value !== "") {
      params.set(key, String(value));
    }
  }

  const queryString = params.toString();
  return queryString ? `${url}?${queryString}` : url;
}

function trimTrailingSlash(value: string) {
  return value.replace(/\/+$/, "");
}
