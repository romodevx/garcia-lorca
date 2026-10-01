import { describe, expect, it } from "vitest";

import { monthLabel, monthRange, shiftMonth } from "./period";

describe("the month the module looks at", () => {
  it("spans the whole month, February of a leap year included", () => {
    expect(monthRange("2026-10-15")).toEqual({ from: "2026-10-01", to: "2026-10-31" });
    expect(monthRange("2028-02-03")).toEqual({ from: "2028-02-01", to: "2028-02-29" });
  });

  it("moves a month at a time across the year", () => {
    expect(shiftMonth("2026-12-31", 1)).toBe("2027-01-01");
    expect(shiftMonth("2026-03-31", -1)).toBe("2026-02-01");
  });

  it("names it in Spanish", () => {
    expect(monthLabel("2026-10-15")).toBe("Octubre de 2026");
  });
});
