import { describe, expect, it } from "vitest";

import { dayLabel, monthsList, unfinishedMonths } from "./format";

describe("dayLabel", () => {
  it("reads a date on the school's calendar", () => {
    expect(dayLabel("2026-12-01T18:00:00Z")).toMatch(/1 dic.* 2026/);
  });

  it.each([null, undefined, ""])("says — when there is no date (%s)", (iso) => {
    expect(dayLabel(iso)).toBe("—");
  });
});

describe("unfinishedMonths", () => {
  const months = ["2027-04", "2027-05", "2027-06"];

  it("prefers the server's list", () => {
    expect(unfinishedMonths(months, ["2027-06"], "2027-04")).toEqual(["2027-06"]);
  });

  it("without it, the term months after the current month", () => {
    expect(unfinishedMonths(months, undefined, "2027-04")).toEqual(["2027-05", "2027-06"]);
    expect(unfinishedMonths(months, [], "2027-04")).toEqual(["2027-05", "2027-06"]);
  });

  it("nothing once the last month has come", () => {
    expect(unfinishedMonths(months, undefined, "2027-06")).toEqual([]);
  });
});

describe("monthsList", () => {
  it("is short names separated by commas", () => {
    expect(monthsList(["2027-04", "2027-05", "2027-06"])).toBe("Abr, May, Jun");
  });
});
