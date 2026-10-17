"use client";

import { useRef, useState, type ReactElement } from "react";

import { Btn } from "../../../_design/Controls";
import { DrawerShell } from "../../../_design/DrawerShell";
import { TextArea, useRequired } from "../../../_design/form";
import type { GradingPeriod, PeriodsClient } from "../../../grades/periods";
import { ErrorAlert } from "../../config/parts";
import { periodTitle } from "./format";
import { reopenRefusal } from "./refusals";

export interface ReopenPeriodDialogProps {
  period: GradingPeriod;
  client: PeriodsClient;
  onClose: () => void;
  onReopened: () => void;
  /** The period was already open (someone reopened it first): reload and say so. */
  onStale: (message: string) => void;
}

/** The longest reason the server stores (CC-308). */
const MAX_REASON = 500;

/** Shown under the reason when it runs past MAX_REASON. */
const REASON_TOO_LONG = "Máximo 500 caracteres";

/**
 * Reopening lets grades change again; the families keep the current boleta until the next close issues a new version.
 * Why it is reopened is required (CC-308): the server stores it with who did it, so a later "¿quién cambió esta boleta?" has an answer.
 */
export function ReopenPeriodDialog({ period, client, onClose, onReopened, onStale }: ReopenPeriodDialogProps): ReactElement {
  const [error, setError] = useState<string | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  const [reason, setReason] = useState("");
  const [tooLong, setTooLong] = useState(false);
  const required = useRequired(["reason"]);
  // A ref, not the state: two clicks inside one render would both read `busy` as false.
  const inFlight = useRef(false);

  const reopen = async (): Promise<void> => {
    if (inFlight.current) return;
    const trimmed = reason.trim();
    if (!required.check({ reason: trimmed })) return;
    if ([...trimmed].length > MAX_REASON) {
      setTooLong(true);
      return;
    }
    inFlight.current = true;
    setBusy(true);
    try {
      await client.reopen(period.termId, trimmed);
      onReopened();
    } catch (err) {
      const refusal = reopenRefusal(err);
      if (refusal.kind === "stale") {
        onStale(refusal.toast);
        return;
      }
      setError(refusal.text);
    } finally {
      inFlight.current = false;
      setBusy(false);
    }
  };

  const footer = (
    <>
      <button type="button" onClick={onClose} style={{ minHeight: 44, color: "var(--text-muted)", fontWeight: 600, cursor: "pointer" }}>Cancelar</button>
      <div style={{ flex: 1 }} />
      <Btn kind="forest" onClick={() => void reopen()}>{busy ? "Reabriendo…" : "Reabrir periodo"}</Btn>
    </>
  );

  return (
    <DrawerShell title={`Reabrir ${periodTitle(period)}`} icon="lock" size="sm" onClose={onClose} footer={footer}>
      <div style={{ display: "grid", gap: 12, fontSize: 15, color: "var(--text)" }}>
        <p>Los docentes y la dirección podrán volver a modificar las calificaciones de {period.termName.toLowerCase()}.</p>
        <p style={{ color: "var(--text-muted)" }}>
          Las familias siguen viendo la boleta actual (v{period.version}). Al volver a cerrar se emite la v{period.version + 1} como “Boleta actualizada”.
        </p>
        <TextArea
          label="Motivo"
          required
          rows={3}
          value={reason}
          help="Queda registrado con tu nombre: por qué se vuelven a modificar las calificaciones."
          error={required.errors.reason ?? (tooLong ? REASON_TOO_LONG : undefined)}
          onClearError={() => {
            required.clear("reason");
            setTooLong(false);
          }}
          onChange={setReason}
        />
        {error !== undefined && <ErrorAlert message={error} />}
      </div>
    </DrawerShell>
  );
}
