import { describe, expect, it } from "vitest";

import type { Cycle } from "../../config/api";
import { proposeCycle } from "./cycleProposal";

const cycle = (startsOn: string, endsOn: string): Cycle => ({ id: startsOn, name: "X", status: "draft", startsOn, endsOn });

// ISSUE-T14-023: the new-cycle drawer opened with no dates at all.
describe("proposeCycle", () => {
  it("with no cycle yet, proposes the school year today falls in (August to July)", () => {
    expect(proposeCycle([], "2026-10-17")).toEqual({ name: "2026-2027", startsOn: "2026-08-01", endsOn: "2027-07-31" });
  });

  it("from January to July, that school year began the August before", () => {
    expect(proposeCycle([], "2027-03-02")).toEqual({ name: "2026-2027", startsOn: "2026-08-01", endsOn: "2027-07-31" });
    expect(proposeCycle([], "2027-07-31")).toEqual({ name: "2026-2027", startsOn: "2026-08-01", endsOn: "2027-07-31" });
  });

  it("August 1st already belongs to the new school year", () => {
    expect(proposeCycle([], "2027-08-01")).toEqual({ name: "2027-2028", startsOn: "2027-08-01", endsOn: "2028-07-31" });
  });

  it("with cycles, proposes the year after the latest one", () => {
    const cycles = [cycle("2026-08-24", "2027-07-09"), cycle("2025-08-25", "2026-07-10")];
    expect(proposeCycle(cycles, "2026-10-17")).toEqual({ name: "2027-2028", startsOn: "2027-08-01", endsOn: "2028-07-31" });
  });
});
