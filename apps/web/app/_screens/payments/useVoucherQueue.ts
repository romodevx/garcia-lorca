"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { API_URL } from "../../api-url";
import { listPendingVouchers, type PaymentVoucher } from "../../finance/vouchers";

export type VoucherQueueStatus = "off" | "loading" | "ready" | "failed";

export interface VoucherQueue {
  status: VoucherQueueStatus;
  /** Oldest first, as the gateway sends them. */
  vouchers: readonly PaymentVoucher[];
  /** Pupils with a voucher waiting: the search's chip reads this, no request per row. */
  pendingStudents: ReadonlySet<string>;
  reload: () => void;
}

const NONE: readonly PaymentVoucher[] = [];

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
      .then((rows) => { if (live) { setVouchers(rows); setStatus("ready"); } })
      .catch(() => { if (live) { setStatus("failed"); } });
    return () => { live = false; };
  }, [enabled, reads]);

  const reload = useCallback(() => setReads((n) => n + 1), []);
  const shown = enabled ? vouchers : NONE;
  const pendingStudents = useMemo(() => new Set(shown.map((v) => v.studentId)), [shown]);
  return { status: enabled ? status : "off", vouchers: shown, pendingStudents, reload };
}
