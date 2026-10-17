import { type NextRequest } from "next/server";

/**
 * Whether this browser carries a session cookie at all (T13).
 *
 * The auth cookies are HttpOnly, so the page cannot see them, and asking the
 * gateway "who am I" without one costs two 401s (/me, then /auth/refresh) that
 * every browser logs as errors on the login screen. The web server CAN see them:
 * it answers whether one is there — never what it holds — and always with a
 * 200, so a visitor who never signed in sees a clean console.
 *
 * It lives under /api/v1/auth because gl_refresh is set with that Path
 * (auth.go): anywhere else the browser would never send it. As a static
 * segment it takes precedence over the gateway proxy's catch-all
 * app/api/[...path]; the gateway has no route of this name.
 *
 * Presence is a hint, not authentication: the gateway still decides whether
 * the session is alive. Cookies are host-scoped, not port-scoped, so this also
 * holds locally, where the gateway is on another port of the same host.
 */

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

/** The gateway's session cookies (services/gateway/internal/handler/auth.go). */
const SESSION_COOKIES = ["gl_access", "gl_refresh"] as const;

export function GET(request: NextRequest): Response {
  const present = SESSION_COOKIES.some((name) => request.cookies.has(name));
  return new Response(JSON.stringify({ present }), {
    status: 200,
    headers: { "content-type": "application/json", "cache-control": "no-store" },
  });
}
