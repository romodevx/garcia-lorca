import { fireEvent, screen, waitFor } from "@testing-library/react-native";
import { router } from "expo-router";
import { clearTokens, saveTokens } from "../api";
import { HomeScreen } from "./HomeScreen";
import { renderWithProviders } from "../test/renderWithProviders";

describe("HomeScreen summary cards", () => {
  beforeEach(() => jest.clearAllMocks());
  afterEach(async () => {
    await clearTokens();
  });

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

  it("shows the error state (not fabricated neutral cards) when the child bootstrap fails (bug_005)", async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
    const fetchMock = jest.fn(async (url: string) => {
      const u = String(url);
      if (u.includes("/family/students")) return { ok: false, status: 500, json: async () => ({}) };
      return { ok: true, status: 200, json: async () => ({ notifications: [] }) };
    });
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    await renderWithProviders(<HomeScreen />);

    await waitFor(() => expect(screen.getByText("No pudimos cargar la información")).toBeTruthy());
    expect(screen.getByRole("button", { name: "Reintentar" })).toBeTruthy();
    // The fabricated neutral attendance card must not be shown.
    expect(screen.queryByText("Sin novedades")).toBeNull();
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
