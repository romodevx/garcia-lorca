/**
 * The gateway's session cookies, as services/gateway/internal/handler/auth.go
 * declares them (`accessCookie`, `refreshCookie`, `refreshPath`).
 * cookiePaths.contract.test.ts reads that file and fails when they drift.
 *
 * The browser sends gl_refresh ONLY to URLs below REFRESH_COOKIE_PATH, so
 * anything on the web origin that must see it (the session hint) lives there.
 */
export const ACCESS_COOKIE = "gl_access";
export const REFRESH_COOKIE = "gl_refresh";
export const REFRESH_COOKIE_PATH = "/api/v1/auth";
