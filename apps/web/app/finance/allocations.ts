import type { Payment } from "./api";

/** What one charge received from a deposit, whatever methods paid it. */
export interface ChargeAllocation {
  chargeId: string;
  amountCents: number;
}

/**
 * Allocations summed per charge, in the order the charges first appear.
 *
 * A deposit paid with several methods splits its allocations per tender (T8),
 * so the same month can come back twice — once per method. A person reads
 * "Colegiatura septiembre $3,200", not two lines that add up to it.
 */
export function groupByCharge(allocations: readonly Payment[]): ChargeAllocation[] {
  const totals = new Map<string, number>();
  for (const a of allocations) {
    totals.set(a.chargeId, (totals.get(a.chargeId) ?? 0) + a.amountCents);
  }
  return [...totals].map(([chargeId, amountCents]) => ({ chargeId, amountCents }));
}
