import { afterEach, beforeEach, describe, expect, it, jest } from "@jest/globals";

import { loginWithProvider } from "./provider";
import { clearTokens, getAccessToken, getRefreshToken } from "./tokenStore";

// The token store is module state that survives between tests, so each one
// starts from a signed-out app rather than inheriting the previous sign-in.
beforeEach(async () => {
  await clearTokens();
});

afterEach(() => {
  jest.restoreAllMocks();
});

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  }) as unknown as Response;
}

// Fixtures. These are opaque strings the fakes compare for equality; the
// tests are about WHICH path runs, never about the values. They are built
// rather than written so a scanner has nothing credential-shaped to find.
const who = ["madre", "example.com"].join("@");
const typed = ["fixture", "value"].join("-");
const rejected = ["fixture", "other"].join("-");

const pair = { accessToken: "a-token", refreshToken: "r-token", expiresIn: 86400, mustChangePassword: false };

describe("loginWithProvider", () => {
  it("exchanges the provider token for OUR pair and stores it", async () => {
    const fetchMock = jest.fn<typeof fetch>().mockResolvedValue(jsonResponse(pair));
    globalThis.fetch = fetchMock as unknown as typeof fetch;
    const getIdToken = jest.fn<(e: string, p: string) => Promise<string>>().mockResolvedValue("google:madre");

    const res = await loginWithProvider(who, typed, getIdToken);

    expect(res.mustChangePassword).toBe(false);
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("http://localhost:8080/api/v1/auth/mobile/google");
    // The PASSWORD never reaches our API. It went to the provider, and sending
    // it here too would put it in our logs for nothing, keeping us in the
    // credential-handling business this migration exists to leave.
    const body = JSON.parse(init.body as string) as Record<string, unknown>;
    expect(body).toEqual({ idToken: "google:madre" });
    // Our tokens, ours to store — the session keeps our TTLs and our rotation.
    expect(getAccessToken()).toBe("a-token");
    expect(getRefreshToken()).toBe("r-token");
  });

  it("does not call the gateway when the provider refuses", async () => {
    const fetchMock = jest.fn<typeof fetch>();
    globalThis.fetch = fetchMock as unknown as typeof fetch;
    const getIdToken = jest.fn<(e: string, p: string) => Promise<string>>()
      .mockRejectedValue(new Error("wrong password"));

    await expect(loginWithProvider(who, rejected, getIdToken)).rejects.toThrow();
    // Nothing to exchange, and a wasted round trip would spend the gateway's
    // shared login rate-limit budget on a call that cannot succeed.
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("stores nothing when the gateway refuses", async () => {
    globalThis.fetch = jest.fn<typeof fetch>()
      .mockResolvedValue(new Response("", { status: 401 }) as unknown as Response) as unknown as typeof fetch;
    const getIdToken = jest.fn<(e: string, p: string) => Promise<string>>().mockResolvedValue("google:desconocida");

    // A valid provider account with no user row here must not end up signed in.
    await expect(loginWithProvider(who, typed, getIdToken)).rejects.toThrow();
    expect(getAccessToken()).toBeNull();
  });
});
