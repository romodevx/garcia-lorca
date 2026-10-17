import type { CashUpPayment, Deposit } from "../../finance/api";

/** One counted row of the corte: a deposit, or a payment taken outside one. */
export type CashUpEntry =
  | { kind: "deposit"; key: string; deposit: Deposit }
  | { kind: "payment"; key: string; payment: CashUpPayment };

interface Ranked {
  entry: CashUpEntry;
  at: number;
  /** Same instant: the printed folio when there is one, else the id. */
  tiebreak: string;
}

const LOCALE = "es-MX";
/** A printed folio, "U-2026-000001" or "P-2026-000004". */
const FOLIO = /[A-Z]-\d{4}-\d+/;

function instant(iso: string): number {
  const at = Date.parse(iso);
  return Number.isNaN(at) ? Number.MAX_SAFE_INTEGER : at;
}

/** A counter sale's U- folio: in its reference, or in what it paid for ("Uniformes · U-2026-000001"). */
function looseTiebreak(p: CashUpPayment): string {
  return FOLIO.exec(p.reference)?.[0] ?? FOLIO.exec(p.description)?.[0] ?? p.id;
}

/**
 * The day's counted rows in the order the money came in (ISSUE-T14-206): the
 * server sends deposits and loose payments (a counter sale's) as two lists,
 * each in time order, and showing one after the other put 09:53 above 08:35.
 * Same instant: by folio, so two receipts keyed together read in folio order.
 * A back-dated deposit carries the day it is dated and no hour (paidAt at
 * midnight), so it opens that day's list. O(n log n) over a day's rows.
 */
export function cashUpTimeline(deposits: readonly Deposit[], payments: readonly CashUpPayment[]): CashUpEntry[] {
  const ranked: Ranked[] = [
    ...deposits.map((d): Ranked => ({ entry: { kind: "deposit", key: `d-${d.id}`, deposit: d }, at: instant(d.paidAt), tiebreak: d.folio || d.id })),
    ...payments.map((p): Ranked => ({ entry: { kind: "payment", key: `p-${p.id}`, payment: p }, at: instant(p.paidAt), tiebreak: looseTiebreak(p) })),
  ];
  ranked.sort((a, b) => a.at - b.at || a.tiebreak.localeCompare(b.tiebreak, LOCALE, { numeric: true }));
  return ranked.map((r) => r.entry);
}
