import { apiFetch } from "./client";
import { ApiError } from "./errors";

/**
 * The family's payment vouchers (comprobantes, ADR 0040): a photo or PDF of a
 * transfer, sent for the office to apply. Never money — the amount and the day
 * only prefill what the office confirms. Both routes are scoped to the caller's
 * own child server-side (another child answers 404).
 */

export type PaymentVoucherStatus = "pending" | "applied" | "rejected";

/** A voucher as the family sees it: what it sent and what the office did with it. */
export interface FamilyPaymentVoucher {
  id: string;
  status: PaymentVoucherStatus;
  claimedAmountCents?: number;
  /** YYYY-MM-DD. */
  claimedPaidOn?: string;
  fileContentType?: string;
  /** The receipt's number; only when applied. */
  depositFolio?: string;
  /** Only when rejected. */
  rejectionReason?: string;
  createdAt: string;
  decidedAt?: string;
}

export interface FamilyPaymentVoucherResult {
  voucher: FamilyPaymentVoucher;
  /** This file was already sent for the child; nothing was stored. */
  replayed: boolean;
}

/** A local file as React Native's FormData sends it. */
export interface VoucherFile {
  uri: string;
  name: string;
  type: string;
}

export interface VoucherUpload {
  file: VoucherFile;
  amountCents?: number;
  /** YYYY-MM-DD. */
  paidOn?: string;
}

/** Why a voucher was not taken, in the terms the screen words its message by. */
export type VoucherSendReason =
  | "too_large"
  | "limit"
  | "conflict"
  | "rate_limited"
  | "invalid"
  | "session"
  | "unavailable"
  | "network"
  | "server";

export class VoucherSendError extends Error {
  constructor(
    public readonly reason: VoucherSendReason,
    public readonly retryAfterSeconds?: number,
  ) {
    super(`voucher not sent: ${reason}`);
    this.name = "VoucherSendError";
  }
}

const VOUCHER_LIMIT_CODE = "VOUCHER_LIMIT";

function vouchersPath(studentId: string): string {
  return `/api/v1/family/students/${encodeURIComponent(studentId)}/payment-vouchers`;
}

function refusalOf(res: Response): VoucherSendError {
  switch (res.status) {
    case 400:
      return new VoucherSendError("invalid");
    case 401:
    case 403:
      return new VoucherSendError("session");
    case 404:
      return new VoucherSendError("unavailable");
    case 409:
      return new VoucherSendError(res.headers.get("X-Error-Code") === VOUCHER_LIMIT_CODE ? "limit" : "conflict");
    case 413:
      return new VoucherSendError("too_large");
    case 429: {
      const wait = Number.parseInt(res.headers.get("Retry-After") ?? "", 10);
      return new VoucherSendError("rate_limited", Number.isFinite(wait) && wait > 0 ? wait : undefined);
    }
    default:
      return new VoucherSendError("server");
  }
}

function formOf(upload: VoucherUpload): FormData {
  const form = new FormData();
  // React Native's FormData streams a { uri, name, type } part from disk.
  form.append("file", upload.file as unknown as Blob);
  if (upload.amountCents !== undefined) form.append("amountCents", String(upload.amountCents));
  if (upload.paidOn !== undefined) form.append("paidOn", upload.paidOn);
  return form;
}

/**
 * Sends one voucher for the child. The same file again resolves with
 * `replayed: true`. Every refusal is a {@link VoucherSendError}.
 */
export async function sendPaymentVoucher(studentId: string, upload: VoucherUpload): Promise<FamilyPaymentVoucherResult> {
  let res: Response;
  try {
    // No Content-Type: the platform writes it, boundary included.
    res = await apiFetch(vouchersPath(studentId), { method: "POST", body: formOf(upload) });
  } catch {
    throw new VoucherSendError("network");
  }
  if (!res.ok) throw refusalOf(res);
  return (await res.json()) as FamilyPaymentVoucherResult;
}

/** Reads the child's vouchers, newest first (as the API orders them). Throws an {@link ApiError} on any non-2xx. */
export async function listPaymentVouchers(studentId: string): Promise<FamilyPaymentVoucher[]> {
  const res = await apiFetch(vouchersPath(studentId));
  if (!res.ok) throw new ApiError(res.status, "failed to load payment vouchers");
  const data = (await res.json()) as { vouchers?: FamilyPaymentVoucher[] | null };
  return data.vouchers ?? [];
}
