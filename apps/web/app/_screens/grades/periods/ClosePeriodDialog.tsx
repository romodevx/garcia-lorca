"use client";

import { useCallback, useEffect, useRef, useState, type ReactElement } from "react";

import { Btn } from "../../../_design/Controls";
import { DrawerShell } from "../../../_design/DrawerShell";
import { MISSING_GRADES, type CloseResult, type GradingPeriod, type PeriodCaptureProgress, type PeriodsClient } from "../../../grades/periods";
import { GradeRequestError } from "../../../grades/requestError";
import { EmptyHint, ErrorAlert } from "../../config/parts";
import { CaptureReview, captureVerdict } from "./CaptureReview";
import { periodTitle } from "./format";
import { closeRefusal } from "./refusals";

export interface ClosePeriodDialogProps {
  period: GradingPeriod;
  client: PeriodsClient;
  onClose: () => void;
  onClosed: (result: CloseResult) => void;
  /** The period was already closed by someone else: reload and say so. */
  onStale: (message: string) => void;
}

type ProgressLoad = { kind: "loading" } | { kind: "ready"; progress: PeriodCaptureProgress } | { kind: "failed" };

/**
 * Review before closing: closing publishes a boleta per pupil to the parents'
 * app and locks the period for everyone, so this confirmation earns its place.
 * Unless the capture is honestly complete, the only way forward is the
 * explicit "de todos modos".
 */
export function ClosePeriodDialog({ period, client, onClose, onClosed, onStale }: ClosePeriodDialogProps): ReactElement {
  const [load, setLoad] = useState<ProgressLoad>({ kind: "loading" });
  const [error, setError] = useState<string | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  // A ref, not the state: two clicks inside one render would both read `busy` as false.
  const inFlight = useRef(false);

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
    if (inFlight.current) return;
    inFlight.current = true;
    setBusy(true);
    setError(undefined);
    try {
      onClosed(await client.close(period.termId, force));
    } catch (err) {
      const refusal = closeRefusal(err);
      if (refusal.kind === "stale") {
        onStale(refusal.toast);
        return;
      }
      setError(refusal.text);
      // Someone deleted a grade meanwhile: show what is missing now.
      if (err instanceof GradeRequestError && err.code === MISSING_GRADES) await fetchProgress();
    } finally {
      inFlight.current = false;
      setBusy(false);
    }
  };

  const verdict = load.kind === "ready" ? captureVerdict(load.progress) : undefined;
  const footer = (
    <>
      <button type="button" onClick={onClose} style={{ minHeight: 44, color: "var(--text-muted)", fontWeight: 600, cursor: "pointer" }}>Cancelar</button>
      <div style={{ flex: 1 }} />
      {verdict?.complete === true && <Btn kind="forest" icon="lock" onClick={() => void close(false)}>{busy ? "Cerrando…" : "Cerrar periodo"}</Btn>}
      {verdict?.complete === false && !verdict.blocked && <Btn danger icon="warning" onClick={() => void close(true)}>{busy ? "Cerrando…" : "Cerrar de todos modos"}</Btn>}
    </>
  );

  return (
    <DrawerShell title={`Cerrar ${periodTitle(period)}`} subtitle="Se publica una boleta por alumno en la app de padres." icon="lock" onClose={onClose} footer={footer}>
      <div style={{ display: "grid", gap: 16 }}>
        {load.kind === "loading" && <EmptyHint>Revisando la captura…</EmptyHint>}
        {load.kind === "failed" && <ErrorAlert message="No se pudo revisar la captura del periodo." />}
        {load.kind === "ready" && verdict !== undefined && (
          <>
            <CaptureReview period={period} progress={load.progress} verdict={verdict} />
            <p style={{ fontSize: 14, color: "var(--text-muted)" }}>
              Al cerrar, nadie podrá modificar las calificaciones de este periodo hasta que la dirección reabra el periodo.
            </p>
          </>
        )}
        {error !== undefined && <ErrorAlert message={error} />}
      </div>
    </DrawerShell>
  );
}
