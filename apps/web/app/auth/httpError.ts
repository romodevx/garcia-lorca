/**
 * An HTTP status that came back from the gateway, kept with the error.
 *
 * It exists so callers can tell a REFUSAL from a FAILURE. `Error("refresh: HTTP
 * 500")` forced everyone who cared to parse that sentence or, far more often,
 * to give up and treat every rejection the same — which is how a busy database
 * during a session restore became "you are signed out" and sent somebody who
 * had a perfectly good session back to the login screen.
 *
 * Its own module, not api.ts, because the tests that need to construct one mock
 * api.ts wholesale; a class defined there would be replaced along with it.
 */
export class HttpError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "HttpError";
    this.status = status;
  }
}

/** The statuses that mean the server looked at the credential and said no. */
const REFUSED_STATUSES = new Set([401, 403]);

/**
 * Whether the server REFUSED the request, as opposed to failing to answer it.
 *
 * Anything that is not a recognised refusal — a 500, a 503, a 429, a dropped
 * connection, an error with no status at all — is a failure. Erring that way is
 * deliberate: mistaking a failure for a refusal ends a live session, and
 * mistaking a refusal for a failure only costs a retry that will be refused
 * again.
 */
export function isRefusal(err: unknown): boolean {
  return err instanceof HttpError && REFUSED_STATUSES.has(err.status);
}
