"use client";

import type { ReactElement } from "react";

import { dayWithYearLabel } from "../../../_lib/localDate";
import type { GradingPeriod, PeriodCaptureProgress } from "../../../grades/periods";
import { MissingList } from "./MissingList";
import { ProgressBar } from "./ProgressBar";

/** Whether the capture can honestly be called complete, and what stands in the way. */
export interface CaptureVerdict {
  complete: boolean;
  unfinished: boolean;
  nothingExpected: boolean;
  /** Groups whose grade lacks this period: the server refuses any close, forced or not. */
  blocked: boolean;
}

export function captureVerdict(progress: PeriodCaptureProgress): CaptureVerdict {
  const nothingExpected = progress.expected === 0;
  const blocked = progress.groupsWithoutPeriod.length > 0;
  return {
    complete: !nothingExpected && !blocked && progress.missing.length === 0 && !progress.unfinished,
    unfinished: progress.unfinished,
    nothingExpected,
    blocked,
  };
}

const WARN = { fontSize: 15, fontWeight: 700, color: "var(--warn-fg)" } as const;

function unfinishedText(endsOn: string | null): string {
  return endsOn === null ? "Aún no termina el periodo" : `Aún no termina el periodo (termina el ${dayWithYearLabel(endsOn)})`;
}

/**
 * The review before closing. "Todas las calificaciones están capturadas" is
 * said only when it is true: something was expected, nothing is missing,
 * every group has the period and it has ended.
 */
export function CaptureReview({ period, progress, verdict }: { period: GradingPeriod; progress: PeriodCaptureProgress; verdict: CaptureVerdict }): ReactElement {
  return (
    <>
      <ProgressBar captured={progress.captured} expected={progress.expected} />
      {verdict.blocked && (
        <div role="note" style={{ display: "grid", gap: 4 }}>
          <p style={WARN}>{`Sin periodo ${period.ordinal} configurado: ${progress.groupsWithoutPeriod.join(", ")}`}</p>
          <p style={{ fontSize: 14, color: "var(--text-muted)" }}>
            El periodo no se puede cerrar hasta que Configuración › Periodos por grado les dé uno a esos grados.
          </p>
        </div>
      )}
      {verdict.unfinished && <p style={WARN}>{unfinishedText(progress.endsOn)}</p>}
      {verdict.nothingExpected && (
        <p style={WARN}>No hay calificaciones que revisar en este periodo: aún no tiene materias con docente en sus grupos.</p>
      )}
      {progress.missing.length > 0 && <MissingList missing={progress.missing} />}
      {verdict.complete && <p style={{ fontSize: 15, color: "var(--ok-fg)", fontWeight: 600 }}>Todas las calificaciones están capturadas.</p>}
    </>
  );
}
