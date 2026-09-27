import { act, fireEvent, screen, waitFor } from "@testing-library/react-native";
import { router } from "expo-router";
import { getAccessToken, saveTokens } from "@/src/api";
import { renderWithProviders } from "@/src/test/renderWithProviders";
import { readRegisteredToken, saveRegisteredToken } from "@/src/push/pushStore";
import MasRoute from "../../app/mas";

describe("MasRoute logout", () => {
  beforeEach(() => {
    (router.replace as jest.Mock).mockClear();
  });

  it("clears the stored tokens and returns to the login screen on Cerrar sesión", async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
    expect(getAccessToken()).toBe("a1");

    await renderWithProviders(<MasRoute />);

    await act(async () => {
      fireEvent.press(screen.getByLabelText("Cerrar sesión"));
    });

    // Privacy boundary: the token pair must not survive sign-out on a shared device.
    await waitFor(() => expect(getAccessToken()).toBeNull());
    expect(router.replace).toHaveBeenCalledWith("/");
  });

  it("removes this phone's push token BEFORE the session tokens are cleared", async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
    await saveRegisteredToken("ExponentPushToken[xyz]");
    const calls: { url: string; method?: string; bearer?: string }[] = [];
    globalThis.fetch = jest.fn(async (url: string, init?: RequestInit) => {
      calls.push({
        url: String(url),
        method: init?.method,
        bearer: (init?.headers as Record<string, string> | undefined)?.Authorization,
      });
      return { ok: true, status: 204, json: async () => ({ notifications: [], students: [] }) };
    }) as unknown as typeof fetch;

    await renderWithProviders(<MasRoute />);
    await act(async () => {
      fireEvent.press(screen.getByLabelText("Cerrar sesión"));
    });

    await waitFor(() => expect(router.replace).toHaveBeenCalledWith("/"));
    const del = calls.find((c) => c.method === "DELETE");
    expect(del?.url).toContain("/api/v1/family/devices/ExponentPushToken%5Bxyz%5D");
    expect(del?.bearer).toBe("Bearer a1");
    expect(await readRegisteredToken()).toBeNull();
  });
});
