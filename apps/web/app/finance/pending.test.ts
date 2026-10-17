import { describe, expect, it } from "vitest";

import { isOwed, owedCents, pendingCents } from "./pending";

// ISSUE-T14-020: Finanzas showed a cancelled charge's amount as its balance.
describe("owedCents", () => {
  it("is what a charge still asks for: nothing when it is cancelled or overpaid", () => {
    expect(owedCents({ status: "partial", remainingCents: 1000 })).toBe(1000);
    expect(owedCents({ status: "cancelled", remainingCents: 350000 })).toBe(0);
    expect(owedCents({ status: "paid", remainingCents: -20 })).toBe(0);
  });
});

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
