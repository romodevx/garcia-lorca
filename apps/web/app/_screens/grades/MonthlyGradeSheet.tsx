"use client";

import { type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import type { GradeSheetClient } from "../../grades/monthly";
import { EmptyHint, ErrorAlert } from "../config/parts";
import { GradeSheetTable } from "./GradeSheetTable";
import { MonthChips } from "./MonthChips";
import { useGradeSheet } from "./useGradeSheet";
import { usePendingSaves } from "./usePendingSaves";

export interface MonthlyGradeSheetProps {
  assignmentId: string;
  /** Teacher (`/teaching/grades`) or director (`/grades`) — same contract. */
  client: GradeSheetClient;
}

/**
 * The monthly capture sheet both roles share: pick the month, then type one
 * grade per student, Enter/Tab to the next. Every cell saves itself on leaving
 * and says how it went; nothing waits for a "Guardar" at the bottom. Built for
 * a teacher on a tablet in the classroom: large fields, no sideways scroll.
 */
export function MonthlyGradeSheet({ assignmentId, client }: MonthlyGradeSheetProps): ReactElement {
  const { load, lastSheet, month, chooseMonth, reload } = useGradeSheet(client, assignmentId);
  const { pending, track } = usePendingSaves();

  return (
    <div>
      {lastSheet !== undefined && lastSheet.months.length > 0 && (
        <MonthChips months={lastSheet.months} selected={month ?? lastSheet.month} onChoose={chooseMonth} />
      )}
      {load.kind === "loading" && <EmptyHint>Cargando lista…</EmptyHint>}
      {load.kind === "unavailable" && <EmptyHint>{load.reason}</EmptyHint>}
      {load.kind === "failed" && (
        <div style={{ display: "grid", gap: 10, justifyItems: "start" }}>
          <ErrorAlert message="No se pudo cargar la lista de calificaciones." />
          <Btn kind="ghost" icon="clock" onClick={reload}>Reintentar</Btn>
        </div>
      )}
      {load.kind === "ready" && (
        <GradeSheetTable key={`${load.sheet.assignmentId}:${load.sheet.month}`} sheet={load.sheet} client={client} pending={pending} track={track} />
      )}
    </div>
  );
}
