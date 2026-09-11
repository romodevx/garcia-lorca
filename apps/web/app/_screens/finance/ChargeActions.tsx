"use client";

import { useState, type ReactElement } from "react";
import { useTranslations } from "next-intl";
import { Btn } from "../../_design/Controls";
import type { Charge } from "../../finance/api";
import { pesosToCents } from "../../_lib/money";
import { TextField } from "../../_design/form";
import { ErrorAlert } from "./parts";

export interface ChargeActionsProps {
  charge: Charge;
  /** Rejects with the gateway's reason so it can be shown next to the form. */
  onDiscount: (chargeId: string, discountCents: number, reason: string) => Promise<void>;
  /** Rejects with the gateway's reason. */
  onCancel: (chargeId: string) => Promise<void>;
}

/**
 * The secondary money actions of one charge, revealed under its row: forgive part
 * of it (a beca or an adjustment) or void it entirely.
 *
 * Both are consequential and irreversible from the UI, so they are deliberately
 * NOT one stray click away — the discount needs an amount AND a reason (the
 * gateway's audit trail), and cancelling asks for a second, explicit
 * confirmation. The gateway remains the authority: it recomputes the charge and
 * this panel only shows what it answers, or why it refused.
 */
export function ChargeActions({ charge, onDiscount, onCancel }: ChargeActionsProps): ReactElement {
  const t = useTranslations("finance");
  const [discount, setDiscount] = useState("");
  const [reason, setReason] = useState("");
  const [confirmingCancel, setConfirmingCancel] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);

  const detailOf = (cause: unknown): string => (cause instanceof Error ? cause.message : String(cause));

  const submitDiscount = async (): Promise<void> => {
    setError(undefined);
    const discountCents = pesosToCents(discount);
    if (Number.isNaN(discountCents) || discountCents <= 0) {
      setError(t("error.amount"));
      return;
    }
    if (reason.trim() === "") {
      setError(t("error.reasonRequired"));
      return;
    }
    setPending(true);
    try {
      await onDiscount(charge.id, discountCents, reason.trim());
      setDiscount("");
      setReason("");
    } catch (cause) {
      setError(t("error.discount", { detail: detailOf(cause) }));
    } finally {
      setPending(false);
    }
  };

  const submitCancel = async (): Promise<void> => {
    setError(undefined);
    if (!confirmingCancel) {
      setConfirmingCancel(true);
      return;
    }
    setPending(true);
    try {
      await onCancel(charge.id);
    } catch (cause) {
      setError(t("error.cancel", { detail: detailOf(cause) }));
    } finally {
      setConfirmingCancel(false);
      setPending(false);
    }
  };

  return (
    <div
      style={{
        display: "grid",
        gap: 12,
        margin: "0 20px 12px",
        padding: "14px 16px",
        borderRadius: "var(--r-md)",
        background: "var(--surface-3)",
        border: "1px solid var(--border)",
      }}
    >
      <div style={{ display: "flex", gap: 12, flexWrap: "wrap", alignItems: "flex-end" }}>
        <div style={{ flex: "1 1 160px", minWidth: 0 }}>
          <TextField label={t("field.discount")} value={discount} onChange={setDiscount} placeholder="0.00" />
        </div>
        <div style={{ flex: "2 1 220px", minWidth: 0 }}>
          <TextField label={t("field.reason")} value={reason} onChange={setReason} />
        </div>
        <Btn kind="forest" icon="scholarship" onClick={() => void submitDiscount()} disabled={pending} style={{ height: 44, fontSize: 14 }}>
          {t("action.applyDiscount")}
        </Btn>
        <Btn
          kind="outline"
          icon={confirmingCancel ? "warning" : "close"}
          danger={confirmingCancel}
          onClick={() => void submitCancel()}
          disabled={pending}
          style={{ height: 44, fontSize: 14 }}
        >
          {confirmingCancel ? t("action.confirmCancel") : t("action.cancelCharge")}
        </Btn>
      </div>
      <p style={{ margin: 0, fontSize: 12.5, color: "var(--text-faint)" }}>{t("hint.discountReason")}</p>
      {error !== undefined && <ErrorAlert message={error} />}
    </div>
  );
}
