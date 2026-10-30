/** An answer that is not a success. The message is the one that Dolly (or Graph) wrote. */
export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message);
  }
}

async function request<T>(base: string, method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(base + path, {
    method,
    // the admin API refuses a write that is not JSON (another web page cannot send that)
    headers: { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) {
    let message = `The server answered with status ${res.status}.`;
    try {
      message = (await res.json()).error.message ?? message;
    } catch {
      // the body is not the error of Graph: keep the sentence above
    }
    throw new ApiError(message, res.status);
  }
  return (res.status === 204 ? undefined : await res.json()) as T;
}

/** A client for the admin API of one API: /_dolly/<name>. */
export function adminClient(name: string) {
  const base = `/_dolly/${name}`;
  return {
    get: <T>(path: string) => request<T>(base, "GET", path),
    post: <T = void>(path: string, body?: unknown) => request<T>(base, "POST", path, body),
    put: <T = void>(path: string, body?: unknown) => request<T>(base, "PUT", path, body),
    del: <T = void>(path: string) => request<T>(base, "DELETE", path),
  };
}

export const graph = adminClient("graph");
export const login = adminClient("login");
