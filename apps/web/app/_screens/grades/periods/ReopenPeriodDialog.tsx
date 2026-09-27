"use client";

import { useState, type ReactElement } from "react";

import { Btn } from "../../../_design/Controls";
import { DrawerShell } from "../../../_design/DrawerShell";
import { GradeRequestError } from "../../../grades/monthly";
import type { GradingPeriod, PeriodsClient } from "../../../grades/periods";
import { ErrorAlert } from "../../config/parts";
import { periodTitle } from "./format";

export interface ReopenPeriodDialogProps {
  period: GradingPeriod;
  client: PeriodsClient;
  onClose: () => void;
  onReopened: () => void;
}

const REOPEN_FAILED = "No se pudo reabrir el periodo. Intenta de nuevo.";

/** Reopening lets grades change again; the families keep the current boleta until the next close issues a new version. */
export function ReopenPeriodDialog({ period, client, onClose, onReopened }: ReopenPeriodDialogProps): ReactElement {
  const [error, setError] = useState<string | undefined>(undefined);
  const [busy, setBusy] = useState(false);

  const reopen = async (): Promise<void> => {
    if (busy) return;
    setBusy(true);
    try {
      await client.reopen(period.termId);
      onReopened();
    } catch (err) {
      setError(err instanceof GradeRequestError && err.detail !== "" ? err.detail : REOPEN_FAILED);
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
        {error !== undefined && <ErrorAlert message={error} />}
      </div>
    </DrawerShell>
  );
}
