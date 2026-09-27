import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as configApi from "../../config/api";
import * as gradesApi from "../../grades/api";
import * as teachersApi from "../../teachers/api";
import { DirectorCapture } from "./DirectorCapture";

vi.mock("../../config/api", () => ({
  listLabelledGroups: vi.fn(),
  listSubjects: vi.fn(),
  listLevels: vi.fn(),
  listGradeLevels: vi.fn(),
}));
vi.mock("../../teachers/api", () => ({
  listTeachers: vi.fn(),
  listTeachingAssignments: vi.fn(),
}));
vi.mock("../../grades/api", () => ({ gradeSheetClient: vi.fn() }));

const config = vi.mocked(configApi);
const teachers = vi.mocked(teachersApi);
const grades = vi.mocked(gradesApi);

const client = { load: vi.fn(), save: vi.fn(), remove: vi.fn() };
/** The field itself, not its (?) help button. */
const CAMPO = { selector: "select" } as const;

const levels = [
  { id: "prim", name: "PRIMARIA", sepCode: "", enrollmentGranularity: "cycle" as const },
  { id: "sec", name: "SECUNDARIA", sepCode: "", enrollmentGranularity: "cycle" as const },
];
const gradeLevels = [
  { id: "p1", levelId: "prim", name: "1° PRIMARIA", ordinal: 1 },
  { id: "p2", levelId: "prim", name: "2° PRIMARIA", ordinal: 2 },
  { id: "s1", levelId: "sec", name: "1° SECUNDARIA", ordinal: 1 },
];
const group = (id: string, gradeLevelId: string, name: string, label: string) => ({ id, cycleId: "c1", gradeLevelId, name, capacity: 30, label });

beforeEach(() => {
  teachers.listTeachingAssignments.mockResolvedValue([
    { id: "a-sec", cycleId: "c1", groupId: "g-s1a", subjectId: "sub-hist", teacherId: "t1" },
    { id: "a-p2", cycleId: "c1", groupId: "g-p2a", subjectId: "sub-mat2", teacherId: "t1" },
    { id: "a-p1", cycleId: "c1", groupId: "g-p1a", subjectId: "sub-mat1", teacherId: "t2" },
  ]);
  teachers.listTeachers.mockResolvedValue([
    { id: "t1", firstName: "Ada", lastName: "Lovelace", email: "a@gl.mx", userId: "" },
    { id: "t2", firstName: "Alan", lastName: "Turing", email: "b@gl.mx", userId: "" },
  ]);
  config.listLevels.mockResolvedValue(levels);
  config.listGradeLevels.mockResolvedValue(gradeLevels);
  config.listLabelledGroups.mockResolvedValue([
    group("g-s1a", "s1", "A", "1° SECUNDARIA · A"),
    group("g-p2a", "p2", "A", "2° PRIMARIA · A"),
    group("g-p1a", "p1", "A", "1° PRIMARIA · A"),
  ]);
  config.listSubjects.mockImplementation(async (_base, gradeLevelId) => ({
    p1: [{ id: "sub-mat1", gradeLevelId: "p1", name: "Matemáticas" }],
    p2: [{ id: "sub-mat2", gradeLevelId: "p2", name: "Matemáticas" }],
    s1: [{ id: "sub-hist", gradeLevelId: "s1", name: "Historia" }],
  })[gradeLevelId] ?? []);
  grades.gradeSheetClient.mockReturnValue(client);
  client.load.mockResolvedValue({
    assignmentId: "a-p2",
    month: "2026-09",
    months: ["2026-09"],
    scale: { min: 5, max: 10, decimals: 1, passing: 6 },
    editable: true,
    rows: [{ enrollmentId: "en1", studentId: "st1", name: "Grace Hopper", value: null, updatedAt: null }],
  });
});

afterEach(() => vi.clearAllMocks());

describe("DirectorCapture", () => {
  it("lists every assignment as group · subject · teacher, in school order", async () => {
    render(<DirectorCapture onBack={() => {}} />);

    const picker = await screen.findByLabelText("Asignación", CAMPO);
    await waitFor(() => expect(within(picker).getAllByRole("option")).toHaveLength(4));
    const labels = within(picker).getAllByRole("option").map((o) => o.textContent).filter((t) => t !== "");
    expect(labels).toEqual([
      "1° PRIMARIA · A · Matemáticas · Alan Turing",
      "2° PRIMARIA · A · Matemáticas · Ada Lovelace",
      "1° SECUNDARIA · A · Historia · Ada Lovelace",
    ]);
  });

  it("opens the chosen assignment's monthly sheet, with no term to pick", async () => {
    const user = userEvent.setup();
    render(<DirectorCapture onBack={() => {}} />);

    const picker = await screen.findByLabelText("Asignación", CAMPO);
    await within(picker).findByRole("option", { name: "2° PRIMARIA · A · Matemáticas · Ada Lovelace" });
    await user.selectOptions(picker, "a-p2");

    expect(await screen.findByRole("textbox", { name: "Calificación de Grace Hopper" })).toBeInTheDocument();
    expect(client.load).toHaveBeenCalledWith({ assignmentId: "a-p2" });
    expect(screen.queryByLabelText("Periodo")).toBeNull();
  });

  it("asks for an assignment before showing any sheet", async () => {
    render(<DirectorCapture onBack={() => {}} />);
    expect(await screen.findByText("Elige una asignación para capturar sus calificaciones del mes.")).toBeInTheDocument();
    expect(client.load).not.toHaveBeenCalled();
  });

  it("says so when the assignments cannot be loaded", async () => {
    teachers.listTeachingAssignments.mockRejectedValue(new Error("boom"));
    render(<DirectorCapture onBack={() => {}} />);
    expect(await screen.findByRole("alert")).toHaveTextContent("No se pudieron cargar las asignaciones.");
  });
});
