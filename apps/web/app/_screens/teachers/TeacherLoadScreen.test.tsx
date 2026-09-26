import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as configApi from "../../config/api";
import type { GradeLevel, LabelledGroup, Level, Subject } from "../../config/api";
import * as teachersApi from "../../teachers/api";
import type { TeachingAssignment } from "../../teachers/api";
import { TeacherLoadScreen } from "./TeacherLoadScreen";

vi.mock("../../teachers/api", async (importOriginal) => ({
  ...(await importOriginal<typeof teachersApi>()),
  listTeachers: vi.fn(),
  listTeachingAssignments: vi.fn(),
  assignTeachingLoad: vi.fn(),
  deleteTeachingAssignment: vi.fn(),
}));
vi.mock("../../config/api", async (importOriginal) => ({
  ...(await importOriginal<typeof configApi>()),
  listCycles: vi.fn(),
  listLevels: vi.fn(),
  listGradeLevels: vi.fn(),
  listLabelledGroups: vi.fn(),
  listSubjects: vi.fn(),
}));

const teachers = vi.mocked(teachersApi);
const config = vi.mocked(configApi);

/** Asks for the control itself, not its (?) help button. */
const CAMPO = { selector: "input, select, textarea" } as const;

const levels: Level[] = [
  { id: "prim", name: "Primaria", sepCode: "", enrollmentGranularity: "cycle" },
  { id: "sec", name: "Secundaria", sepCode: "", enrollmentGranularity: "cycle" },
];
const grades: GradeLevel[] = [
  { id: "p1", levelId: "prim", name: "1° Primaria", ordinal: 1 },
  { id: "s1", levelId: "sec", name: "1° Secundaria", ordinal: 1 },
  { id: "s2", levelId: "sec", name: "2° Secundaria", ordinal: 2 },
];
const group = (id: string, gradeLevelId: string, name: string, gradeName: string): LabelledGroup => ({
  id,
  cycleId: "c1",
  gradeLevelId,
  name,
  capacity: 30,
  label: `${gradeName} · ${name}`,
});
const groups: LabelledGroup[] = [
  group("p1A", "p1", "A", "1° Primaria"),
  group("s1A", "s1", "A", "1° Secundaria"),
  group("s1B", "s1", "B", "1° Secundaria"),
  group("s2A", "s2", "A", "2° Secundaria"),
];
const subjectsByGrade: Record<string, Subject[]> = {
  p1: [{ id: "mat-p1", gradeLevelId: "p1", name: "Matemáticas" }],
  s1: [
    { id: "mat-s1", gradeLevelId: "s1", name: "Matemáticas" },
    { id: "fis-s1", gradeLevelId: "s1", name: "Física" },
  ],
  s2: [{ id: "mat-s2", gradeLevelId: "s2", name: "Matemáticas" }],
};
const ada = { id: "ada", firstName: "Ada", lastName: "Lovelace", email: "ada@escuela.mx", userId: "" };
const ana = { id: "ana", firstName: "Ana", lastName: "Ruiz", email: "ana@escuela.mx", userId: "" };
const ta = (id: string, groupId: string, subjectId: string, teacherId: string): TeachingAssignment => ({
  id,
  cycleId: "c1",
  groupId,
  subjectId,
  teacherId,
});
const emptyResult = { created: [], reassigned: [], skipped: [], conflicts: [] };

function withAssignments(assignments: TeachingAssignment[]): void {
  teachers.listTeachingAssignments.mockResolvedValue(assignments);
}

beforeEach(() => {
  teachers.listTeachers.mockResolvedValue([ada, ana]);
  withAssignments([]);
  teachers.assignTeachingLoad.mockResolvedValue(emptyResult);
  teachers.deleteTeachingAssignment.mockResolvedValue(undefined);
  config.listCycles.mockResolvedValue([
    { id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" },
  ]);
  config.listLevels.mockResolvedValue(levels);
  config.listGradeLevels.mockResolvedValue(grades);
  config.listLabelledGroups.mockResolvedValue(groups);
  config.listSubjects.mockImplementation(async (_base, gradeLevelId) => subjectsByGrade[gradeLevelId] ?? []);
});
afterEach(() => vi.clearAllMocks());

async function renderPage(onBack = vi.fn()): Promise<ReturnType<typeof userEvent.setup>> {
  const user = userEvent.setup();
  render(<TeacherLoadScreen teacherId="ada" onBack={onBack} />);
  await screen.findByRole("heading", { name: "Carga académica de Ada Lovelace" });
  return user;
}

async function chooseSubject(user: ReturnType<typeof userEvent.setup>, name: string): Promise<void> {
  await user.selectOptions(await screen.findByLabelText("Asignatura", CAMPO), name);
}

describe("TeacherLoadScreen", () => {
  it("names the teacher and goes back to Docentes", async () => {
    const onBack = vi.fn();
    const user = await renderPage(onBack);

    await user.click(screen.getByRole("button", { name: /Volver a Docentes/ }));
    expect(onBack).toHaveBeenCalled();
  });

  it("reads the ACTIVE cycle when there are several, and offers the others", async () => {
    config.listCycles.mockResolvedValue([
      { id: "c0", name: "2025-2026", status: "closed", startsOn: "2025-08-01", endsOn: "2026-06-30" },
      { id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" },
    ]);
    await renderPage();

    await waitFor(() => expect(config.listLabelledGroups).toHaveBeenCalledWith(expect.any(String), "c1"));
    expect(teachers.listTeachingAssignments).toHaveBeenCalledWith(expect.any(String), { cycleId: "c1" });
    expect(screen.getByLabelText("Ciclo", CAMPO)).toHaveValue("c1");
  });

  it("shows an empty state when the teacher has no load", async () => {
    await renderPage();
    expect(await screen.findByText(/Aún no tiene asignaturas en este ciclo/)).toBeInTheDocument();
  });

  it("groups the current load by subject name, groups as chips", async () => {
    withAssignments([
      ta("a1", "s1A", "mat-s1", "ada"),
      ta("a2", "s2A", "mat-s2", "ada"),
      ta("a3", "s1B", "fis-s1", "ada"),
      ta("x1", "p1A", "mat-p1", "ana"),
    ]);
    await renderPage();

    const math = await screen.findByRole("region", { name: "Matemáticas" });
    expect(within(math).getByText("1° Secundaria · A")).toBeInTheDocument();
    expect(within(math).getByText("2° Secundaria · A")).toBeInTheDocument();
    expect(within(math).queryByText("1° Primaria · A")).not.toBeInTheDocument();
    const physics = screen.getByRole("region", { name: "Física" });
    expect(within(physics).getByText("1° Secundaria · B")).toBeInTheDocument();
  });

  it("removes a chip at once, and Deshacer gives the slot back through bulk", async () => {
    withAssignments([ta("a1", "s1A", "mat-s1", "ada")]);
    const user = await renderPage();

    await user.click(await screen.findByRole("button", { name: "Quitar Matemáticas de 1° Secundaria · A" }));

    expect(teachers.deleteTeachingAssignment).toHaveBeenCalledWith(expect.any(String), "a1");
    expect(await screen.findByText("Quitaste Matemáticas de 1° Secundaria · A")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Quitar Matemáticas/ })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Deshacer" }));
    expect(teachers.assignTeachingLoad).toHaveBeenCalledWith(expect.any(String), {
      teacherId: "ada",
      items: [{ groupId: "s1A", subjectId: "mat-s1" }],
      replaceConflicts: false,
    });
  });

  it("resolves the subject by name to each grade's own row", async () => {
    teachers.assignTeachingLoad.mockResolvedValue({
      ...emptyResult,
      created: [ta("n1", "p1A", "mat-p1", "ada"), ta("n2", "s2A", "mat-s2", "ada")],
    });
    const user = await renderPage();
    await chooseSubject(user, "Matemáticas");

    await user.click(screen.getByRole("checkbox", { name: "1° Primaria · A" }));
    await user.click(screen.getByRole("checkbox", { name: "2° Secundaria · A" }));
    await user.click(screen.getByRole("button", { name: "Asignar (2 grupos)" }));

    expect(teachers.assignTeachingLoad).toHaveBeenCalledWith(expect.any(String), {
      teacherId: "ada",
      items: [
        { groupId: "p1A", subjectId: "mat-p1" },
        { groupId: "s2A", subjectId: "mat-s2" },
      ],
      replaceConflicts: false,
    });
    expect(await screen.findByText("2 grupos asignados")).toBeInTheDocument();
  });

  it("disables a group whose grade lacks the subject, saying why", async () => {
    const user = await renderPage();
    await chooseSubject(user, "Física");

    const s2A = screen.getByRole("checkbox", { name: "2° Secundaria · A" });
    expect(s2A).toBeDisabled();
    expect(s2A).toHaveAccessibleDescription("2° Secundaria no tiene Física");
    expect(screen.getByRole("checkbox", { name: "1° Primaria · A" })).toBeDisabled();
    expect(screen.getByRole("checkbox", { name: "1° Secundaria · A" })).toBeEnabled();
  });

  it("marks groups already this teacher's, and labels those another teacher holds", async () => {
    withAssignments([ta("a1", "s1A", "mat-s1", "ada"), ta("x1", "s1B", "mat-s1", "ana")]);
    const user = await renderPage();
    await chooseSubject(user, "Matemáticas");

    const mine = screen.getByRole("checkbox", { name: "1° Secundaria · A" });
    expect(mine).toBeChecked();
    expect(mine).toBeDisabled();
    expect(mine).toHaveAccessibleDescription("ya asignado");
    const held = screen.getByRole("checkbox", { name: "1° Secundaria · B" });
    expect(held).toBeEnabled();
    expect(held).toHaveAccessibleDescription("La imparte Ana Ruiz");
  });

  it("'Todos los de Secundaria' picks only the eligible groups of that level, tri-state", async () => {
    withAssignments([ta("a1", "s1A", "mat-s1", "ada")]);
    const user = await renderPage();
    await chooseSubject(user, "Matemáticas");

    const all = screen.getByRole("checkbox", { name: "Todos los de Secundaria" });
    await user.click(all);

    expect(screen.getByRole("checkbox", { name: "1° Secundaria · B" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "2° Secundaria · A" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "1° Primaria · A" })).not.toBeChecked();
    expect(all).toBeChecked();

    await user.click(screen.getByRole("checkbox", { name: "2° Secundaria · A" }));
    expect((all as HTMLInputElement).indeterminate).toBe(true);

    await user.click(screen.getByRole("button", { name: "Asignar (1 grupo)" }));
    expect(teachers.assignTeachingLoad).toHaveBeenCalledWith(expect.any(String), {
      teacherId: "ada",
      items: [{ groupId: "s1B", subjectId: "mat-s1" }],
      replaceConflicts: false,
    });
  });

  it("'Todos' leaves out a grade that lacks the subject", async () => {
    const user = await renderPage();
    await chooseSubject(user, "Física");

    await user.click(screen.getByRole("checkbox", { name: "Todos los de Secundaria" }));
    expect(screen.getByRole("button", { name: "Asignar (2 grupos)" })).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "2° Secundaria · A" })).not.toBeChecked();
  });

  it("offers Reemplazar for conflicts and re-sends ONLY those with replaceConflicts", async () => {
    teachers.assignTeachingLoad.mockResolvedValueOnce({
      ...emptyResult,
      created: [ta("n1", "s2A", "mat-s2", "ada")],
      conflicts: [{ groupId: "s1B", subjectId: "mat-s1", assignmentId: "x1", teacherId: "ana", teacherName: "Ana Ruiz" }],
    });
    const user = await renderPage();
    await chooseSubject(user, "Matemáticas");
    await user.click(screen.getByRole("checkbox", { name: "1° Secundaria · B" }));
    await user.click(screen.getByRole("checkbox", { name: "2° Secundaria · A" }));
    await user.click(screen.getByRole("button", { name: "Asignar (2 grupos)" }));

    const notice = await screen.findByRole("region", { name: /ya tienen? docente/ });
    expect(within(notice).getByText(/Matemáticas · 1° Secundaria · B/)).toBeInTheDocument();
    expect(within(notice).getByText(/La imparte Ana Ruiz/)).toBeInTheDocument();

    await user.click(within(notice).getByRole("button", { name: "Reemplazar" }));
    expect(teachers.assignTeachingLoad).toHaveBeenLastCalledWith(expect.any(String), {
      teacherId: "ada",
      items: [{ groupId: "s1B", subjectId: "mat-s1" }],
      replaceConflicts: true,
    });
    await waitFor(() => expect(screen.queryByRole("region", { name: /ya tienen? docente/ })).not.toBeInTheDocument());
  });

  it("'Dejar como está' closes the conflict notice without writing", async () => {
    teachers.assignTeachingLoad.mockResolvedValueOnce({
      ...emptyResult,
      conflicts: [{ groupId: "s1B", subjectId: "mat-s1", assignmentId: "x1", teacherId: "ana", teacherName: "Ana Ruiz" }],
    });
    const user = await renderPage();
    await chooseSubject(user, "Matemáticas");
    await user.click(screen.getByRole("checkbox", { name: "1° Secundaria · B" }));
    await user.click(screen.getByRole("button", { name: "Asignar (1 grupo)" }));

    await user.click(await screen.findByRole("button", { name: "Dejar como está" }));
    expect(screen.queryByRole("region", { name: /ya tienen? docente/ })).not.toBeInTheDocument();
    expect(teachers.assignTeachingLoad).toHaveBeenCalledTimes(1);
  });

  it("never disables Asignar: with no subject it says so and focuses the subject", async () => {
    const user = await renderPage();
    const submit = await screen.findByRole("button", { name: "Asignar (0 grupos)" });
    expect(submit).toBeEnabled();

    await user.click(submit);

    const subject = screen.getByLabelText("Asignatura", CAMPO);
    expect(subject).toHaveAttribute("aria-invalid", "true");
    await waitFor(() => expect(subject).toHaveFocus());
    expect(teachers.assignTeachingLoad).not.toHaveBeenCalled();
  });

  it("with a subject but no group it says so and focuses the first control of the grid", async () => {
    const user = await renderPage();
    await chooseSubject(user, "Matemáticas");

    await user.click(screen.getByRole("button", { name: "Asignar (0 grupos)" }));

    expect(screen.getByText("Elige al menos un grupo.")).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "Todos los de Primaria" })).toHaveFocus();
    expect(teachers.assignTeachingLoad).not.toHaveBeenCalled();
  });
});
