import { renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as config from "../../config/api";
import * as students from "../../students/api";
import { useStudentRoster } from "./useStudentRoster";

vi.mock("../../config/api", () => ({
  listLevels: vi.fn(),
  listGradeLevels: vi.fn(),
  listCycles: vi.fn(),
  listLabelledGroups: vi.fn(),
}));
vi.mock("../../students/api", () => ({
  listStudents: vi.fn(),
  listEnrollments: vi.fn(),
}));
const c = vi.mocked(config);
const s = vi.mocked(students);

const pupil = (id: string, lastName: string, admissionStatus: students.AdmissionStatus): students.Student => ({
  id, curp: "", firstName: "Ana", lastName, level: "Primaria", levelId: "pri", gradeGroup: "A", guardianName: "", admissionStatus,
});

beforeEach(() => {
  s.listStudents.mockResolvedValue([pupil("1", "Zapata", "enrolled"), pupil("2", "Arce", "accepted"), pupil("3", "Baja", "withdrawn"), pupil("4", "No", "rejected")]);
  s.listEnrollments.mockResolvedValue([{ id: "e1", studentId: "1", cycleId: "c1", groupId: "g1", termId: "", status: "active", enrolledAt: "" }]);
  c.listLevels.mockResolvedValue([{ id: "pri", name: "Primaria", sepCode: "", enrollmentGranularity: "cycle" }]);
  c.listGradeLevels.mockResolvedValue([{ id: "pri-1", levelId: "pri", name: "1°", ordinal: 1 }]);
  c.listCycles.mockResolvedValue([{ id: "c0", name: "Viejo", status: "closed", startsOn: "", endsOn: "" }, { id: "c1", name: "2026", status: "active", startsOn: "", endsOn: "" }] as config.Cycle[]);
  c.listLabelledGroups.mockResolvedValue([{ id: "g1", cycleId: "c1", gradeLevelId: "pri-1", name: "A", capacity: 30, label: "1° PRIMARIA · A" }]);
});
afterEach(() => vi.clearAllMocks());

describe("useStudentRoster", () => {
  it("places the active cycle's pupils, leaving out withdrawn and rejected ones", async () => {
    const { result } = renderHook(() => useStudentRoster());
    await waitFor(() => expect(result.current.status).toBe("ok"));
    expect(c.listLabelledGroups).toHaveBeenCalledWith(expect.any(String), "c1");
    expect(result.current.students.map((p) => p.id).sort()).toEqual(["1", "2"]);
    const zapata = result.current.students.find((p) => p.id === "1");
    expect(zapata).toMatchObject({ levelId: "pri", gradeLevelId: "pri-1", groupId: "g1", room: "1° PRIMARIA · A" });
    expect(result.current.catalog.groups).toHaveLength(1);
  });

  it("says it failed when the pupils cannot be read", async () => {
    s.listStudents.mockRejectedValue(new Error("x"));
    const { result } = renderHook(() => useStudentRoster());
    await waitFor(() => expect(result.current.status).toBe("failed"));
  });
});
