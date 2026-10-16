import { describe, expect, it } from "vitest";

import { schoolISODate, schoolMonthStart, SCHOOL_TIME_ZONE } from "./schoolDay";

// ADR 0035: the school's day is Morelia's (America/Mexico_City, UTC-6 all year
// since 2022), whatever zone the device is set to. The server decides "today"
// the same way, so the counter's default date and its "no future date" check
// agree with it.
describe("schoolISODate", () => {
  const cases: ReadonlyArray<[string, number, string]> = [
    ["19:00 on the 15th in Morelia (01:00 UTC on the 16th)", Date.UTC(2026, 9, 16, 1, 0), "2026-10-15"],
    ["23:59 on the 15th in Morelia", Date.UTC(2026, 9, 16, 5, 59), "2026-10-15"],
    ["00:00 on the 16th in Morelia", Date.UTC(2026, 9, 16, 6, 0), "2026-10-16"],
    ["the last evening of the year", Date.UTC(2027, 0, 1, 3, 0), "2026-12-31"],
  ];
  it.each(cases)("%s", (_name, instant, want) => {
    expect(schoolISODate(new Date(instant))).toBe(want);
  });

  it("names the school's zone once", () => {
    expect(SCHOOL_TIME_ZONE).toBe("America/Mexico_City");
  });
});

describe("schoolMonthStart", () => {
  it("is the first of the school's month, not the UTC one", () => {
    expect(schoolMonthStart(new Date(Date.UTC(2026, 10, 1, 2, 0)))).toBe("2026-10-01");
    expect(schoolMonthStart(new Date(Date.UTC(2026, 10, 1, 7, 0)))).toBe("2026-11-01");
  });
});
