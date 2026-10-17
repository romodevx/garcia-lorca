import type { CashUpPayment, Deposit } from "../../finance/api";

/** One counted row of the corte: a deposit, or a payment taken outside one. */
export type CashUpEntry =
  | { kind: "deposit"; key: string; deposit: Deposit }
  | { kind: "payment"; key: string; payment: CashUpPayment };

interface Ranked {
  entry: CashUpEntry;
  at: number;
  folio: string;
}

const LOCALE = "es-MX";

function instant(iso: string): number {
  const at = Date.parse(iso);
  return Number.isNaN(at) ? Number.MAX_SAFE_INTEGER : at;
}

/**
 * The day's counted rows in the order the money came in (ISSUE-T14-206): the
 * server sends deposits and loose payments (a counter sale's) as two lists,
 * each in time order, and showing one after the other put 09:53 above 08:35.
 * Same instant: by folio, so two receipts keyed together read in folio order.
 * O(n log n) over a day's rows.
 */
export function cashUpTimeline(deposits: readonly Deposit[], payments: readonly CashUpPayment[]): CashUpEntry[] {
  const ranked: Ranked[] = [
    ...deposits.map((d): Ranked => ({ entry: { kind: "deposit", key: `d-${d.id}`, deposit: d }, at: instant(d.paidAt), folio: d.folio ?? d.id })),
    ...payments.map((p): Ranked => ({ entry: { kind: "payment", key: `p-${p.id}`, payment: p }, at: instant(p.paidAt), folio: p.reference !== "" ? p.reference : p.id })),
  ];
  ranked.sort((a, b) => a.at - b.at || a.folio.localeCompare(b.folio, LOCALE, { numeric: true }));
  return ranked.map((r) => r.entry);
}
