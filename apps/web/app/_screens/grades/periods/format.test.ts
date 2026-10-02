import { describe, expect, it } from "vitest";

import { dateRangeLabel, dayLabel } from "./format";

describe("dayLabel", () => {
  it("reads a date on the school's calendar", () => {
    expect(dayLabel("2026-12-01T18:00:00Z")).toMatch(/1 dic.* 2026/);
  });

  it.each([null, undefined, ""])("says — when there is no date (%s)", (iso) => {
    expect(dayLabel(iso)).toBe("—");
  });
});

describe("dateRangeLabel", () => {
  it("names both days of the period, as calendar days", () => {
    expect(dateRangeLabel("2026-08-24", "2026-11-30")).toBe("24 ago 2026 – 30 nov 2026");
  });

  it("is undefined when the period has no dates", () => {
    expect(dateRangeLabel(null, null)).toBeUndefined();
    expect(dateRangeLabel("2026-08-24", null)).toBeUndefined();
  });
});
