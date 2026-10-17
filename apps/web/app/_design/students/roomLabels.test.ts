import { renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as configApi from "../../config/api";
import * as studentsApi from "../../students/api";
import { roomLabelsOf, useRoomLabels } from "./roomLabels";

vi.mock("../../config/api", () => ({ listCycles: vi.fn(), listLabelledGroups: vi.fn() }));
vi.mock("../../students/api", () => ({ listEnrollments: vi.fn() }));

const config = vi.mocked(configApi);
const students = vi.mocked(studentsApi);

const enrollment = (studentId: string, groupId: string, status: studentsApi.EnrollmentStatus = "active"): studentsApi.Enrollment => ({
  id: `e-${studentId}-${groupId}`, studentId, cycleId: "c1", groupId, termId: "", status, enrolledAt: "2026-08-20",
});
const group = (id: string, label: string): configApi.LabelledGroup => ({ id, cycleId: "c1", gradeLevelId: "g3", name: label.slice(-1), capacity: 30, label });

afterEach(() => vi.clearAllMocks());

// ISSUE-T14-018: "PRIMARIA · A" at the counter and in Finanzas, while the
// receipt said "3° PRIMARIA · A". The grade is what tells two A's apart.
describe("roomLabelsOf", () => {
  it("names each pupil's classroom with its grade, from the active enrollment", () => {
    const labels = roomLabelsOf(
      [enrollment("s1", "gA"), enrollment("s2", "gB", "withdrawn"), enrollment("s2", "gA")],
      [group("gA", "3° PRIMARIA · A"), group("gB", "3° PRIMARIA · B")],
    );

    expect(labels.get("s1")).toBe("3° PRIMARIA · A");
    expect(labels.get("s2")).toBe("3° PRIMARIA · A");
  });

  it("knows nothing of a pupil with no active enrollment or an unknown group", () => {
    const labels = roomLabelsOf([enrollment("s1", "gZ"), enrollment("s2", "gA", "withdrawn")], [group("gA", "3° PRIMARIA · A")]);

    expect(labels.has("s1")).toBe(false);
    expect(labels.has("s2")).toBe(false);
  });
});

describe("useRoomLabels", () => {
  it("reads the active cycle's groups and enrollments", async () => {
    config.listCycles.mockResolvedValue([
      { id: "old", name: "2025-2026", status: "closed", startsOn: "2025-08-01", endsOn: "2026-06-30" },
      { id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" },
    ]);
    config.listLabelledGroups.mockResolvedValue([group("gA", "3° PRIMARIA · A")]);
    students.listEnrollments.mockResolvedValue([enrollment("s1", "gA")]);

    const { result } = renderHook(() => useRoomLabels());

    await waitFor(() => expect(result.current.get("s1")).toBe("3° PRIMARIA · A"));
    expect(config.listLabelledGroups).toHaveBeenCalledWith(expect.any(String), "c1");
    expect(students.listEnrollments).toHaveBeenCalledWith(expect.any(String), { cycleId: "c1" });
  });

  it("stays empty when a read fails: the screen falls back to the stored group", async () => {
    config.listCycles.mockResolvedValue([{ id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" }]);
    config.listLabelledGroups.mockRejectedValue(new Error("down"));
    students.listEnrollments.mockResolvedValue([enrollment("s1", "gA")]);

    const { result } = renderHook(() => useRoomLabels());

    await waitFor(() => expect(config.listLabelledGroups).toHaveBeenCalled());
    expect(result.current.size).toBe(0);
  });
});
