import { afterEach, describe, expect, it, vi } from "vitest";

import { loginWithProvider } from "./provider";

afterEach(() => vi.restoreAllMocks());

/**
 * Signing in through the identity provider (F4 of
 * .claude/docs/PLAN-identity-platform.md).
 *
 * The person types their email and password into the SAME form as before. What
 * changes is who checks them: the provider does, and hands back an ID token,
 * which we exchange at the gateway for the HttpOnly cookies the app already
 * runs on.
 *
 * That exchange is the whole reason this function exists rather than the app
 * carrying the provider's token around. The provider's SDK keeps that token in
 * IndexedDB, readable by any script on the page; our cookies are not readable at
 * all. The token must touch the gateway once and then be forgotten.
 *
 * Getting the token is injected, so these tests never reach the network and the
 * exchange is what gets asserted — not Firebase.
 */

const okResponse = (body: unknown): Response =>
  new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });

describe("loginWithProvider", () => {
  it("exchanges the provider's id token for our session cookies", async () => {
    const fetchMock = vi.fn().mockResolvedValue(okResponse({ expiresIn: 900, mustChangePassword: false }));
    vi.stubGlobal("fetch", fetchMock);
    const getIdToken = vi.fn().mockResolvedValue("id-token-abc");

    const res = await loginWithProvider("http://gw", "ana@garcialorca.mx", "pw", getIdToken);

    expect(getIdToken).toHaveBeenCalledWith("ana@garcialorca.mx", "pw");
    expect(fetchMock).toHaveBeenCalledWith(
      "http://gw/api/v1/auth/google",
      expect.objectContaining({ method: "POST", credentials: "include" }),
    );
    // credentials:"include" is what lets the browser STORE the Set-Cookie the
    // gateway replies with. Without it the exchange succeeds and the session is
    // dropped on the floor.
    expect(res).toEqual({ expiresIn: 900, mustChangePassword: false });
  });

  it("sends the id token and never the password", async () => {
    const fetchMock = vi.fn().mockResolvedValue(okResponse({ expiresIn: 900, mustChangePassword: false }));
    vi.stubGlobal("fetch", fetchMock);

    await loginWithProvider("http://gw", "ana@garcialorca.mx", "hunter2", vi.fn().mockResolvedValue("id-token-abc"));

    const body = String((fetchMock.mock.calls[0]?.[1] as RequestInit).body);
    expect(body).toContain("id-token-abc");
    // The password went to the provider and has no business reaching our API.
    // Sending it too would put it in our logs and our request history for no
    // gain, and would keep us in the credential-handling business we are
    // leaving.
    expect(body).not.toContain("hunter2");
  });

  it("returns no tokens to the caller", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(okResponse({ expiresIn: 900, mustChangePassword: false })));

    const res = await loginWithProvider("http://gw", "ana@garcialorca.mx", "pw", vi.fn().mockResolvedValue("t"));

    // Same contract as the password login: the session lives in cookies JS
    // cannot read, and the body carries only metadata.
    expect(res).not.toHaveProperty("accessToken");
    expect(res).not.toHaveProperty("idToken");
  });

  it("fails when the provider rejects the credentials, without calling the gateway", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const getIdToken = vi.fn().mockRejectedValue(new Error("auth/wrong-password"));

    await expect(
      loginWithProvider("http://gw", "ana@garcialorca.mx", "mal", getIdToken),
    ).rejects.toThrow();
    // No token, nothing to exchange. Calling the gateway anyway would be a
    // wasted round trip against the shared login rate limit.
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("fails when the gateway refuses the token", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 401 })));

    // The provider vouched for them, we did not. Somebody with a valid provider
    // account but no user row here must not end up signed in.
    await expect(
      loginWithProvider("http://gw", "ajeno@otro.com", "pw", vi.fn().mockResolvedValue("t")),
    ).rejects.toThrow(/401/);
  });
});
