/** What went wrong with a finance write, as far as the screen needs to tell. */
export type FailureKind = "conflict" | "forbidden" | "network" | "other";

const CONFLICT = 409;
const FORBIDDEN = 403;

/**
 * Sorts a rejected finance call: the gateway's 409 (already done, or a
 * duplicate), its 403, a request that never reached it (fetch's TypeError), or
 * anything else. Reads the `status` the api client attaches to its errors.
 */
export function failureKind(cause: unknown): FailureKind {
  if (cause instanceof TypeError) return "network";
  const status = typeof cause === "object" && cause !== null && "status" in cause ? cause.status : undefined;
  if (status === CONFLICT) return "conflict";
  if (status === FORBIDDEN) return "forbidden";
  return "other";
}
