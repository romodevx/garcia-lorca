"use client";

import { useCallback, useEffect, useState } from "react";
import { API_URL } from "../../../api-url";
import { listPendingVouchers, type PaymentVoucher } from "../../../finance/vouchers";
import { voucherRefusal, type VoucherRefusal } from "./voucherRefusal";

/** What the counter knows of vouchers (ADR 0040). Absent: module_payment_vouchers is off. */
export interface CollectVouchers {
  /** Opened from the queue: this money IS that voucher. */
  byVoucher?: PaymentVoucher;
  /** A voucher was applied, rejected or found changed: the queue must be read again. */
  onChanged: () => void;
  /** The cashier rejected `byVoucher`. */
  onRejected?: () => void;
}

/** The radio that says the money is none of the pupil's vouchers. */
export const NONE_OF_THEM = "none";

export const PICK_ERROR = "Elige cuál comprobante es este pago, o marca que es otro.";

/** The voucher fields of the deposit request. */
export interface VoucherParts {
  voucherId?: string;
  declinedVoucherIds?: string[];
}

export interface VoucherChoice {
  /** The pupil's pending vouchers, for a payment registered by hand. */
  pending: readonly PaymentVoucher[];
  /** One pending: the cashier said this money is another payment. */
  declined: boolean;
  setDeclined: (declined: boolean) => void;
  /** Two or more pending: a voucher id, NONE_OF_THEM, or nothing yet. */
  picked: string | undefined;
  pick: (choice: string) => void;
  error: string | undefined;
  /** The fields to send, or undefined (and `error` set) while the cashier must still choose. */
  parts: () => VoucherParts | undefined;
  /** A voucher's refusal of the deposit, acted on; undefined when it was not one. */
  refuse: (err: unknown) => VoucherRefusal | undefined;
}

function partsFor(pending: readonly PaymentVoucher[], declined: boolean, picked: string | undefined): VoucherParts | undefined {
  const [only] = pending;
  if (pending.length === 1 && only !== undefined) {
    // The server links the only one by itself; saying nothing is "yes, it is that one".
    return declined ? { declinedVoucherIds: [only.id] } : {};
  }
  if (pending.length < 2) return {};
  if (picked === undefined) return undefined;
  return picked === NONE_OF_THEM ? { declinedVoucherIds: pending.map((v) => v.id) } : { voucherId: picked };
}

/**
 * Which voucher, if any, the money at the counter is. Paying BY a voucher the
 * answer is given; by hand, the pupil's pending vouchers are read once when
 * the drawer opens and again when the server says there are more than shown.
 */
export function useVoucherChoice(studentId: string, vouchers: CollectVouchers | undefined): VoucherChoice {
  const byVoucher = vouchers?.byVoucher;
  const asks = vouchers !== undefined && byVoucher === undefined;
  const [pending, setPending] = useState<readonly PaymentVoucher[]>([]);
  const [declined, setDeclined] = useState(false);
  const [picked, setPicked] = useState<string | undefined>(undefined);
  const [error, setError] = useState<string | undefined>(undefined);
  const [reads, setReads] = useState(0);

  useEffect(() => {
    if (!asks) return;
    let live = true;
    listPendingVouchers(API_URL, studentId)
      .then((rows) => { if (live) setPending(rows); })
      // Unknown: the server still links or refuses on its own (409), and the
      // counter then reads them again.
      .catch(() => { if (live) setPending([]); });
    return () => { live = false; };
  }, [asks, studentId, reads]);

  const pick = useCallback((choice: string) => { setPicked(choice); setError(undefined); }, []);

  const parts = useCallback((): VoucherParts | undefined => {
    if (byVoucher !== undefined) return { voucherId: byVoucher.id };
    if (!asks) return {};
    const chosen = partsFor(pending, declined, picked);
    setError(chosen === undefined ? PICK_ERROR : undefined);
    return chosen;
  }, [byVoucher, asks, pending, declined, picked]);

  const refuse = useCallback((err: unknown): VoucherRefusal | undefined => {
    if (vouchers === undefined) return undefined;
    const refusal = voucherRefusal(err, byVoucher !== undefined);
    if (refusal === undefined) return undefined;
    vouchers.onChanged();
    if (refusal.kind === "choose") {
      setPicked(undefined);
      setError(PICK_ERROR);
      setReads((n) => n + 1);
    }
    return refusal;
  }, [vouchers, byVoucher]);

  return { pending, declined, setDeclined, picked, pick, error, parts, refuse };
}
