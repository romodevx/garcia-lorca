"use client";

import { useRef, useState, type KeyboardEvent, type ReactElement } from "react";
import { Btn } from "../../../_design/Controls";
import { TextField } from "../../../_design/form";
import { API_URL } from "../../../api-url";
import { rejectVoucher, type PaymentVoucher } from "../../../finance/vouchers";
import { voucherRefusal } from "./voucherRefusal";

const MIN_REASON = 3;
const MAX_REASON = 300;

/** What is wrong with a rejection's reason, if anything (3 to 300 characters, like the server). */
export function rejectReasonError(reason: string): string | undefined {
  const length = reason.trim().length;
  if (length === 0) return "Escribe el motivo: la familia lo lee en la app.";
  if (length < MIN_REASON) return `El motivo necesita al menos ${MIN_REASON} letras.`;
  return length > MAX_REASON ? `El motivo no puede pasar de ${MAX_REASON} caracteres.` : undefined;
}

export interface VoucherRejectProps {
  voucher: PaymentVoucher;
  onRejected: () => void;
  /** The voucher moved under the cashier (applied, or changed): the queue is stale. */
  onChanged: () => void;
}

/**
 * Rejecting is one field, so it opens in place (house rule: 1–4 fields inline),
 * inside the counter's form. Enter there rejects — it must never register the
 * payment the form around it is for.
 */
export function VoucherReject({ voucher, onRejected, onChanged }: VoucherRejectProps): ReactElement {
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  const [error, setError] = useState<string | undefined>(undefined);
  const [saving, setSaving] = useState(false);
  const inFlight = useRef(false);

  const reject = async (): Promise<void> => {
    if (inFlight.current) return;
    const wrong = rejectReasonError(reason);
    setError(wrong);
    if (wrong !== undefined) return;
    inFlight.current = true;
    setSaving(true);
    try {
      await rejectVoucher(API_URL, voucher.id, reason.trim());
      onRejected();
    } catch (cause) {
      const refusal = voucherRefusal(cause, true);
      if (refusal !== undefined) onChanged();
      setError(refusal?.message ?? (cause instanceof Error ? cause.message : "No se pudo rechazar el comprobante."));
    } finally {
      inFlight.current = false;
      setSaving(false);
    }
  };

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>): void => {
    if (e.key !== "Enter") return;
    e.preventDefault();
    void reject();
  };

  if (!open) {
    return <div><Btn kind="outline" icon="close" onClick={() => setOpen(true)}>Rechazar comprobante</Btn></div>;
  }
  return (
    <div onKeyDown={onKeyDown} style={{ display: "flex", flexDirection: "column", gap: 10 }}>
      <TextField
        label="Motivo del rechazo"
        value={reason}
        onChange={(v) => { setReason(v); if (error !== undefined && rejectReasonError(v) === undefined) setError(undefined); }}
        error={error}
        help="La familia lo lee en la app. De 3 a 300 caracteres."
        required
      />
      <div style={{ display: "flex", gap: 10 }}>
        <Btn kind="outline" busy={saving} onClick={() => void reject()}>{saving ? "Rechazando…" : "Confirmar rechazo"}</Btn>
        <Btn kind="ghost" onClick={() => { setOpen(false); setError(undefined); }}>No rechazar</Btn>
      </div>
    </div>
  );
}
