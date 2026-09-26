// Regression: ISSUE-001 (T3) — two same-named subjects in different grades
// read alike in the teacher's picker; once the grade travels apart from the
// letter, the list title must still read the whole group, not just "A".
// Found by /qa on 2026-09-26
// Report: .gstack/qa-reports/qa-report-localhost-3002-2026-09-26-t3.md
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { AttendanceSheet } from "../../attendance/sheet";
import * as teachingApi from "../../teaching/api";
import type { AttendanceClass } from "../../teaching/api";
import { TeacherAttendance } from "./TeacherAttendance";

vi.mock("../../teaching/api", () => ({
  listAttendanceClasses: vi.fn(),
  getTeachingAttendanceSheet: vi.fn(),
  saveTeachingAttendance: vi.fn(),
}));

const teaching = vi.mocked(teachingApi);
const TODAY = "2026-09-01";

const mat = (groupId: string, grade: string, subjectId: string): AttendanceClass => ({
  groupId,
  groupLabel: `${grade} · A`,
  groupName: "A",
  gradeLevelName: grade,
  levelName: "SECUNDARIA",
  mode: "per_class",
  subjectId,
  subjectName: "MATEMÁTICAS",
  assignmentId: `asg-${groupId}`,
});

const sheet: AttendanceSheet = {
  mode: "per_class",
  date: TODAY,
  editable: true,
  windowDays: 7,
  session: null,
  rows: [{ enrollmentId: "en1", studentId: "st1", name: "Ada Lovelace", status: "present", note: "" }],
};

beforeEach(() => {
  window.history.replaceState(null, "", "/asistencia");
  teaching.listAttendanceClasses.mockResolvedValue([mat("g1", "1° SECUNDARIA", "s1"), mat("g2", "2° SECUNDARIA", "s2")]);
  teaching.getTeachingAttendanceSheet.mockResolvedValue(sheet);
  teaching.saveTeachingAttendance.mockResolvedValue(undefined);
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("TeacherAttendance — same subject name in two grades (ISSUE-001)", () => {
  it("each MATEMÁTICAS choice names its grade, and the list title reads the whole group", async () => {
    const user = userEvent.setup();
    render(<TeacherAttendance onBack={() => {}} today={TODAY} />);

    const first = await screen.findByRole("button", { name: /MATEMÁTICAS.*1° SECUNDARIA/ });
    expect(screen.getByRole("button", { name: /MATEMÁTICAS.*2° SECUNDARIA/ })).toBeInTheDocument();

    await user.click(first);
    await screen.findByRole("button", { name: "Ada Lovelace: Presente" });
    expect(screen.getByText("Pase de lista · 1° SECUNDARIA · A · MATEMÁTICAS")).toBeInTheDocument();
  });
});
