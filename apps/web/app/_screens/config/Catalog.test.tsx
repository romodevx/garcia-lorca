import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as api from "../../config/api";
import { Catalog } from "./Catalog";

// The screen talks to the gateway exclusively through the config api client, so
// we mock it wholesale and assert the calls (the fetch itself is covered by
// app/config/api.test.ts). Neutral literals only.
vi.mock("../../config/api", () => ({
  listCycles: vi.fn(),
  createCycle: vi.fn(),
  listLevels: vi.fn(),
  createLevel: vi.fn(),
  listGradeLevels: vi.fn(),
  createGradeLevel: vi.fn(),
  listSubjects: vi.fn(),
  createSubject: vi.fn(),
  listGroups: vi.fn(),
  createGroup: vi.fn(),
  listTerms: vi.fn(),
  createTerm: vi.fn(),
  listTeachers: vi.fn(),
  createTeacher: vi.fn(),
  listTeachingAssignments: vi.fn(),
  createTeachingAssignment: vi.fn(),
}));

const mocked = vi.mocked(api);

beforeEach(() => {
  mocked.listCycles.mockResolvedValue([]);
  mocked.listLevels.mockResolvedValue([]);
  mocked.listGradeLevels.mockResolvedValue([]);
  mocked.listSubjects.mockResolvedValue([]);
  mocked.listGroups.mockResolvedValue([]);
  mocked.listTerms.mockResolvedValue([]);
  mocked.listTeachers.mockResolvedValue([]);
  mocked.listTeachingAssignments.mockResolvedValue([]);
});

afterEach(() => vi.clearAllMocks());

describe("Catalog — cycles", () => {
  it("creates a cycle and lists it", async () => {
    const user = userEvent.setup();
    mocked.createCycle.mockResolvedValue({
      id: "c1",
      name: "2026-2027",
      status: "draft",
      startsOn: "2026-08-01",
      endsOn: "2027-06-30",
    });

    render(<Catalog onBack={() => {}} />);

    // Initial load settles into the cycle form (no cycles yet).
    const nameInput = await screen.findByLabelText("Nombre del ciclo");
    await user.type(nameInput, "2026-2027");
    fireEvent.change(screen.getByLabelText("Inicio del ciclo"), { target: { value: "2026-08-01" } });
    fireEvent.change(screen.getByLabelText("Fin del ciclo"), { target: { value: "2027-06-30" } });

    await user.click(screen.getByRole("button", { name: "Crear ciclo" }));

    await waitFor(() =>
      expect(mocked.createCycle).toHaveBeenCalledWith(expect.any(String), {
        name: "2026-2027",
        startsOn: "2026-08-01",
        endsOn: "2027-06-30",
      }),
    );
    // The freshly created cycle now appears in the picker (as a selectable chip).
    // Scope to the chip button — the cycle name also appears as an <option> in
    // the assignments cycle dropdown, so a bare text query would be ambiguous.
    expect(await screen.findByRole("button", { name: /2026-2027/ })).toBeInTheDocument();
  });
});

describe("Catalog — nested level → grade level", () => {
  it("adds a level, then a grade level under it with an auto-suggested ordinal", async () => {
    const user = userEvent.setup();
    mocked.listCycles.mockResolvedValue([
      { id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" },
    ]);
    mocked.createLevel.mockResolvedValue({
      id: "l1",
      name: "Primaria",
      sepCode: "PRI",
      enrollmentGranularity: "cycle",
    });
    mocked.createGradeLevel.mockResolvedValue({ id: "g1", levelId: "l1", name: "1° primaria", ordinal: 1 });

    render(<Catalog onBack={() => {}} />);

    // Create a level. Granularity is a required, defaultless choice.
    await user.type(await screen.findByLabelText("Nombre del nivel"), "Primaria");
    await user.type(screen.getByLabelText("Clave SEP"), "PRI");
    await user.click(screen.getByRole("radio", { name: "Por ciclo" }));
    await user.click(screen.getByRole("button", { name: "Crear nivel" }));

    await waitFor(() =>
      expect(mocked.createLevel).toHaveBeenCalledWith(expect.any(String), {
        name: "Primaria",
        sepCode: "PRI",
        enrollmentGranularity: "cycle",
      }),
    );

    // Creating the level selects it, which loads its (empty) grade levels and
    // reveals the grade-level form with the next ordinal (1) pre-filled.
    await waitFor(() => expect(mocked.listGradeLevels).toHaveBeenCalledWith(expect.any(String), "l1"));

    const gradeForm = await screen.findByRole("form", { name: "Agregar grado" });
    await user.type(within(gradeForm).getByLabelText("Nombre del grado"), "1° primaria");
    await user.click(within(gradeForm).getByRole("button", { name: "Crear grado" }));

    await waitFor(() =>
      expect(mocked.createGradeLevel).toHaveBeenCalledWith(expect.any(String), {
        levelId: "l1",
        name: "1° primaria",
        ordinal: 1,
      }),
    );
  });
});

describe("Catalog — teachers", () => {
  it("creates a teacher with first/last name + email (userId is never sent)", async () => {
    const user = userEvent.setup();
    mocked.createTeacher.mockResolvedValue({
      id: "tch1",
      firstName: "Ada",
      lastName: "Lovelace",
      email: "ada@example.com",
      userId: "",
    });

    render(<Catalog onBack={() => {}} />);

    const form = await screen.findByRole("form", { name: "Agregar profesor" });
    await user.type(within(form).getByLabelText("Nombre"), "Ada");
    await user.type(within(form).getByLabelText("Apellidos"), "Lovelace");
    await user.type(within(form).getByLabelText("Correo"), "ada@example.com");
    await user.click(within(form).getByRole("button", { name: "Crear profesor" }));

    await waitFor(() =>
      expect(mocked.createTeacher).toHaveBeenCalledWith(expect.any(String), {
        firstName: "Ada",
        lastName: "Lovelace",
        email: "ada@example.com",
      }),
    );
    // The new teacher appears in the roster.
    expect(await screen.findByText(/Ada Lovelace/)).toBeInTheDocument();
  });
});

describe("Catalog — teaching assignments (the knot)", () => {
  it("scopes the subject dropdown to the selected group's grade level and assigns without cycleId", async () => {
    const user = userEvent.setup();
    mocked.listCycles.mockResolvedValue([
      { id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" },
    ]);
    mocked.listTeachers.mockResolvedValue([
      { id: "tch1", firstName: "Ada", lastName: "Lovelace", email: "ada@example.com", userId: "" },
    ]);
    // The active cycle auto-loads its groups; the group carries gradeLevelId "g1".
    mocked.listGroups.mockResolvedValue([{ id: "grp1", cycleId: "c1", gradeLevelId: "g1", name: "A", capacity: 30 }]);
    // Subjects are fetched for the GROUP's grade level, not a navigation choice.
    mocked.listSubjects.mockResolvedValue([{ id: "s1", gradeLevelId: "g1", name: "Matemáticas" }]);
    mocked.createTeachingAssignment.mockResolvedValue({
      id: "ta1",
      cycleId: "c1",
      groupId: "grp1",
      subjectId: "s1",
      teacherId: "tch1",
    });

    render(<Catalog onBack={() => {}} />);

    const form = await screen.findByRole("form", { name: "Asignar profesor" });
    // Cycle auto-selected → groups loaded for that cycle.
    await waitFor(() => expect(mocked.listGroups).toHaveBeenCalledWith(expect.any(String), "c1"));

    await user.selectOptions(within(form).getByLabelText("Grupo"), "grp1");

    // The subject list is scoped to the GROUP's grade level (g1), never a global list.
    await waitFor(() => expect(mocked.listSubjects).toHaveBeenCalledWith(expect.any(String), "g1"));
    const subjectSelect = within(form).getByLabelText("Materia");
    expect(await within(subjectSelect).findByRole("option", { name: "Matemáticas" })).toBeInTheDocument();

    await user.selectOptions(subjectSelect, "s1");
    await user.selectOptions(within(form).getByLabelText("Profesor"), "tch1");
    await user.click(within(form).getByRole("button", { name: "Asignar profesor" }));

    await waitFor(() =>
      expect(mocked.createTeachingAssignment).toHaveBeenCalledWith(expect.any(String), {
        groupId: "grp1",
        subjectId: "s1",
        teacherId: "tch1",
      }),
    );
    // cycleId must never be part of the create payload — the server derives it.
    const payload = mocked.createTeachingAssignment.mock.calls[0]?.[1];
    expect(payload).not.toHaveProperty("cycleId");
  });
});
