import { fireEvent, screen } from "@testing-library/react-native";
import { router } from "expo-router";
import { HomeScreen } from "./HomeScreen";
import { renderWithProviders } from "../test/renderWithProviders";

describe("HomeScreen summary cards", () => {
  beforeEach(() => jest.clearAllMocks());

  it("routes the attendance card to Asistencia", async () => {
    await renderWithProviders(<HomeScreen />);
    fireEvent.press(screen.getByLabelText("Asistencia de hoy"));
    expect(router.push).toHaveBeenCalledWith("/(tabs)/asistencia");
  });

  it("routes the balance card to Cuenta", async () => {
    await renderWithProviders(<HomeScreen />);
    fireEvent.press(screen.getByLabelText("Saldo"));
    expect(router.push).toHaveBeenCalledWith("/(tabs)/cuenta");
  });

  it("routes the avisos tile to Avisos", async () => {
    await renderWithProviders(<HomeScreen />);
    fireEvent.press(screen.getByLabelText("Avisos"));
    expect(router.push).toHaveBeenCalledWith("/(tabs)/avisos");
  });

  it("routes the next-event tile to Eventos", async () => {
    await renderWithProviders(<HomeScreen />);
    fireEvent.press(screen.getByLabelText("Próximo evento"));
    expect(router.push).toHaveBeenCalledWith("/eventos");
  });
});
