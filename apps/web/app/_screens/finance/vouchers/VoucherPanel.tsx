"use client";

import type { ReactElement } from "react";
import { shortDayLabel } from "../../../_lib/localDate";
import type { PaymentVoucher } from "../../../finance/vouchers";
import { claimSentence, uploadedOn } from "./labels";
import { RecentDeposits } from "./RecentDeposits";
import { VoucherPreview } from "./VoucherPreview";
import { VoucherReject } from "./VoucherReject";

export interface VoucherPanelProps {
  voucher: PaymentVoucher;
  onRejected: () => void;
  onChanged: () => void;
}

/**
 * The voucher the counter was opened on, above the amount: the file, what the
 * family claims, what was already registered since, and the way to reject it.
 * The claim only prefills; the cashier confirms what the file shows.
 */
export function VoucherPanel({ voucher, onRejected, onChanged }: VoucherPanelProps): ReactElement {
  return (
    <section aria-labelledby="voucher-panel-title" style={{ display: "flex", flexDirection: "column", gap: 12, padding: "16px 18px", borderRadius: "var(--r-md)", border: "1px solid var(--border)", background: "var(--surface)" }}>
      <div>
        <h3 id="voucher-panel-title" style={{ fontSize: 16, fontWeight: 800, color: "var(--text)", margin: 0 }}>Comprobante de la familia</h3>
        <p style={{ fontSize: 13.5, color: "var(--text-muted)", margin: "4px 0 0" }}>
          {`Enviado por ${voucher.uploadedByName} el ${shortDayLabel(uploadedOn(voucher))}. ${claimSentence(voucher)}`}
        </p>
      </div>
      <VoucherPreview voucher={voucher} />
      <RecentDeposits voucher={voucher} />
      <VoucherReject voucher={voucher} onRejected={onRejected} onChanged={onChanged} />
    </section>
  );
}
