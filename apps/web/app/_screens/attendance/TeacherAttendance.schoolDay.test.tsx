import { render, screen } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";

import * as teachingApi from "../../teaching/api";
import { TeacherAttendance } from "./TeacherAttendance";

// ADR 0035: "today" is the school's day (Morelia), never the device clock's.
// The school-day helper is pinned to a day the device clock would not give.
vi.mock("../../_lib/schoolDay", () => ({ schoolISODate: () => "2026-10-14" }));
vi.mock("../../teaching/api", () => ({
  listAttendanceClasses: vi.fn(),
  getTeachingAttendanceSheet: vi.fn(),
  saveTeachingAttendance: vi.fn(),
}));

const teaching = vi.mocked(teachingApi);

beforeEach(() => {
  window.history.replaceState(null, "", "/asistencia");
  teaching.listAttendanceClasses.mockResolvedValue([{ groupId: "g1", groupLabel: "1° PRIMARIA · A", levelName: "Primaria", mode: "daily" }]);
  teaching.getTeachingAttendanceSheet.mockResolvedValue({
    mode: "daily",
    date: "2026-10-14",
    editable: true,
    windowDays: 7,
    session: null,
    rows: [{ enrollmentId: "en1", studentId: "st1", name: "Ada Lovelace", status: "present", note: "" }],
  });
});

it("opens the teacher's list on the school's day, not the device's", async () => {
  render(<TeacherAttendance onBack={() => {}} />);
  await screen.findByRole("button", { name: "Ada Lovelace: Presente" });
  expect(teaching.getTeachingAttendanceSheet).toHaveBeenCalledWith({ groupId: "g1", date: "2026-10-14" });
});
