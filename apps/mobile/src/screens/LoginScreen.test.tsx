import { fireEvent, screen } from "@testing-library/react-native";
import { LoginScreen } from "./LoginScreen";
import { renderWithProviders } from "../test/renderWithProviders";

describe("LoginScreen", () => {
  it("renders the portal branding", async () => {
    await renderWithProviders(<LoginScreen onLogin={() => undefined} />);
    expect(screen.getByText(/Portal para padres y tutores/)).toBeTruthy();
  });

  it("submits via the Ingresar button", async () => {
    const onLogin = jest.fn();
    await renderWithProviders(<LoginScreen onLogin={onLogin} />);
    fireEvent.press(screen.getByRole("button", { name: "Ingresar" }));
    expect(onLogin).toHaveBeenCalledTimes(1);
  });

  it("also submits via Face ID", async () => {
    const onLogin = jest.fn();
    await renderWithProviders(<LoginScreen onLogin={onLogin} />);
    fireEvent.press(screen.getByRole("button", { name: "Entrar con Face ID" }));
    expect(onLogin).toHaveBeenCalledTimes(1);
  });
});
