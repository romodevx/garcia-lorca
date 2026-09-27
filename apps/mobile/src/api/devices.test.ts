import AsyncStorage from "@react-native-async-storage/async-storage";
import { registerDevice, unregisterDevice } from "./devices";
import { clearTokens, saveTokens } from "./tokenStore";

function respond(status: number): Response {
  return { ok: status >= 200 && status < 300, status, json: async () => ({}) } as unknown as Response;
}

describe("family devices API", () => {
  beforeEach(async () => {
    await AsyncStorage.clear();
    await clearTokens();
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });

  it("POSTs the push token and platform with the Bearer header", async () => {
    const fetchMock = jest.fn().mockResolvedValue(respond(204));
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    await registerDevice("ExponentPushToken[abc]", "android");

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toContain("/api/v1/family/devices");
    expect(init.method).toBe("POST");
    expect(JSON.parse(String(init.body))).toEqual({ token: "ExponentPushToken[abc]", platform: "android" });
    expect((init.headers as Record<string, string>).Authorization).toBe("Bearer a1");
    expect((init.headers as Record<string, string>)["Content-Type"]).toBe("application/json");
  });

  it("rejects when the gateway refuses the registration", async () => {
    globalThis.fetch = jest.fn().mockResolvedValue(respond(500)) as unknown as typeof fetch;
    await expect(registerDevice("t", "ios")).rejects.toMatchObject({ status: 500 });
  });

  it("DELETEs the url-encoded token", async () => {
    const fetchMock = jest.fn().mockResolvedValue(respond(204));
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    await unregisterDevice("ExponentPushToken[abc]");

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toContain("/api/v1/family/devices/ExponentPushToken%5Babc%5D");
    expect(init.method).toBe("DELETE");
  });
});
