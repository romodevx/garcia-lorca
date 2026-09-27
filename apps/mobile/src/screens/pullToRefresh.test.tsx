import { act, screen, waitFor } from "@testing-library/react-native";
import { type ReactElement } from "react";
import { clearTokens, saveTokens } from "../api";
import { ScreenScroll } from "../components/ScreenScroll";
import { renderWithProviders } from "../test/renderWithProviders";
import { AccountScreen } from "./AccountScreen";
import { AnnouncementsScreen } from "./AnnouncementsScreen";
import { GradesScreen } from "./GradesScreen";

const kid = { id: "s1", firstName: "Mateo", lastName: "Herrera", level: "Primaria", levelId: "l1", gradeGroup: "3° A" };

function mockGateway(): jest.Mock {
  const fetchMock = jest.fn(async (url: string) => {
    const u = String(url);
    if (u.endsWith("/family/students")) return { ok: true, status: 200, json: async () => ({ students: [kid] }) };
    return { ok: true, status: 200, json: async () => ({ notifications: [], gradeEntries: [], charges: [], payments: [] }) };
  });
  globalThis.fetch = fetchMock as unknown as typeof fetch;
  return fetchMock;
}

const callsTo = (mock: jest.Mock, suffix: string): number =>
  mock.mock.calls.filter((c) => String((c as unknown[])[0]).split("?")[0]?.endsWith(suffix)).length;

describe.each<[string, () => ReactElement, string]>([
  ["Avisos", () => <AnnouncementsScreen />, "/family/notifications"],
  ["Cuenta", () => <AccountScreen />, "/family/students/s1/account"],
  ["Calificaciones", () => <GradesScreen />, "/family/students/s1/grades/monthly"],
])("%s pull-to-refresh", (_name, ui, endpoint) => {
  beforeEach(async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });
  afterEach(async () => {
    await clearTokens();
  });

  it("reloads the screen's own data when pulled", async () => {
    const fetchMock = mockGateway();
    await renderWithProviders(<ScreenScroll>{ui()}</ScreenScroll>);
    await waitFor(() => expect(callsTo(fetchMock, endpoint)).toBeGreaterThan(0));
    await waitFor(() => expect(screen.getByTestId("screen-scroll").props.refreshControl).toBeTruthy());
    const before = callsTo(fetchMock, endpoint);

    const control = screen.getByTestId("screen-scroll").props.refreshControl as { props: { onRefresh: () => void } };
    await act(async () => control.props.onRefresh());

    await waitFor(() => expect(callsTo(fetchMock, endpoint)).toBe(before + 1));
  });
});

describe.each<[string, () => ReactElement, string, string]>([
  ["Cuenta", () => <AccountScreen />, "/family/students/s1/account", "No pudimos cargar tu cuenta"],
  ["Calificaciones", () => <GradesScreen />, "/family/students/s1/grades/monthly", "No pudimos cargar las calificaciones"],
])("%s failed refresh", (_name, ui, endpoint, fullScreenError) => {
  beforeEach(async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });
  afterEach(async () => {
    await clearTokens();
  });

  it("keeps the content and shows an inline notice instead of a full-screen error", async () => {
    const fetchMock = mockGateway();
    await renderWithProviders(<ScreenScroll>{ui()}</ScreenScroll>);
    await waitFor(() => expect(callsTo(fetchMock, endpoint)).toBeGreaterThan(0));
    await waitFor(() => expect(screen.getByTestId("screen-scroll").props.refreshControl).toBeTruthy());

    fetchMock.mockImplementation(async (url: string) =>
      String(url).split("?")[0]?.endsWith(endpoint)
        ? { ok: false, status: 500, json: async () => ({}) }
        : { ok: true, status: 200, json: async () => ({ students: [kid], notifications: [] }) },
    );
    const control = screen.getByTestId("screen-scroll").props.refreshControl as { props: { onRefresh: () => void } };
    await act(async () => control.props.onRefresh());

    await waitFor(() => expect(screen.getByText("No pudimos actualizar. Desliza para reintentar.")).toBeTruthy());
    expect(screen.queryByText(fullScreenError)).toBeNull();
  });
});
