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

describe("when the refresh merely FAILS", () => {
  // THE DEFECT THIS PINS. Every non-200 used to mean "you are signed out": the
  // refresh was read as `res.ok`, so a 500 from a busy database, a 429 from the
  // throttle, or a dropped connection all announced a lost session and put
  // somebody back on the login screen with a session that was perfectly valid.
  //
  // It correlated with how many people were working, which is what made it look
  // like other people's logins were ending yours: every active session rotates
  // on a timer, each rotation is another draw against a small connection pool,
  // and whoever drew the failure was thrown out.
  //
  // A refusal is the server saying no. A failure is the server not answering.
  // Only the first one ends a session.
  const failures: Array<[string, () => Promise<Response>]> = [
    ["a 500 from a busy database", async () => new Response("", { status: 500 })],
    ["a 503 while the service wakes up", async () => new Response("", { status: 503 })],
    ["a 429 from the throttle", async () => new Response("", { status: 429 })],
    ["a dropped connection", () => Promise.reject(new TypeError("network error"))],
  ];

  for (const [name, refreshOutcome] of failures) {
    it(`does not end the session on ${name}`, async () => {
      const seen: string[] = [];
      const stop = onSessionLost(() => seen.push("lost"));
      const fetchMock = vi
        .fn()
        .mockResolvedValueOnce(new Response("", { status: 401 })) // original
        .mockImplementationOnce(refreshOutcome); // refresh
      vi.stubGlobal("fetch", fetchMock);

      const out = await authFetch("http://gw/api/v1/config/grade-levels", { method: "POST" });

      expect(seen).toEqual([]);
      // The caller still gets its 401 — nothing downstream changes shape, and
      // the request is retryable because the session was never ended.
      expect(out.status).toBe(401);
      stop();
    });
  }

  // A 403 IS a refusal: the gateway answers it for a refresh token it will not
  // honour. It has to keep ending the session.
  it("still ends the session when the refresh is refused with 403", async () => {
    const seen: string[] = [];
    const stop = onSessionLost(() => seen.push("lost"));
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(new Response("", { status: 401 })) // original
      .mockResolvedValueOnce(new Response("", { status: 403 })); // refresh → refused
    vi.stubGlobal("fetch", fetchMock);

    await authFetch("http://gw/api/v1/config/grade-levels", { method: "POST" });

    expect(seen).toEqual(["lost"]);
    stop();
  });
});

describe("a refused re-entered password is not a dead session", () => {
  // Deleting a level asks for the director's password again. A wrong one comes
  // back as 401 + X-Error-Code: REAUTH_FAILED. Treating that as an expired
  // token rotated the session and replayed the delete — a second wrong-password
  // attempt nobody typed.
  it("hands REAUTH_FAILED back without refreshing or retrying", async () => {
    const lost = vi.fn();
    const off = onSessionLost(lost);
    const fetchMock = vi.fn().mockResolvedValue(new Response("Contraseña incorrecta", { status: 401, headers: { "X-Error-Code": "REAUTH_FAILED" } }));
    vi.stubGlobal("fetch", fetchMock);

    const out = await authFetch("http://localhost:8080/api/v1/config/levels/l1/delete", { method: "POST" });

    expect(out.status).toBe(401);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(lost).not.toHaveBeenCalled();
    off();
    vi.unstubAllGlobals();
  });
});
