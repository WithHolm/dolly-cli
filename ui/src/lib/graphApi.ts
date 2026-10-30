import { ApiError, graph, login } from "./api";

// The portal is a client of the Graph API, as any tool is. It signs in as its own service principal ("Dolly portal") with
// the client credentials grant, and calls /v1.0 with the token. Its calls are not throttled and not in the list of calls: the
// service principal has both flags.

const SCOPE = "https://graph.microsoft.com/.default";
const BASE = "/v1.0";

let token: { value: string; until: number } | null = null;
let pending: Promise<string> | null = null;

async function signIn(): Promise<string> {
  const [{ appId }, info] = await Promise.all([graph.get<{ appId: string }>("/portal"), login.get<{ token: string }>("/info")]);
  const res = await fetch(info.token, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    // Dolly accepts any secret from a client that has none kept. The portal has none to keep.
    body: new URLSearchParams({ grant_type: "client_credentials", client_id: appId, client_secret: "dolly-portal", scope: SCOPE }),
  });
  if (!res.ok) throw new ApiError(`The portal could not sign in to the Graph API (status ${res.status}).`, res.status);
  const t = (await res.json()) as { access_token: string; expires_in: number };
  // one minute before it ends, so a call does not start with a token that is about to stop
  token = { value: t.access_token, until: Date.now() + (t.expires_in - 60) * 1000 };
  return token.value;
}

function accessToken(fresh = false): Promise<string> {
  if (!fresh && token && token.until > Date.now()) return Promise.resolve(token.value);
  return (pending ??= signIn().finally(() => (pending = null)));
}

/** The error of a Graph call: the message is the one that Graph (or Dolly) wrote. */
async function failed(res: Response): Promise<ApiError> {
  let message = `The Graph API answered with status ${res.status}.`;
  try {
    message = (await res.json()).error.message ?? message;
  } catch {
    // the body is not the error of Graph: keep the sentence above
  }
  return new ApiError(message, res.status);
}

async function call(method: string, path: string, eventual: boolean, body?: unknown): Promise<Response> {
  const send = async (fresh: boolean) =>
    fetch(BASE + path, {
      method,
      headers: {
        Authorization: `Bearer ${await accessToken(fresh)}`,
        ...(eventual ? { ConsistencyLevel: "eventual" } : {}),
        ...(body === undefined ? {} : { "Content-Type": "application/json" }),
      },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  let res = await send(false);
  if (res.status === 401) res = await send(true); // the token ended early (a restart, or a new tenant ID): sign in again
  return res;
}

/** GET a Graph path (for example `/users?$top=5`). `eventual` adds the header ConsistencyLevel: eventual (an advanced query). */
export async function graphGet<T>(path: string, eventual = false): Promise<T> {
  const res = await call("GET", path, eventual);
  if (!res.ok) throw await failed(res);
  return (await res.json()) as T;
}

/** POST, PATCH, or DELETE on a Graph path. The writes that the portal does answer 204 with no body. */
export async function graphSend(method: "POST" | "PATCH" | "DELETE", path: string, body?: unknown): Promise<void> {
  const res = await call(method, path, false, body);
  if (!res.ok) throw await failed(res);
}

export const graphPatch = (path: string, body: unknown) => graphSend("PATCH", path, body);

export interface BatchRequest {
  id: string;
  url: string; // without /v1.0
  headers?: Record<string, string>;
}
export interface BatchResponse<T = unknown> {
  id: string;
  status: number;
  body: T;
}

/** Many GETs in one call (at most 20 each, as in Graph). The answers come in the order of the requests. */
export async function graphBatch<T = unknown>(requests: BatchRequest[]): Promise<BatchResponse<T>[]> {
  const out: BatchResponse<T>[] = [];
  for (let i = 0; i < requests.length; i += 20) {
    const chunk = requests.slice(i, i + 20).map((r) => ({ method: "GET", ...r }));
    const res = await call("POST", "/$batch", false, { requests: chunk });
    if (!res.ok) throw await failed(res);
    const { responses } = (await res.json()) as { responses: BatchResponse<T>[] };
    const byId = new Map(responses.map((r) => [r.id, r]));
    out.push(...chunk.map((r) => byId.get(r.id)!));
  }
  return out;
}

/** A value that is in a Graph list: the list has `value`, and a next page has `@odata.nextLink`. */
export interface GraphList<T> {
  value: T[];
  "@odata.count"?: number;
  "@odata.nextLink"?: string;
}

/** Every item of a list, with the next pages read. nextLink is a full URL: the portal keeps only the path and the query. */
export async function graphAll<T>(path: string, eventual = false): Promise<T[]> {
  const items: T[] = [];
  let next: string | undefined = path;
  while (next) {
    const page: GraphList<T> = await graphGet<GraphList<T>>(next, eventual);
    items.push(...page.value);
    next = page["@odata.nextLink"] ? new URL(page["@odata.nextLink"]).pathname.replace(BASE, "") + new URL(page["@odata.nextLink"]).search : undefined;
  }
  return items;
}
