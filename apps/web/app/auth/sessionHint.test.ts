import { afterEach, describe, expect, it, vi } from "vitest";

import { REFRESH_COOKIE_PATH } from "./cookiePaths";
import { mayHaveSession, SESSION_HINT_PATH } from "./sessionHint";

afterEach(() => vi.unstubAllGlobals());

const answer = (body: unknown, status = 200): Response => new Response(JSON.stringify(body), { status });

describe("mayHaveSession (T13: no 401 probes before anyone signed in)", () => {
  it("is false only when the web server says there is no session cookie", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(answer({ present: false })));
    expect(await mayHaveSession()).toBe(false);
  });

  // gl_refresh is set with Path=/api/v1/auth: a hint anywhere else never sees
  // it, and a visitor whose access cookie expired would be sent to the login.
  it("lives under the refresh cookie's path, so the browser sends that cookie to it", () => {
    expect(REFRESH_COOKIE_PATH).toBe("/api/v1/auth");
    expect(SESSION_HINT_PATH.startsWith(`${REFRESH_COOKIE_PATH}/`)).toBe(true);
  });

  it("asks its own origin, never the gateway", async () => {
    const fetchMock = vi.fn().mockResolvedValue(answer({ present: true }));
    vi.stubGlobal("fetch", fetchMock);
    expect(await mayHaveSession()).toBe(true);
    expect(fetchMock).toHaveBeenCalledWith("/api/v1/auth/session-hint", expect.objectContaining({ cache: "no-store", credentials: "same-origin" }));
  });

  // Unsure is "maybe": a live session must never be sent to the login screen.
  it.each([
    ["the server fails", () => Promise.resolve(answer({ error: "x" }, 500))],
    ["the network fails", () => Promise.reject(new TypeError("offline"))],
    ["an unexpected body", () => Promise.resolve(answer({}))],
  ])("is true when %s", async (_name, impl) => {
    vi.stubGlobal("fetch", vi.fn(impl));
    expect(await mayHaveSession()).toBe(true);
  });
});
