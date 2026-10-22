/**
 * The office's side of the families' payment vouchers (ADR 0040):
 * `/api/v1/finance/payment-vouchers`, behind `module_payment_vouchers` and
 * `module_finance`. Reads answer to `collections:read`, the rejection to
 * `payments:write`. Paying by a voucher is `registerDeposit` with `voucherId`.
 *
 * UX only; the gateway is the security boundary.
 */
import { authFetch } from "../auth/authFetch";
import { failure } from "./api";
import { toVoucher, type PaymentVoucher, type WirePaymentVoucher } from "./voucherWire";

export type { PaymentVoucher, VoucherContentType, VoucherStatus } from "./voucherWire";

/** The machine codes a voucher's refusal rides in `X-Error-Code` with. */
export const VOUCHER_CODES = {
  alreadyApplied: "VOUCHER_ALREADY_APPLIED",
  rejected: "VOUCHER_REJECTED",
  choiceRequired: "VOUCHER_CHOICE_REQUIRED",
  limit: "VOUCHER_LIMIT",
} as const;

const VOUCHERS_PATH = "/api/v1/finance/payment-vouchers";

function url(baseURL: string, path: string): string {
  return `${baseURL.replace(/\/+$/, "")}${path}`;
}

async function ok(res: Response): Promise<Response> {
  if (!res.ok) {
    throw await failure(res);
  }
  return res;
}

/**
 * The vouchers still waiting for the office, newest first (the API's order;
 * the queue turns it into oldest first): the whole school's,
 * or one pupil's. One request either way — the queue never asks per row.
 */
export async function listPendingVouchers(baseURL: string, studentId?: string): Promise<PaymentVoucher[]> {
  const search = new URLSearchParams({ status: "pending" });
  if (studentId !== undefined && studentId !== "") {
    search.set("studentId", studentId);
  }
  const res = await ok(await authFetch(url(baseURL, `${VOUCHERS_PATH}?${search.toString()}`), { credentials: "include" }));
  const data = (await res.json()) as { vouchers?: WirePaymentVoucher[] };
  return (data.vouchers ?? []).map(toVoucher);
}

/**
 * The voucher's file, read with the session — there is no public URL. The
 * Blob is typed with the voucher's own (allow-listed) kind, never with what
 * the response says, so a preview can only ever be an image or a PDF.
 */
export async function getVoucherFile(baseURL: string, voucher: Pick<PaymentVoucher, "id" | "fileContentType">): Promise<Blob> {
  const res = await ok(await authFetch(url(baseURL, `${VOUCHERS_PATH}/${encodeURIComponent(voucher.id)}/file`), { credentials: "include" }));
  return new Blob([await res.arrayBuffer()], { type: voucher.fileContentType });
}

/** Refuse a pending voucher with the reason the family reads (3 to 300 characters). */
export async function rejectVoucher(baseURL: string, id: string, reason: string): Promise<PaymentVoucher> {
  const res = await ok(await authFetch(url(baseURL, `${VOUCHERS_PATH}/${encodeURIComponent(id)}/reject`), {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ reason }),
  }));
  const data = (await res.json()) as { voucher: WirePaymentVoucher };
  return toVoucher(data.voucher);
}
