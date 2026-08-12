import AsyncStorage from "@react-native-async-storage/async-storage";
import { listFamilyNotifications } from "./notifications";
import { clearTokens, saveTokens } from "./tokenStore";

function jsonOk(body: unknown): Response {
  return { ok: true, status: 200, json: async () => body } as unknown as Response;
}

describe("listFamilyNotifications", () => {
  beforeEach(async () => {
    await AsyncStorage.clear();
    await clearTokens();
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });

  it("returns the notifications carried in the envelope", async () => {
    globalThis.fetch = jest
      .fn()
      .mockResolvedValue(
        jsonOk({
          notifications: [
            {
              id: "n1",
              studentId: "s1",
              type: "attendance",
              title: "Asistencia registrada",
              body: "Mateo llegó a tiempo.",
              createdAt: "2026-08-12T10:00:00Z",
              readAt: "",
            },
          ],
        }),
      ) as unknown as typeof fetch;

    const list = await listFamilyNotifications();

    expect(list).toHaveLength(1);
    expect(list[0]).toMatchObject({ id: "n1", type: "attendance", readAt: "" });
  });

  it("returns an empty array when the envelope carries a null list", async () => {
    globalThis.fetch = jest.fn().mockResolvedValue(jsonOk({ notifications: null })) as unknown as typeof fetch;
    expect(await listFamilyNotifications()).toEqual([]);
  });

  it("throws an ApiError on a non-2xx response", async () => {
    globalThis.fetch = jest
      .fn()
      .mockResolvedValue({ ok: false, status: 403, json: async () => ({}) } as unknown as Response) as unknown as typeof fetch;
    await expect(listFamilyNotifications()).rejects.toMatchObject({ status: 403 });
  });
});
