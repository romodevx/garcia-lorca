import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as gradesApi from "../../grades/api";
import * as configApi from "../../config/api";
import * as teachersApi from "../../teachers/api";
import * as studentsApi from "../../students/api";
import { GradeCapture } from "./GradeCapture";

// The screens read labelled groups (ISSUE-002); label them from the listGroups mock.
beforeEach(() => {
  vi.mocked(configApi.listLabelledGroups).mockImplementation(async (...args) =>
    (await vi.mocked(configApi.listGroups)(...args)).map((g) => ({ ...g, label: g.name })));
});


// The surface talks to the gateway exclusively through the grades + config +
// students api clients, so we mock all three wholesale and assert the calls (the
// fetch itself is covered by app/grades/api.test.ts). Neutral literals only.
vi.mock("../../grades/api", () => ({
  captureGrade: vi.fn(),
  getRoster: vi.fn(),
  listGradeEntries: vi.fn(),
}));
vi.mock("../../config/api", () => ({
  listTerms: vi.fn(),
  listGroups: vi.fn(), listLabelledGroups: vi.fn(),
  listSubjects: vi.fn(),
}));
vi.mock("../../teachers/api", () => ({
  listTeachers: vi.fn(),
  listTeachingAssignments: vi.fn(),
}));
vi.mock("../../students/api", () => ({
  listStudents: vi.fn(),
}));

const grades = vi.mocked(gradesApi);
const config = vi.mocked(configApi);
const teachers = vi.mocked(teachersApi);
const students = vi.mocked(studentsApi);

const assignment = { id: "asg1", cycleId: "c1", groupId: "grp1", subjectId: "sub1", teacherId: "tch1" };
const group = { id: "grp1", cycleId: "c1", gradeLevelId: "gl1", name: "4° A", capacity: 30 };
const subject = { id: "sub1", gradeLevelId: "gl1", name: "Matemáticas" };
const teacher = { id: "tch1", firstName: "Miguel", lastName: "Vázquez", email: "docente@example.com", userId: "" };
const term = { id: "t1", cycleId: "c1", levelId: "lvl1", name: "Bloque 3", ordinal: 3, startsOn: "2026-01-01", endsOn: "2026-03-31" };

const ada = {
  id: "st1",
  curp: "AAAA000000AAAAAA01",
  firstName: "Ada",
  lastName: "Lovelace",
  level: "Primaria",
  levelId: "",
  gradeGroup: "4° A",
  guardianName: "Marisol Ríos",
  admissionStatus: "enrolled" as const,
};

const rosterRow = { enrollmentId: "en1", studentId: "st1", value: 8, hasValue: true };

beforeEach(() => {
  teachers.listTeachingAssignments.mockResolvedValue([assignment]);
  config.listGroups.mockResolvedValue([group]);
  config.listSubjects.mockResolvedValue([subject]);
  teachers.listTeachers.mockResolvedValue([teacher]);
  config.listTerms.mockResolvedValue([term]);
  students.listStudents.mockResolvedValue([ada]);
  grades.getRoster.mockResolvedValue([rosterRow]);
});

afterEach(() => vi.clearAllMocks());

/** Selects the assignment + term and waits for the roster grid to render. */
async function openRoster(user: ReturnType<typeof userEvent.setup>): Promise<HTMLElement> {
  const picker = await screen.findByRole("form", { name: "Elegir asignación y periodo" });
  // The assignment option shows the resolved "group · subject · teacher" label.
  await within(picker).findByRole("option", { name: "4° A · Matemáticas · Miguel Vázquez" });
  await user.selectOptions(within(picker).getByLabelText("Asignación"), "asg1");
  await within(picker).findByRole("option", { name: "Bloque 3" });
  await user.selectOptions(within(picker).getByLabelText("Periodo"), "t1");

  await waitFor(() => expect(grades.getRoster).toHaveBeenCalledWith(expect.any(String), { assignmentId: "asg1", termId: "t1" }));
  return screen.findByRole("table", { name: "Roster de calificaciones" });
}

describe("GradeCapture — roster render", () => {
  it("renders one row per enrolled student, resolving the name and the existing value", async () => {
    const user = userEvent.setup();
    render(<GradeCapture onBack={() => {}} />);

    const grid = await openRoster(user);
    expect(within(grid).getByText("Ada Lovelace")).toBeInTheDocument();
    // Pre-existing value is pre-filled into the editable input.
    expect(within(grid).getByLabelText("Calificación de Ada Lovelace")).toHaveValue(8);
  });
});

describe("GradeCapture — capture", () => {
  it("captures a grade with {assignmentId, enrollmentId, termId, value} and NO subject/teacher", async () => {
    const user = userEvent.setup();
    grades.captureGrade.mockResolvedValue({ id: "ge1", enrollmentId: "en1", subjectId: "sub1", termId: "t1", teacherId: "tch1", value: 9.5 });
    render(<GradeCapture onBack={() => {}} />);

    const grid = await openRoster(user);
    const input = within(grid).getByLabelText("Calificación de Ada Lovelace");
    await user.clear(input);
    await user.type(input, "9.5");
    await user.click(within(grid).getByRole("button", { name: "Guardar" }));

    await waitFor(() =>
      expect(grades.captureGrade).toHaveBeenCalledWith(expect.any(String), {
        assignmentId: "asg1",
        enrollmentId: "en1",
        termId: "t1",
        value: 9.5,
      }),
    );
    const payload = grades.captureGrade.mock.calls[0]?.[1];
    expect(payload).not.toHaveProperty("subjectId");
    expect(payload).not.toHaveProperty("teacherId");
  });

  it("blocks an out-of-range value (11) client-side without calling the api", async () => {
    const user = userEvent.setup();
    render(<GradeCapture onBack={() => {}} />);

    const grid = await openRoster(user);
    const input = within(grid).getByLabelText("Calificación de Ada Lovelace");
    await user.clear(input);
    await user.type(input, "11");
    await user.click(within(grid).getByRole("button", { name: "Guardar" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/entre 0 y 10/);
    expect(grades.captureGrade).not.toHaveBeenCalled();
  });
});
