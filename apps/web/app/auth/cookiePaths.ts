/**
 * The path the gateway sets the refresh cookie (gl_refresh) under:
 * `refreshPath` in services/gateway/internal/handler/auth.go. The browser sends
 * that cookie ONLY to URLs below it, so anything on the web origin that must
 * see it (the session hint) has to live here too. Change both together.
 */
export const REFRESH_COOKIE_PATH = "/api/v1/auth";
