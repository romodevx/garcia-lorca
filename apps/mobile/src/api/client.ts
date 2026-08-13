import { refresh } from "./auth";
import { apiUrl } from "./config";
import { getAccessToken } from "./tokenStore";

/**
 * Authenticated fetch wrapper. It attaches the stored access token as a Bearer
 * header and, on a 401, transparently rotates the session once and retries the
 * original request — so a caller never has to handle an expired access token.
 *
 * The refresh is shared across concurrent callers via a single in-flight promise,
 * so a burst of 401s triggers exactly one rotation, not a stampede.
 */

let refreshInFlight: Promise<boolean> | null = null;

function refreshOnce(): Promise<boolean> {
  refreshInFlight ??= refresh().finally(() => {
    refreshInFlight = null;
  });
  return refreshInFlight;
}

// A plain object (not the Headers global) keeps this working under the jest-expo
// environment, where the whatwg Headers class is not guaranteed to be present.
function withBearer(init: RequestInit): RequestInit {
  const token = getAccessToken();
  const headers: Record<string, string> = { ...(init.headers as Record<string, string> | undefined) };
  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }
  return { ...init, headers };
}

/**
 * Fetches an API path with the Bearer token attached. On a 401 it refreshes once
 * and replays the request; if the refresh fails it returns the original 401 so the
 * caller can route the user back to sign-in.
 */
export async function apiFetch(path: string, init: RequestInit = {}): Promise<Response> {
  const res = await fetch(apiUrl(path), withBearer(init));
  if (res.status !== 401) {
    return res;
  }
  const refreshed = await refreshOnce();
  if (!refreshed) {
    // The dead session is signalled globally via emitSessionLost() inside
    // refresh() (api/session), which the app root uses to redirect to Login;
    // here we just surface the original 401 to the caller.
    return res;
  }
  return fetch(apiUrl(path), withBearer(init));
}
