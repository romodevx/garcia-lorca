import AsyncStorage from "@react-native-async-storage/async-storage";
import { login, logout, refresh } from "./auth";
import { onSessionLost } from "./session";
import { clearTokens, getAccessToken, getRefreshToken, saveTokens } from "./tokenStore";

/** Minimal Response stand-in for the mocked global fetch. */
function response(status: number, body: unknown): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  } as unknown as Response;
}

describe("auth.login", () => {
  beforeEach(async () => {
    await AsyncStorage.clear();
    await clearTokens();
  });

  it("posts the credentials to the mobile login route and stores the returned pair", async () => {
    const fetchMock = jest
      .fn()
      .mockResolvedValue(
        response(200, { accessToken: "a1", refreshToken: "r1", expiresIn: 900, mustChangePassword: false }),
      );
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    const result = await login("director@garcialorca.edu.mx", "Admin123.");

    expect(result.mustChangePassword).toBe(false);
    expect(getAccessToken()).toBe("a1");
    expect(getRefreshToken()).toBe("r1");
    const [urlArg, init] = fetchMock.mock.calls[0];
    expect(String(urlArg)).toContain("/api/v1/auth/mobile/login");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body)).toEqual({ email: "director@garcialorca.edu.mx", password: "Admin123." });
  });

  it("surfaces the mustChangePassword flag from the body", async () => {
    globalThis.fetch = jest
      .fn()
      .mockResolvedValue(response(200, { accessToken: "a1", refreshToken: "r1", expiresIn: 900, mustChangePassword: true })) as unknown as typeof fetch;
    const result = await login("u", "p");
    expect(result.mustChangePassword).toBe(true);
  });

  it("throws and stores nothing when the credentials are rejected", async () => {
    globalThis.fetch = jest.fn().mockResolvedValue(response(401, {})) as unknown as typeof fetch;
    await expect(login("u", "bad")).rejects.toMatchObject({ status: 401 });
    expect(getAccessToken()).toBeNull();
  });
});

describe("auth.refresh", () => {
  beforeEach(async () => {
    await AsyncStorage.clear();
    await clearTokens();
  });

  it("returns false without calling the network when there is no refresh token", async () => {
    const fetchMock = jest.fn();
    globalThis.fetch = fetchMock as unknown as typeof fetch;
    expect(await refresh()).toBe(false);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("rotates the pair using the stored refresh token", async () => {
    await saveTokens({ accessToken: "stale", refreshToken: "r1" });
    const fetchMock = jest
      .fn()
      .mockResolvedValue(response(200, { accessToken: "a2", refreshToken: "r2", expiresIn: 900 }));
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    expect(await refresh()).toBe(true);
    expect(getAccessToken()).toBe("a2");
    expect(getRefreshToken()).toBe("r2");
    const [urlArg, init] = fetchMock.mock.calls[0];
    expect(String(urlArg)).toContain("/api/v1/auth/mobile/refresh");
    expect(JSON.parse(init.body)).toEqual({ refreshToken: "r1" });
  });

  it("clears the tokens and returns false when the refresh token is rejected", async () => {
    await saveTokens({ accessToken: "stale", refreshToken: "r1" });
    globalThis.fetch = jest.fn().mockResolvedValue(response(401, {})) as unknown as typeof fetch;
    expect(await refresh()).toBe(false);
    expect(getRefreshToken()).toBeNull();
  });

  it("emits the session-lost signal when the gateway rejects the refresh token", async () => {
    await saveTokens({ accessToken: "stale", refreshToken: "r1" });
    globalThis.fetch = jest.fn().mockResolvedValue(response(401, {})) as unknown as typeof fetch;
    const onLost = jest.fn();
    const unsubscribe = onSessionLost(onLost);

    expect(await refresh()).toBe(false);

    // A rejected refresh token is a genuine dead session — the one true moment
    // that must force navigation back to Login.
    expect(onLost).toHaveBeenCalledTimes(1);
    unsubscribe();
  });

  it("drops the refreshed pair when a logout happens mid-refresh (no resurrection)", async () => {
    await saveTokens({ accessToken: "stale", refreshToken: "r1" });
    // Hold the network response open so we can log out while the POST is in flight.
    let resolveFetch: (r: Response) => void = () => undefined;
    const fetchPromise = new Promise<Response>((resolve) => {
      resolveFetch = resolve;
    });
    globalThis.fetch = jest.fn().mockReturnValue(fetchPromise) as unknown as typeof fetch;

    const refreshPromise = refresh();
    // The guardian signs out before the refresh resolves.
    await logout();
    // Only now does the gateway answer 200 with a fresh pair.
    resolveFetch(response(200, { accessToken: "a2", refreshToken: "r2", expiresIn: 900 }));

    // The in-flight refresh must NOT resurrect the just-cleared session.
    expect(await refreshPromise).toBe(false);
    expect(getAccessToken()).toBeNull();
    expect(getRefreshToken()).toBeNull();
  });

  it("does NOT emit session-lost when there is no refresh token (pre-login)", async () => {
    globalThis.fetch = jest.fn() as unknown as typeof fetch;
    const onLost = jest.fn();
    const unsubscribe = onSessionLost(onLost);

    expect(await refresh()).toBe(false);

    // No token yet means no network call and no dead session — must stay silent.
    expect(onLost).not.toHaveBeenCalled();
    unsubscribe();
  });
});
