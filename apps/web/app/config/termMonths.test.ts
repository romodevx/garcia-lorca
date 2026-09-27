import { describe, expect, it } from "vitest";

import {
  assignMonth,
  cycleMonths,
  savePlan,
  trimesterInputs,
  unassignedMonths,
  type MonthAssignment,
} from "./termMonths";

describe("cycleMonths", () => {
  it("lists every month the cycle touches, both ends included", () => {
    expect(cycleMonths("2026-08-24", "2027-07-09")).toEqual([
      "2026-08", "2026-09", "2026-10", "2026-11", "2026-12",
      "2027-01", "2027-02", "2027-03", "2027-04", "2027-05", "2027-06", "2027-07",
    ]);
  });

  it("is empty for an impossible range", () => {
    expect(cycleMonths("2027-01-01", "2026-12-31")).toEqual([]);
  });
});

describe("assignMonth", () => {
  const before: MonthAssignment = { t1: ["2026-09", "2026-10"], t2: ["2026-12"] };

  it("adds a free month to the tapped term, kept in calendar order", () => {
    expect(assignMonth(before, "t2", "2026-11")).toEqual({ t1: ["2026-09", "2026-10"], t2: ["2026-11", "2026-12"] });
  });

  it("moves a month out of the other term in one tap (a month lives in one term only)", () => {
    expect(assignMonth(before, "t2", "2026-10")).toEqual({ t1: ["2026-09"], t2: ["2026-10", "2026-12"] });
  });

  it("tapping a month already in that term takes it out", () => {
    expect(assignMonth(before, "t1", "2026-09")).toEqual({ t1: ["2026-10"], t2: ["2026-12"] });
  });

  it("never mutates the input", () => {
    assignMonth(before, "t2", "2026-10");
    expect(before).toEqual({ t1: ["2026-09", "2026-10"], t2: ["2026-12"] });
  });
});

describe("savePlan", () => {
  it("saves the term that LOST a month before the one that gained it (else the server says MONTH_IN_OTHER_TERM)", () => {
    const before: MonthAssignment = { t1: ["2026-09", "2026-10"], t2: [] };
    const after: MonthAssignment = { t1: ["2026-09"], t2: ["2026-10"] };
    expect(savePlan(before, after)).toEqual([
      { termId: "t1", months: ["2026-09"] },
      { termId: "t2", months: ["2026-10"] },
    ]);
  });

  it("skips the terms that did not change", () => {
    const before: MonthAssignment = { t1: ["2026-09"], t2: [], t3: ["2027-03"] };
    const after: MonthAssignment = { t1: ["2026-09"], t2: ["2026-10"], t3: ["2027-03"] };
    expect(savePlan(before, after)).toEqual([{ termId: "t2", months: ["2026-10"] }]);
  });
});

describe("unassignedMonths", () => {
  it("lists the cycle's months no term has yet", () => {
    expect(unassignedMonths(["2026-09", "2026-10", "2026-11"], { t1: ["2026-10"] })).toEqual(["2026-09", "2026-11"]);
  });
});

describe("trimesterInputs", () => {
  it("proposes three trimesters with their ordinals and dates covering the cycle in thirds", () => {
    const inputs = trimesterInputs({ id: "c1", startsOn: "2026-08-01", endsOn: "2027-07-31" }, "l1");
    expect(inputs.map((t) => [t.name, t.ordinal])).toEqual([
      ["1.er trimestre", 1],
      ["2.º trimestre", 2],
      ["3.er trimestre", 3],
    ]);
    expect(inputs.every((t) => t.cycleId === "c1" && t.levelId === "l1")).toBe(true);
    expect(inputs[0]?.startsOn).toBe("2026-08-01");
    expect(inputs[2]?.endsOn).toBe("2027-07-31");
    // Contiguous: each third starts the day after the previous one ends.
    expect(inputs[0]?.endsOn).toBe("2026-11-30");
    expect(inputs[1]?.startsOn).toBe("2026-12-01");
    expect(inputs[1]?.endsOn).toBe("2027-03-31");
    expect(inputs[2]?.startsOn).toBe("2027-04-01");
  });
});
