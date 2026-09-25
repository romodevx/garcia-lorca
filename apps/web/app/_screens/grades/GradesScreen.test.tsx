import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as configApi from "../../config/api";
import * as teachersApi from "../../teachers/api";
import * as studentsApi from "../../students/api";
import { GradesScreen } from "./GradesScreen";

// The screens read labelled groups (ISSUE-002); label them from the listGroups mock.
beforeEach(() => {
  vi.mocked(configApi.listLabelledGroups).mockImplementation(async (...args) =>
    (await vi.mocked(configApi.listGroups)(...args)).map((g) => ({ ...g, label: g.name })));
});


vi.mock("../../config/api", () => ({
  listGroups: vi.fn(), listLabelledGroups: vi.fn(),
  listSubjects: vi.fn(),
  listTerms: vi.fn(),
}));
vi.mock("../../teachers/api", () => ({
  listTeachers: vi.fn(),
  listTeachingAssignments: vi.fn(),
}));
vi.mock("../../grades/api", () => ({ getRoster: vi.fn(), captureGrade: vi.fn() }));
vi.mock("../../students/api", () => ({ listStudents: vi.fn() }));

const config = vi.mocked(configApi);
const teachers = vi.mocked(teachersApi);
const students = vi.mocked(studentsApi);

beforeEach(() => {
  teachers.listTeachingAssignments.mockResolvedValue([{ id: "a1", cycleId: "c1", groupId: "g1", subjectId: "s1", teacherId: "t1" }]);
  config.listGroups.mockResolvedValue([{ id: "g1", cycleId: "c1", gradeLevelId: "gl1", name: "1° A", capacity: 30 }]);
  config.listSubjects.mockResolvedValue([{ id: "s1", gradeLevelId: "gl1", name: "Matemáticas" }]);
  teachers.listTeachers.mockResolvedValue([{ id: "t1", firstName: "Ada", lastName: "Lovelace", email: "a@gl.mx", userId: "" }]);
  config.listTerms.mockResolvedValue([{ id: "term1", cycleId: "c1", levelId: "gl1", name: "Bloque 1", ordinal: 1, startsOn: "2026-08-31", endsOn: "2026-10-31" }]);
  students.listStudents.mockResolvedValue([]);
});
afterEach(() => vi.clearAllMocks());

describe("GradesScreen", () => {
  it("renders the header, selectors and stat cards, listing the assignment", async () => {
    render(<GradesScreen onBack={() => {}} />);
    expect(await screen.findByRole("heading", { name: "Calificaciones" })).toBeInTheDocument();
    expect(screen.getByLabelText("Asignación")).toBeInTheDocument();
    expect(screen.getByLabelText("Periodo")).toBeInTheDocument();
    expect(screen.getByText("Capturadas")).toBeInTheDocument();
    // the assignment option is built from group · subject · teacher
    expect(await screen.findByRole("option", { name: "1° A · Matemáticas · Ada Lovelace" })).toBeInTheDocument();
  });
});
