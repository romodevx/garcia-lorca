"use client";

import { useCallback, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import type { GradeSheetClient } from "../../grades/monthly";
import { EmptyHint, ErrorAlert } from "../config/parts";
import type { GradeSaveStore } from "./gradeSaveStore";
import { GradeSheetTable } from "./GradeSheetTable";
import { MonthChips } from "./MonthChips";
import { useGradeSheet } from "./useGradeSheet";
import { confirmLeavingUnsaved } from "./useGradeSaves";

export interface MonthlyGradeSheetProps {
  assignmentId: string;
  /** Teacher (`/teaching/grades`) or director (`/grades`) — same contract. */
  client: GradeSheetClient;
  /** Owned by the capture screen (`useGradeSaves`), above this keyed sheet. */
  saves: GradeSaveStore;
}

/**
 * The monthly capture sheet both roles share: pick the month, then type one
 * grade per student, Enter/Tab to the next. Every cell saves itself on leaving
 * and says how it went; nothing waits for a "Guardar" at the bottom. Built for
 * a teacher on a tablet in the classroom: large fields, no sideways scroll.
 */
export function MonthlyGradeSheet({ assignmentId, client, saves }: MonthlyGradeSheetProps): ReactElement {
  const { load, lastSheet, month, chooseMonth, reload } = useGradeSheet(client, assignmentId, saves);
  const shownMonth = month ?? lastSheet?.month;

  const choose = useCallback((next: string) => {
    if (next === shownMonth) return;
    const unsaved = shownMonth === undefined ? 0 : saves.unsavedCount({ assignmentId, month: shownMonth });
    if (confirmLeavingUnsaved(unsaved)) chooseMonth(next);
  }, [assignmentId, chooseMonth, saves, shownMonth]);

  return (
    <div>
      {lastSheet !== undefined && lastSheet.months.length > 0 && shownMonth !== undefined && (
        <MonthChips months={lastSheet.months} selected={shownMonth} currentMonth={lastSheet.currentMonth} closedMonths={lastSheet.closedMonths} onChoose={choose} />
      )}
      {load.kind === "loading" && <EmptyHint>Cargando lista…</EmptyHint>}
      {load.kind === "unavailable" && <EmptyHint>{load.reason}</EmptyHint>}
      {load.kind === "failed" && (
        <div style={{ display: "grid", gap: 10, justifyItems: "start" }}>
          <ErrorAlert message="No se pudo cargar la lista de calificaciones." />
          <Btn kind="ghost" icon="clock" onClick={reload}>Reintentar</Btn>
        </div>
      )}
      {load.kind === "ready" && <GradeSheetTable key={`${load.sheet.assignmentId}:${load.sheet.month}`} sheet={load.sheet} saves={saves} />}
    </div>
  );
}
