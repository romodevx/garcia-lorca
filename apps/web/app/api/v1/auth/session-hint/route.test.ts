import { describe, expect, it } from "vitest";
import { NextRequest } from "next/server";

import { GET } from "./route";

function request(cookie?: string): NextRequest {
  return new NextRequest("https://web.example/api/v1/auth/session-hint", cookie === undefined ? {} : { headers: { cookie } });
}

describe("GET /api/v1/auth/session-hint — is there a session cookie to probe with (T13)", () => {
  it.each([
    ["the access cookie", "gl_access=secret-a", true],
    // The access cookie lives minutes; a returning visitor often holds only the
    // 24 h refresh cookie, which the browser sends under /api/v1/auth alone.
    ["only the refresh cookie", "other=1; gl_refresh=secret-r", true],
    ["no session cookie", "other=1", false],
    ["no cookie at all", undefined, false],
  ])("%s → present %s, always a 200", async (_name, cookie, present) => {
    const res = await GET(request(cookie));
    expect(res.status).toBe(200);
    expect(await res.json()).toEqual({ present });
  });

  it("never echoes a cookie's value, and is never cached", async () => {
    const res = await GET(request("gl_access=secret-a; gl_refresh=secret-r"));
    expect(await res.text()).not.toMatch(/secret/);
    expect(res.headers.get("cache-control")).toBe("no-store");
  });
});
