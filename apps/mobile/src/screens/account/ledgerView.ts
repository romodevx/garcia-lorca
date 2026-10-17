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
  const today = calendarDay(asOf);
  const happened = movements.filter((m) => calendarDay(m.date) <= today).reverse();
  const upcoming = movements.filter((m) => calendarDay(m.date) > today);
  return { happened, upcoming };
}

const ISO_DAY_LENGTH = "YYYY-MM-DD".length;

/**
 * The "YYYY-MM-DD" part of an ISO value. The wire sends bare days today; a
 * datetime would otherwise compare after its own day ("2026-10-17T…" >
 * "2026-10-17"). The day as written is the school's day, so no zone math.
 */
function calendarDay(iso: string): string {
  return iso.slice(0, ISO_DAY_LENGTH);
}

/** One method of a deposit and the part it brought (T8). */
export interface Tender {
  method: string;
  amountCents: number;
}

/**
 * One row of the account as the family reads it: a ledger line, or a whole
 * deposit that finance lists one line per tender (ISSUE-CC-315).
 */
export interface LedgerRow {
  /** The row's line; for a deposit, its tenders summed into `amountCents`. */
  movement: FamilyMovement;
  /** The deposit's tenders when it came in more than one, in ledger order; else empty. */
  tenders: Tender[];
}

/**
 * Folds each deposit's lines into one row, placed where its first line was.
 * Finance writes a line per tender and every one carries the WHOLE deposit's
 * `appliedTo`, so two rows read as two payments of everything. Lines without
 * a deposit pass through untouched. O(n), order kept.
 */
export function groupDeposits(movements: readonly FamilyMovement[]): LedgerRow[] {
  const groups: FamilyMovement[][] = [];
  const byDeposit = new Map<string, FamilyMovement[]>();
  for (const m of movements) {
    const group = m.depositId ? byDeposit.get(m.depositId) : undefined;
    if (group !== undefined) {
      group.push(m);
      continue;
    }
    const fresh = [m];
    groups.push(fresh);
    if (m.depositId) byDeposit.set(m.depositId, fresh);
  }
  return groups.map(toRow);
}

function toRow(lines: FamilyMovement[]): LedgerRow {
  const [first] = lines as [FamilyMovement, ...FamilyMovement[]];
  if (lines.length === 1) return { movement: first, tenders: [] };
  // A credit note completed with cash is still a payment: the row looks like one.
  const lead = lines.find((line) => line.kind === "payment") ?? first;
  const amountCents = lines.reduce((sum, line) => sum + line.amountCents, 0);
  return {
    movement: { ...lead, amountCents },
    tenders: lines.map((line) => ({ method: line.method ?? "", amountCents: line.amountCents })),
  };
}
