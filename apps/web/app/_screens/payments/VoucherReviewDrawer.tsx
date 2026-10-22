"use client";

import type { ReactElement } from "react";
import { DrawerShell } from "../../_design/DrawerShell";
import type { PaymentVoucher } from "../../finance/vouchers";
import { VoucherPanel } from "../finance/vouchers/VoucherPanel";

export interface VoucherReviewDrawerProps {
  voucher: PaymentVoucher;
  onClose: () => void;
  onRejected: () => void;
  onChanged: () => void;
}

/**
 * A voucher whose pupil is not on the counter's roll (withdrawn, or not yet
 * listed): it can be seen and rejected, never collected — there is no pupil
 * here to take the money for.
 */
export function VoucherReviewDrawer({ voucher, onClose, onRejected, onChanged }: VoucherReviewDrawerProps): ReactElement {
  return (
    <DrawerShell title="Revisar comprobante" subtitle="El alumno no está en la lista: sólo se puede revisar o rechazar" icon="documents" size="lg" onClose={onClose}>
      <VoucherPanel voucher={voucher} onChanged={onChanged} onRejected={() => { onChanged(); onRejected(); }} />
    </DrawerShell>
  );
}
