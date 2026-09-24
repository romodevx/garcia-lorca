import { describe, expect, it } from "vitest";

import { localISODate } from "./localDate";

describe("localISODate", () => {
  // The office's day, not UTC's: at 19:00 in Mexico City UTC is already
  // tomorrow, and a payment typed then belongs to today.
  it("is the local calendar day as aaaa-mm-dd", () => {
    expect(localISODate(new Date(2026, 8, 4, 19, 30))).toBe("2026-09-04");
    expect(localISODate(new Date(2026, 0, 1, 0, 5))).toBe("2026-01-01");
  });
});
