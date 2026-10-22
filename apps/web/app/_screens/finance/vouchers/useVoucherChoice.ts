"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { API_URL } from "../../../api-url";
import { HttpError } from "../../../auth/httpError";
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

export const READ_FAILED = "No pudimos revisar si hay comprobantes; si cobras, el sistema lo ligará solo.";

/**
 * A refusal the server answered without storing anything (4xx: a voucher's
 * state, CC-202, validation). Anything else — no answer, a 5xx — may have
 * stored the deposit, so the retry must be the same request.
 */
function nothingStored(err: unknown): boolean {
  return err instanceof HttpError && err.status >= 400 && err.status < 500;
}

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
  /** The pupil's vouchers could not be read: the server still links on its own. */
  readFailed: boolean;
  /**
   * A payment was sent with this choice and its fate is unknown: the choice
   * cannot change until the server answers that nothing was stored.
   */
  locked: boolean;
  /** The fields to send, or undefined (and `error` set) while the cashier must still choose. */
  parts: () => VoucherParts | undefined;
  /** About to send `parts`: from now on every retry repeats them. */
  sending: (parts: VoucherParts) => void;
  /** The deposit failed: frees the choice when nothing was stored, and acts on a voucher's refusal (undefined when it was not one). */
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
  const [readFailed, setReadFailed] = useState(false);
  // The parts of the attempt in flight or of unknown fate (tech-lead B1): with
  // two vouchers, v1 sent and lost, then "ninguno" retried, the second request
  // carries another key — a second deposit for the same money.
  const sent = useRef<VoucherParts | undefined>(undefined);
  const [locked, setLocked] = useState(false);

  useEffect(() => {
    if (!asks) return;
    let live = true;
    listPendingVouchers(API_URL, studentId)
      .then((rows) => { if (live) { setPending(rows); setReadFailed(false); } })
      // Unknown: the server still links or refuses on its own (409), and the
      // counter then reads them again. Said, not hidden.
      .catch(() => { if (live) { setPending([]); setReadFailed(true); } });
    return () => { live = false; };
  }, [asks, studentId, reads]);

  const pick = useCallback((choice: string) => { setPicked(choice); setError(undefined); }, []);

  const parts = useCallback((): VoucherParts | undefined => {
    if (byVoucher !== undefined) return { voucherId: byVoucher.id };
    if (!asks) return {};
    if (sent.current !== undefined) return sent.current;
    const chosen = partsFor(pending, declined, picked);
    setError(chosen === undefined ? PICK_ERROR : undefined);
    return chosen;
  }, [byVoucher, asks, pending, declined, picked]);

  const sending = useCallback((parts: VoucherParts) => {
    sent.current = parts;
    setLocked(true);
  }, []);

  const refuse = useCallback((err: unknown): VoucherRefusal | undefined => {
    if (nothingStored(err)) {
      sent.current = undefined;
      setLocked(false);
    }
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

  return { pending, declined, setDeclined, picked, pick, error, readFailed, locked, parts, sending, refuse };
}
