import { act, fireEvent, screen, waitFor } from "@testing-library/react-native";
import { type ReactElement } from "react";
import { Pressable, Text } from "react-native";
import { clearTokens, saveTokens } from "../api";
import { useAppState } from "../state/AppState";
import { requestFreshData } from "../state/freshness";
import { renderWithProviders } from "../test/renderWithProviders";
import { GradesScreen } from "./GradesScreen";

type Child = { id: string; firstName: string; lastName: string; level: string; levelId: string; gradeGroup: string };
type Subject = { subjectId: string; subjectName: string; grades: Record<string, number> };
type Reply = { status: number; subjects?: Subject[]; gradingEnabled?: boolean; scale?: typeof SCALE | null } | "hang";

const MONTHS = ["2026-08", "2026-09", "2026-10"];
const SCALE = { min: 5, max: 10, decimals: 1, passing: 6 };

const mateo: Child = { id: "s1", firstName: "Mateo", lastName: "Herrera", level: "Primaria", levelId: "l1", gradeGroup: "3° A" };
const lucia: Child = { id: "s2", firstName: "Lucía", lastName: "Herrera", level: "Primaria", levelId: "l1", gradeGroup: "1° A" };

const subject = (subjectName: string, grades: Record<string, number>): Subject => ({
  subjectId: `sub-${subjectName}`,
  subjectName,
  grades,
});

/** Routes family requests; `replies` answers the monthly grades per student id. */
function mockGateway(replies: Record<string, Reply | (() => Reply)>, children: Child[] = [mateo]): jest.Mock {
  const fetchMock = jest.fn(async (url: string) => {
    const u = String(url);
    const match = /\/family\/students\/([^/]+)\/grades\/monthly/.exec(u);
    if (match) {
      const entry = replies[match[1] ?? ""] ?? { status: 200 };
      const reply = typeof entry === "function" ? entry() : entry;
      if (reply === "hang") return new Promise(() => undefined);
      if (reply.status !== 200) return { ok: false, status: reply.status, json: async () => ({}) };
      return {
        ok: true,
        status: 200,
        json: async () => ({
          cycleName: "2026-2027",
          months: MONTHS,
          ...(reply.scale === null ? {} : { scale: reply.scale ?? SCALE }),
          gradingEnabled: reply.gradingEnabled ?? true,
          subjects: reply.subjects ?? [],
        }),
      };
    }
    if (u.endsWith("/family/students")) return { ok: true, status: 200, json: async () => ({ students: children }) };
    return { ok: true, status: 200, json: async () => ({ notifications: [] }) };
  });
  globalThis.fetch = fetchMock as unknown as typeof fetch;
  return fetchMock;
}

const gradeCalls = (mock: jest.Mock): number =>
  mock.mock.calls.filter((c) => String((c as unknown[])[0]).includes("/grades/monthly")).length;

const render = (ui: ReactElement = <GradesScreen currentMonth="2026-10" />): Promise<unknown> => renderWithProviders(ui);

const selectedChip = (): string | undefined =>
  screen.getAllByRole("button").find((b) => b.props.accessibilityState?.selected)?.props.accessibilityLabel as string | undefined;

describe("GradesScreen (monthly grades)", () => {
  beforeEach(async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });
  afterEach(async () => {
    await clearTokens();
  });

  it("opens on the latest month that has a grade and shows it with the scale decimals", async () => {
    mockGateway({ s1: { status: 200, subjects: [subject("Matemáticas", { "2026-08": 8, "2026-09": 9.5 })] } });
    await render();

    await waitFor(() => expect(screen.getByLabelText("Matemáticas, septiembre: 9.5")).toBeTruthy());
    expect(selectedChip()).toBe("Septiembre 2026");
  });

  it("opens on the current month when nothing is captured yet", async () => {
    mockGateway({ s1: { status: 200, subjects: [subject("Matemáticas", {})] } });
    await render(<GradesScreen currentMonth="2026-09" />);

    await waitFor(() => expect(screen.getByText("Matemáticas")).toBeTruthy());
    expect(selectedChip()).toBe("Septiembre 2026");
  });

  it("switches month when a chip is pressed", async () => {
    mockGateway({ s1: { status: 200, subjects: [subject("Matemáticas", { "2026-08": 8, "2026-09": 9.5 })] } });
    await render();
    await waitFor(() => expect(screen.getByLabelText("Matemáticas, septiembre: 9.5")).toBeTruthy());

    await fireEvent.press(screen.getByRole("button", { name: "Agosto 2026" }));

    expect(screen.getByLabelText("Matemáticas, agosto: 8")).toBeTruthy();
    expect(selectedChip()).toBe("Agosto 2026");
  });

  it("says clearly when a subject has no grade yet for the month", async () => {
    mockGateway({
      s1: { status: 200, subjects: [subject("Matemáticas", { "2026-09": 9 }), subject("Inglés", {})] },
    });
    await render();

    await waitFor(() => expect(screen.getByText("Inglés")).toBeTruthy());
    expect(screen.getByText("Sin calificación aún")).toBeTruthy();
  });

  it("marks a failing grade with the word Reprobada, not only colour", async () => {
    mockGateway({ s1: { status: 200, subjects: [subject("Historia", { "2026-09": 5 })] } });
    await render();

    await waitFor(() => expect(screen.getByText("Reprobada")).toBeTruthy());
    expect(screen.getByLabelText("Historia, septiembre: 5, reprobada")).toBeTruthy();
    expect(screen.getByText(/Aprobatoria: 6/)).toBeTruthy();
  });

  it("averages the month's captured subjects", async () => {
    mockGateway({
      s1: {
        status: 200,
        subjects: [subject("Matemáticas", { "2026-09": 9 }), subject("Español", { "2026-09": 8 }), subject("Inglés", {})],
      },
    });
    await render();

    await waitFor(() => expect(screen.getByLabelText("Promedio del mes: 8.5")).toBeTruthy());
  });

  it("keeps the qualitative placeholder for preescolar", async () => {
    mockGateway({ s1: { status: 200, gradingEnabled: false } });
    await render();

    await waitFor(() => expect(screen.getByText("Próximamente")).toBeTruthy());
    expect(screen.queryByText(/Promedio/)).toBeNull();
  });

  it("shows grades captured before the level stopped grading, as stored, without a passing line", async () => {
    mockGateway({ s1: { status: 200, gradingEnabled: false, scale: null, subjects: [subject("Matemáticas", { "2026-09": 5.25 })] } });
    await render();

    await waitFor(() => expect(screen.getByLabelText("Matemáticas, septiembre: 5.25")).toBeTruthy());
    expect(screen.queryByText("Próximamente")).toBeNull();
    expect(screen.queryByText(/Aprobatoria/)).toBeNull();
    expect(screen.queryByText("Reprobada")).toBeNull();
  });

  it("shows captured grades for a qualitative level too, instead of the placeholder", async () => {
    const kinder: Child = { ...mateo, level: "Kínder" };
    mockGateway({ s1: { status: 200, gradingEnabled: false, scale: null, subjects: [subject("Lenguaje", { "2026-09": 9 })] } }, [kinder]);
    await render();

    await waitFor(() => expect(screen.getByLabelText("Lenguaje, septiembre: 9")).toBeTruthy());
    expect(screen.queryByText("Próximamente")).toBeNull();
  });

  it("never marks failing against a 0–0 scale", async () => {
    mockGateway({ s1: { status: 200, gradingEnabled: false, scale: { min: 0, max: 0, decimals: 0, passing: 0 }, subjects: [subject("Arte", { "2026-09": 3 })] } });
    await render();

    await waitFor(() => expect(screen.getByLabelText("Arte, septiembre: 3")).toBeTruthy());
    expect(screen.queryByText(/Aprobatoria/)).toBeNull();
  });

  it("says the child was not found on a 404", async () => {
    mockGateway({ s1: { status: 404 } });
    await render();

    await waitFor(() => expect(screen.getByText("No encontramos a este alumno")).toBeTruthy());
  });

  it("shows an error with retry, and retrying loads the grades", async () => {
    let fail = true;
    const fetchMock = mockGateway({
      s1: () => (fail ? { status: 500 } : { status: 200, subjects: [subject("Matemáticas", { "2026-09": 9.5 })] }),
    });
    await render();
    await waitFor(() => expect(screen.getByText("No pudimos cargar las calificaciones")).toBeTruthy());
    const before = gradeCalls(fetchMock);

    fail = false;
    await fireEvent.press(screen.getByRole("button", { name: "Reintentar" }));

    await waitFor(() => expect(screen.getByLabelText("Matemáticas, septiembre: 9.5")).toBeTruthy());
    expect(gradeCalls(fetchMock)).toBe(before + 1);
  });

  it("clears one child's grades the moment another child is selected", async () => {
    mockGateway({ s1: { status: 200, subjects: [subject("Matemáticas", { "2026-09": 9.5 })] }, s2: "hang" }, [mateo, lucia]);
    await render(
      <>
        <PickChild id="s2" />
        <GradesScreen currentMonth="2026-10" />
      </>,
    );
    await waitFor(() => expect(screen.getByLabelText("Matemáticas, septiembre: 9.5")).toBeTruthy());

    await fireEvent.press(screen.getByText("pick s2"));

    await waitFor(() => expect(screen.queryByText("Matemáticas")).toBeNull());
  });

  it("opens a subject's sheet with its grade for every month of the cycle", async () => {
    mockGateway({
      s1: { status: 200, subjects: [subject("Matemáticas", { "2026-08": 8, "2026-09": 5 }), subject("Inglés", { "2026-09": 9 })] },
    });
    await render();
    await waitFor(() => expect(screen.getByLabelText("Matemáticas, septiembre: 5, reprobada")).toBeTruthy());
    expect(screen.queryByLabelText("Matemáticas, agosto: 8")).toBeNull();

    await fireEvent.press(screen.getByLabelText("Matemáticas, septiembre: 5, reprobada"));

    expect(screen.getByLabelText("Matemáticas, agosto: 8")).toBeTruthy();
    expect(screen.getAllByLabelText("Matemáticas, septiembre: 5, reprobada").length).toBe(2);
    expect(screen.getByLabelText("Matemáticas, octubre: sin calificación aún")).toBeTruthy();
    expect(screen.getByText("—")).toBeTruthy();
    expect(screen.getByLabelText("Promedio del ciclo: 6.5")).toBeTruthy();
    expect(screen.queryByLabelText(/Inglés, agosto/)).toBeNull();
  });

  it("reloads when the app comes back to the foreground", async () => {
    const fetchMock = mockGateway({ s1: { status: 200, subjects: [subject("Matemáticas", { "2026-09": 9 })] } });
    await render();
    await waitFor(() => expect(screen.getByText("Matemáticas")).toBeTruthy());
    const before = gradeCalls(fetchMock);

    await act(async () => requestFreshData());

    await waitFor(() => expect(gradeCalls(fetchMock)).toBe(before + 1));
  });
});

function PickChild({ id }: { id: string }): ReactElement {
  const { selectChild } = useAppState();
  return (
    <Pressable onPress={() => selectChild(id)}>
      <Text>{`pick ${id}`}</Text>
    </Pressable>
  );
}
