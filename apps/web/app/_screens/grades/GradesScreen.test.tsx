import { render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as configApi from "../../config/api";
import * as teachersApi from "../../teachers/api";
import { GradesScreen } from "./GradesScreen";

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

const config = vi.mocked(configApi);
const teachers = vi.mocked(teachersApi);

beforeEach(() => {
  teachers.listTeachingAssignments.mockResolvedValue([{ id: "a1", cycleId: "c1", groupId: "g1", subjectId: "s1", teacherId: "t1" }]);
  config.listLabelledGroups.mockResolvedValue([{ id: "g1", cycleId: "c1", gradeLevelId: "gl1", name: "A", capacity: 30, label: "1° A" }]);
  config.listSubjects.mockResolvedValue([{ id: "s1", gradeLevelId: "gl1", name: "Matemáticas" }]);
  config.listLevels.mockResolvedValue([]);
  config.listGradeLevels.mockResolvedValue([]);
  teachers.listTeachers.mockResolvedValue([{ id: "t1", firstName: "Ada", lastName: "Lovelace", email: "a@gl.mx", userId: "" }]);
});
afterEach(() => vi.clearAllMocks());

describe("GradesScreen", () => {
  it("renders the header and the assignment picker, with no term selector", async () => {
    render(<GradesScreen onBack={() => {}} />);
    expect(await screen.findByRole("heading", { name: "Calificaciones" })).toBeInTheDocument();
    const picker = screen.getByLabelText("Asignación", { selector: "select" });
    // the assignment option is built from group · subject · teacher
    expect(await within(picker).findByRole("option", { name: "1° A · Matemáticas · Ada Lovelace" })).toBeInTheDocument();
    expect(screen.queryByLabelText("Periodo")).toBeNull();
  });
});
