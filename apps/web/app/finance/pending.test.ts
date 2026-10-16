import { describe, expect, it } from "vitest";

import { isOwed, pendingCents } from "./pending";

const charge = (status: "pending" | "partial" | "paid" | "cancelled", remainingCents: number) => ({ status, remainingCents });

describe("pendingCents", () => {
  it("sums what is still owed, never a cancelled charge nor an overpaid one", () => {
    expect(pendingCents([charge("pending", 1000), charge("partial", 500), charge("cancelled", 4000), charge("paid", -20)])).toBe(1500);
  });
});

describe("isOwed", () => {
  it("is a live charge with something left", () => {
    expect(isOwed(charge("pending", 1))).toBe(true);
    expect(isOwed(charge("cancelled", 1000))).toBe(false);
    expect(isOwed(charge("paid", 0))).toBe(false);
  });
});
