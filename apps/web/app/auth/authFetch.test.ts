import { afterEach, describe, expect, it, vi } from "vitest";
import { authFetch, onSessionLost } from "./authFetch";

function res(status: number): Response {
  return new Response(null, { status });
}

afterEach(() => vi.restoreAllMocks());

describe("authFetch", () => {
  it("passes a successful response through untouched", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(res(200));
    const out = await authFetch("http://localhost:8080/api/v1/students");
    expect(out.status).toBe(200);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("on 401 refreshes once and retries the original request", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(res(401)) // original → expired
      .mockResolvedValueOnce(res(200)) // refresh → ok
      .mockResolvedValueOnce(res(200)); // retry → ok
    const out = await authFetch("http://localhost:8080/api/v1/students");
    expect(out.status).toBe(200);
    expect(fetchMock).toHaveBeenCalledTimes(3);
    expect(String(fetchMock.mock.calls[1]?.[0])).toContain("/api/v1/auth/refresh");
  });

  it("returns the 401 when refresh fails (no session to recover)", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(res(401)) // original
      .mockResolvedValueOnce(res(401)); // refresh → fail
    const out = await authFetch("http://localhost:8080/api/v1/students");
    expect(out.status).toBe(401);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("does not try to refresh a 401 from the refresh endpoint itself", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(res(401));
    const out = await authFetch("http://localhost:8080/api/v1/auth/refresh", { method: "POST" });
    expect(out.status).toBe(401);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});

describe("when the session is truly gone", () => {
  // The bug this exists for (dev, 2026-09-07): the owner's staff session
  // expired, POST /config/grade-levels came back 401, the refresh came back 401
  // too, and authFetch quietly returned the 401 to the caller. Every panel wraps
  // its calls in `catch { setError("...no exista ya con ese orden") }`, so the
  // app blamed the owner's data for a dead session. He asked what "orden" even
  // meant, which is the right question to ask a message that made no sense.
  //
  // A refused refresh is the one unambiguous "you are signed out" moment there
  // is. Somebody has to be told, and only authFetch knows it happened.
  it("announces it once, so the app can say so instead of guessing", async () => {
    const seen: string[] = [];
    const stop = onSessionLost(() => seen.push("lost"));
    // The original call 401s, and so does the refresh.
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 401 })));

    const res = await authFetch("http://gw/api/v1/config/grade-levels", { method: "POST" });

    expect(res.status).toBe(401);
    expect(seen).toEqual(["lost"]);
    stop();
  });

  it("stays quiet when the refresh WORKS — that is not a lost session", async () => {
    const seen: string[] = [];
    const stop = onSessionLost(() => seen.push("lost"));
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(new Response("", { status: 401 })) // original
      .mockResolvedValueOnce(new Response("", { status: 200 })) // refresh
      .mockResolvedValueOnce(new Response("", { status: 200 })); // retry
    vi.stubGlobal("fetch", fetchMock);

    await authFetch("http://gw/api/v1/config/grade-levels", { method: "POST" });

    expect(seen).toEqual([]);
    stop();
  });

  it("stays quiet on the login call itself — a wrong password is not a lost session", async () => {
    const seen: string[] = [];
    const stop = onSessionLost(() => seen.push("lost"));
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 401 })));

    await authFetch("http://gw/api/v1/auth/login", { method: "POST" });

    expect(seen).toEqual([]);
    stop();
  });
});
