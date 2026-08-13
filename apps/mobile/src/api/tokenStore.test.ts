import AsyncStorage from "@react-native-async-storage/async-storage";
import {
  clearTokens,
  currentSessionGeneration,
  getAccessToken,
  getRefreshToken,
  loadTokens,
  saveTokens,
} from "./tokenStore";

describe("tokenStore", () => {
  beforeEach(async () => {
    await AsyncStorage.clear();
    await clearTokens();
  });

  it("exposes no tokens before anything is stored", async () => {
    expect(getAccessToken()).toBeNull();
    expect(getRefreshToken()).toBeNull();
    expect(await loadTokens()).toBeNull();
  });

  it("persists a token pair and exposes it in memory", async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
    expect(getAccessToken()).toBe("a1");
    expect(getRefreshToken()).toBe("r1");
    expect(await loadTokens()).toEqual({ accessToken: "a1", refreshToken: "r1" });
  });

  it("clears the pair from memory and storage", async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
    await clearTokens();
    expect(getAccessToken()).toBeNull();
    expect(await loadTokens()).toBeNull();
  });

  it("advances the session generation on every clear (but not on save)", async () => {
    const before = currentSessionGeneration();
    // Saving a pair must NOT advance the generation — only a sign-out does.
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
    expect(currentSessionGeneration()).toBe(before);

    await clearTokens();
    expect(currentSessionGeneration()).toBe(before + 1);

    await clearTokens();
    expect(currentSessionGeneration()).toBe(before + 2);
  });
});
