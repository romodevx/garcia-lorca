import type { RegisterDepositInput, Tender } from "../../finance/api";

/**
 * One key per payment ATTEMPT, minted when the drawer opens and kept across
 * retries: the gateway dedupes on it, so a retry after a network error returns
 * the deposit already stored instead of charging the family twice.
 */
export function newAttemptKey(): string {
  return globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

export interface DepositRequestParts {
  studentId: string;
  amountCents: number;
  /** How it is paid, method by method; they add up to the amount. */
  tenders: readonly Tender[];
  attemptKey: string;
  /** The day the family paid, or "" for today (the server then keeps the hour). */
  paidOn: string;
  /** The charges the money may pay; absent means the counter's own rule. */
  chargeIds?: readonly string[];
  grantEarlyPayment?: boolean;
}

/**
 * The one way a payment is taken (ISSUE-T14-003): a deposit, at the counter and
 * in Finanzas alike, so every payment gets its folio, its ticket and its place
 * in the cut under who collected it.
 */
export function depositRequest(parts: DepositRequestParts): RegisterDepositInput {
  const [first] = parts.tenders;
  return {
    studentId: parts.studentId,
    amountCents: parts.amountCents,
    // The first tender, for a gateway that reads only one method; with
    // `tenders` present the server ignores these.
    method: first?.method ?? "cash",
    reference: first?.reference ?? "",
    ...(first?.cardType !== undefined ? { cardType: first.cardType } : {}),
    tenders: [...parts.tenders],
    idempotencyKey: parts.attemptKey,
    ...(parts.paidOn !== "" ? { paidAt: parts.paidOn } : {}),
    ...(parts.chargeIds !== undefined ? { chargeIds: [...parts.chargeIds] } : {}),
    ...(parts.grantEarlyPayment === true ? { grantEarlyPayment: true } : {}),
  };
}
