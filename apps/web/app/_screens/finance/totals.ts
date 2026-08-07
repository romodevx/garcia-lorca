import type { Charge } from "../../finance/api";

/** Aggregates of the charges currently in view. All sums are integer CENTS. */
export interface ChargeTotals {
  remainingCents: number;
  paidCents: number;
  /** How many charges still owe something. */
  withBalance: number;
  count: number;
}

/**
 * Sum the visible charges. Cents are integers, so the additions are exact — no
 * float rounding can creep into a money total.
 */
export function totalsOf(charges: readonly Charge[]): ChargeTotals {
  let remainingCents = 0;
  let paidCents = 0;
  let withBalance = 0;
  for (const charge of charges) {
    remainingCents += charge.remainingCents;
    paidCents += charge.paidCents;
    if (charge.remainingCents > 0) withBalance += 1;
  }
  return { remainingCents, paidCents, withBalance, count: charges.length };
}

/** Charges a cashier can still collect against. */
export function collectable(charges: readonly Charge[]): Charge[] {
  return charges.filter((charge) => charge.remainingCents > 0 && charge.status !== "cancelled");
}
