import { describe, expect, it } from "vitest";

import { dayWithYearLabel, localISODate, shortDayLabel } from "./localDate";

describe("localISODate", () => {
  // The office's day, not UTC's: at 19:00 in Mexico City UTC is already
  // tomorrow, and a payment typed then belongs to today.
  it("is the local calendar day as aaaa-mm-dd", () => {
    expect(localISODate(new Date(2026, 8, 4, 19, 30))).toBe("2026-09-04");
    expect(localISODate(new Date(2026, 0, 1, 0, 5))).toBe("2026-01-01");
  });
});

describe("day labels of an aaaa-mm-dd date", () => {
  it("read the calendar day it names, whatever the time zone", () => {
    expect(shortDayLabel("2026-08-24")).toBe("24 ago");
    expect(dayWithYearLabel("2027-07-09")).toBe("9 jul 2027");
  });
});
