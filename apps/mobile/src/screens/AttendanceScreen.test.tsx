import { screen, waitFor } from "@testing-library/react-native";
import { clearTokens, saveTokens } from "../api";
import { AttendanceScreen } from "./AttendanceScreen";
import { renderWithProviders } from "../test/renderWithProviders";

const child = { id: "s1", firstName: "Mateo", lastName: "Herrera", level: "primaria", gradeGroup: "3° A" };

type Rec = { id: string; date: string; status: string; note: string };

/** Routes each family request; `records` null makes the attendance endpoint fail. */
function mockFetch(records: Rec[] | null): jest.Mock {
  const fetchMock = jest.fn(async (url: string) => {
    const u = String(url);
    if (u.includes("/family/students/s1/attendance")) {
      if (records === null) return { ok: false, status: 500, json: async () => ({}) };
      return { ok: true, status: 200, json: async () => ({ records }) };
    }
    if (u.includes("/family/students")) return { ok: true, status: 200, json: async () => ({ students: [child] }) };
    return { ok: true, status: 200, json: async () => ({ notifications: [] }) };
  });
  globalThis.fetch = fetchMock as unknown as typeof fetch;
  return fetchMock;
}

describe("AttendanceScreen (real family attendance)", () => {
  beforeEach(async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });
  afterEach(async () => {
    await clearTokens();
  });

  it("renders the real records with their status labels", async () => {
    mockFetch([
      { id: "a1", date: "2026-06-04", status: "absent", note: "Sin justificar" },
      { id: "a2", date: "2026-06-10", status: "excused", note: "Cita médica" },
      { id: "a3", date: "2026-06-02", status: "late", note: "8:10 a.m." },
    ]);

    await renderWithProviders(<AttendanceScreen />);

    await waitFor(() => expect(screen.getByText("Falta")).toBeTruthy());
    expect(screen.getByText("Justificada")).toBeTruthy();
    expect(screen.getByText("Retardo")).toBeTruthy();
    expect(screen.getByText("Cita médica")).toBeTruthy();
  });

  it("derives faltas / retardos / justificadas counts from the records", async () => {
    mockFetch([
      { id: "a1", date: "2026-06-04", status: "absent", note: "" },
      { id: "a2", date: "2026-06-05", status: "absent", note: "" },
      { id: "a3", date: "2026-06-10", status: "excused", note: "" },
      { id: "a4", date: "2026-06-02", status: "late", note: "" },
    ]);

    await renderWithProviders(<AttendanceScreen />);

    await waitFor(() => expect(screen.getByLabelText("Faltas").props.children).toBe("2"));
    expect(screen.getByLabelText("Retardos").props.children).toBe("1");
    expect(screen.getByLabelText("Justificadas").props.children).toBe("1");
  });

  it("shows an empty state when there are no records", async () => {
    mockFetch([]);

    await renderWithProviders(<AttendanceScreen />);

    await waitFor(() => expect(screen.getByText("Sin registros de asistencia")).toBeTruthy());
  });

  it("shows an error state with retry when the load fails", async () => {
    mockFetch(null);

    await renderWithProviders(<AttendanceScreen />);

    await waitFor(() => expect(screen.getByText("No pudimos cargar la asistencia")).toBeTruthy());
    expect(screen.getByRole("button", { name: "Reintentar" })).toBeTruthy();
  });
});
