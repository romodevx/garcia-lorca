import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as studentsApi from "../../students/api";
import * as configApi from "../../config/api";
import * as csv from "../../_lib/csv";
import { StudentsScreen } from "./StudentsScreen";

// T4: level → grade → group chips, chained, over the pupils' ACTIVE enrolment
// of the cycle; the view lives in the URL; "Seleccionar todos" over the result.

vi.mock("../../students/api", async (importOriginal) => ({
  ...(await importOriginal<typeof studentsApi>()),
  listStudents: vi.fn(),
  listEnrollments: vi.fn(),
}));
vi.mock("../../config/api", () => ({
  listCycles: vi.fn(),
  listGroups: vi.fn(),
  listLabelledGroups: vi.fn(),
  listLevels: vi.fn(),
  listGradeLevels: vi.fn(),
}));
vi.mock("../../_lib/csv", async (importOriginal) => ({
  ...(await importOriginal<typeof csv>()),
  downloadCsv: vi.fn(),
}));

const students = vi.mocked(studentsApi);
const config = vi.mocked(configApi);

const pupil = (id: string, last: string, first: string, levelId: string, level: string, gradeGroup: string, status: studentsApi.AdmissionStatus = "enrolled"): studentsApi.Student => ({
  id, curp: `CURP${id}`, firstName: first, lastName: last, level, levelId, gradeGroup, guardianName: "Tutor", admissionStatus: status,
});
const active = (studentId: string, groupId: string): studentsApi.Enrollment => ({
  id: `e-${studentId}`, studentId, cycleId: "c1", groupId, termId: "", status: "active", enrolledAt: "2026-08-20",
});
const group = (id: string, gradeLevelId: string, name: string, label: string): configApi.LabelledGroup => ({
  id, cycleId: "c1", gradeLevelId, name, capacity: 25, label,
} as configApi.LabelledGroup);

beforeEach(() => {
  window.history.replaceState(null, "", "/alumnos");
  students.listStudents.mockResolvedValue([
    pupil("1", "Arce", "Ana", "pre", "Preescolar", "A"),
    pupil("2", "Bello", "Beto", "pre", "Preescolar", "B"),
    pupil("3", "Cano", "Ceci", "pre", "Preescolar", "B", "pending"),
    pupil("4", "Duarte", "Dani", "pri", "Primaria", "B"),
    pupil("5", "Estrada", "Eva", "pre", "Preescolar", ""),
  ]);
  students.listEnrollments.mockResolvedValue([active("1", "pre1a"), active("2", "pre1b"), active("3", "pre1b"), active("4", "pri1b")]);
  config.listCycles.mockResolvedValue([{ id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" }]);
  config.listLevels.mockResolvedValue([
    { id: "pre", name: "Preescolar", sepCode: "PRE", enrollmentGranularity: "cycle" },
    { id: "pri", name: "Primaria", sepCode: "PRI", enrollmentGranularity: "cycle" },
  ]);
  config.listGradeLevels.mockResolvedValue([
    { id: "pre-1", levelId: "pre", name: "1°", ordinal: 1 },
    { id: "pri-1", levelId: "pri", name: "1°", ordinal: 1 },
  ]);
  config.listLabelledGroups.mockResolvedValue([
    group("pre1a", "pre-1", "A", "1° PREESCOLAR · A"),
    group("pre1b", "pre-1", "B", "1° PREESCOLAR · B"),
    group("pri1b", "pri-1", "B", "1° PRIMARIA · B"),
  ]);
});
afterEach(() => vi.clearAllMocks());

const rows = (): string[] =>
  screen.queryAllByRole("button", { name: /^Ver ficha de / }).map((b) => (b.getAttribute("aria-label") ?? "").replace("Ver ficha de ", ""));
const chipIn = (row: string, name: RegExp): HTMLElement => within(screen.getByRole("group", { name: row })).getByRole("button", { name });

describe("StudentsScreen — filtros encadenados (T4)", () => {
  it("reads the cycle's enrolments once, for the active cycle", async () => {
    render(<StudentsScreen onBack={() => {}} />);
    await screen.findByText("Arce Ana");
    await waitFor(() => expect(students.listEnrollments).toHaveBeenCalledWith(expect.any(String), { cycleId: "c1" }));
  });

  it("reaches 1°B preescolar in three clicks", async () => {
    const user = userEvent.setup();
    render(<StudentsScreen onBack={() => {}} />);
    await screen.findByText("Arce Ana");

    await user.click(chipIn("Nivel", /^Preescolar/));
    await user.click(await waitFor(() => chipIn("Grado", /^1°/)));
    await user.click(chipIn("Grupo", /^B/));

    expect(rows()).toEqual(["Bello Beto", "Cano Ceci"]);
  });

  it("names the classroom with its grade, not only the letter", async () => {
    render(<StudentsScreen onBack={() => {}} />);
    await screen.findByText("Arce Ana");
    await waitFor(() => expect(screen.getByTestId("per-group")).toHaveTextContent("1° PRIMARIA · B"));
    expect(screen.getAllByText("1° PREESCOLAR · B").length).toBeGreaterThan(0);
  });

  it("keeps the view in the URL, so Back comes back to it", async () => {
    const user = userEvent.setup();
    render(<StudentsScreen onBack={() => {}} />);
    await screen.findByText("Arce Ana");

    await user.click(screen.getByRole("button", { name: /^Trámite/ }));
    await user.click(chipIn("Nivel", /^Preescolar/));

    const params = new URLSearchParams(window.location.search);
    expect(params.get("nivel")).toBe("pre");
    expect(params.get("estado")).toBe("pending");
    expect(window.location.pathname).toBe("/alumnos");
  });

  it("opens with the view the URL names", async () => {
    window.history.replaceState(null, "", "/alumnos?nivel=pre&grado=pre-1&grupo=pre1b&estado=active");
    render(<StudentsScreen onBack={() => {}} />);
    await waitFor(() => expect(rows()).toEqual(["Bello Beto"]));
    expect(chipIn("Grupo", /^B/)).toHaveAttribute("aria-pressed", "true");
  });

  it("selects exactly the filtered pupils and exports only them", async () => {
    const user = userEvent.setup();
    render(<StudentsScreen onBack={() => {}} />);
    await screen.findByText("Arce Ana");

    await user.click(chipIn("Nivel", /^Preescolar/));
    await user.click(screen.getByRole("button", { name: "Seleccionar todos (4)" }));
    expect(screen.getByRole("status")).toHaveTextContent("4 seleccionados");

    await user.click(screen.getByRole("checkbox", { name: "Seleccionar a Arce Ana" }));
    expect(screen.getByRole("status")).toHaveTextContent("3 seleccionados");

    await user.click(screen.getByRole("button", { name: "Exportar selección" }));
    const file = vi.mocked(csv.downloadCsv).mock.calls[0]?.[1] ?? "";
    expect(file).toContain("Bello");
    expect(file).toContain("Estrada");
    expect(file).not.toContain("Arce");
    expect(file).not.toContain("Duarte");
  });

  it("Limpiar also returns the status tab to Todos", async () => {
    const user = userEvent.setup();
    render(<StudentsScreen onBack={() => {}} />);
    await screen.findByText("Arce Ana");
    await user.click(screen.getByRole("button", { name: /^Trámite/ }));
    expect(rows()).toEqual(["Cano Ceci"]);

    await user.click(screen.getByRole("button", { name: "Limpiar" }));
    expect(rows()).toHaveLength(5);
  });
});
