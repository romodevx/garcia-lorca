import type { CardType, PaymentMethod, Tender } from "../../finance/api";
import { formatCents, pesosToCents } from "../../_lib/money";

/**
 * The arithmetic of a payment split across methods (T8) — pure, so the three
 * places that take money (the counter, a single charge, the uniforms till)
 * share one rule and one set of tests.
 */

/** More than this and the counter is doing something a spreadsheet should. */
export const MAX_TENDERS = 4;

export const AMOUNT_ERROR = "Escribe un monto mayor a cero.";
export const SUM_ERROR = "La suma de las formas de pago debe ser igual al monto a pagar.";

/** One row of the editor, as typed. The amount stays text until it is sent. */
export interface TenderRow {
  key: string;
  method: PaymentMethod;
  /** Kept while another method is chosen, so going back to card remembers it. */
  cardType: CardType;
  amount: string;
  reference: string;
}

/** The order methods are offered in, and the order a new row picks from. */
export const TENDER_METHODS: readonly PaymentMethod[] = ["cash", "card", "transfer", "credit_note"];

const METHOD_LABEL: Record<string, string> = {
  cash: "Efectivo",
  card: "Tarjeta",
  transfer: "Transferencia",
  credit_note: "Nota de crédito",
  mixed: "Combinado",
};

const CARD_LABEL: Record<CardType, string> = { debit: "Tarjeta de débito", credit: "Tarjeta de crédito" };

/** A method as the office says it; a card names its kind when known. */
export function tenderLabel(method: string, cardType?: CardType): string {
  if (method === "card" && cardType !== undefined) return CARD_LABEL[cardType];
  return METHOD_LABEL[method] ?? method;
}

/** What a row's amount is worth, or NaN when it is not readable money. */
function rowCents(row: TenderRow): number {
  return pesosToCents(row.amount);
}

/** What the readable amounts add up to. An unreadable one covers nothing. */
export function coveredCents(rows: readonly TenderRow[]): number {
  return rows.reduce((sum, row) => {
    const cents = rowCents(row);
    return Number.isFinite(cents) ? sum + cents : sum;
  }, 0);
}

/** "$500.00" left to cover, or "excede por $100.00" when the rows go over. */
export function remainingLabel(totalCents: number, covered: number): string {
  const left = totalCents - covered;
  return left < 0 ? `excede por ${formatCents(-left)}` : formatCents(left);
}

/** The first method no row uses yet: completing a credit note is usually cash. */
export function nextMethod(rows: readonly TenderRow[]): PaymentMethod {
  const used = new Set(rows.map((r) => r.method));
  return TENDER_METHODS.find((m) => !used.has(m)) ?? "cash";
}

/** What is wrong with one amount on its own, if anything. */
export function amountError(row: TenderRow): string | undefined {
  const cents = rowCents(row);
  return Number.isFinite(cents) && cents > 0 ? undefined : AMOUNT_ERROR;
}

/**
 * What blocks the submit, by row key. Each amount must be above zero; once
 * they all are, they must add up to the amount to pay — the mismatch is put on
 * the last row, the one a cashier uses to complete the payment.
 */
export function tenderErrors(rows: readonly TenderRow[], totalCents: number): Record<string, string> {
  const errors: Record<string, string> = {};
  for (const row of rows) {
    const wrong = amountError(row);
    if (wrong !== undefined) errors[row.key] = wrong;
  }
  const last = rows[rows.length - 1];
  if (Object.keys(errors).length === 0 && last !== undefined && coveredCents(rows) !== totalCents) {
    errors[last.key] = SUM_ERROR;
  }
  return errors;
}

/** The rows as the wire wants them: cents, a reference only when there is one, the card kind only on a card. */
export function toTenders(rows: readonly TenderRow[]): Tender[] {
  return rows.map((row) => {
    const reference = row.reference.trim();
    return {
      method: row.method,
      amountCents: rowCents(row),
      ...(reference !== "" ? { reference } : {}),
      ...(row.method === "card" ? { cardType: row.cardType } : {}),
    };
  });
}

/**
 * How a payment was made, in one line: "Nota de crédito $500.00 + Efectivo
 * $1,000.00". A single tender is just its method — its amount is already on
 * the row that shows it.
 */
export function tenderSummary(tenders: readonly Pick<Tender, "method" | "amountCents" | "cardType">[]): string {
  if (tenders.length === 1) {
    const [only] = tenders;
    return only === undefined ? "" : tenderLabel(only.method, only.cardType);
  }
  return tenders.map((t) => `${tenderLabel(t.method, t.cardType)} ${formatCents(t.amountCents)}`).join(" + ");
}
