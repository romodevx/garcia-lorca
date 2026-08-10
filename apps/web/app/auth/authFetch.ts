/**
 * fetch that transparently recovers an expired access token. Access cookies live
 * ~15 min; without this every request after that failed with a 401 until a full
 * page reload. On a 401, authFetch rotates the session via /auth/refresh (using
 * the longer-lived refresh cookie) and retries the original request once.
 *
 * The refresh is shared across concurrent callers (a single in-flight promise),
 * so a burst of 401s triggers exactly one rotation, not a stampede.
 */
const REFRESH_PATH = "/api/v1/auth/refresh";
const LOGIN_PATH = "/api/v1/auth/login";

let refreshInFlight: Promise<boolean> | null = null;

function refreshOnce(origin: string): Promise<boolean> {
  refreshInFlight ??= fetch(`${origin}${REFRESH_PATH}`, { method: "POST", credentials: "include" })
    .then((res) => res.ok)
    .catch(() => false)
    .finally(() => {
      refreshInFlight = null;
    });
  return refreshInFlight;
}

export async function authFetch(input: string, init?: RequestInit): Promise<Response> {
  const res = await fetch(input, init);
  if (res.status !== 401) return res;
  // Never try to refresh the auth calls themselves — that would recurse.
  if (input.includes(REFRESH_PATH) || input.includes(LOGIN_PATH)) return res;

  const origin = new URL(input, globalThis.location?.href ?? "http://localhost").origin;
  const refreshed = await refreshOnce(origin);
  if (!refreshed) return res;
  return fetch(input, init);
}
