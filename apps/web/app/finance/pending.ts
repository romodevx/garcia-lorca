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
