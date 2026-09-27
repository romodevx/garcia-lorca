"use client";

import { useRef, useState, type ReactElement } from "react";

import { Btn } from "../../../_design/Controls";
import { DrawerShell } from "../../../_design/DrawerShell";
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

/** Reopening lets grades change again; the families keep the current boleta until the next close issues a new version. */
export function ReopenPeriodDialog({ period, client, onClose, onReopened, onStale }: ReopenPeriodDialogProps): ReactElement {
  const [error, setError] = useState<string | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  // A ref, not the state: two clicks inside one render would both read `busy` as false.
  const inFlight = useRef(false);

  const reopen = async (): Promise<void> => {
    if (inFlight.current) return;
    inFlight.current = true;
    setBusy(true);
    try {
      await client.reopen(period.termId);
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
        {error !== undefined && <ErrorAlert message={error} />}
      </div>
    </DrawerShell>
  );
}
