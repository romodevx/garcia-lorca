/**
 * Transport-level error carrying the HTTP status the gateway returned, so callers
 * (and screens) can branch on it — e.g. a 401 means "sign in again", a 403 means
 * "module or permission unavailable".
 */
export class ApiError extends Error {
  constructor(public readonly status: number, message?: string) {
    super(message ?? `HTTP ${status}`);
    this.name = "ApiError";
  }
}
