import { describe, expect, it } from "vitest";

import type { GradeLevel, LabelledGroup, Level } from "./api";
import { sortBySchoolOrder } from "./schoolOrder";

const level = (id: string): Level => ({ id, name: id, sepCode: "", enrollmentGranularity: "cycle" });
const grade = (id: string, levelId: string, ordinal: number): GradeLevel => ({ id, levelId, name: id, ordinal });
const group = (id: string, gradeLevelId: string, name: string): LabelledGroup => ({ id, cycleId: "c1", gradeLevelId, name, capacity: 30, label: id });

describe("sortBySchoolOrder", () => {
  it("orders by level (catalogue order), then grade ordinal, then group letter (es-MX)", () => {
    const levels = [level("pre"), level("pri"), level("sec")];
    const grades = [grade("sec1", "sec", 1), grade("pri2", "pri", 2), grade("pri1", "pri", 1), grade("pre3", "pre", 3)];
    const groups = [
      group("sec1-A", "sec1", "A"),
      group("pri2-A", "pri2", "A"),
      group("pri1-B", "pri1", "B"),
      group("pri1-A", "pri1", "A"),
      group("pre3-A", "pre3", "A"),
      group("orphan", "unknown", "A"),
    ];

    expect(sortBySchoolOrder(groups, { levels, grades }).map((g) => g.id)).toEqual(["pre3-A", "pri1-A", "pri1-B", "pri2-A", "sec1-A", "orphan"]);
  });

  it("does not mutate its input", () => {
    const groups = [group("b", "x", "B"), group("a", "x", "A")];
    sortBySchoolOrder(groups, { levels: [], grades: [] });
    expect(groups.map((g) => g.id)).toEqual(["b", "a"]);
  });
});
