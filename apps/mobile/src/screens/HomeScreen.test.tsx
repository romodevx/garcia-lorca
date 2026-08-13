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

  it("does not render fake agenda events (next-event tile + list hidden)", async () => {
    // Agenda has no backend yet (Phase 9d): the mock `events` and the static
    // `today.nextEvent` must not be shown — a fake event would mislead the parent.
    await renderWithProviders(<HomeScreen />);

    expect(screen.queryByLabelText("Próximo evento")).toBeNull();
    expect(screen.queryByText("Suspensión de clases")).toBeNull();
    expect(screen.queryByText("Festival de Primavera")).toBeNull();
  });
});
