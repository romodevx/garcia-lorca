"use client";

import type { ReactElement } from "react";

import type { GradingPeriod, PeriodCaptureProgress } from "../../../grades/periods";
import { monthsList, schoolMonth, unfinishedMonths } from "./format";
import { MissingList } from "./MissingList";
import { ProgressBar } from "./ProgressBar";

/** Whether the capture can honestly be called complete, and what stands in the way. */
export interface CaptureVerdict {
  complete: boolean;
  unfinished: string[];
  nothingExpected: boolean;
}

export function captureVerdict(period: GradingPeriod, progress: PeriodCaptureProgress, now: Date): CaptureVerdict {
  const months = progress.months.length > 0 ? progress.months : period.months;
  const unfinished = unfinishedMonths(months, progress.unfinishedMonths, schoolMonth(now));
  const nothingExpected = progress.expected === 0;
  return { complete: !nothingExpected && progress.missing.length === 0 && unfinished.length === 0, unfinished, nothingExpected };
}

const WARN = { fontSize: 15, fontWeight: 700, color: "var(--warn-fg)" } as const;

/**
 * The review before closing. "Todas las calificaciones están capturadas" is
 * said only when it is true: something was expected, nothing is missing and
 * every month of the term has finished.
 */
export function CaptureReview({ progress, verdict }: { progress: PeriodCaptureProgress; verdict: CaptureVerdict }): ReactElement {
  return (
    <>
      <ProgressBar captured={progress.captured} expected={progress.expected} />
      {verdict.unfinished.length > 0 && <p style={WARN}>{`Aún no terminan sus meses: ${monthsList(verdict.unfinished)}`}</p>}
      {verdict.nothingExpected && (
        <p style={WARN}>No hay calificaciones que revisar en este periodo: aún no tiene materias con docente en sus grupos.</p>
      )}
      {progress.missing.length > 0 && <MissingList missing={progress.missing} />}
      {verdict.complete && <p style={{ fontSize: 15, color: "var(--ok-fg)", fontWeight: 600 }}>Todas las calificaciones están capturadas.</p>}
    </>
  );
}
