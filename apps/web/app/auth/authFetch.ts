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

/**
 * Subscribers told that the session is gone for good.
 *
 * A REFUSED REFRESH is the one unambiguous "you are signed out" moment there
 * is, and until now nobody was told about it: authFetch handed the 401 back to
 * whoever called, and every panel wraps its calls in a blanket catch that
 * reports a domain error. So a session that had simply expired came out as "No
 * se pudo crear el grado. Revisa que no exista ya con ese orden" — the app
 * blaming the user's data for its own dead session (dev, 2026-09-07).
 *
 * Announced HERE because this is the only place that knows the difference
 * between "this one call was refused" and "there is no session any more".
 */
type SessionLostListener = () => void;

const sessionLostListeners = new Set<SessionLostListener>();

/** Subscribe. Returns the unsubscribe, so effects can clean up. */
export function onSessionLost(listener: SessionLostListener): () => void {
  sessionLostListeners.add(listener);
  return () => sessionLostListeners.delete(listener);
}

function announceSessionLost(): void {
  for (const listener of sessionLostListeners) {
    // One bad listener must not stop the others from hearing it.
    try {
      listener();
    } catch {
      // ignored on purpose
    }
  }
}

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
  if (!refreshed) {
    // The refresh cookie is gone or refused: this is not a slow token, it is a
    // finished session. Say so once; the caller still gets its 401 so nothing
    // downstream changes shape.
    announceSessionLost();
    return res;
  }
  return fetch(input, init);
}
