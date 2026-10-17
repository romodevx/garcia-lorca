// QA 2026-10-15, ISSUE-018: Docentes listed the groups in whatever order the
// server returned them, while Asistencia sorted its own copy. The order now
// lives where every screen gets its groups: labelGroups.
import { describe, expect, it } from "vitest";

import { labelGroups, type GradeLevel, type Group, type Level } from "./api";

const level = (id: string, name: string, sortOrder?: number): Level => ({ id, name, sepCode: "", enrollmentGranularity: "cycle", ...(sortOrder !== undefined ? { sortOrder } : {}) });
const grade = (id: string, levelId: string, ordinal: number): GradeLevel => ({ id, levelId, name: `${ordinal}°`, ordinal });
const group = (id: string, gradeLevelId: string, name: string): Group => ({ id, cycleId: "c1", gradeLevelId, name, capacity: 30 });

describe("labelGroups — school order (ISSUE-018)", () => {
  const grades = [grade("sec1", "sec", 1), grade("pri2", "pri", 2), grade("pri1", "pri", 1), grade("pre3", "pre", 3)];
  const groups = [
    group("sec1-A", "sec1", "A"),
    group("pri2-A", "pri2", "A"),
    group("pri1-B", "pri1", "B"),
    group("pri1-A", "pri1", "A"),
    group("pre3-A", "pre3", "A"),
  ];

  it("orders by the level's place, then grade, then group name", () => {
    // Levels handed over out of order: the school's own sort_order decides.
    const levels = [level("sec", "SECUNDARIA", 3), level("pre", "PREESCOLAR", 1), level("pri", "PRIMARIA", 2)];

    expect(labelGroups(groups, grades, levels).map((g) => g.id)).toEqual(["pre3-A", "pri1-A", "pri1-B", "pri2-A", "sec1-A"]);
  });

  it("keeps each level together even when the levels cannot be read", () => {
    const ids = labelGroups(groups, grades, []).map((g) => g.id);

    expect(ids.indexOf("pri1-A")).toBeLessThan(ids.indexOf("pri1-B"));
    expect(ids.indexOf("pri1-B")).toBeLessThan(ids.indexOf("pri2-A"));
    const levelOf = (id: string): string => id.slice(0, 3);
    const runs = ids.map(levelOf).filter((l, i, all) => i === 0 || all[i - 1] !== l);
    expect(runs).toHaveLength(3);
  });

  // sortOrder is 1-based ("first = 1"): a level without one ranks by its
  // 1-based list position, so it never jumps ahead of the level the school
  // numbered first just by being listed first.
  it("ranks a level without sortOrder on the same 1-based scale", () => {
    const levels = [level("z", "SIN ORDEN"), level("a", "PRIMERO", 1)];
    const twoGrades = [grade("z1", "z", 1), grade("a1", "a", 1)];
    const ids = labelGroups([group("zg", "z1", "A"), group("ag", "a1", "A")], twoGrades, levels).map((g) => g.id);

    expect(ids).toEqual(["ag", "zg"]);
  });
});
