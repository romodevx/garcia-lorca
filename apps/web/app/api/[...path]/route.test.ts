import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { NextRequest } from "next/server";

import { DELETE, GET, PATCH, POST, PUT } from "./route";

const GATEWAY = "https://gateway-abc.us-central1.run.app";

function ctx(path: string[]): { params: Promise<{ path: string[] }> } {
  // Next 16 hands params as a Promise. Destructuring it synchronously throws at
  // runtime, not build time — so the tests must await it the same way.
  return { params: Promise.resolve({ path }) };
}

function upstream(init: { status?: number; headers?: [string, string][]; body?: string } = {}) {
  const headers = new Headers();
  for (const [k, v] of init.headers ?? []) headers.append(k, v);
  return new Response(init.body ?? "{}", { status: init.status ?? 200, headers });
}

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  vi.stubEnv("GATEWAY_URL", GATEWAY);
  fetchMock = vi.fn().mockResolvedValue(upstream());
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("same-origin gateway proxy", () => {
  // The whole reason this route exists: the auth cookies are SameSite=Strict and
  // run.app is on the Public Suffix List, so web-x.run.app and gateway-y.run.app
  // are different sites and the browser would never send them.
  it("re-emits BOTH Set-Cookie headers separately", async () => {
    fetchMock.mockResolvedValue(
      upstream({
        headers: [
          ["set-cookie", "gl_access=aaa; Path=/; HttpOnly; Secure; SameSite=Strict"],
          ["set-cookie", "gl_refresh=bbb; Path=/api/v1/auth; HttpOnly; Secure; SameSite=Strict"],
        ],
      }),
    );

    const res = await POST(
      new NextRequest(`https://web.example/api/v1/auth/login`, { method: "POST", body: "{}" }),
      ctx(["v1", "auth", "login"]),
    );

    const cookies = res.headers.getSetCookie();
    expect(cookies).toHaveLength(2);
    expect(cookies[0]).toContain("gl_access=aaa");
    expect(cookies[1]).toContain("gl_refresh=bbb");
    // Joining them into one string is the failure that looks like a working
    // login and then kills the session ~15 minutes later when refresh fails.
    expect(cookies[1]).not.toContain("gl_access");
  });

  it("forwards the query string", async () => {
    // GET /api/v1/finance/charges REQUIRES a filter (bounded-reads guard), so
    // dropping the search string turns the first screen after login into a 400.
    await GET(
      new NextRequest(`https://web.example/api/v1/finance/charges?cycleId=abc&limit=50`),
      ctx(["v1", "finance", "charges"]),
    );

    expect(fetchMock.mock.calls[0]?.[0]).toBe(`${GATEWAY}/api/v1/finance/charges?cycleId=abc&limit=50`);
  });

  it("does not forward the client Host header", async () => {
    // Cloud Run routes by Host. Forwarding web-x.run.app to the gateway makes
    // Cloud Run's frontend answer 404 with no application log anywhere.
    await GET(
      new NextRequest(`https://web.example/api/v1/me`, { headers: { host: "web.example", cookie: "gl_access=x" } }),
      ctx(["v1", "me"]),
    );

    const sent = new Headers(fetchMock.mock.calls[0]?.[1]?.headers as HeadersInit);
    expect(sent.get("host")).toBeNull();
    expect(sent.get("cookie")).toBe("gl_access=x");
  });

  it("forwards application headers it has never heard of", async () => {
    // A narrow allowlist looked safe and silently broke the money path: the
    // gateway REQUIRES Idempotency-Key on a uniforms counter sale and rejects the
    // request without it, so the proxy dropping the header turned a working sale
    // into a 400 that pointed at the caller rather than at the proxy.
    await POST(
      new NextRequest(`https://web.example/api/v1/uniforms/counter-sale`, {
        method: "POST",
        body: "{}",
        headers: { "idempotency-key": "sale-42", "x-custom-thing": "keep-me" },
      }),
      ctx(["v1", "uniforms", "counter-sale"]),
    );

    const sent = new Headers(fetchMock.mock.calls[0]?.[1]?.headers as HeadersInit);
    expect(sent.get("idempotency-key")).toBe("sale-42");
    expect(sent.get("x-custom-thing")).toBe("keep-me");
  });

  it("strips hop-by-hop and encoding headers from the response", async () => {
    // undici transparently gunzips upstream. Re-emitting content-encoding gives
    // the browser ERR_CONTENT_DECODING_FAILED on anything big enough to be
    // compressed — the roster and the charges list, not the login.
    fetchMock.mockResolvedValue(
      upstream({
        headers: [
          ["content-encoding", "gzip"],
          ["content-length", "9999"],
          ["transfer-encoding", "chunked"],
          ["connection", "keep-alive"],
          ["content-type", "application/json"],
        ],
      }),
    );

    const res = await GET(new NextRequest(`https://web.example/api/v1/students`), ctx(["v1", "students"]));

    expect(res.headers.get("content-encoding")).toBeNull();
    expect(res.headers.get("content-length")).toBeNull();
    expect(res.headers.get("transfer-encoding")).toBeNull();
    expect(res.headers.get("connection")).toBeNull();
    expect(res.headers.get("content-type")).toBe("application/json");
  });

  it("supports every verb the gateway routes, including PUT", async () => {
    // The gateway has PUT routes for tuition plans, concepts and uniform items.
    // A handler exporting only GET/POST answers 405 on the money screens.
    for (const [name, fn] of [["PUT", PUT], ["PATCH", PATCH], ["DELETE", DELETE]] as const) {
      fetchMock.mockClear();
      const res = await fn(
        new NextRequest(`https://web.example/api/v1/finance/tuition-plans`, {
          method: name,
          ...(name === "DELETE" ? {} : { body: "{}" }),
        }),
        ctx(["v1", "finance", "tuition-plans"]),
      );
      expect(res.status, `${name} should be proxied`).toBe(200);
      expect(fetchMock.mock.calls[0]?.[1]?.method).toBe(name);
    }
  });

  it("refuses paths outside /api/v1/", async () => {
    const res = await GET(new NextRequest(`https://web.example/api/internal/debug`), ctx(["internal", "debug"]));
    expect(res.status).toBe(404);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("refuses percent-encoded traversal", async () => {
    // Plain ../ is normalised away before routing, so it can never reach here.
    // Encoded segments can.
    const res = await GET(
      new NextRequest(`https://web.example/api/v1/%2e%2e%2f%2e%2e%2fadmin`),
      ctx(["v1", "%2e%2e%2f%2e%2e%2fadmin"]),
    );
    expect(res.status).toBe(404);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("never attaches a body to GET", async () => {
    await GET(new NextRequest(`https://web.example/api/v1/me`), ctx(["v1", "me"]));
    expect(fetchMock.mock.calls[0]?.[1]?.body).toBeUndefined();
  });

  it("returns 502 with a distinguishable payload when the gateway is unreachable", async () => {
    // A blank 500 is indistinguishable from "not authenticated" in the UI.
    fetchMock.mockRejectedValue(new Error("connect ECONNREFUSED"));
    const res = await GET(new NextRequest(`https://web.example/api/v1/me`), ctx(["v1", "me"]));
    expect(res.status).toBe(502);
    await expect(res.json()).resolves.toMatchObject({ error: "gateway_unavailable" });
  });

  it("fails loudly when GATEWAY_URL is not configured", async () => {
    // Misconfiguration must not look like an auth problem.
    vi.stubEnv("GATEWAY_URL", "");
    const res = await GET(new NextRequest(`https://web.example/api/v1/me`), ctx(["v1", "me"]));
    expect(res.status).toBe(500);
    await expect(res.json()).resolves.toMatchObject({ error: "gateway_not_configured" });
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
