"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { API_URL } from "../../api-url";
import { listPendingVouchers, type PaymentVoucher } from "../../finance/vouchers";

export type VoucherQueueStatus = "off" | "loading" | "ready" | "failed";

export interface VoucherQueue {
  status: VoucherQueueStatus;
  /** Oldest first: the counter clears them in the order they came (the API sends newest first). */
  vouchers: readonly PaymentVoucher[];
  /** Pupils with a voucher waiting: the search's chip reads this, no request per row. */
  pendingStudents: ReadonlySet<string>;
  reload: () => void;
}

const NONE: readonly PaymentVoucher[] = [];

/**
 * FIFO for the front desk (design §6). Compared as instants, not as text: the
 * gateway's RFC 3339 carries a variable number of fractional digits. Ties by id.
 */
function oldestFirst(rows: readonly PaymentVoucher[]): PaymentVoucher[] {
  return [...rows].sort((a, b) => Date.parse(a.createdAt) - Date.parse(b.createdAt) || a.id.localeCompare(b.id));
}

/**
 * The vouchers families sent from the app and nobody has decided yet (ADR
 * 0040). One read for the whole school; `enabled` false (the module's flag is
 * off) asks for nothing at all.
 */
export function useVoucherQueue(enabled: boolean): VoucherQueue {
  const [status, setStatus] = useState<VoucherQueueStatus>(enabled ? "loading" : "off");
  const [vouchers, setVouchers] = useState<readonly PaymentVoucher[]>(NONE);
  const [reads, setReads] = useState(0);

  useEffect(() => {
    if (!enabled) {
      setStatus("off");
      return;
    }
    let live = true;
    setStatus("loading");
    listPendingVouchers(API_URL)
      .then((rows) => { if (live) { setVouchers(oldestFirst(rows)); setStatus("ready"); } })
      .catch(() => { if (live) { setStatus("failed"); } });
    return () => { live = false; };
  }, [enabled, reads]);

  const reload = useCallback(() => setReads((n) => n + 1), []);
  const shown = enabled ? vouchers : NONE;
  const pendingStudents = useMemo(() => new Set(shown.map((v) => v.studentId)), [shown]);
  return { status: enabled ? status : "off", vouchers: shown, pendingStudents, reload };
}
