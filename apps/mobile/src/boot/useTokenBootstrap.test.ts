import { renderHook, waitFor } from "@testing-library/react-native";
import * as tokenStore from "../api/tokenStore";
import { useTokenBootstrap } from "./useTokenBootstrap";

describe("useTokenBootstrap", () => {
  it("hydrates the token cache exactly once on boot and reports ready when done", async () => {
    const loadSpy = jest.spyOn(tokenStore, "loadTokens").mockResolvedValue(null);

    const { result } = await renderHook(() => useTokenBootstrap());

    await waitFor(() => expect(result.current).toBe(true));
    expect(loadSpy).toHaveBeenCalledTimes(1);

    loadSpy.mockRestore();
  });

  it("still reports ready when hydration finds a stored pair", async () => {
    const loadSpy = jest
      .spyOn(tokenStore, "loadTokens")
      .mockResolvedValue({ accessToken: "a1", refreshToken: "r1" });

    const { result } = await renderHook(() => useTokenBootstrap());
    await waitFor(() => expect(result.current).toBe(true));
    expect(loadSpy).toHaveBeenCalledTimes(1);

    loadSpy.mockRestore();
  });

  it("reports ready when token hydration rejects without leaking an unhandled rejection", async () => {
    const loadSpy = jest.spyOn(tokenStore, "loadTokens").mockRejectedValue(new Error("storage unavailable"));

    const { result } = await renderHook(() => useTokenBootstrap());
    await waitFor(() => expect(result.current).toBe(true));

    loadSpy.mockRestore();
  });
});
