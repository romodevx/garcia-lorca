import AsyncStorage from "@react-native-async-storage/async-storage";
import { apiFetch } from "./client";
import { clearTokens, saveTokens } from "./tokenStore";

function response(status: number, body: unknown): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  } as unknown as Response;
}

describe("apiFetch", () => {
  beforeEach(async () => {
    await AsyncStorage.clear();
    await clearTokens();
  });

  it("attaches the stored access token as a Bearer header", async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
    const fetchMock = jest.fn().mockResolvedValue(response(200, {}));
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    await apiFetch("/api/v1/family/notifications");

    const init = fetchMock.mock.calls[0][1];
    expect(init.headers.Authorization).toBe("Bearer a1");
  });

  it("refreshes once on a 401 and retries the request with the fresh token", async () => {
    await saveTokens({ accessToken: "stale", refreshToken: "r1" });
    const fetchMock = jest
      .fn()
      .mockResolvedValueOnce(response(401, {})) // original request: token expired
      .mockResolvedValueOnce(response(200, { accessToken: "fresh", refreshToken: "r2", expiresIn: 900 })) // refresh
      .mockResolvedValueOnce(response(200, { ok: true })); // retry
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    const res = await apiFetch("/api/v1/family/notifications");

    expect(res.status).toBe(200);
    expect(fetchMock).toHaveBeenCalledTimes(3);
    const retryInit = fetchMock.mock.calls[2][1];
    expect(retryInit.headers.Authorization).toBe("Bearer fresh");
  });

  it("returns the original 401 when the refresh also fails", async () => {
    await saveTokens({ accessToken: "stale", refreshToken: "r1" });
    const fetchMock = jest
      .fn()
      .mockResolvedValueOnce(response(401, {})) // original request
      .mockResolvedValueOnce(response(401, {})); // refresh rejected
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    const res = await apiFetch("/api/v1/family/notifications");

    expect(res.status).toBe(401);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});
