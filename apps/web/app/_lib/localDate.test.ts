import { describe, expect, it } from "vitest";

import { dayWithYearLabel, localISODate, shortDayLabel } from "./localDate";

describe("localISODate", () => {
  // The school's day, not UTC's nor the device's: at 19:30 in Mexico City UTC
  // is already tomorrow, and a payment typed then belongs to today — also on a
  // laptop set to another zone (ADR 0035).
  it("is the school's calendar day as aaaa-mm-dd", () => {
    expect(localISODate(new Date(Date.UTC(2026, 8, 5, 1, 30)))).toBe("2026-09-04");
    expect(localISODate(new Date(Date.UTC(2026, 0, 1, 6, 5)))).toBe("2026-01-01");
  });
});

describe("day labels of an aaaa-mm-dd date", () => {
  it("read the calendar day it names, whatever the time zone", () => {
    expect(shortDayLabel("2026-08-24")).toBe("24 ago");
    expect(dayWithYearLabel("2027-07-09")).toBe("9 jul 2027");
  });
});
