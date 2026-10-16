import AsyncStorage from "@react-native-async-storage/async-storage";
import { cleanup, waitFor } from "@testing-library/react-native";
import { router } from "expo-router";
import { clearTokens, loadTokens } from "@/src/api/tokenStore";
import { renderWithProviders } from "@/src/test/renderWithProviders";
import IndexRoute from "../../app/index";

describe("IndexRoute session restoration", () => {
  afterEach(async () => {
    cleanup();
    await clearTokens();
  });

  it("resumes a persisted guardian session after a cold relaunch", async () => {
    // Simulate a fresh JS process: durable storage has the pair, while the
    // synchronous in-memory mirror starts empty until the root bootstrap loads it.
    await clearTokens();
    await AsyncStorage.multiSet([
      ["gl.auth.accessToken", "persisted-access"],
      ["gl.auth.refreshToken", "persisted-refresh"],
    ]);
    expect(await loadTokens()).toEqual({
      accessToken: "persisted-access",
      refreshToken: "persisted-refresh",
    });
    (router.replace as jest.Mock).mockClear();

    await renderWithProviders(<IndexRoute />);

    await waitFor(() => expect(router.replace).toHaveBeenCalledWith("/(tabs)"));
  });
});
