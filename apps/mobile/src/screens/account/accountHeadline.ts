import type { FamilyStatement } from "../../api";

/** The account's leading figure, as Cuenta and Inicio both show it. */
export interface AccountHeadline {
  label: string;
  /** What to print: never negative (credit is said in words, T5). */
  amountCents: number;
  /** Nothing to pay: the "Al corriente" chip. */
  settled: boolean;
  /** The school's day is known, so the figure is today's and the balance a second line. */
  hasToday: boolean;
  /** The cycle's whole balance; negative is credit. */
  balanceCents: number;
}

/**
 * What the school is asking for TODAY leads; the pending balance sits beside
 * it. Both are true at once, and a parent shown only the balance is told a
 * number nobody is asking them for — they are also the one person who cannot
 * walk to the office and ask which is which (QA 21-oct: Inicio judged the
 * whole cycle and said "Pendiente" where Cuenta said "Al corriente").
 *
 * An EMPTY asOf means the day is unknown. Rendering the day's figure as $0.00
 * there would tell a family that owes money that they are settled — a
 * fabricated "Al corriente" (bug_005). So the headline falls back to the
 * balance; below zero that balance is the school owing the family (T5).
 *
 * dueTodayCents is what makes the month current, early-payment reward
 * included (ISSUE-T14-106) — not what is overdue.
 */
export function accountHeadline(statement: Pick<FamilyStatement, "totals" | "dueTodayCents" | "asOf">): AccountHeadline {
  const hasToday = statement.asOf !== "";
  const balanceCents = statement.totals.balanceCents;
  const headCents = hasToday ? statement.dueTodayCents : balanceCents;
  const settled = headCents <= 0;
  const isCredit = !hasToday && balanceCents < 0;
  const label = hasToday ? "Para estar al corriente" : isCredit ? "Saldo a favor" : settled ? "Saldo actual" : "Saldo por pagar";
  return {
    label,
    amountCents: isCredit ? -headCents : Math.max(headCents, 0),
    settled,
    hasToday,
    balanceCents,
  };
}
