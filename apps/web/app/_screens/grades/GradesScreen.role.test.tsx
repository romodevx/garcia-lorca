import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as configApi from "../../config/api";
import * as gradesApi from "../../grades/api";
import * as studentsApi from "../../students/api";
import * as teachingApi from "../../teaching/api";
import { GradesScreen } from "./GradesScreen";

// Both the director path (grades/config/students) and the teacher path (teaching)
// are mocked so we can assert WHICH client set is exercised for each role.
vi.mock("../../config/api", () => ({
  listTeachingAssignments: vi.fn(),
  listGroups: vi.fn(),
  listSubjects: vi.fn(),
  listTeachers: vi.fn(),
  listTerms: vi.fn(),
}));
vi.mock("../../grades/api", () => ({ getRoster: vi.fn(), captureGrade: vi.fn() }));
vi.mock("../../students/api", () => ({ listStudents: vi.fn() }));
vi.mock("../../teaching/api", () => ({
  listAssignments: vi.fn(),
  listTerms: vi.fn(),
  getRoster: vi.fn(),
  captureGrade: vi.fn(),
}));

const config = vi.mocked(configApi);
const grades = vi.mocked(gradesApi);
const students = vi.mocked(studentsApi);
const teaching = vi.mocked(teachingApi);

beforeEach(() => {
  config.listTeachingAssignments.mockResolvedValue([]);
  config.listGroups.mockResolvedValue([]);
  config.listSubjects.mockResolvedValue([]);
  config.listTeachers.mockResolvedValue([]);
  config.listTerms.mockResolvedValue([]);
  grades.getRoster.mockResolvedValue([]);
  students.listStudents.mockResolvedValue([]);
  teaching.listAssignments.mockResolvedValue([]);
  teaching.listTerms.mockResolvedValue([]);
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
    expect(grades.getRoster).not.toHaveBeenCalled();
    expect(config.listTeachingAssignments).not.toHaveBeenCalled();
  });

  it("renders the DIRECTOR surface for a grades:read principal (director wins over teaching:read)", async () => {
    render(<GradesScreen onBack={() => {}} permissions={["grades:read", "teaching:read"]} />);

    // The director subtitle distinguishes the full surface.
    expect(await screen.findByText(/un clic por alumno/)).toBeInTheDocument();
    await waitFor(() => expect(config.listTeachingAssignments).toHaveBeenCalled());

    // The director path never touches the teacher client.
    expect(teaching.listAssignments).not.toHaveBeenCalled();
  });
});
