import { HttpError } from "../../../auth/httpError";
import { VOUCHER_CODES } from "../../../finance/vouchers";

/**
 * What the counter does with a voucher's refusal: say it and refresh the queue
 * ("gone": the voucher is no longer payable as it was shown), or ask the
 * cashier which voucher this money is ("choose"). Nothing was taken either way.
 */
export interface VoucherRefusal {
  kind: "gone" | "choose";
  message: string;
}

const NOT_FOUND = 404;
const CHANGED = "VOUCHER_CHANGED";

export const CHOOSE_MESSAGE = "Este alumno tiene varios comprobantes por revisar: elige cuál corresponde a este pago, o marca que es otro.";
const REJECTED_MESSAGE = "Este comprobante ya fue rechazado: no se puede cobrar con él.";
const CHANGED_MESSAGE = "Este comprobante cambió mientras lo veías. Ciérralo y ábrelo de nuevo desde la lista.";
const APPLIED_MESSAGE = "Este comprobante ya se aplicó a otro pago.";

/** The server's sentence, as a sentence: "este comprobante…" → "Este comprobante…." */
function sentence(text: string | undefined, fallback: string): string {
  const t = (text ?? "").trim();
  if (t === "") return fallback;
  const capital = t.charAt(0).toLocaleUpperCase("es-MX") + t.slice(1);
  return /[.!?]$/.test(capital) ? capital : `${capital}.`;
}

/**
 * The refusal, when `err` is one a voucher caused; undefined for anything
 * else (CC-202's 409 without a code included), which the counter handles as
 * it always did. `byVoucher`: the request carried a voucherId, so a 404 means
 * the voucher, not the pupil.
 */
export function voucherRefusal(err: unknown, byVoucher: boolean): VoucherRefusal | undefined {
  if (!(err instanceof HttpError)) return undefined;
  switch (err.errorCode) {
    case VOUCHER_CODES.alreadyApplied:
      return { kind: "gone", message: sentence(err.serverText, APPLIED_MESSAGE) };
    case VOUCHER_CODES.rejected:
      return { kind: "gone", message: REJECTED_MESSAGE };
    case VOUCHER_CODES.choiceRequired:
      return { kind: "choose", message: CHOOSE_MESSAGE };
    case CHANGED:
      return { kind: "gone", message: CHANGED_MESSAGE };
    default:
      return byVoucher && err.status === NOT_FOUND ? { kind: "gone", message: CHANGED_MESSAGE } : undefined;
  }
}
