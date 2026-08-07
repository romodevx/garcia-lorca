import { describe, expect, it } from "vitest";

import type { Charge, ChargeStatus } from "../../finance/api";
import { collectable, totalsOf } from "./totals";

/**
 * The KPI cards and the ventanilla's charge list are both derived here, so a
 * wrong aggregate misreports what the school is owed. Cents are integers, so
 * every sum must be exact — a float total would be a money bug.
 */

function charge(fields: Partial<Charge> & { id: string }): Charge {
  return {
    studentId: "st1",
    cycleId: "cy1",
    conceptId: "c1",
    description: "Colegiatura",
    amountCents: 0,
    discountCents: 0,
    dueDate: "2026-09-05",
    status: "pending" satisfies ChargeStatus,
    paidCents: 0,
    remainingCents: 0,
    ...fields,
  };
}

describe("totalsOf", () => {
  it("returns zeroes for an empty view (the all-clear state)", () => {
    expect(totalsOf([])).toEqual({ remainingCents: 0, paidCents: 0, withBalance: 0, count: 0 });
  });

  it("counts a fully settled charge in `count` but not in `withBalance`", () => {
    const settled = charge({ id: "ch1", amountCents: 185000, paidCents: 185000, remainingCents: 0, status: "paid" });
    expect(totalsOf([settled])).toEqual({ remainingCents: 0, paidCents: 185000, withBalance: 0, count: 1 });
  });

  it("sums cents exactly across a mixed view", () => {
    const rows = [
      charge({ id: "ch1", amountCents: 185000, paidCents: 0, remainingCents: 185000 }),
      charge({ id: "ch2", amountCents: 185000, paidCents: 50025, remainingCents: 134975, status: "partial" }),
      charge({ id: "ch3", amountCents: 90010, paidCents: 90010, remainingCents: 0, status: "paid" }),
      charge({ id: "ch4", amountCents: 40000, paidCents: 0, remainingCents: 0, status: "cancelled" }),
    ];

    const totals = totalsOf(rows);

    expect(totals).toEqual({ remainingCents: 320_000 - 25, paidCents: 140_035, withBalance: 2, count: 4 });
    expect(Number.isInteger(totals.remainingCents)).toBe(true);
    expect(Number.isInteger(totals.paidCents)).toBe(true);
  });

  it("ignores a cancelled charge's remaining balance being zero (no phantom debt)", () => {
    const cancelled = charge({ id: "ch1", amountCents: 185000, remainingCents: 0, status: "cancelled" });
    expect(totalsOf([cancelled]).withBalance).toBe(0);
  });
});

describe("collectable", () => {
  const rows: readonly Charge[] = [
    charge({ id: "pending", remainingCents: 185000 }),
    charge({ id: "partial", paidCents: 50000, remainingCents: 135000, status: "partial" }),
    charge({ id: "paid", paidCents: 185000, remainingCents: 0, status: "paid" }),
    charge({ id: "cancelled", remainingCents: 185000, status: "cancelled" }),
  ];

  it("keeps only charges with a balance that are not cancelled", () => {
    expect(collectable(rows).map((row) => row.id)).toEqual(["pending", "partial"]);
  });

  it("returns an empty list for an empty view", () => {
    expect(collectable([])).toEqual([]);
  });

  it("never returns a cancelled charge even when it still shows a balance", () => {
    const stale = charge({ id: "ch1", remainingCents: 185000, status: "cancelled" });
    expect(collectable([stale])).toEqual([]);
  });

  it("does not mutate the input list", () => {
    const input = [...rows];
    collectable(input);
    expect(input).toHaveLength(rows.length);
  });
});
