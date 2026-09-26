// Regression: ISSUE-001 (T3) — the teacher's attendance picker offered
// "MATEMÁTICAS · 2 grupos" beside "MATEMÁTICAS · 1 grupo" with nothing telling
// 1° from 2° Secundaria: per-class options never carried the group's grade.
// Found by /qa on 2026-09-26
// Report: .gstack/qa-reports/qa-report-localhost-3002-2026-09-26-t3.md
import { describe, expect, it } from "vitest";

import type { AttendanceClass } from "../../teaching/api";
import { groupLabel, resolveSelection } from "../teaching/selection";
import { toAttendanceOptions } from "./attendanceOptions";

const perClass = (groupId: string, grade: string, subjectId: string, assignmentId: string): AttendanceClass => ({
  groupId,
  groupLabel: `${grade} · A`,
  groupName: "A",
  gradeLevelName: grade,
  levelName: "SECUNDARIA",
  mode: "per_class",
  subjectId,
  subjectName: "MATEMÁTICAS",
  assignmentId,
});

describe("toAttendanceOptions — same subject name in two grades (ISSUE-001)", () => {
  const classes: AttendanceClass[] = [
    perClass("g1", "1° SECUNDARIA", "s-mat-1", "a1"),
    perClass("g2", "2° SECUNDARIA", "s-mat-2", "a2"),
    { groupId: "gp", groupLabel: "1° PRIMARIA · B", groupName: "B", gradeLevelName: "1° PRIMARIA", levelName: "PRIMARIA", mode: "daily" },
  ];

  it("the subject step tells the two MATEMÁTICAS apart by grade", () => {
    const { options, bySubject } = toAttendanceOptions(classes);
    const { subjects } = resolveSelection(options, {}, bySubject, true);
    const mats = subjects.filter((s) => s.name === "MATEMÁTICAS");
    expect(mats.map((s) => s.grade)).toEqual(["1° SECUNDARIA", "2° SECUNDARIA"]);
  });

  it("the group still reads '1° SECUNDARIA · A' once, never with the grade twice", () => {
    const { options } = toAttendanceOptions(classes);
    expect(options.filter((o) => o.subjectName === "MATEMÁTICAS").map(groupLabel)).toEqual(["1° SECUNDARIA · A", "2° SECUNDARIA · A"]);
  });

  it("'Lista del día' spans grades, so it shows no grade; its groups keep the full label", () => {
    const { options, bySubject } = toAttendanceOptions(classes);
    const { subjects } = resolveSelection(options, {}, bySubject, true);
    expect(subjects.find((s) => s.name === "Lista del día")?.grade).toBeUndefined();
    expect(groupLabel(options.find((o) => o.groupId === "gp")!)).toBe("1° PRIMARIA · B");
  });

  it("an older gateway without the split fields still shows the full label", () => {
    const { options } = toAttendanceOptions([{ ...perClass("g1", "1° SECUNDARIA", "s", "a"), groupName: undefined, gradeLevelName: undefined }]);
    expect(groupLabel(options[0]!)).toBe("1° SECUNDARIA · A");
  });
});
