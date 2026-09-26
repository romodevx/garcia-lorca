import { act, fireEvent, screen, waitFor } from "@testing-library/react-native";
import { clearTokens, saveTokens } from "../api";
import type { AttendanceDay } from "../api/attendance";
import { ScreenScroll } from "../components/ScreenScroll";
import { requestFreshData } from "../state/freshness";
import { renderWithProviders } from "../test/renderWithProviders";
import { AttendanceScreen } from "./AttendanceScreen";

const child = { id: "s1", firstName: "Mateo", lastName: "Herrera", level: "Primaria", levelId: "l1", gradeGroup: "3° A" };
const SUMMARY = { present: 15, absent: 2, late: 1, excused: 3, partial: 1 };

type MonthReply = { status: number; days?: AttendanceDay[]; summary?: typeof SUMMARY };

const day = (over: Partial<AttendanceDay>): AttendanceDay => ({
  date: "2026-09-28",
  status: "present",
  mode: "daily",
  classes: [],
  ...over,
});

/** The school's today the fake gateway answers with; tests move it to roll midnight. */
let serverToday = "2026-09-28";

/** Routes family requests; `months` answers attendance per YYYY-MM. */
function mockGateway(months: Record<string, MonthReply | (() => MonthReply)>): jest.Mock {
  const fetchMock = jest.fn(async (url: string) => {
    const u = String(url);
    const match = /\/family\/students\/s1\/attendance\?month=(\d{4}-\d{2})/.exec(u);
    if (match) {
      const entry = months[match[1] ?? ""];
      const reply = typeof entry === "function" ? entry() : (entry ?? { status: 200, days: [] });
      if (reply.status !== 200) return { ok: false, status: reply.status, json: async () => ({}) };
      return {
        ok: true,
        status: 200,
        json: async () => ({ month: match[1], today: serverToday, days: reply.days ?? [], summary: reply.summary ?? SUMMARY }),
      };
    }
    if (u.endsWith("/family/students")) return { ok: true, status: 200, json: async () => ({ students: [child] }) };
    return { ok: true, status: 200, json: async () => ({ notifications: [] }) };
  });
  globalThis.fetch = fetchMock as unknown as typeof fetch;
  return fetchMock;
}

const attendanceCalls = (mock: jest.Mock, month: string): number =>
  mock.mock.calls.filter((c) => String((c as unknown[])[0]).includes(`attendance?month=${month}`)).length;

const render = (initialMonth = "2026-09"): Promise<unknown> =>
  renderWithProviders(<AttendanceScreen initialMonth={initialMonth} />);

describe("AttendanceScreen", () => {
  beforeEach(async () => {
    serverToday = "2026-09-28";
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });
  afterEach(async () => {
    await clearTokens();
  });

  describe("Hoy card", () => {
    it("says the child attended today on a daily list", async () => {
      mockGateway({ "2026-09": { status: 200, days: [day({ status: "present" })] } });
      await render();
      await waitFor(() => expect(screen.getByLabelText("Hoy: Asistió")).toBeTruthy());
    });

    it("counts the attended classes on a per-class day and lists each class with its status", async () => {
      const classes = [
        { subjectName: "Español", status: "present" as const },
        { subjectName: "Matemáticas", status: "late" as const },
        { subjectName: "Inglés", status: "absent" as const, note: "Sin aviso" },
        { subjectName: "Arte", status: "present" as const },
        { subjectName: "Música", status: "present" as const },
        { subjectName: "Educación física", status: "excused" as const },
      ];
      mockGateway({ "2026-09": { status: 200, days: [day({ status: "partial", mode: "per_class", classes })] } });
      await render();
      await waitFor(() => expect(screen.getByLabelText("Hoy: Asistió a 4 de 6 clases")).toBeTruthy());
      expect(screen.getByLabelText("Inglés: Faltó")).toBeTruthy();
      expect(screen.getByText("Sin aviso")).toBeTruthy();
    });

    it("says nothing is known yet (never a false 'not taken') when today has no entry", async () => {
      mockGateway({ "2026-09": { status: 200, days: [day({ date: "2026-09-25", status: "absent" })] } });
      await render();
      await waitFor(() => expect(screen.getByLabelText("Hoy: Sin datos todavía")).toBeTruthy());
      expect(screen.queryByLabelText("Hoy: Lista aún no tomada")).toBeNull();
    });

    it("reads the note aloud with today's status", async () => {
      mockGateway({ "2026-09": { status: 200, days: [day({ status: "late", note: "Llegó 8:15" })] } });
      await render();
      await waitFor(() => expect(screen.getByLabelText("Hoy: Llegó tarde. Llegó 8:15")).toBeTruthy());
    });

    it("says the list has not been taken yet for a not_taken day", async () => {
      mockGateway({ "2026-09": { status: 200, days: [day({ status: "not_taken" })] } });
      await render();
      await waitFor(() => expect(screen.getByLabelText("Hoy: Lista aún no tomada")).toBeTruthy());
    });
  });

  it("shows the month's summary counts with their words", async () => {
    mockGateway({ "2026-09": { status: 200, days: [day({})] } });
    await render();
    await waitFor(() => expect(screen.getByLabelText("Faltas: 2")).toBeTruthy());
    expect(screen.getByLabelText("Retardos: 1")).toBeTruthy();
    expect(screen.getByLabelText("Justificadas: 3")).toBeTruthy();
    expect(screen.getByLabelText("Días parciales: 1")).toBeTruthy();
  });

  it("lists the days with a text pill and note, and expands a per-class day", async () => {
    mockGateway({
      "2026-09": {
        status: 200,
        days: [
          day({ date: "2026-09-28", status: "late", note: "Llegó 8:15" }),
          day({
            date: "2026-09-25",
            status: "partial",
            mode: "per_class",
            classes: [{ subjectName: "Historia", status: "absent" }],
          }),
        ],
      },
    });
    await render();
    await waitFor(() => expect(screen.getByText("Lun 28 sep")).toBeTruthy());
    expect(screen.getByText("Retardo")).toBeTruthy();
    expect(screen.getByLabelText("Lun 28 sep: Retardo. Llegó 8:15")).toBeTruthy();
    expect(screen.getAllByText("Llegó 8:15").length).toBeGreaterThan(0);

    expect(screen.queryByLabelText("Historia: Faltó")).toBeNull();
    await act(async () => fireEvent.press(screen.getByLabelText("Vie 25 sep: Parcial. Ver clases")));
    expect(screen.getByLabelText("Historia: Faltó")).toBeTruthy();
    expect(screen.getByLabelText("Vie 25 sep: Parcial. Ocultar clases")).toBeTruthy();
  });

  it("moves to the previous month and never past the current one", async () => {
    const fetchMock = mockGateway({
      "2026-09": { status: 200, days: [day({})] },
      "2026-08": { status: 200, days: [day({ date: "2026-08-31", status: "absent" })] },
    });
    await render();
    await waitFor(() => expect(screen.getByText("Septiembre 2026")).toBeTruthy());
    expect(screen.getByLabelText("Mes siguiente").props.accessibilityState).toMatchObject({ disabled: true });

    fireEvent.press(screen.getByLabelText("Mes anterior"));

    await waitFor(() => expect(screen.getByText("Agosto 2026")).toBeTruthy());
    await waitFor(() => expect(screen.getByText("Lun 31 ago")).toBeTruthy());
    expect(attendanceCalls(fetchMock, "2026-08")).toBe(1);
    // Today's card still speaks about today, not about August.
    expect(screen.getByLabelText("Hoy: Asistió")).toBeTruthy();
    expect(screen.getByLabelText("Mes siguiente").props.accessibilityState).toMatchObject({ disabled: false });
  });

  it("says so when a month has no school days recorded", async () => {
    mockGateway({ "2026-09": { status: 200, days: [] } });
    await render();
    await waitFor(() => expect(screen.getByText("Sin días registrados en este mes")).toBeTruthy());
  });

  it("shows an error with retry, and retrying loads the month", async () => {
    let calls = 0;
    mockGateway({
      "2026-09": () => {
        calls += 1;
        return calls === 1 ? { status: 500 } : { status: 200, days: [day({})] };
      },
    });
    await render();
    await waitFor(() => expect(screen.getByText("No pudimos cargar la asistencia")).toBeTruthy());

    fireEvent.press(screen.getByRole("button", { name: "Reintentar" }));

    await waitFor(() => expect(screen.getByLabelText("Hoy: Asistió")).toBeTruthy());
  });

  it("says the child was not found on a 404 (never an empty month)", async () => {
    mockGateway({ "2026-09": { status: 404 } });
    await render();
    await waitFor(() => expect(screen.getByText("No encontramos a este alumno")).toBeTruthy());
    expect(screen.queryByText("Sin días registrados en este mes")).toBeNull();
  });

  it("shows the error state when the child bootstrap fails (bug_005)", async () => {
    globalThis.fetch = jest.fn(async (url: string) =>
      String(url).endsWith("/family/students")
        ? { ok: false, status: 500, json: async () => ({}) }
        : { ok: true, status: 200, json: async () => ({ notifications: [] }) },
    ) as unknown as typeof fetch;
    await render();
    await waitFor(() => expect(screen.getByText("No pudimos cargar la asistencia")).toBeTruthy());
  });

  it("reloads the month on pull-to-refresh and when fresh data is requested", async () => {
    const fetchMock = mockGateway({ "2026-09": { status: 200, days: [day({})] } });
    await renderWithProviders(
      <ScreenScroll>
        <AttendanceScreen initialMonth="2026-09" />
      </ScreenScroll>,
    );
    await waitFor(() => expect(screen.getByLabelText("Hoy: Asistió")).toBeTruthy());
    const before = attendanceCalls(fetchMock, "2026-09");

    const control = screen.getByTestId("screen-scroll").props.refreshControl as { props: { onRefresh: () => void } };
    await act(async () => control.props.onRefresh());
    await waitFor(() => expect(attendanceCalls(fetchMock, "2026-09")).toBe(before + 1));

    await act(async () => requestFreshData());
    await waitFor(() => expect(attendanceCalls(fetchMock, "2026-09")).toBe(before + 2));
  });

  describe("the school's month, not the phone's", () => {
    it("re-keys to the school's month when the phone clock is behind", async () => {
      const fetchMock = mockGateway({ "2026-09": { status: 200, days: [day({})] } });
      await render("2026-08");
      await waitFor(() => expect(screen.getByText("Septiembre 2026")).toBeTruthy());
      await waitFor(() => expect(screen.getByLabelText("Hoy: Asistió")).toBeTruthy());
      expect(attendanceCalls(fetchMock, "2026-09")).toBeGreaterThan(0);
      expect(screen.getByLabelText("Mes siguiente").props.accessibilityState).toMatchObject({ disabled: true });
    });

    it("clamps to the school's month when the phone clock is ahead", async () => {
      mockGateway({ "2026-09": { status: 200, days: [day({})] } });
      await render("2026-10");
      await waitFor(() => expect(screen.getByText("Septiembre 2026")).toBeTruthy());
      await waitFor(() => expect(screen.getByLabelText("Hoy: Asistió")).toBeTruthy());
      expect(screen.queryByText("Octubre 2026")).toBeNull();
      expect(screen.getByLabelText("Mes siguiente").props.accessibilityState).toMatchObject({ disabled: true });
    });

    it("follows the school into a new month when the app comes back after midnight", async () => {
      const fetchMock = mockGateway({
        "2026-09": { status: 200, days: [day({})] },
        "2026-10": { status: 200, days: [day({ date: "2026-10-01", status: "absent" })] },
      });
      await render();
      await waitFor(() => expect(screen.getByLabelText("Hoy: Asistió")).toBeTruthy());

      serverToday = "2026-10-01";
      await act(async () => requestFreshData());

      await waitFor(() => expect(screen.getByText("Octubre 2026")).toBeTruthy());
      await waitFor(() => expect(screen.getByLabelText("Hoy: Faltó")).toBeTruthy());
      expect(attendanceCalls(fetchMock, "2026-10")).toBeGreaterThan(0);
    });
  });

  it("keeps the month on screen when a background reload fails, with an inline notice", async () => {
    let calls = 0;
    mockGateway({
      "2026-09": () => {
        calls += 1;
        return calls === 1 ? { status: 200, days: [day({})] } : { status: 500 };
      },
    });
    await render();
    await waitFor(() => expect(screen.getByLabelText("Hoy: Asistió")).toBeTruthy());

    await act(async () => requestFreshData());

    await waitFor(() => expect(screen.getByText("No pudimos actualizar. Desliza para reintentar.")).toBeTruthy());
    expect(screen.getByLabelText("Hoy: Asistió")).toBeTruthy();
    expect(screen.queryByText("No pudimos cargar la asistencia")).toBeNull();
  });
});
