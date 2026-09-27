import AsyncStorage from "@react-native-async-storage/async-storage";
import { clearTokens, saveTokens } from "../api";
import { readPendingUnregisters, readRegisteredToken, savePendingUnregister, saveRegisteredToken } from "./pushStore";
import { retryPendingUnregisters, unregisterPush } from "./unregister";

const reply = (status: number) => jest.fn(async () => ({ ok: status < 300, status, json: async () => ({}) }));

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
    // The server may still hold it: remember to delete it on the next launch.
    expect(await readPendingUnregisters()).toEqual(["t1"]);
  });

  it("keeps a pending delete when the gateway answers an error", async () => {
    await saveRegisteredToken("t1");
    globalThis.fetch = reply(500) as unknown as typeof fetch;

    await unregisterPush();

    expect(await readPendingUnregisters()).toEqual(["t1"]);
  });

  it("does not leave a pending delete when the DELETE succeeded", async () => {
    await saveRegisteredToken("t1");
    globalThis.fetch = reply(204) as unknown as typeof fetch;

    await unregisterPush();

    expect(await readPendingUnregisters()).toEqual([]);
  });

  it("with the session already gone, skips the network and leaves a pending delete", async () => {
    await saveRegisteredToken("t1");
    await clearTokens();
    const fetchMock = jest.fn();
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    await unregisterPush();

    expect(fetchMock).not.toHaveBeenCalled();
    expect(await readRegisteredToken()).toBeNull();
    expect(await readPendingUnregisters()).toEqual(["t1"]);
  });

  it("does nothing when this phone never registered", async () => {
    const fetchMock = jest.fn();
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    await unregisterPush();
    expect(fetchMock).not.toHaveBeenCalled();
  });
});

describe("retryPendingUnregisters", () => {
  beforeEach(async () => {
    await AsyncStorage.clear();
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });
  afterEach(async () => {
    await clearTokens();
  });

  it("is done at once when nothing is pending", async () => {
    const fetchMock = jest.fn();
    globalThis.fetch = fetchMock as unknown as typeof fetch;
    await expect(retryPendingUnregisters()).resolves.toBe(true);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("deletes each pending token and forgets it", async () => {
    await savePendingUnregister("t1");
    const fetchMock = reply(204);
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    await expect(retryPendingUnregisters()).resolves.toBe(true);

    expect((fetchMock.mock.calls[0] as unknown as [string, RequestInit])[1].method).toBe("DELETE");
    expect(await readPendingUnregisters()).toEqual([]);
  });

  it("keeps the pending token and reports failure when the DELETE fails again", async () => {
    await savePendingUnregister("t1");
    globalThis.fetch = reply(503) as unknown as typeof fetch;

    await expect(retryPendingUnregisters()).resolves.toBe(false);
    expect(await readPendingUnregisters()).toEqual(["t1"]);
  });

  it("drops a token the gateway rejects as malformed (a retry can never succeed)", async () => {
    await savePendingUnregister("bad");
    globalThis.fetch = reply(400) as unknown as typeof fetch;

    await expect(retryPendingUnregisters()).resolves.toBe(true);
    expect(await readPendingUnregisters()).toEqual([]);
  });
});
