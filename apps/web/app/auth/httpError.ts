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
  /** The body the gateway sent with the refusal, when it sent one. */
  readonly serverText?: string;
  /** The gateway's machine code for the refusal (X-Error-Code), when it sent one. */
  readonly errorCode?: string;

  constructor(status: number, message: string, detail: { serverText?: string; errorCode?: string } = {}) {
    super(message);
    this.name = "HttpError";
    this.status = status;
    this.serverText = detail.serverText;
    this.errorCode = detail.errorCode;
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

/** HTTP 429: the gateway's throttle (10 a minute on sensitive writes). */
const TOO_MANY_REQUESTS = 429;

/** What a person reads when the throttle stopped them. */
export const TOO_MANY_ATTEMPTS = "Demasiados intentos. Espera un minuto y vuelve a intentarlo.";

/**
 * The throttle's sentence when `err` is a 429, from any module's error class
 * (each carries a numeric `status`); undefined otherwise. A 429 is neither a
 * dropped connection nor a refusal: waiting is the fix, so the screen must say
 * so instead of "check your connection" (C2-012).
 */
export function tooManyAttempts(err: unknown): string | undefined {
  if (typeof err !== "object" || err === null || !("status" in err)) return undefined;
  return err.status === TOO_MANY_REQUESTS ? TOO_MANY_ATTEMPTS : undefined;
}
