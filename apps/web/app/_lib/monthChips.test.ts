import { describe, expect, it } from "vitest";

import { monthChips } from "./monthChips";

describe("monthChips", () => {
  it("names months short, in Spanish", () => {
    expect(monthChips(["2026-09", "2026-10"]).map((c) => c.label)).toEqual(["Sep", "Oct"]);
  });

  it("adds the year only when a short name repeats", () => {
    expect(monthChips(["2025-09", "2026-06", "2026-09"]).map((c) => c.label)).toEqual(["Sep 2025", "Jun", "Sep 2026"]);
  });

  it("gives every chip its full name", () => {
    expect(monthChips(["2026-01"])[0]).toEqual({ month: "2026-01", label: "Ene", fullName: "enero de 2026" });
  });
});
