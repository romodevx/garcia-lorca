import type { FamilyMovement } from "../../api";

/** The ledger as a parent reads it: what has happened, and what is still to come. */
export interface LedgerView {
  /** Lines dated today or earlier, newest first — the order of a bank app. */
  happened: FamilyMovement[];
  /** Lines dated after today (the cycle's later months), soonest first. */
  upcoming: FamilyMovement[];
}

/**
 * Splits finance's date-ordered ledger at the school's day (`asOf`,
 * "YYYY-MM-DD"). Every month of the cycle is on the ledger, so a plain
 * newest-first list opened on June of next year and pushed today's payments to
 * the bottom (ISSUE-T14-106). ISO days compare as strings. Without a day there
 * is nothing to split at: everything reads newest first, as before.
 */
export function splitLedger(movements: readonly FamilyMovement[], asOf: string): LedgerView {
  if (asOf === "") {
    return { happened: [...movements].reverse(), upcoming: [] };
  }
  const happened = movements.filter((m) => m.date <= asOf).reverse();
  const upcoming = movements.filter((m) => m.date > asOf);
  return { happened, upcoming };
}
