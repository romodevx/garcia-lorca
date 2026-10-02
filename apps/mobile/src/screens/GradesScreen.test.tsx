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

const PERIODS = [
  { id: "p1", ordinal: 1, name: "1.er trimestre", startsOn: "2026-08-24", endsOn: "2026-11-30" },
  { id: "p2", ordinal: 2, name: "2.º trimestre", startsOn: "2026-12-01", endsOn: "2027-03-15" },
  { id: "p3", ordinal: 3, name: "3.er trimestre", startsOn: "2027-03-16", endsOn: "2027-07-10" },
];
const SCALE = { min: 5, max: 10, decimals: 0, passing: 6 };
/** Inside the 2.º trimestre. */
const TODAY = "2027-01-20";

const mateo: Child = { id: "s1", firstName: "Mateo", lastName: "Herrera", level: "Primaria", levelId: "l1", gradeGroup: "3° A" };
const lucia: Child = { id: "s2", firstName: "Lucía", lastName: "Herrera", level: "Primaria", levelId: "l1", gradeGroup: "1° A" };

const subject = (subjectName: string, grades: Record<string, number>): Subject => ({
  subjectId: `sub-${subjectName}`,
  subjectName,
  grades,
});

/** Routes family requests; `replies` answers the period grades per student id. */
function mockGateway(replies: Record<string, Reply | (() => Reply)>, children: Child[] = [mateo]): jest.Mock {
  const fetchMock = jest.fn(async (url: string) => {
    const u = String(url);
    const match = /\/family\/students\/([^/]+)\/grades\/periods/.exec(u);
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
          periods: PERIODS,
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
  mock.mock.calls.filter((c) => String((c as unknown[])[0]).includes("/grades/periods")).length;

const render = (ui: ReactElement = <GradesScreen today={TODAY} />): Promise<unknown> => renderWithProviders(ui);

const selectedChip = (): string | undefined =>
  screen.getAllByRole("button").find((b) => b.props.accessibilityState?.selected)?.props.accessibilityLabel as string | undefined;

describe("GradesScreen (grades per evaluation period)", () => {
  beforeEach(async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });
  afterEach(async () => {
    await clearTokens();
  });

  it("opens on the period that contains today and shows each grade exactly as captured", async () => {
    mockGateway({ s1: { status: 200, subjects: [subject("Matemáticas", { p1: 8, p2: 9 })] } });
    await render();

    await waitFor(() => expect(screen.getByLabelText(/^Matemáticas, 2\.º trimestre: 9/)).toBeTruthy());
    expect(selectedChip()).toBe("2.º trimestre");
  });

  it("between periods, opens on the latest one that already started", async () => {
    mockGateway({ s1: { status: 200, subjects: [subject("Matemáticas", { p1: 8 })] } });
    await render(<GradesScreen today="2027-12-01" />);

    await waitFor(() => expect(screen.getByText("Matemáticas")).toBeTruthy());
    expect(selectedChip()).toBe("3.er trimestre");
  });

  it("switches period when a chip is pressed", async () => {
    mockGateway({ s1: { status: 200, subjects: [subject("Matemáticas", { p1: 8, p2: 9 })] } });
    await render();
    await waitFor(() => expect(screen.getByLabelText(/^Matemáticas, 2\.º trimestre: 9/)).toBeTruthy());

    await fireEvent.press(screen.getByRole("button", { name: "1.er trimestre" }));

    expect(screen.getByLabelText(/^Matemáticas, 1\.er trimestre: 8/)).toBeTruthy();
    expect(selectedChip()).toBe("1.er trimestre");
  });

  it("shows a dash when a subject has no grade yet for the period", async () => {
    mockGateway({ s1: { status: 200, subjects: [subject("Matemáticas", { p2: 9 }), subject("Inglés", {})] } });
    await render();

    await waitFor(() => expect(screen.getByText("Inglés")).toBeTruthy());
    expect(screen.getByLabelText(/^Inglés, 2\.º trimestre: sin calificación aún/)).toBeTruthy();
    expect(screen.getAllByText("—").length).toBeGreaterThan(0);
  });

  it("marks a failing grade with the word Reprobada, not only colour", async () => {
    mockGateway({ s1: { status: 200, subjects: [subject("Historia", { p2: 5 })] } });
    await render();

    await waitFor(() => expect(screen.getByText("Reprobada")).toBeTruthy());
    expect(screen.getByLabelText(/^Historia, 2\.º trimestre: 5, reprobada/)).toBeTruthy();
    expect(screen.getByText(/Aprobatoria: 6/)).toBeTruthy();
  });

  it("shows each subject's average across periods, truncated to 2 decimals", async () => {
    mockGateway({ s1: { status: 200, subjects: [subject("Matemáticas", { p1: 9, p2: 9, p3: 8 })] } });
    await render();

    await waitFor(() => expect(screen.getByText("Promedio 8.66")).toBeTruthy());
    expect(screen.getByLabelText("Matemáticas, 2.º trimestre: 9. Promedio 8.66")).toBeTruthy();
  });

  it("shows the general average: the exact mean of the subjects' exact averages", async () => {
    mockGateway({
      s1: {
        status: 200,
        subjects: [subject("Matemáticas", { p1: 9, p2: 9, p3: 8 }), subject("Español", { p1: 9, p2: 8, p3: 8 }), subject("Inglés", {})],
      },
    });
    await render();

    await waitFor(() => expect(screen.getByLabelText("Promedio general: 8.5")).toBeTruthy());
  });

  it("keeps the qualitative placeholder for preescolar", async () => {
    mockGateway({ s1: { status: 200, gradingEnabled: false } });
    await render();

    await waitFor(() => expect(screen.getByText("Próximamente")).toBeTruthy());
    expect(screen.queryByText(/Promedio/)).toBeNull();
  });

  it("shows grades captured before the level stopped grading, as stored, without a passing line", async () => {
    mockGateway({ s1: { status: 200, gradingEnabled: false, scale: null, subjects: [subject("Matemáticas", { p2: 5.25 })] } });
    await render();

    await waitFor(() => expect(screen.getByLabelText(/^Matemáticas, 2\.º trimestre: 5\.25/)).toBeTruthy());
    expect(screen.queryByText("Próximamente")).toBeNull();
    expect(screen.queryByText(/Aprobatoria/)).toBeNull();
    expect(screen.queryByText("Reprobada")).toBeNull();
  });

  it("never marks failing against a 0–0 scale", async () => {
    mockGateway({ s1: { status: 200, gradingEnabled: false, scale: { min: 0, max: 0, decimals: 0, passing: 0 }, subjects: [subject("Arte", { p2: 3 })] } });
    await render();

    await waitFor(() => expect(screen.getByLabelText(/^Arte, 2\.º trimestre: 3/)).toBeTruthy());
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
      s1: () => (fail ? { status: 500 } : { status: 200, subjects: [subject("Matemáticas", { p2: 9 })] }),
    });
    await render();
    await waitFor(() => expect(screen.getByText("No pudimos cargar las calificaciones")).toBeTruthy());
    const before = gradeCalls(fetchMock);

    fail = false;
    await fireEvent.press(screen.getByRole("button", { name: "Reintentar" }));

    await waitFor(() => expect(screen.getByLabelText(/^Matemáticas, 2\.º trimestre: 9/)).toBeTruthy());
    expect(gradeCalls(fetchMock)).toBe(before + 1);
  });

  it("clears one child's grades the moment another child is selected", async () => {
    mockGateway({ s1: { status: 200, subjects: [subject("Matemáticas", { p2: 9 })] }, s2: "hang" }, [mateo, lucia]);
    await render(
      <>
        <PickChild id="s2" />
        <GradesScreen today={TODAY} />
      </>,
    );
    await waitFor(() => expect(screen.getByText("Matemáticas")).toBeTruthy());

    await fireEvent.press(screen.getByText("pick s2"));

    await waitFor(() => expect(screen.queryByText("Matemáticas")).toBeNull());
  });

  it("opens a subject's sheet with its grade for every period and its average", async () => {
    mockGateway({ s1: { status: 200, subjects: [subject("Matemáticas", { p1: 8, p2: 5 }), subject("Inglés", { p2: 9 })] } });
    await render();
    await waitFor(() => expect(screen.getByLabelText(/^Matemáticas, 2\.º trimestre: 5, reprobada/)).toBeTruthy());
    expect(screen.queryByLabelText("Matemáticas, 1.er trimestre: 8")).toBeNull();

    await fireEvent.press(screen.getByLabelText(/^Matemáticas, 2\.º trimestre: 5, reprobada/));

    expect(screen.getByLabelText("Matemáticas, 1.er trimestre: 8")).toBeTruthy();
    expect(screen.getByLabelText("Matemáticas, 2.º trimestre: 5, reprobada")).toBeTruthy();
    expect(screen.getByLabelText("Matemáticas, 3.er trimestre: sin calificación aún")).toBeTruthy();
    expect(screen.getByLabelText("Promedio del ciclo: 6.5")).toBeTruthy();
    expect(screen.queryByLabelText(/Inglés, 1\.er trimestre/)).toBeNull();
  });

  it("reloads when the app comes back to the foreground", async () => {
    const fetchMock = mockGateway({ s1: { status: 200, subjects: [subject("Matemáticas", { p2: 9 })] } });
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
