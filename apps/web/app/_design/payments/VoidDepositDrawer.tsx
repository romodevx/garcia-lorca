"use client";

import { useState, type ReactElement } from "react";

import { API_URL } from "../../api-url";
import { voidDeposit } from "../../finance/api";
import { failureMessage } from "../../finance/failureMessage";
import { Btn } from "../Controls";
import { DrawerShell } from "../DrawerShell";
import { Icon } from "../Icon";
import { TextArea, useRequired } from "../form";

export interface VoidDepositDrawerProps {
  depositId: string;
  /** The receipt's folio, as the statement shows it. */
  folio: string;
  /** Voided: the message for the statement to say. */
  onVoided: (message: string) => void;
  onCancel: () => void;
}

const REQUIRED = ["reason"] as const;
const VOID_FAILURE = {
  conflict: "Este pago ya estaba anulado o corregido. Vuelve a abrir el estado de cuenta.",
  forbiddenAction: "anular pagos",
  other: "No se pudo anular el pago. Intenta de nuevo.",
};

/**
 * Cancelling a registered payment (CC-203). It does not undo — the receipt
 * stays as cancelled and what it settled is owed again — so, unlike the rest of
 * the house, this asks first, and asks why (the gateway refuses it without a
 * reason). It replaces the statement while open: one dialog at a time.
 */
export function VoidDepositDrawer({ depositId, folio, onVoided, onCancel }: VoidDepositDrawerProps): ReactElement {
  const [reason, setReason] = useState("");
  const [saving, setSaving] = useState(false);
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const required = useRequired(REQUIRED);
  const named = folio === "" ? "este pago" : `el pago ${folio}`;

  const submit = (): void => {
    if (saving || !required.check({ reason })) return;
    setSaving(true);
    setProblem(undefined);
    voidDeposit(API_URL, depositId, reason.trim())
      .then(() => onVoided(`${folio === "" ? "Pago" : `Pago ${folio}`} anulado. Lo que pagó vuelve a deberse.`))
      .catch((cause: unknown) => setProblem(failureMessage(cause, VOID_FAILURE)))
      .finally(() => setSaving(false));
  };

  return (
    <DrawerShell title={`Anular pago ${folio}`.trim()} icon="warning" size="sm" onClose={onCancel} onSubmit={submit} dismissible={reason.trim() === ""}
      footer={
        <div style={{ display: "flex", alignItems: "center", gap: 12, width: "100%" }}>
          <Btn kind="ghost" onClick={onCancel}>No anular</Btn>
          <div style={{ flex: 1 }} />
          <Btn type="submit" danger icon="trash" busy={saving}>Anular pago</Btn>
        </div>
      }>
      <div style={{ display: "grid", gap: 16 }}>
        <p style={{ margin: 0, display: "flex", gap: 8, fontSize: 15, lineHeight: 1.5, color: "var(--bad-fg)", fontWeight: 600 }}>
          <Icon name="warning" size={18} />Anular {named} no se puede deshacer.
        </p>
        <p style={{ margin: 0, fontSize: 15, lineHeight: 1.5, color: "var(--text)" }}>
          El comprobante queda en el historial como cancelado, sale del corte del día y lo que pagó vuelve a deberse. Si sólo hay que cambiar el monto o la forma de pago, usa Corregir.
        </p>
        <TextArea label="Motivo" required value={reason} onChange={setReason} onClearError={() => required.clear("reason")} error={required.errors.reason} help="Queda junto al comprobante cancelado." />
        {problem !== undefined && <p role="alert" style={{ margin: 0, color: "var(--bad-fg)", fontWeight: 600 }}>{problem}</p>}
      </div>
    </DrawerShell>
  );
}
