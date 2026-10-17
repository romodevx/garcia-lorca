"use client";

import { useEffect, useState } from "react";

import { API_URL } from "../../api-url";
import { previewDeposit, type DepositPreview } from "../../finance/api";

/** How long the typing has to stop before we ask the server what it covers. */
const PREVIEW_DEBOUNCE_MS = 350;

export interface DepositPreviewQuery {
  studentId: string;
  /** What would be handed over, in cents; anything not positive asks nothing. */
  amountCents: number;
  /** The day the family paid, or "" for today. */
  paidOn: string;
  /** The charges the money may pay; absent means the counter's own rule. */
  chargeIds?: readonly string[];
  /** false stops asking (a wrong date, a receipt already on screen). */
  enabled: boolean;
}

/**
 * What an amount WOULD cover, asked of the same planner that takes the money
 * once the typing settles — so the preview can never drift from what happens:
 * the late fee it writes, the pronto pago it applies, the credit it leaves.
 * Shared by the counter and Finanzas' Registrar pago (review B1).
 */
export function useDepositPreview({ studentId, amountCents, paidOn, chargeIds, enabled }: DepositPreviewQuery): DepositPreview | null {
  const [preview, setPreview] = useState<DepositPreview | null>(null);
  const chargeKey = chargeIds?.join(",") ?? "";
  useEffect(() => {
    if (!enabled || !Number.isFinite(amountCents) || amountCents <= 0) {
      setPreview(null);
      return;
    }
    let live = true;
    const timer = setTimeout(() => {
      previewDeposit(API_URL, {
        studentId,
        amountCents,
        ...(paidOn !== "" ? { paidAt: paidOn } : {}),
        ...(chargeKey !== "" ? { chargeIds: chargeKey.split(",") } : {}),
      })
        .then((p) => { if (live) setPreview(p); })
        .catch(() => { if (live) setPreview(null); });
    }, PREVIEW_DEBOUNCE_MS);
    return () => { live = false; clearTimeout(timer); };
  }, [studentId, amountCents, paidOn, chargeKey, enabled]);
  return preview;
}
