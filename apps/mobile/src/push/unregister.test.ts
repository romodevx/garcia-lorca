import AsyncStorage from "@react-native-async-storage/async-storage";
import { clearTokens, saveTokens } from "../api";
import { readRegisteredToken, saveRegisteredToken } from "./pushStore";
import { unregisterPush } from "./unregister";

describe("unregisterPush", () => {
  beforeEach(async () => {
    await AsyncStorage.clear();
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });
  afterEach(async () => {
    await clearTokens();
  });

  it("DELETEs the registered token and forgets it", async () => {
    await saveRegisteredToken("ExponentPushToken[xyz]");
    const fetchMock = jest.fn(async () => ({ ok: true, status: 204, json: async () => ({}) }));
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    await unregisterPush();

    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toContain("/api/v1/family/devices/ExponentPushToken%5Bxyz%5D");
    expect(init.method).toBe("DELETE");
    expect(await readRegisteredToken()).toBeNull();
  });

  it("is best-effort: a failed DELETE never blocks sign-out", async () => {
    await saveRegisteredToken("t1");
    globalThis.fetch = jest.fn(async () => {
      throw new Error("offline");
    }) as unknown as typeof fetch;

    await expect(unregisterPush()).resolves.toBeUndefined();
    expect(await readRegisteredToken()).toBeNull();
  });

  it("does nothing when this phone never registered", async () => {
    const fetchMock = jest.fn();
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    await unregisterPush();
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
