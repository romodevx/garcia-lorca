// The Avisos view files each group under its level and shows "1° A" there
// (PLAN-redesign-avisos D3/E7). It needs the grade a group belongs to, which
// labelGroups already has in hand: the fields are additive, and a group whose
// grade is unknown simply goes without them.
import { describe, expect, it } from "vitest";

import { labelGroups } from "./api";

const levels = [{ id: "lp", name: "PRIMARIA" }];
const grades = [{ id: "p2", levelId: "lp", name: "2°", ordinal: 2 }];

describe("labelGroups grade fields", () => {
  it("carries the grade's name, level and ordinal next to the label", () => {
    const [group] = labelGroups([{ id: "a", cycleId: "c1", gradeLevelId: "p2", name: "A", capacity: 30 }], grades, levels);
    expect(group).toEqual({
      id: "a", cycleId: "c1", gradeLevelId: "p2", name: "A", capacity: 30,
      label: "2° PRIMARIA · A", gradeName: "2°", levelId: "lp", ordinal: 2,
    });
  });

  it("omits them when the grade is unknown", () => {
    const [group] = labelGroups([{ id: "z", cycleId: "c1", gradeLevelId: "gone", name: "Z", capacity: 30 }], grades, levels);
    expect(group?.label).toBe("Z");
    expect(group).not.toHaveProperty("gradeName");
    expect(group).not.toHaveProperty("levelId");
    expect(group).not.toHaveProperty("ordinal");
  });
});
