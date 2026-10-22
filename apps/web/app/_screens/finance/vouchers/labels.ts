import type { PaymentVoucher } from "../../../finance/vouchers";
import { formatCents } from "../../../_lib/money";
import { shortDayLabel } from "../../../_lib/localDate";
import { schoolISODate } from "../../../_lib/schoolDay";

/** The school day the family sent it (ADR 0035), aaaa-mm-dd. */
export function uploadedOn(v: PaymentVoucher): string {
  return schoolISODate(new Date(v.createdAt));
}

/** "20 oct · $3,200.00 · Rosa Pérez": one voucher, told apart from another of the same pupil. */
export function voucherSummary(v: PaymentVoucher): string {
  const amount = v.claimedAmountCents === undefined ? "sin importe" : formatCents(v.claimedAmountCents);
  return `Subido el ${shortDayLabel(uploadedOn(v))} · ${amount} · ${v.uploadedByName}`;
}

/** What the family said it paid, as a sentence. */
export function claimSentence(v: PaymentVoucher): string {
  const amount = v.claimedAmountCents === undefined ? undefined : formatCents(v.claimedAmountCents);
  const day = v.claimedPaidOn === undefined ? undefined : shortDayLabel(v.claimedPaidOn);
  if (amount !== undefined && day !== undefined) return `Dice que pagó ${amount} el ${day}.`;
  if (amount !== undefined) return `Dice que pagó ${amount}; no dijo cuándo.`;
  if (day !== undefined) return `Dice que pagó el ${day}; no dijo cuánto.`;
  return "No dijo cuánto ni cuándo pagó: revísalo en el comprobante.";
}
