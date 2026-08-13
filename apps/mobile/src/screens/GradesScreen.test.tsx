import { screen, waitFor } from "@testing-library/react-native";
import { clearTokens, saveTokens } from "../api";
import { GradesScreen } from "./GradesScreen";
import { renderWithProviders } from "../test/renderWithProviders";

type Child = { id: string; firstName: string; lastName: string; level: string; gradeGroup: string };
type Entry = { id: string; subjectId: string; termId: string; teacherId: string; value: number; subjectName: string; termName: string };

/** Routes each family request; `entries` null makes the grades endpoint fail. */
function mockFetch(child: Child, entries: Entry[] | null): jest.Mock {
  const fetchMock = jest.fn(async (url: string) => {
    const u = String(url);
    if (u.includes("/family/students/s1/grades")) {
      if (entries === null) return { ok: false, status: 500, json: async () => ({}) };
      return { ok: true, status: 200, json: async () => ({ gradeEntries: entries }) };
    }
    if (u.includes("/family/students")) return { ok: true, status: 200, json: async () => ({ students: [child] }) };
    return { ok: true, status: 200, json: async () => ({ notifications: [] }) };
  });
  globalThis.fetch = fetchMock as unknown as typeof fetch;
  return fetchMock;
}

const primaria: Child = { id: "s1", firstName: "Mateo", lastName: "Herrera", level: "primaria", gradeGroup: "3° A" };
const kinder: Child = { id: "s1", firstName: "Sofía", lastName: "Herrera", level: "kinder", gradeGroup: "K3" };

function entry(subjectName: string, termName: string, value: number, id: string): Entry {
  return { id, subjectId: `sub-${subjectName}`, termId: `t-${termName}`, teacherId: "te1", value, subjectName, termName };
}

describe("GradesScreen (real family grades)", () => {
  beforeEach(async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });
  afterEach(async () => {
    await clearTokens();
  });

  it("groups numeric entries by subject and shows the real average", async () => {
    mockFetch(primaria, [
      entry("Español", "1er periodo", 9.2, "g1"),
      entry("Español", "2do periodo", 9.0, "g2"),
      entry("Matemáticas", "1er periodo", 8.0, "g3"),
    ]);

    await renderWithProviders(<GradesScreen />);

    await waitFor(() => expect(screen.getByText("Español")).toBeTruthy());
    expect(screen.getByText("Matemáticas")).toBeTruthy();
    // Promedio general = (9.2 + 9.0 + 8.0) / 3 = 8.73 → "8.7".
    expect(screen.getByLabelText("Promedio").props.children).toBe("8.7");
  });

  it("keeps subjects distinct by id when the catalog degrades names to empty (bug_004)", async () => {
    // A config outage makes the gateway return subjectName === "" for every entry
    // (a tested contract). Grouping by name would collapse them all into one bogus
    // "Materia" row; grouping by the stable subjectId must keep them distinct.
    mockFetch(primaria, [
      { id: "g1", subjectId: "sub-a", termId: "t1", teacherId: "te1", value: 10, subjectName: "", termName: "1er periodo" },
      { id: "g2", subjectId: "sub-b", termId: "t1", teacherId: "te1", value: 6, subjectName: "", termName: "1er periodo" },
      { id: "g3", subjectId: "sub-c", termId: "t1", teacherId: "te1", value: 8, subjectName: "", termName: "1er periodo" },
    ]);

    await renderWithProviders(<GradesScreen />);

    // Three distinct subjects → three rows, each its own average, NOT one merged row.
    await waitFor(() => expect(screen.getByText("3 materias")).toBeTruthy());
    const nameless = screen.getAllByText("Materia sin nombre");
    expect(nameless).toHaveLength(3);
  });

  it("upgrades the fallback label when a later entry for the same subject carries a name", async () => {
    mockFetch(primaria, [
      { id: "g1", subjectId: "sub-a", termId: "t1", teacherId: "te1", value: 9, subjectName: "", termName: "1er periodo" },
      { id: "g2", subjectId: "sub-a", termId: "t2", teacherId: "te1", value: 9, subjectName: "Español", termName: "2do periodo" },
    ]);

    await renderWithProviders(<GradesScreen />);

    await waitFor(() => expect(screen.getByText("1 materias")).toBeTruthy());
    expect(screen.getByText("Español")).toBeTruthy();
    expect(screen.queryByText("Materia sin nombre")).toBeNull();
  });

  it("shows the qualitative-pending state for a kínder child (no numeric grades fabricated)", async () => {
    mockFetch(kinder, [entry("Pensamiento", "1er periodo", 10, "g1")]);

    await renderWithProviders(<GradesScreen />);

    await waitFor(() => expect(screen.getByText("Próximamente")).toBeTruthy());
    // The numeric average must never be shown for a qualitative level.
    expect(screen.queryByText("Promedio general")).toBeNull();
  });

  it("shows an empty state when a numeric child has no entries yet", async () => {
    mockFetch(primaria, []);

    await renderWithProviders(<GradesScreen />);

    await waitFor(() => expect(screen.getByText("Aún no hay calificaciones")).toBeTruthy());
  });

  it("shows an error state with retry when the load fails", async () => {
    mockFetch(primaria, null);

    await renderWithProviders(<GradesScreen />);

    await waitFor(() => expect(screen.getByText("No pudimos cargar las calificaciones")).toBeTruthy());
    expect(screen.getByRole("button", { name: "Reintentar" })).toBeTruthy();
  });

  it("shows the error state (not a fabricated empty state) when the child bootstrap fails (bug_005)", async () => {
    const fetchMock = jest.fn(async (url: string) => {
      const u = String(url);
      if (u.includes("/family/students")) return { ok: false, status: 500, json: async () => ({}) };
      return { ok: true, status: 200, json: async () => ({ notifications: [] }) };
    });
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    await renderWithProviders(<GradesScreen />);

    await waitFor(() => expect(screen.getByText("No pudimos cargar las calificaciones")).toBeTruthy());
    expect(screen.getByRole("button", { name: "Reintentar" })).toBeTruthy();
    expect(screen.queryByText("Aún no hay calificaciones")).toBeNull();
  });
});
