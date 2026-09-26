import { describe, expect, it } from "vitest";

import type { AttendanceClass } from "../../teaching/api";
import { DAILY_SUBJECT, toAttendanceOptions } from "./attendanceOptions";

const daily = (groupId: string, groupLabel: string): AttendanceClass => ({ groupId, groupLabel, levelName: "Primaria", mode: "daily" });
const perClass = (groupId: string, subjectId: string, subjectName: string, assignmentId: string): AttendanceClass => ({
  groupId,
  groupLabel: `SEC · ${groupId}`,
  levelName: "Secundaria",
  mode: "per_class",
  subjectId,
  subjectName,
  assignmentId,
});

describe("toAttendanceOptions", () => {
  it("only daily groups: group-only picking, one option per group", () => {
    const { options, bySubject } = toAttendanceOptions([daily("g1", "1° PRIMARIA · A"), daily("g2", "2° PRIMARIA · B")]);
    expect(bySubject).toBe(false);
    expect(options.map((o) => [o.id, o.groupId, o.groupName])).toEqual([
      ["g1", "g1", "1° PRIMARIA · A"],
      ["g2", "g2", "2° PRIMARIA · B"],
    ]);
    expect(options[0]?.subjectId).toBeUndefined();
  });

  it("per-class items: subject first, each keyed by its assignment and carrying its subject", () => {
    const { options, bySubject } = toAttendanceOptions([perClass("gA", "s-mat", "Matemáticas", "a1")]);
    expect(bySubject).toBe(true);
    expect(options[0]).toMatchObject({ id: "a1", groupId: "gA", subjectId: "s-mat", subjectName: "Matemáticas" });
    expect(options[0]?.attendanceSubjectId).toBe("s-mat");
  });

  it("mixed: the daily groups become the 'Lista del día' subject, with no subject sent to the server", () => {
    const { options } = toAttendanceOptions([daily("g1", "1° PRIMARIA · A"), perClass("gA", "s-mat", "Matemáticas", "a1")]);
    const day = options.find((o) => o.groupId === "g1");
    expect(day).toMatchObject({ id: `${DAILY_SUBJECT}:g1`, subjectId: DAILY_SUBJECT, subjectName: "Lista del día" });
    expect(day?.attendanceSubjectId).toBeUndefined();
  });

  it("a group listed twice as daily is one option", () => {
    expect(toAttendanceOptions([daily("g1", "A"), daily("g1", "A")]).options).toHaveLength(1);
  });
});
