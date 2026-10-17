import { fireEvent, screen, waitFor } from "@testing-library/react-native";
import { router } from "expo-router";
import { clearTokens, saveTokens } from "../api";
import { HomeScreen } from "./HomeScreen";

const mockDeviceMonth = jest.fn<string, []>();
jest.mock("./attendanceView", () => {
  const actual = jest.requireActual("./attendanceView");
  return { ...actual, deviceMonth: () => mockDeviceMonth() || actual.deviceMonth() };
});
import { renderWithProviders } from "../test/renderWithProviders";

// ISSUE-T14-108: the "Hoy" card drew the green "presente" check for every
// status, a justified absence included. It now uses the status's own icon,
// the same one the Asistencia tab shows.
describe("HomeScreen today's attendance icon", () => {
  const kid = { id: "s1", firstName: "Alumna", lastName: "Prueba", level: "Primaria", levelId: "l1", gradeGroup: "B" };

  function gatewayWithToday(status: string): void {
    globalThis.fetch = jest.fn(async (url: string) => {
      const u = String(url);
      if (u.includes("/family/students/s1/attendance")) {
        const day = { date: "2026-10-16", status, mode: "daily", classes: [], note: "Cita médica" };
        return {
          ok: true,
          status: 200,
          json: async () => ({ month: "2026-10", today: "2026-10-16", days: [day], summary: {} }),
        };
      }
      if (u.endsWith("/family/students")) return { ok: true, status: 200, json: async () => ({ students: [kid] }) };
      return { ok: true, status: 200, json: async () => ({ notifications: [] }) };
    }) as unknown as typeof fetch;
  }

  beforeEach(async () => {
    mockDeviceMonth.mockReturnValue("2026-10");
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });
  afterEach(async () => {
    await clearTokens();
  });

  it("shows a justified absence with its own icon, not the present check", async () => {
    gatewayWithToday("excused");

    await renderWithProviders(<HomeScreen />);

    await waitFor(() => expect(screen.getByText("Falta justificada")).toBeTruthy());
    expect(screen.getByTestId("today-status-document")).toBeTruthy();
    expect(screen.queryByTestId("today-status-check-circle")).toBeNull();
  });

  it("keeps the check for a day the child attended", async () => {
    gatewayWithToday("present");

    await renderWithProviders(<HomeScreen />);

    await waitFor(() => expect(screen.getByText("Asistió")).toBeTruthy());
    expect(screen.getByTestId("today-status-check-circle")).toBeTruthy();
  });
});

describe("HomeScreen summary cards", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockDeviceMonth.mockReturnValue("");
  });
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

  // T5: a balance below zero is the school owing the family — said as credit,
  // never "Saldo por pagar $0.00 · Pendiente".
  it("names a negative balance as credit on the balance card", async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
    const kid = { id: "s1", firstName: "Mateo", lastName: "Herrera", level: "Primaria", levelId: "l1", gradeGroup: "3° A" };
    globalThis.fetch = jest.fn(async (url: string) => {
      const u = String(url);
      if (u.includes("/family/students/s1/statement")) {
        return {
          ok: true,
          status: 200,
          json: async () => ({ movements: [], totals: { chargedCents: 0, discountCents: 0, paidCents: 50000, balanceCents: -50000, notYetDueCents: 0 }, dueTodayCents: 0, asOf: "2026-10-01" }),
        };
      }
      if (u.endsWith("/family/students")) return { ok: true, status: 200, json: async () => ({ students: [kid] }) };
      return { ok: true, status: 200, json: async () => ({ notifications: [] }) };
    }) as unknown as typeof fetch;

    await renderWithProviders(<HomeScreen />);

    await waitFor(() => expect(screen.getByText("Saldo a favor")).toBeTruthy());
    expect(screen.getByText("$500.00")).toBeTruthy();
    expect(screen.getByText("Al corriente")).toBeTruthy();
    expect(screen.queryByText("Saldo por pagar")).toBeNull();
    expect(screen.queryByText("Pendiente")).toBeNull();
  });

  it("does not render fake agenda events (next-event tile + list hidden)", async () => {
    // Agenda has no backend yet (Phase 9d): the mock `events` and the static
    // `today.nextEvent` must not be shown — a fake event would mislead the parent.
    await renderWithProviders(<HomeScreen />);

    expect(screen.queryByLabelText("Próximo evento")).toBeNull();
    expect(screen.queryByText("Suspensión de clases")).toBeNull();
    expect(screen.queryByText("Festival de Primavera")).toBeNull();
  });

  it("shows today's attendance from the school's monthly read", async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
    const kid = { id: "s1", firstName: "Mateo", lastName: "Herrera", level: "Primaria", levelId: "l1", gradeGroup: "3° A" };
    globalThis.fetch = jest.fn(async (url: string) => {
      const u = String(url);
      if (u.includes("/family/students/s1/attendance?month=")) {
        return {
          ok: true,
          status: 200,
          json: async () => ({
            month: "2026-09",
            today: "2026-09-28",
            days: [{ date: "2026-09-28", status: "absent", mode: "daily", classes: [] }],
            summary: { present: 0, absent: 1, late: 0, excused: 0, partial: 0 },
          }),
        };
      }
      if (u.endsWith("/family/students")) return { ok: true, status: 200, json: async () => ({ students: [kid] }) };
      return { ok: true, status: 200, json: async () => ({ notifications: [], charges: [], payments: [] }) };
    }) as unknown as typeof fetch;

    await renderWithProviders(<HomeScreen />);

    await waitFor(() => expect(screen.getByText("Faltó")).toBeTruthy());
  });

  it("reads today from the school's month even when the phone clock is a month behind", async () => {
    mockDeviceMonth.mockReturnValue("2026-08");
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
    const kid = { id: "s1", firstName: "Mateo", lastName: "Herrera", level: "Primaria", levelId: "l1", gradeGroup: "3° A" };
    globalThis.fetch = jest.fn(async (url: string) => {
      const u = String(url);
      const match = /attendance\?month=(\d{4}-\d{2})/.exec(u);
      if (match) {
        const days = match[1] === "2026-09" ? [{ date: "2026-09-28", status: "absent", mode: "daily", classes: [] }] : [];
        return { ok: true, status: 200, json: async () => ({ month: match[1], today: "2026-09-28", days, summary: {} }) };
      }
      if (u.endsWith("/family/students")) return { ok: true, status: 200, json: async () => ({ students: [kid] }) };
      return { ok: true, status: 200, json: async () => ({ notifications: [], charges: [], payments: [] }) };
    }) as unknown as typeof fetch;

    await renderWithProviders(<HomeScreen />);

    await waitFor(() => expect(screen.getByText("Faltó")).toBeTruthy());
    expect(screen.queryByText("Lista aún no tomada")).toBeNull();
  });
});
