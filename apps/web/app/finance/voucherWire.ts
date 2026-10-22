/**
 * A family's payment voucher as the office reads it (ADR 0040), and its wire
 * mapping. Its own module, with no imports, so both the deposit client
 * (`linkedVoucher`) and the voucher client can read it without a cycle.
 *
 * Never money: `claimedAmountCents` and `claimedPaidOn` only prefill what the
 * cashier confirms.
 */

export type VoucherStatus = "pending" | "applied" | "rejected";

/** The three kinds the gateway accepts, proved by the file's bytes. */
export type VoucherContentType = "image/jpeg" | "image/png" | "application/pdf";

export interface PaymentVoucher {
  id: string;
  studentId: string;
  status: VoucherStatus;
  /** Absent when the family did not say. */
  claimedAmountCents?: number;
  /** aaaa-mm-dd; absent when the family did not say. */
  claimedPaidOn?: string;
  fileContentType: VoucherContentType;
  fileSizeBytes: number;
  /** The guardian who sent it. */
  uploadedByName: string;
  depositId?: string;
  /** The receipt it paid, P-<year>-<six digits>; only when applied. */
  depositFolio?: string;
  decidedByName?: string;
  decidedAt?: string;
  rejectionReason?: string;
  createdAt: string;
}

export type WirePaymentVoucher = Omit<PaymentVoucher, "status" | "fileContentType"> & {
  status: string;
  fileContentType: string;
};

const STATUSES: readonly VoucherStatus[] = ["pending", "applied", "rejected"];
const CONTENT_TYPES: readonly VoucherContentType[] = ["image/jpeg", "image/png", "application/pdf"];

function oneOf<T extends string>(allowed: readonly T[], value: string, fallback: T): T {
  return (allowed as readonly string[]).includes(value) ? (value as T) : fallback;
}

/** The optional fields kept only when present: an absent claim is not a zero. */
function present<K extends keyof PaymentVoucher>(key: K, value: PaymentVoucher[K] | "" | 0 | undefined): Partial<PaymentVoucher> {
  return value === undefined || value === "" || value === 0 ? {} : { [key]: value };
}

export function toVoucher(v: WirePaymentVoucher): PaymentVoucher {
  return {
    id: v.id,
    studentId: v.studentId,
    status: oneOf(STATUSES, v.status, "pending"),
    fileContentType: oneOf(CONTENT_TYPES, v.fileContentType, "application/pdf"),
    fileSizeBytes: v.fileSizeBytes,
    uploadedByName: v.uploadedByName,
    createdAt: v.createdAt,
    ...present("claimedAmountCents", v.claimedAmountCents),
    ...present("claimedPaidOn", v.claimedPaidOn),
    ...present("depositId", v.depositId),
    ...present("depositFolio", v.depositFolio),
    ...present("decidedByName", v.decidedByName),
    ...present("decidedAt", v.decidedAt),
    ...present("rejectionReason", v.rejectionReason),
  };
}
