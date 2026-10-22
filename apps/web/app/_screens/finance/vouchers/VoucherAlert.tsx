"use client";

import { useEffect, useRef, type ReactElement } from "react";

export interface VoucherAlertState {
  message: string;
  /** Bumped on every refusal: the same sentence twice must take focus again. */
  attempt: number;
}

/**
 * A voucher refused the payment and nothing was taken: said at the top of the
 * counter, with focus moved there so the cashier reads it before retrying.
 */
export function VoucherAlert({ alert }: { alert: VoucherAlertState }): ReactElement {
  const ref = useRef<HTMLParagraphElement>(null);
  useEffect(() => {
    ref.current?.focus({ preventScroll: true });
    ref.current?.scrollIntoView?.({ block: "center", inline: "nearest" });
  }, [alert.attempt]);
  return (
    <p ref={ref} role="alert" tabIndex={-1} style={{ margin: 0, padding: "12px 16px", borderRadius: "var(--r-md)", background: "var(--bad-bg)", color: "var(--bad-fg)", fontSize: 14.5, fontWeight: 600 }}>
      {alert.message}
    </p>
  );
}
