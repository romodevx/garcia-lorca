import { fireEvent, screen } from "@testing-library/react-native";
import { router } from "expo-router";
import { HOME_ROUTE } from "@/src/navigation/routes";
import { renderWithProviders } from "@/src/test/renderWithProviders";
import { DetailHeader } from "./DetailHeader";

jest.mock("expo-router", () => ({
  router: { push: jest.fn(), replace: jest.fn(), back: jest.fn(), navigate: jest.fn(), canGoBack: jest.fn() },
  useLocalSearchParams: () => ({}),
}));

describe("DetailHeader back button", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("goes back to the screen the family came from", async () => {
    (router.canGoBack as jest.Mock).mockReturnValue(true);
    await renderWithProviders(<DetailHeader title="Aviso" />);

    fireEvent.press(screen.getByRole("button", { name: "Atrás" }));

    expect(router.back).toHaveBeenCalledTimes(1);
    expect(router.replace).not.toHaveBeenCalled();
  });

  it("goes to Inicio when a deep link opened this screen with nothing behind it", async () => {
    (router.canGoBack as jest.Mock).mockReturnValue(false);
    await renderWithProviders(<DetailHeader title="Aviso" />);

    fireEvent.press(screen.getByRole("button", { name: "Atrás" }));

    expect(router.replace).toHaveBeenCalledWith(HOME_ROUTE);
    expect(router.back).not.toHaveBeenCalled();
  });
});
