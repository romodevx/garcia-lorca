import { act, fireEvent, screen, waitFor } from "@testing-library/react-native";
import { router } from "expo-router";
import { getAccessToken, saveTokens } from "@/src/api";
import { renderWithProviders } from "@/src/test/renderWithProviders";
import MasRoute from "./mas";

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
});
