import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as configApi from "../../config/api";
import * as teachersApi from "../../teachers/api";
import * as gradesApi from "../../grades/api";
import * as studentsApi from "../../students/api";
import * as teachingApi from "../../teaching/api";
import { GradesScreen } from "./GradesScreen";

// Both the director path (grades/config/students) and the teacher path (teaching)
// are mocked so we can assert WHICH client set is exercised for each role.
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
vi.mock("../../grades/api", () => ({ gradeSheetClient: vi.fn(() => ({ load: vi.fn(), save: vi.fn(), remove: vi.fn() })) }));
vi.mock("../../students/api", () => ({ listStudents: vi.fn() }));
vi.mock("../../teaching/api", () => ({
  listAssignments: vi.fn(),
  teachingGradeSheet: { load: vi.fn(), save: vi.fn(), remove: vi.fn() },
}));

const config = vi.mocked(configApi);
const teachers = vi.mocked(teachersApi);
const grades = vi.mocked(gradesApi);
const students = vi.mocked(studentsApi);
const teaching = vi.mocked(teachingApi);

beforeEach(() => {
  teachers.listTeachingAssignments.mockResolvedValue([]);
  config.listLabelledGroups.mockResolvedValue([]);
  config.listSubjects.mockResolvedValue([]);
  config.listLevels.mockResolvedValue([]);
  config.listGradeLevels.mockResolvedValue([]);
  teachers.listTeachers.mockResolvedValue([]);
  teaching.listAssignments.mockResolvedValue([]);
});

afterEach(() => vi.clearAllMocks());

describe("GradesScreen — role-adaptive branch", () => {
  it("renders the TEACHER surface for a teaching:read-only principal, with NO students-list/grades/config call", async () => {
    render(<GradesScreen onBack={() => {}} permissions={["teaching:read"]} />);

    // The teacher subtitle distinguishes the self-scoped surface.
    expect(await screen.findByText(/solo tus grupos/)).toBeInTheDocument();
    await waitFor(() => expect(teaching.listAssignments).toHaveBeenCalled());

    // A teacher must never hit the director-only clients (they'd 403).
    expect(students.listStudents).not.toHaveBeenCalled();
    expect(grades.gradeSheetClient).not.toHaveBeenCalled();
    expect(teachers.listTeachingAssignments).not.toHaveBeenCalled();
  });

  it("renders the DIRECTOR surface for a grades:read principal (director wins over teaching:read)", async () => {
    render(<GradesScreen onBack={() => {}} permissions={["grades:read", "teaching:read"]} />);

    // The director subtitle distinguishes the full surface.
    expect(await screen.findByText(/se guarda sola/)).toBeInTheDocument();
    await waitFor(() => expect(teachers.listTeachingAssignments).toHaveBeenCalled());

    // The director path never touches the teacher client.
    expect(teaching.listAssignments).not.toHaveBeenCalled();
    // The permissions reach the director surface: grades:read opens the Periodos tab.
    expect(screen.getByRole("tab", { name: "Periodos" })).toBeInTheDocument();
  });
});
