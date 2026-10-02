"use client";

import { useCallback, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import type { GradeSheetClient } from "../../grades/periodSheet";
import { EmptyHint, ErrorAlert } from "../config/parts";
import type { GradeSaveStore } from "./gradeSaveStore";
import { GradeSheetTable } from "./GradeSheetTable";
import { PeriodChips } from "./PeriodChips";
import { useGradeSheet } from "./useGradeSheet";
import { confirmLeavingUnsaved } from "./useGradeSaves";

export interface PeriodGradeSheetProps {
  assignmentId: string;
  /** Teacher (`/teaching/grades`) or director (`/grades`) — same contract. */
  client: GradeSheetClient;
  /** Owned by the capture screen (`useGradeSaves`), above this keyed sheet. */
  saves: GradeSaveStore;
}

export const NO_PERIODS_TEXT = "Este grado no tiene periodos de evaluación configurados";
export const NO_PERIODS_HINT = "La dirección los da de alta en Configuración › Periodos por grado.";

/**
 * The capture sheet both roles share (T15): pick the evaluation period, then
 * type one whole-number grade per student, Enter/Tab to the next. Every cell
 * saves itself on leaving and says how it went; nothing waits for a "Guardar"
 * at the bottom. Built for a teacher on a tablet in the classroom: large
 * fields, no sideways scroll.
 */
export function PeriodGradeSheet({ assignmentId, client, saves }: PeriodGradeSheetProps): ReactElement {
  const { load, lastSheet, periodId, choosePeriod, reload } = useGradeSheet(client, assignmentId, saves);
  const shownPeriod = periodId ?? lastSheet?.periodId ?? undefined;

  const choose = useCallback((next: string) => {
    if (next === shownPeriod) return;
    const unsaved = shownPeriod === undefined ? 0 : saves.unsavedCount({ assignmentId, periodId: shownPeriod });
    if (confirmLeavingUnsaved(unsaved)) choosePeriod(next);
  }, [assignmentId, choosePeriod, saves, shownPeriod]);

  if (load.kind === "ready" && load.sheet.periods.length === 0) {
    return (
      <div style={{ display: "grid", gap: 6 }}>
        <EmptyHint>{NO_PERIODS_TEXT}</EmptyHint>
        <p style={{ fontSize: 14, color: "var(--text-muted)" }}>{NO_PERIODS_HINT}</p>
      </div>
    );
  }

  return (
    <div>
      {lastSheet !== undefined && lastSheet.periods.length > 0 && shownPeriod !== undefined && (
        <PeriodChips sheet={lastSheet} selected={shownPeriod} onChoose={choose} />
      )}
      {load.kind === "loading" && <EmptyHint>Cargando lista…</EmptyHint>}
      {load.kind === "unavailable" && <EmptyHint>{load.reason}</EmptyHint>}
      {load.kind === "failed" && (
        <div style={{ display: "grid", gap: 10, justifyItems: "start" }}>
          <ErrorAlert message="No se pudo cargar la lista de calificaciones." />
          <Btn kind="ghost" icon="clock" onClick={reload}>Reintentar</Btn>
        </div>
      )}
      {load.kind === "ready" && <GradeSheetTable key={`${load.sheet.assignmentId}:${load.sheet.periodId ?? ""}`} sheet={load.sheet} saves={saves} />}
    </div>
  );
}
