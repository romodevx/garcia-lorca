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
 * A 401 that refuses a RE-ENTERED password (deleting a level asks for the
 * director's again), not the session's token. Refreshing and replaying it would
 * file a second wrong-password attempt nobody typed.
 */
const REAUTH_FAILED = "REAUTH_FAILED";

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
 * between "this one call was refused" and "there is no session any more" — and
 * between either of those and a refresh that simply did not get an answer. See
 * RefreshOutcome: that third case must never reach these listeners.
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

/**
 * What came back from a rotation attempt. THREE outcomes, not two, and the
 * third one is the bug this type exists to make unrepresentable.
 *
 *  - "rotated": a new pair was issued. Retry the original call.
 *  - "refused": the server looked at the refresh token and said no (401/403).
 *    The session is over. This is the only outcome that ends it.
 *  - "failed":  the server did not answer — a 500 from a busy database, a 503
 *    while an instance wakes up, a 429 from the throttle, a dropped
 *    connection. NOTHING is known about the session, so nothing is decided
 *    about it.
 *
 * "failed" used to be folded into "refused" (the whole thing was `res.ok`), and
 * that is how a database hiccup during a rotation put somebody back on the
 * login screen holding a session that was perfectly valid. It got worse the
 * busier the school was — every active session rotates on a timer, each
 * rotation is another draw against a small connection pool — so it read as
 * other people's logins ending yours.
 */
type RefreshOutcome = "rotated" | "refused" | "failed";

/** The statuses that mean the server REFUSED the refresh token, not that it failed. */
const REFUSED_STATUSES = new Set([401, 403]);

let refreshInFlight: Promise<RefreshOutcome> | null = null;

function refreshOnce(origin: string): Promise<RefreshOutcome> {
  refreshInFlight ??= fetch(`${origin}${REFRESH_PATH}`, { method: "POST", credentials: "include" })
    .then((res): RefreshOutcome => {
      if (res.ok) return "rotated";
      return REFUSED_STATUSES.has(res.status) ? "refused" : "failed";
    })
    // A rejected fetch is the network, never a verdict on the session.
    .catch((): RefreshOutcome => "failed")
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
  if (res.headers.get("X-Error-Code") === REAUTH_FAILED) return res;

  const origin = new URL(input, globalThis.location?.href ?? "http://localhost").origin;
  const outcome = await refreshOnce(origin);
  if (outcome === "rotated") return fetch(input, init);

  if (outcome === "refused") {
    // The refresh cookie is gone or refused: this is not a slow token, it is a
    // finished session. Say so once.
    announceSessionLost();
  }
  // Either way the caller gets its 401, so nothing downstream changes shape. On
  // "failed" that 401 is a retryable error and the session is left alone.
  return res;
}
