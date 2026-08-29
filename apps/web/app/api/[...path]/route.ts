import { type NextRequest } from "next/server";

/**
 * Same-origin proxy from the web app to the gateway.
 *
 * WHY THIS EXISTS — do not delete it as redundant. The auth cookies the gateway
 * sets are `HttpOnly; Secure; SameSite=Strict` with no Domain attribute
 * (services/gateway/internal/handler/auth.go). `run.app` is on the Public Suffix
 * List, so `web-x.run.app` and `gateway-y.run.app` are DIFFERENT SITES: a
 * browser would never send those cookies from one to the other. Login would
 * appear to succeed and every subsequent /me would return 401.
 *
 * Routing the API through the web app's own origin makes the cookies
 * first-party. The alternative is a custom domain fronting both, which costs
 * money and DNS; this costs one file. When a real domain exists, this can go.
 *
 * GATEWAY_URL is read PER REQUEST, not baked at build time, so pointing the web
 * app at a different gateway is `gcloud run services update --set-env-vars`
 * rather than an image rebuild. That also removes the deploy ordering problem: a
 * Next `rewrites()` destination is frozen into routes-manifest.json at build
 * time, but the gateway's URL only exists after `terraform apply`.
 */

// Never cache. Beyond correctness this is a security control: a cached
// /api/v1/me response served to a different visitor is a session leak.
export const dynamic = "force-dynamic";
export const runtime = "nodejs";

/** Only the gateway's versioned public surface may be reached through here. */
const ALLOWED_PREFIX = "/api/v1/";

/**
 * Request headers that must NOT be forwarded upstream.
 *
 * This started as an allowlist, which looked safer and quietly broke the app: the
 * gateway requires `Idempotency-Key` on a uniforms counter sale, the allowlist had
 * never heard of it, and the sale came back as a 400 blaming the caller. Any
 * allowlist here is a promise to predict every header the API will ever use, and
 * that promise cannot be kept.
 *
 * A denylist is the standard reverse-proxy contract and keeps the one that
 * actually matters out: `Host`. Cloud Run routes by it, so forwarding the
 * client's makes Cloud Run's frontend answer 404 with no application log on
 * either side. The rest are hop-by-hop headers that describe THIS connection and
 * are meaningless on the next one.
 */
const BLOCKED_REQUEST_HEADERS = new Set([
  "host",
  "connection",
  "keep-alive",
  "transfer-encoding",
  "upgrade",
  "proxy-authorization",
  "proxy-connection",
  "content-length", // fetch recomputes it for the body it actually sends
]);

/**
 * Response headers that must NOT be copied through.
 * `content-encoding` is the subtle one: fetch decompresses the upstream body
 * transparently, so re-emitting `gzip` alongside plain bytes gives the browser
 * ERR_CONTENT_DECODING_FAILED — on every response large enough to be compressed,
 * which means the student roster and the charges list but not the login.
 */
const STRIPPED_RESPONSE_HEADERS = new Set([
  "content-encoding",
  "content-length",
  "transfer-encoding",
  "connection",
  "keep-alive",
]);

const UPSTREAM_TIMEOUT_MS = 30_000;
const METHODS_WITHOUT_BODY = new Set(["GET", "HEAD"]);

function json(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json", "cache-control": "no-store" },
  });
}

/** Rejects encoded traversal. Literal `../` is normalised away before routing. */
function hasEncodedTraversal(pathname: string): boolean {
  return /%2e/i.test(pathname) || /%2f/i.test(pathname) || pathname.includes("..");
}

/**
 * Next 16 hands route params as a PROMISE. Destructuring it synchronously throws
 * at runtime, not at build time, so the type is spelled out here deliberately.
 */
interface RouteContext {
  params: Promise<{ path: string[] }>;
}

async function proxy(request: NextRequest, context: RouteContext): Promise<Response> {
  const gatewayURL = process.env.GATEWAY_URL?.replace(/\/+$/, "");
  if (!gatewayURL) {
    // Loud on purpose: a misconfigured gateway must not be mistaken for an auth
    // failure by whoever is looking at the screen.
    return json(500, { error: "gateway_not_configured" });
  }

  const { pathname, search } = request.nextUrl;
  // Two independent checks on purpose. The pathname is what actually gets
  // forwarded, so it is the one that matters; the matched segments are a second
  // opinion from Next's own router in case normalisation ever differs.
  const { path } = await context.params;
  if (!pathname.startsWith(ALLOWED_PREFIX) || path[0] !== "v1" || hasEncodedTraversal(pathname)) {
    return json(404, { error: "not_found" });
  }

  const headers = new Headers();
  request.headers.forEach((value, name) => {
    if (!BLOCKED_REQUEST_HEADERS.has(name.toLowerCase())) headers.set(name, value);
  });

  const init: RequestInit & { duplex?: "half" } = {
    method: request.method,
    headers,
    redirect: "manual",
    cache: "no-store",
    signal: AbortSignal.timeout(UPSTREAM_TIMEOUT_MS),
  };
  if (!METHODS_WITHOUT_BODY.has(request.method)) {
    // Stream rather than buffer. `duplex: "half"` is REQUIRED when the body is a
    // stream; without it fetch throws on every POST.
    init.body = request.body;
    init.duplex = "half";
  }

  let upstream: Response;
  try {
    upstream = await fetch(`${gatewayURL}${pathname}${search}`, init);
  } catch {
    return json(502, { error: "gateway_unavailable" });
  }

  const responseHeaders = new Headers();
  upstream.headers.forEach((value, name) => {
    const lower = name.toLowerCase();
    // set-cookie is handled below: forEach collapses repeated headers into one
    // comma-joined string, which would merge gl_access and gl_refresh into a
    // single malformed cookie and silently kill the session on refresh.
    if (lower === "set-cookie" || STRIPPED_RESPONSE_HEADERS.has(lower)) return;
    responseHeaders.set(name, value);
  });
  for (const cookie of upstream.headers.getSetCookie()) {
    responseHeaders.append("set-cookie", cookie);
  }

  return new Response(upstream.body, { status: upstream.status, headers: responseHeaders });
}

// Route Handlers dispatch per exported verb; anything not exported answers 405.
// The gateway routes PUT (tuition plans, concepts, uniform items and settings),
// so exporting only GET/POST would 405 exactly the money screens.
export const GET = proxy;
export const POST = proxy;
export const PUT = proxy;
export const PATCH = proxy;
export const DELETE = proxy;
export const HEAD = proxy;
export const OPTIONS = proxy;
