"use client";

import { useCallback, useEffect, useState, type ReactElement } from "react";

import { Btn } from "../../../_design/Controls";
import { DrawerShell } from "../../../_design/DrawerShell";
import { GradeRequestError } from "../../../grades/monthly";
import { MISSING_GRADES, type CloseResult, type GradingPeriod, type PeriodCaptureProgress, type PeriodsClient } from "../../../grades/periods";
import { EmptyHint, ErrorAlert } from "../../config/parts";
import { periodTitle } from "./format";
import { MissingList } from "./MissingList";
import { ProgressBar } from "./ProgressBar";

export interface ClosePeriodDialogProps {
  period: GradingPeriod;
  client: PeriodsClient;
  onClose: () => void;
  onClosed: (result: CloseResult) => void;
}

type ProgressLoad = { kind: "loading" } | { kind: "ready"; progress: PeriodCaptureProgress } | { kind: "failed" };

const CLOSE_FAILED = "No se pudo cerrar el periodo. Intenta de nuevo.";

function refusalText(err: unknown): string {
  return err instanceof GradeRequestError && err.detail !== "" ? err.detail : CLOSE_FAILED;
}

/**
 * Review before closing: closing publishes a boleta per pupil to the parents'
 * app and locks the months for everyone, so this confirmation earns its place.
 * With captures missing, the only way forward is the explicit "de todos modos".
 */
export function ClosePeriodDialog({ period, client, onClose, onClosed }: ClosePeriodDialogProps): ReactElement {
  const [load, setLoad] = useState<ProgressLoad>({ kind: "loading" });
  const [error, setError] = useState<string | undefined>(undefined);
  const [busy, setBusy] = useState(false);

  const fetchProgress = useCallback(async (): Promise<void> => {
    try {
      setLoad({ kind: "ready", progress: await client.progress(period.termId) });
    } catch {
      setLoad({ kind: "failed" });
    }
  }, [client, period.termId]);

  useEffect(() => {
    void fetchProgress();
  }, [fetchProgress]);

  const close = async (force: boolean): Promise<void> => {
    if (busy) return;
    setBusy(true);
    setError(undefined);
    try {
      onClosed(await client.close(period.termId, force));
    } catch (err) {
      setError(refusalText(err));
      // Someone deleted a grade meanwhile: show what is missing now.
      if (err instanceof GradeRequestError && err.code === MISSING_GRADES) await fetchProgress();
    } finally {
      setBusy(false);
    }
  };

  const missing = load.kind === "ready" ? load.progress.missing : [];
  const footer = (
    <>
      <button type="button" onClick={onClose} style={{ minHeight: 44, color: "var(--text-muted)", fontWeight: 600, cursor: "pointer" }}>Cancelar</button>
      <div style={{ flex: 1 }} />
      {load.kind === "ready" && missing.length === 0 && <Btn kind="forest" icon="lock" onClick={() => void close(false)}>{busy ? "Cerrando…" : "Cerrar periodo"}</Btn>}
      {load.kind === "ready" && missing.length > 0 && <Btn danger icon="warning" onClick={() => void close(true)}>{busy ? "Cerrando…" : "Cerrar de todos modos"}</Btn>}
    </>
  );

  return (
    <DrawerShell title={`Cerrar ${periodTitle(period)}`} subtitle="Se publica una boleta por alumno en la app de padres." icon="lock" onClose={onClose} footer={footer}>
      <div style={{ display: "grid", gap: 16 }}>
        {load.kind === "loading" && <EmptyHint>Revisando la captura…</EmptyHint>}
        {load.kind === "failed" && <ErrorAlert message="No se pudo revisar la captura del periodo." />}
        {load.kind === "ready" && (
          <>
            <ProgressBar captured={load.progress.captured} expected={load.progress.expected} />
            {missing.length === 0
              ? <p style={{ fontSize: 15, color: "var(--ok-fg)", fontWeight: 600 }}>Todas las calificaciones están capturadas.</p>
              : <MissingList missing={missing} />}
            <p style={{ fontSize: 14, color: "var(--text-muted)" }}>
              Al cerrar, nadie podrá modificar las calificaciones de estos meses hasta que la dirección reabra el periodo.
            </p>
          </>
        )}
        {error !== undefined && <ErrorAlert message={error} />}
      </div>
    </DrawerShell>
  );
}
