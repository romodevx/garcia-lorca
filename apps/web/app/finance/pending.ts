import type { ChargeStatus } from "./api";

/** The two fields that decide whether a charge is still owed. */
export interface OwedReading {
  status: ChargeStatus;
  remainingCents: number;
}

/** A live charge with something left: a cancelled one is never owed. */
export function isOwed(charge: OwedReading): boolean {
  return charge.status !== "cancelled" && charge.remainingCents > 0;
}

/** What one charge still asks for, as a balance column shows it: a cancelled one asks for nothing. */
export function owedCents(charge: OwedReading): number {
  return isOwed(charge) ? charge.remainingCents : 0;
}

/**
 * What the charges still ask for — "Por cobrar". One rule for every screen that
 * shows it (Finanzas, the home's KPI): cancelled charges and overpaid ones
 * (negative remaining) count for nothing.
 */
export function pendingCents(charges: readonly OwedReading[]): number {
  return charges.reduce((sum, c) => sum + owedCents(c), 0);
}

/** The two fields that decide what a charge brought in. */
export interface CollectedReading {
  paidCents: number;
  /** What credit notes settled (CC-207); absent from a gateway older than it. */
  creditNoteCents?: number;
}

/**
 * What the charges brought in — "Cobrado". A credit note settles the debt but
 * no money came in (T8), so it is left out, as Ingresos and the cash-up leave
 * it out (CC-207). One rule for Finanzas and the home's KPI.
 */
export function collectedCents(charges: readonly CollectedReading[]): number {
  return charges.reduce((sum, c) => sum + c.paidCents - (c.creditNoteCents ?? 0), 0);
}
