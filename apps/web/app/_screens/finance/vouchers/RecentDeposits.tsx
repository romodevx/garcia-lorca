"use client";

import { useEffect, useState, type ReactElement } from "react";
import { API_URL } from "../../../api-url";
import { listDeposits, type Deposit } from "../../../finance/api";
import type { PaymentVoucher } from "../../../finance/vouchers";
import { formatCents } from "../../../_lib/money";
import { shortDayLabel } from "../../../_lib/localDate";
import { schoolISODate } from "../../../_lib/schoolDay";
import { uploadedOn } from "./labels";

/** The school day a deposit's money changed hands (ADR 0035). */
function dayOf(d: Deposit): string {
  return d.paidOn ?? schoolISODate(new Date(d.paidAt));
}

/** Deposits still counting, on or after `since`: the ones this voucher could duplicate. */
export function depositsSince(deposits: readonly Deposit[], since: string): Deposit[] {
  return deposits.filter((d) => d.voidedAt === undefined && d.supersededAt === undefined && dayOf(d) >= since);
}

/**
 * The human guard against a double payment (design §4 D): what was already
 * registered for the pupil since the day the family says it paid (or sent the
 * voucher). Covers the cashier who declined a link and now applies the
 * voucher, and the parent who paid at the counter AND sent the transfer.
 */
export function RecentDeposits({ voucher }: { voucher: PaymentVoucher }): ReactElement | null {
  const [recent, setRecent] = useState<Deposit[]>([]);
  const since = voucher.claimedPaidOn ?? uploadedOn(voucher);

  useEffect(() => {
    let live = true;
    listDeposits(API_URL, voucher.studentId)
      .then((rows) => { if (live) setRecent(depositsSince(rows, since)); })
      // Unknown is not "nothing registered", but the voucher is still payable:
      // the server's own guards stand behind this one.
      .catch(() => { if (live) setRecent([]); });
    return () => { live = false; };
  }, [voucher.studentId, since]);

  if (recent.length === 0) return null;
  return (
    <div role="note" style={{ padding: "12px 16px", borderRadius: "var(--r-md)", background: "var(--warn-bg)", color: "var(--warn-fg)", fontSize: 14 }}>
      <strong>Revisa que no sea el mismo pago:</strong>
      <ul style={{ margin: "6px 0 0", paddingLeft: 18 }}>
        {recent.map((d) => (
          <li key={d.id}>{`Ya se registró ${formatCents(d.amountCents)} el ${shortDayLabel(dayOf(d))}${d.folio !== undefined ? ` (${d.folio})` : ""}`}</li>
        ))}
      </ul>
    </div>
  );
}
